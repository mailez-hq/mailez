"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Copy, Link2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  calendarFeed, calendarShareCreate, calendarShareDelete, calendarShares,
  type CalendarShare,
} from "@/lib/api";

// ShareDialog manages calendar grants (share with another account) and shows
// the read-only ICS subscription link for external clients.
export function ShareDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const t = useTranslations("calendar");
  const [owned, setOwned] = useState<CalendarShare[]>([]);
  const [feedUrl, setFeedUrl] = useState("");
  const [email, setEmail] = useState("");
  const [readOnly, setReadOnly] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      const [shares, feed] = await Promise.all([calendarShares(), calendarFeed()]);
      setOwned(shares.owned);
      setFeedUrl(feed.url);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  // Clear the stale error and reload on open (error reset is a render-phase
  // adjustment — React-recommended over synchronous setState in an effect).
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) setError("");
  }

  useEffect(() => {
    // Reload on open: load()'s setStates all happen after await — the lint
    // cannot see through the call boundary.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) load();
  }, [open, load]);

  const add = async () => {
    if (!email.trim()) return;
    setBusy(true);
    setError("");
    try {
      await calendarShareCreate(email.trim(), readOnly);
      setEmail("");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "share failed");
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: number) => {
    try {
      await calendarShareDelete(id);
      setOwned((list) => list.filter((s) => s.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "revoke failed");
    }
  };

  const copyFeed = async () => {
    try {
      await navigator.clipboard.writeText(feedUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setError(t("copyFailed"));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("shareCalendar")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          {error && <p className="text-sm text-destructive">{error}</p>}
          <div className="grid gap-1.5">
            <Label htmlFor="share-email">{t("shareWith")}</Label>
            <div className="flex gap-2">
              <Input
                id="share-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="user@example.com"
                onKeyDown={(e) => {
                  if (e.key === "Enter") add();
                }}
              />
              <Button onClick={add} disabled={busy || !email.trim()}>
                {t("shareAdd")}
              </Button>
            </div>
            <div className="flex items-center justify-between pt-1">
              <Label className="text-xs text-muted-foreground">{t("readOnlyLabel")}</Label>
              <Switch checked={readOnly} onCheckedChange={setReadOnly} />
            </div>
          </div>
          {owned.length > 0 && (
            <div className="max-h-40 space-y-1 overflow-auto rounded-md border border-border p-1.5">
              {owned.map((s) => (
                <div key={s.id} className="flex items-center justify-between rounded-md px-2 py-1 text-sm hover:bg-muted/50">
                  <span className="min-w-0 flex-1 truncate">
                    {s.sharee_email}
                    <span className="ml-1.5 text-[10px] text-muted-foreground">
                      {s.read_only ? t("accessRead") : t("accessWrite")}
                    </span>
                  </span>
                  <Button variant="ghost" size="icon-sm" onClick={() => remove(s.id)} title={t("revoke")}>
                    <Trash2 className="size-3.5 text-muted-foreground hover:text-destructive" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          {feedUrl && (
            <div className="rounded-md border border-border bg-muted/30 p-2">
              <p className="mb-1 flex items-center gap-1 text-[11px] text-muted-foreground">
                <Link2 className="size-3" />
                {t("subscriptionUrl")}
              </p>
              <div className="flex items-center gap-1.5">
                <code className="min-w-0 flex-1 truncate rounded border border-border bg-background px-1.5 py-1 text-[10px]">
                  {feedUrl}
                </code>
                <Button size="sm" variant="outline" onClick={copyFeed}>
                  <Copy className="size-3" />
                  {copied ? t("copied") : t("copy")}
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
