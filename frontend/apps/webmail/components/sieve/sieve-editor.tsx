"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ArrowDown, ArrowUp, Check, Code2, Filter, Loader2, Plus, Save, Sparkles, Trash2, X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Switch } from "@/components/ui/switch";
import {
  sieveActivate, sieveDelete, sieveGet, sieveList, sievePut,
  type SieveScript,
} from "@/lib/api";
import {
  compileRules, hasRulesScript, newRuleId, parseRules, RULES_SCRIPT_NAME,
  type SieveAction, type SieveCondition, type SieveConditionField, type SieveConditionOp, type SieveRule,
} from "@/lib/sieve-rules";
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
}`,
  },
  {
    id: "spam",
    labelKey: "tplSpam",
    script: `if header :contains "X-Spam-Flag" "YES" {
  fileinto "Junk";
}`,
  },
];

const selectCls =
  "h-8 rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

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
  const [rules, setRules] = useState<SieveRule[]>([]);
  const [mode, setMode] = useState<"rules" | "script">("rules");
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
      // Prefer the builder's script; fall back to the active/first script.
      const preferred =
        list.find((s) => s.name === RULES_SCRIPT_NAME) ??
        list.find((s) => s.active) ??
        list[0];
      if (preferred) {
        await selectScript(preferred.name);
      } else {
        setCurrent("");
        setContent("");
        setRules([]);
        setMode("rules");
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

  async function selectScript(name: string) {
    setError("");
    try {
      const got = await sieveGet(name);
      setCurrent(name);
      setContent(got.content);
      if (name === RULES_SCRIPT_NAME) {
        const parsed = parseRules(got.content);
        setRules(parsed ?? []);
        setMode("rules");
      } else {
        setRules([]);
        setMode("script");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve get failed");
    }
  }

  async function save(activate: boolean) {
    if (!current) return;
    setSaving(true);
    setError("");
    try {
      const body = current === RULES_SCRIPT_NAME && mode === "rules"
        ? compileRules(rules)
        : content;
      await sievePut(current, body, activate);
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
      await selectScript(name);
    } catch (e) {
      setError(e instanceof Error ? e.message : "sieve create failed");
    }
  }

  async function createRuleset() {
    setError("");
    try {
      await sievePut(RULES_SCRIPT_NAME, compileRules([]), false);
      await load();
      await selectScript(RULES_SCRIPT_NAME);
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
        setRules([]);
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

  // ---- rules editing helpers -------------------------------------------

  function updateRule(id: string, patch: Partial<SieveRule>) {
    setRules((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }
  function moveRule(idx: number, dir: -1 | 1) {
    setRules((rs) => {
      const next = [...rs];
      const j = idx + dir;
      if (j < 0 || j >= next.length) return rs;
      [next[idx], next[j]] = [next[j], next[idx]];
      return next;
    });
  }
  function addRule() {
    setRules((rs) => [
      ...rs,
      {
        id: newRuleId(),
        name: "",
        enabled: true,
        matchAll: true,
        conditions: [{ field: "from", op: "contains", value: "" }],
        actions: [{ type: "markRead" }],
      },
    ]);
  }
  function updateCondition(ruleId: string, idx: number, patch: Partial<SieveCondition>) {
    setRules((rs) =>
      rs.map((r) =>
        r.id === ruleId
          ? { ...r, conditions: r.conditions.map((c, i) => (i === idx ? { ...c, ...patch } : c)) }
          : r,
      ),
    );
  }
  function updateAction(ruleId: string, idx: number, action: SieveAction) {
    setRules((rs) =>
      rs.map((r) =>
        r.id === ruleId
          ? { ...r, actions: r.actions.map((a, i) => (i === idx ? action : a)) }
          : r,
      ),
    );
  }

  const currentActive = scripts.find((s) => s.name === current)?.active ?? false;
  const isRulesScript = current === RULES_SCRIPT_NAME;

  // ----------------------------------------------------------------------

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
            {!hasRulesScript(scripts) && (
              <Button
                size="xs"
                variant="outline"
                className="mt-1.5 w-full"
                onClick={createRuleset}
              >
                <Filter className="size-3" />
                {t("newRuleset")}
              </Button>
            )}
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
                      onClick={() => selectScript(s.name)}
                      className="min-w-0 flex-1 text-left"
                    >
                      <span className="block truncate text-sm font-medium">
                        {s.name === RULES_SCRIPT_NAME ? t("rulesetName") : s.name}
                      </span>
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

          {/* right: rules builder or raw script */}
          <div className="flex min-w-0 flex-col gap-2">
            {isRulesScript && (
              <div className="flex items-center gap-1">
                <Button
                  size="xs"
                  variant={mode === "rules" ? "default" : "outline"}
                  onClick={() => setMode("rules")}
                >
                  <Filter className="size-3" />
                  {t("modeRules")}
                </Button>
                <Button
                  size="xs"
                  variant={mode === "script" ? "default" : "outline"}
                  onClick={() => setMode("script")}
                >
                  <Code2 className="size-3" />
                  {t("modeScript")}
                </Button>
              </div>
            )}

            {isRulesScript && mode === "rules" ? (
              <ScrollArea className="min-h-64 flex-1">
                <div className="space-y-2 pr-1">
                  {rules.length === 0 && (
                    <p className="p-2 text-xs text-muted-foreground">{t("noRules")}</p>
                  )}
                  {rules.map((r, idx) => (
                    <div key={r.id} className="rounded-lg border border-border p-2">
                      <div className="flex items-center gap-1.5">
                        <Switch
                          checked={r.enabled}
                          onCheckedChange={(v) => updateRule(r.id, { enabled: v })}
                          aria-label={t("activate")}
                        />
                        <Input
                          value={r.name}
                          onChange={(e) => updateRule(r.id, { name: e.target.value })}
                          placeholder={t("ruleName")}
                          className="h-7 flex-1 text-xs"
                        />
                        <button
                          type="button"
                          onClick={() => moveRule(idx, -1)}
                          disabled={idx === 0}
                          className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-30"
                          title={t("moveUp")}
                        >
                          <ArrowUp className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => moveRule(idx, 1)}
                          disabled={idx === rules.length - 1}
                          className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-30"
                          title={t("moveDown")}
                        >
                          <ArrowDown className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setRules((rs) => rs.filter((x) => x.id !== r.id))}
                          className="rounded p-0.5 text-muted-foreground hover:text-destructive"
                          title={t("delete")}
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>

                      <div className="mt-1.5 flex items-center gap-1.5">
                        <span className="text-[10px] text-muted-foreground">{t("when")}</span>
                        <select
                          value={r.matchAll ? "all" : "any"}
                          onChange={(e) => updateRule(r.id, { matchAll: e.target.value === "all" })}
                          className={selectCls}
                        >
                          <option value="all">{t("matchAll")}</option>
                          <option value="any">{t("matchAny")}</option>
                        </select>
                      </div>

                      <div className="mt-1 space-y-1">
                        {r.conditions.map((c, ci) => (
                          <div key={ci} className="flex items-center gap-1">
                            <select
                              value={c.field}
                              onChange={(e) =>
                                updateCondition(r.id, ci, { field: e.target.value as SieveConditionField })
                              }
                              className={selectCls}
                            >
                              <option value="from">{t("fieldFrom")}</option>
                              <option value="to">{t("fieldTo")}</option>
                              <option value="subject">{t("fieldSubject")}</option>
                            </select>
                            <select
                              value={c.op}
                              onChange={(e) =>
                                updateCondition(r.id, ci, { op: e.target.value as SieveConditionOp })
                              }
                              className={selectCls}
                            >
                              <option value="contains">{t("opContains")}</option>
                              <option value="is">{t("opIs")}</option>
                              <option value="starts">{t("opStarts")}</option>
                            </select>
                            <Input
                              value={c.value}
                              onChange={(e) => updateCondition(r.id, ci, { value: e.target.value })}
                              className="h-8 flex-1 text-xs"
                            />
                            <button
                              type="button"
                              onClick={() =>
                                updateRule(r.id, {
                                  conditions: r.conditions.filter((_, i) => i !== ci),
                                })
                              }
                              className="rounded p-0.5 text-muted-foreground hover:text-destructive"
                              title={t("removeCondition")}
                            >
                              <X className="size-3.5" />
                            </button>
                          </div>
                        ))}
                        <Button
                          size="xs"
                          variant="ghost"
                          onClick={() =>
                            updateRule(r.id, {
                              conditions: [...r.conditions, { field: "from", op: "contains", value: "" }],
                            })
                          }
                        >
                          <Plus className="size-3" />
                          {t("addCondition")}
                        </Button>
                      </div>

                      <div className="mt-1.5 space-y-1">
                        {r.actions.map((a, ai) => (
                          <div key={ai} className="flex items-center gap-1">
                            <select
                              value={a.type}
                              onChange={(e) => {
                                const type = e.target.value as SieveAction["type"];
                                updateAction(r.id, ai,
                                  type === "moveTo"
                                    ? { type, folder: "" }
                                    : type === "forward"
                                      ? { type, address: "" }
                                      : { type });
                              }}
                              className={selectCls}
                            >
                              <option value="moveTo">{t("actMoveTo")}</option>
                              <option value="markRead">{t("actMarkRead")}</option>
                              <option value="star">{t("actStar")}</option>
                              <option value="forward">{t("actForward")}</option>
                              <option value="discard">{t("actDiscard")}</option>
                            </select>
                            {a.type === "moveTo" && (
                              <Input
                                value={a.folder}
                                onChange={(e) => updateAction(r.id, ai, { type: "moveTo", folder: e.target.value })}
                                placeholder={t("folderPlaceholder")}
                                className="h-8 flex-1 text-xs"
                              />
                            )}
                            {a.type === "forward" && (
                              <Input
                                value={a.address}
                                onChange={(e) => updateAction(r.id, ai, { type: "forward", address: e.target.value })}
                                placeholder={t("addressPlaceholder")}
                                className="h-8 flex-1 text-xs"
                              />
                            )}
                            <button
                              type="button"
                              onClick={() =>
                                updateRule(r.id, { actions: r.actions.filter((_, i) => i !== ai) })
                              }
                              className="rounded p-0.5 text-muted-foreground hover:text-destructive"
                              title={t("removeAction")}
                            >
                              <X className="size-3.5" />
                            </button>
                          </div>
                        ))}
                        <Button
                          size="xs"
                          variant="ghost"
                          onClick={() =>
                            updateRule(r.id, { actions: [...r.actions, { type: "markRead" }] })
                          }
                        >
                          <Plus className="size-3" />
                          {t("addAction")}
                        </Button>
                      </div>
                    </div>
                  ))}
                  <Button size="sm" variant="outline" className="w-full" onClick={addRule}>
                    <Plus className="size-3.5" />
                    {t("addRule")}
                  </Button>
                </div>
              </ScrollArea>
            ) : (
              <>
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
              </>
            )}

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
