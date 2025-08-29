"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { pgpDelete, pgpGenerate, type PgpKey, type PgpStatus } from "@/lib/api";

// PgpSection manages the user's own key (generate/copy/delete) and the
// keyring of imported public keys used to encrypt to external recipients.
export function PgpSection({
  t,
  pgp, setPgp,
  generating, setGenerating,
  copied, setCopied,
  pgpKeys,
  pgpImportEmail, setPgpImportEmail,
  pgpImportKeyText, setPgpImportKeyText,
  pgpImporting,
  pgpDeleting,
  onImportPgpKey,
  onDeletePgpKey,
  setError,
}: {
  t: (key: string) => string;
  pgp: PgpStatus | null; setPgp: (v: PgpStatus | null) => void;
  generating: boolean; setGenerating: (v: boolean) => void;
  copied: boolean; setCopied: (v: boolean) => void;
  pgpKeys: PgpKey[] | null;
  pgpImportEmail: string; setPgpImportEmail: (v: string) => void;
  pgpImportKeyText: string; setPgpImportKeyText: (v: string) => void;
  pgpImporting: boolean;
  pgpDeleting: number | null;
  onImportPgpKey: () => void;
  onDeletePgpKey: (id: number) => void;
  setError: (v: string) => void;
}) {
  return (
    <div className="h-full space-y-3 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("pgp")}</p>
      {pgp === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}
      {pgp && !pgp.has_key && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("pgpNoKey")}</p>
          <Button
            type="button"
            size="sm"
            onClick={async () => {
              setGenerating(true);
              setError("");
              try {
                setPgp(await pgpGenerate());
              } catch (e) {
                setError(e instanceof Error ? e.message : "pgp generate failed");
              } finally {
                setGenerating(false);
              }
            }}
            disabled={generating}
          >
            {generating ? t("pgpGenerating") : t("pgpGenerate")}
          </Button>
        </div>
      )}
      {pgp && pgp.has_key && (
        <div className="space-y-2">
          <p className="font-mono text-xs text-muted-foreground">
            {t("pgpFingerprint")}: {pgp.fingerprint}
          </p>
          <div className="flex flex-wrap gap-1">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                if (!pgp.public_key) return;
                navigator.clipboard?.writeText(pgp.public_key).then(() => {
                  setCopied(true);
                  setTimeout(() => setCopied(false), 1500);
                });
              }}
            >
              {copied ? t("pgpCopied") : t("pgpCopyPublicKey")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="text-muted-foreground hover:text-destructive"
              onClick={async () => {
                if (!window.confirm(t("pgpDeleteConfirm"))) return;
                setError("");
                try {
                  await pgpDelete();
                  setPgp({ has_key: false });
                } catch (e) {
                  setError(e instanceof Error ? e.message : "pgp delete failed");
                }
              }}
            >
              {t("pgpDeleteKey")}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t("pgpNote")}</p>
        </div>
      )}

      {/* Keyring: imported public keys used to encrypt to
          external recipients. Independent of the user's own key. */}
      <div className="space-y-2 border-t pt-3">
        <p className="text-sm font-medium">{t("pgpKeyring")}</p>
        <div className="space-y-1.5">
          <Label>{t("pgpImportEmail")}</Label>
          <Input
            value={pgpImportEmail}
            onChange={(e) => setPgpImportEmail(e.target.value)}
            placeholder="alice@example.com"
            className="text-sm"
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("pgpImportKey")}</Label>
          <textarea
            value={pgpImportKeyText}
            onChange={(e) => setPgpImportKeyText(e.target.value)}
            rows={4}
            placeholder="-----BEGIN PGP PUBLIC KEY BLOCK-----"
            className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
          />
        </div>
        <Button type="button" size="sm" onClick={onImportPgpKey} disabled={pgpImporting}>
          {pgpImporting ? t("pgpImporting") : t("pgpImport")}
        </Button>
        {pgpKeys === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
        {pgpKeys !== null && pgpKeys.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("pgpKeyringEmpty")}</p>
        )}
        <ul className="space-y-1.5">
          {pgpKeys?.map((k) => (
            <li key={k.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{k.email}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">
                  {k.fingerprint.slice(0, 40)}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => onDeletePgpKey(k.id)}
                disabled={pgpDeleting === k.id}
              >
                {pgpDeleting === k.id ? t("pgpDeleting") : t("pgpDeleteKey")}
              </Button>
            </li>
          ))}
        </ul>
        <p className="text-xs text-muted-foreground">{t("pgpKeyringNote")}</p>
      </div>
    </div>
  );
}
