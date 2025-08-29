"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Check, Loader2, Plus, Save, Sparkles, Trash2, X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  sieveActivate, sieveDelete, sieveGet, sieveList, sievePut,
  type SieveScript,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const TEMPLATES: { id: string; labelKey: string; script: string }[] = [
  {
    id: "vacation",
    labelKey: "tplVacation",
    script: `vacation :days 1 "I am currently away and will reply as soon as possible.";`,
  },
  {
    id: "redirect",
    labelKey: "tplRedirect",
    script: `redirect "forward-to@example.com";`,
  },
  {
    id: "fileinto",
    labelKey: "tplFileInto",
    script: `if address :is "from" "sender@example.com" {
  fileinto "Folder";
  stop;
}`,
  },
  {
    id: "spam",
    labelKey: "tplSpam",
    script: `if header :contains "X-Spam-Flag" "YES" {
  fileinto "Junk";
  stop;
}`,
  },
];

export function SieveEditor({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const t = useTranslations("sieve");
  const [scripts, setScripts] = useState<SieveScript[]>([]);
  const [current, setCurrent] = useState("");
  const [content, setContent] = useState("");
  const [newName, setNewName] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  // Show the spinner synchronously when the editor opens (render-phase
  // adjustment — React-recommended over synchronous setState in an effect);
  // the actual load below is asynchronous.
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setLoading(true);
      setError("");
    }
  }

  const load = useCallback(async () => {
    try {
      const list = await sieveList();
      setScripts(list);
      const active = list.find((s) => s.active);
      const first = active || list[0];
      if (first) {
        setCurrent(first.name);
        const got = await sieveGet(first.name);
        setContent(got.content);
      } else {
        setCurrent("");
        setContent("");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Fetch when the editor opens. Every setState inside load() happens after
    // its await — the lint cannot see through the call boundary.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) load();
  }, [open, load]);

  async function select(name: string) {
    if (name === current) return;
    setError("");
    try {
      const got = await sieveGet(name);
      setCurrent(name);
      setContent(got.content);
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve get failed");
    }
  }

  async function save(activate: boolean) {
    if (!current) return;
    setSaving(true);
    setError("");
    try {
      await sievePut(current, content, activate);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve save failed");
    } finally {
      setSaving(false);
    }
  }

  async function create() {
    const name = newName.trim();
    if (!name) return;
    setError("");
    try {
      await sievePut(name, "keep;", false);
      setNewName("");
      await load();
      await select(name);
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve create failed");
    }
  }

  async function remove(name: string) {
    setError("");
    try {
      await sieveDelete(name);
      if (name === current) {
        setCurrent("");
        setContent("");
      }
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve delete failed");
    }
  }

  async function toggleActive() {
    if (!current) return;
    setError("");
    try {
      const script = scripts.find((s) => s.name === current);
      await sieveActivate(script?.active ? "" : current);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve activate failed");
    }
  }

  const currentActive = scripts.find((s) => s.name === current)?.active ?? false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] sm:max-w-3xl">
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle></DialogHeader>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="grid min-h-0 gap-4 md:grid-cols-[220px_1fr]">
          {/* left: script list */}
          <div className="min-w-0">
            <div className="flex gap-1.5">
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") create();
                }}
                placeholder={t("namePlaceholder")}
                className="h-8"
              />
              <Button size="icon-sm" onClick={create} title={t("newRule")}>
                <Plus className="size-3.5" />
              </Button>
            </div>
            <ScrollArea className="mt-2 max-h-80">
              <div className="space-y-1 pr-1">
                {scripts.map((s) => (
                  <div
                    key={s.name}
                    className={cn(
                      "group flex items-center gap-1 rounded-lg border px-2 py-1.5 transition-colors",
                      current === s.name
                        ? "border-primary bg-accent"
                        : "border-border hover:bg-muted",
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => select(s.name)}
                      className="min-w-0 flex-1 text-left"
                    >
                      <span className="block truncate text-sm font-medium">{s.name}</span>
                      <span
                        className={cn(
                          "text-[10px]",
                          s.active ? "text-primary" : "text-muted-foreground",
                        )}
                      >
                        {s.active ? t("active") : t("inactive")}
                      </span>
                    </button>
                    {s.active && <Check className="size-3.5 shrink-0 text-primary" />}
                    <button
                      type="button"
                      onClick={() => remove(s.name)}
                      className="rounded p-0.5 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
                      title={t("delete")}
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                ))}
                {loading && (
                  <p className="flex items-center gap-1.5 p-2 text-xs text-muted-foreground">
                    <Loader2 className="size-3 animate-spin" />
                    {t("loading")}
                  </p>
                )}
                {!loading && scripts.length === 0 && (
                  <p className="p-2 text-xs text-muted-foreground">{t("noScripts")}</p>
                )}
              </div>
            </ScrollArea>
          </div>

          {/* right: editor */}
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-1">
              <span className="mr-1 text-xs text-muted-foreground">{t("templates")}</span>
              {TEMPLATES.map((tp) => (
                <Button
                  key={tp.id}
                  size="xs"
                  variant="outline"
                  onClick={() => setContent((c) => (c ? `${c}\n${tp.script}` : tp.script))}
                >
                  <Sparkles className="size-3" />
                  {t(tp.labelKey)}
                </Button>
              ))}
            </div>
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              spellCheck={false}
              className="min-h-64 flex-1 resize-y rounded-lg border border-input bg-transparent p-3 font-mono text-xs leading-5 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              placeholder='require ["fileinto", "vacation", "redirect"];'
            />
            <div className="flex flex-wrap items-center gap-1">
              {current && (
                <Button variant="ghost" size="sm" onClick={toggleActive}>
                  {currentActive ? <X className="size-3.5" /> : <Check className="size-3.5" />}
                  {currentActive ? t("deactivate") : t("activate")}
                </Button>
              )}
              <div className="ml-auto flex gap-1">
                <Button size="sm" variant="outline" onClick={() => save(false)} disabled={saving || !current}>
                  <Save className="size-3.5" />
                  {t("save")}
                </Button>
                <Button size="sm" onClick={() => save(true)} disabled={saving || !current}>
                  {saving ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Check className="size-3.5" />
                  )}
                  {t("saveAndActivate")}
                </Button>
              </div>
            </div>
          </div>
        </div>
        <DialogFooter />
      </DialogContent>
    </Dialog>
  );
}
