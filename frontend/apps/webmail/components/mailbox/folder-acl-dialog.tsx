"use client";

import { useEffect, useState } from "react";
import { Share2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import {
  mailACL, mailACLDelete, mailACLSet,
  type FolderACLEntry,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// The rights a user may grant on a shared folder (RFC 4314 subset). Each entry
// is shown as a toggleable chip so the UI stays compact.
const ACL_RIGHTS: { right: string; label: string }[] = [
  { right: "l", label: "aclLookup" },
  { right: "r", label: "aclRead" },
  { right: "s", label: "aclSeen" },
  { right: "w", label: "aclWrite" },
  { right: "i", label: "aclInsert" },
  { right: "p", label: "aclPost" },
  { right: "t", label: "aclDelete" },
  { right: "e", label: "aclExpunge" },
  { right: "a", label: "aclAdminister" },
];

function rightsFromSet(set: Set<string>): string {
  // canonical RFC order keeps the stored string stable
  return ACL_RIGHTS.map((r) => r.right).filter((r) => set.has(r)).join("");
}

export function FolderACLDialog({
  folder,
  open,
  onOpenChange,
}: {
  folder: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("mail");
  const [entries, setEntries] = useState<FolderACLEntry[] | null>(null);
  const [myRights, setMyRights] = useState("");
  const [identifier, setIdentifier] = useState("");
  const [rights, setRights] = useState<Set<string>>(new Set());
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = () => {
    setError("");
    mailACL(folder)
      .then((res) => {
        setEntries(res.entries);
        setMyRights(res.my_rights || "");
      })
      .catch((e) => {
        setEntries([]);
        setError(e instanceof Error ? e.message : "load acl failed");
      });
  };

  // Reset the grant form on open transitions (render-phase adjustment —
  // React-recommended over an effect, catches every open path).
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setIdentifier("");
      setRights(new Set());
    }
  }

  useEffect(() => {
    // Reload the ACL list on open. Every setState inside load() happens in
    // .then callbacks — the lint cannot see through the call boundary.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, folder]);

  const toggleRight = (right: string) => {
    setRights((prev) => {
      const next = new Set(prev);
      if (next.has(right)) next.delete(right);
      else next.add(right);
      return next;
    });
  };

  const grant = async () => {
    const id = identifier.trim();
    if (!id) return;
    setBusy(true);
    setError("");
    try {
      await mailACLSet(folder, id, rightsFromSet(rights));
      setIdentifier("");
      setRights(new Set());
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "acl update failed");
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (id: string) => {
    setBusy(true);
    setError("");
    try {
      await mailACLDelete(folder, id);
      setEntries((es) => (es || []).filter((e) => e.identifier !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "acl update failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Share2 className="size-4" />
            {t("folderSharing")}
          </DialogTitle>
        </DialogHeader>

        {myRights && (
          <p className="text-xs text-muted-foreground">
            {t("aclMyRights", { rights: myRights })}
          </p>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}

        {entries === null ? (
          <p className="text-sm text-muted-foreground">{t("loading")}</p>
        ) : (
          <>
            {entries.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("aclEmpty")}</p>
            ) : (
              <div className="space-y-1">
                {entries.map((e) => (
                  <div
                    key={e.identifier}
                    className="group flex items-center gap-2 rounded-lg border border-border px-2.5 py-1.5"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">{e.identifier}</p>
                      <p className="font-mono text-[11px] text-muted-foreground">
                        {e.rights || "—"}
                      </p>
                    </div>
                    <Button
                      size="xs"
                      variant="ghost"
                      title={t("aclRevoke")}
                      className="size-7 shrink-0 p-0 text-muted-foreground hover:text-destructive"
                      disabled={busy}
                      onClick={() => revoke(e.identifier)}
                    >
                      <X className="size-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            )}

            <div className="space-y-2 border-t pt-3">
              <div className="space-y-1.5">
                <Label>{t("aclIdentifier")}</Label>
                <Input
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  placeholder="user@example.com"
                  className="text-sm"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("aclRights")}</Label>
                <div className="flex flex-wrap gap-1">
                  {ACL_RIGHTS.map(({ right, label }) => (
                    <button
                      key={right}
                      type="button"
                      onClick={() => toggleRight(right)}
                      className={cn(
                        "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                        rights.has(right)
                          ? "border-primary bg-primary/10 font-medium text-primary"
                          : "border-border text-muted-foreground hover:bg-muted",
                      )}
                    >
                      {t(label)}
                    </button>
                  ))}
                </div>
              </div>
              <Button
                type="button"
                size="sm"
                onClick={grant}
                disabled={busy || !identifier.trim()}
              >
                {t("aclGrant")}
              </Button>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
