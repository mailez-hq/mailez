"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Check, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ComposeEditor } from "@/components/compose/compose-editor";
import { cn } from "@/lib/utils";
import {
  mailIdentities,
  signatureCreate,
  signatureDelete,
  signatureList,
  signatureUpdate,
  type MailIdentity,
  type Signature,
} from "@/lib/api";


type Draft = {
  id: number | null;
  name: string;
  identity_email: string;
  body_html: string;
  default_for_new: boolean;
  default_for_reply: boolean;
};

const emptyDraft = (): Draft => ({
  id: null,
  name: "",
  identity_email: "",
  body_html: "",
  default_for_new: false,
  default_for_reply: false,
});

function toDraft(sig: Signature): Draft {
  return {
    id: sig.id,
    name: sig.name,
    identity_email: sig.identity_email,
    body_html: sig.body_html,
    default_for_new: sig.default_for_new,
    default_for_reply: sig.default_for_reply,
  };
}

function errorText(e: unknown, fallback: string) {
  return e instanceof Error && e.message ? e.message : fallback;
}

export function SignatureSection() {
  const t = useTranslations("settings");
  const [rows, setRows] = useState<Signature[]>([]);
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [sourceMode, setSourceMode] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([signatureList(), mailIdentities().catch(() => [] as MailIdentity[])])
      .then(([list, ids]) => {
        if (cancelled) return;
        setRows(list);
        setIdentities(ids.filter((idn) => !idn.delegated));
        setDraft(list.length > 0 ? toDraft(list[0]) : emptyDraft());
      })
      .catch((e) => {
        if (!cancelled) setError(errorText(e, t("signatureLoadFailed")));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [t]);

  function patch(next: Partial<Draft>) {
    setDraft((cur) => ({ ...cur, ...next }));
    setNotice("");
  }

  async function refresh(selectId: number | null) {
    const list = await signatureList();
    setRows(list);
    const pick =
      selectId == null ? list[0] : list.find((r) => r.id === selectId) ?? list[0];
    setDraft(pick ? toDraft(pick) : emptyDraft());
  }

  async function save() {
    if (!draft.name.trim()) {
      setError(t("signatureNameRequired"));
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const body = {
        name: draft.name.trim(),
        identity_email: draft.identity_email,
        body_html: draft.body_html,
        default_for_new: draft.default_for_new,
        default_for_reply: draft.default_for_reply,
      };
      const saved =
        draft.id == null
          ? await signatureCreate(body)
          : await signatureUpdate(draft.id, body);
      await refresh(saved.id);
      setNotice(t("signatureSaved"));
    } catch (e) {
      setError(errorText(e, t("signatureSaveFailed")));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (draft.id == null) {
      setDraft(emptyDraft());
      return;
    }
    if (!window.confirm(t("signatureDeleteConfirm"))) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await signatureDelete(draft.id);
      await refresh(null);
      setNotice(t("signatureDeleted"));
    } catch (e) {
      setError(errorText(e, t("signatureSaveFailed")));
    } finally {
      setBusy(false);
    }
  }

  function scopeLabel(identityEmail: string) {
    return identityEmail || t("signatureScopeAll");
  }

  if (loading) {
    return <p className="text-xs text-muted-foreground">{t("loading")}</p>;
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Label>{t("signature")}</Label>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            setDraft(emptyDraft());
            setSourceMode(false);
            setNotice("");
            setError("");
          }}
        >
          <Plus className="size-3.5" />
          {t("newSignature")}
        </Button>
      </div>

      <div className="grid gap-3 sm:grid-cols-[minmax(0,10rem)_1fr]">
        <div className="space-y-1" data-testid="signature-list">
          {rows.length === 0 && (
            <p className="text-xs text-muted-foreground">{t("signatureEmpty")}</p>
          )}
          {rows.map((sig) => {
            const active = draft.id === sig.id;
            return (
              <button
                key={sig.id}
                type="button"
                onClick={() => {
                  setDraft(toDraft(sig));
                  setSourceMode(false);
                  setNotice("");
                  setError("");
                }}
                className={cn(
                  "flex w-full flex-col items-start gap-0.5 rounded-md border px-2 py-1.5 text-left text-xs transition-colors",
                  active
                    ? "border-accent bg-accent/40 text-accent-foreground"
                    : "border-border hover:bg-muted",
                )}
              >
                <span className="font-medium">{sig.name}</span>
                <span className="text-[10px] text-muted-foreground">
                  {scopeLabel(sig.identity_email)}
                </span>
                {(sig.default_for_new || sig.default_for_reply) && (
                  <span className="text-[10px] text-primary">
                    {sig.default_for_new && sig.default_for_reply
                      ? t("signatureDefaultBoth")
                      : sig.default_for_new
                        ? t("signatureDefaultNew")
                        : t("signatureDefaultReply")}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        <div className="space-y-2.5">
          <div className="space-y-1">
            <Label htmlFor="signature-name">{t("signatureName")}</Label>
            <Input
              id="signature-name"
              value={draft.name}
              onChange={(e) => patch({ name: e.target.value })}
              placeholder={t("signatureNamePlaceholder")}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="signature-scope">{t("signatureScope")}</Label>
            <select
              id="signature-scope"
              value={draft.identity_email}
              onChange={(e) => patch({ identity_email: e.target.value })}
              className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
            >
              <option value="">{t("signatureScopeAll")}</option>
              {identities.map((idn) => (
                <option key={idn.email} value={idn.email}>
                  {idn.email}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-wrap items-center gap-4 text-xs">
            <label className="flex items-center gap-1.5">
              <input
                type="checkbox"
                checked={draft.default_for_new}
                onChange={(e) => patch({ default_for_new: e.target.checked })}
                className="size-3.5"
              />
              {t("signatureDefaultNew")}
            </label>
            <label className="flex items-center gap-1.5">
              <input
                type="checkbox"
                checked={draft.default_for_reply}
                onChange={(e) => patch({ default_for_reply: e.target.checked })}
                className="size-3.5"
              />
              {t("signatureDefaultReply")}
            </label>
            <button
              type="button"
              onClick={() => setSourceMode((v) => !v)}
              className="ml-auto text-[11px] text-muted-foreground underline-offset-2 hover:underline"
            >
              {sourceMode ? t("signatureRichMode") : t("signatureHtmlMode")}
            </button>
          </div>

          {sourceMode ? (
            <textarea
              aria-label={t("signature")}
              data-testid="signature-html"
              value={draft.body_html}
              onChange={(e) => patch({ body_html: e.target.value })}
              rows={8}
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs"
            />
          ) : (
            <div className="flex h-56 flex-col" data-testid="signature-editor">
              <ComposeEditor
                value={draft.body_html}
                onChange={(html) => patch({ body_html: html })}
                placeholder={t("signatureBodyPlaceholder")}
              />
            </div>
          )}

          <div className="flex items-center gap-2">
            <Button type="button" size="sm" onClick={save} disabled={busy}>
              {busy ? t("saving") : t("signatureSave")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={remove}
              disabled={busy}
              className="text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="size-3.5" />
              {t("signatureDelete")}
            </Button>
            {notice && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <Check className="size-3" />
                {notice}
              </span>
            )}
          </div>

          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
      </div>

      <p className="text-xs text-muted-foreground">{t("signatureHint")}</p>
    </div>
  );
}
