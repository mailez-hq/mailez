"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { SmimeCert, SmimeStatus } from "@/lib/api";

// SmimeSection manages the user's own S/MIME certificate (PEM or PKCS#12
// import) and the keyring of imported certificates for external recipients.
export function SmimeSection({
  t,
  smime,
  smimeCerts,
  smimeImportMode, setSmimeImportMode,
  smimeCertPem, setSmimeCertPem,
  smimeKeyPem, setSmimeKeyPem,
  smimeP12B64, setSmimeP12B64,
  smimeP12Password, setSmimeP12Password,
  smimeImporting,
  smimeKeyringEmail, setSmimeKeyringEmail,
  smimeKeyringCert, setSmimeKeyringCert,
  smimeKeyringImporting,
  smimeKeyringDeleting,
  onImportSmime,
  onDeleteSmime,
  onImportSmimeCert,
  onDeleteSmimeCert,
}: {
  t: (key: string) => string;
  smime: SmimeStatus | null;
  smimeCerts: SmimeCert[] | null;
  smimeImportMode: "pem" | "p12"; setSmimeImportMode: (v: "pem" | "p12") => void;
  smimeCertPem: string; setSmimeCertPem: (v: string) => void;
  smimeKeyPem: string; setSmimeKeyPem: (v: string) => void;
  smimeP12B64: string; setSmimeP12B64: (v: string) => void;
  smimeP12Password: string; setSmimeP12Password: (v: string) => void;
  smimeImporting: boolean;
  smimeKeyringEmail: string; setSmimeKeyringEmail: (v: string) => void;
  smimeKeyringCert: string; setSmimeKeyringCert: (v: string) => void;
  smimeKeyringImporting: boolean;
  smimeKeyringDeleting: number | null;
  onImportSmime: () => void;
  onDeleteSmime: () => void;
  onImportSmimeCert: () => void;
  onDeleteSmimeCert: (id: number) => void;
}) {
  return (
    <div className="h-full space-y-3 overflow-y-auto p-5">
      <p className="text-sm font-medium">{t("smime")}</p>
      {smime === null && <p className="text-sm text-muted-foreground">{t("loading")}</p>}

      {smime && !smime.has_cert && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">{t("smimeNoCert")}</p>
          <div className="flex gap-1.5">
            <Button
              type="button"
              size="sm"
              variant={smimeImportMode === "pem" ? "default" : "outline"}
              onClick={() => setSmimeImportMode("pem")}
            >
              {t("smimeImportPem")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant={smimeImportMode === "p12" ? "default" : "outline"}
              onClick={() => setSmimeImportMode("p12")}
            >
              {t("smimeImportP12")}
            </Button>
          </div>
          {smimeImportMode === "pem" ? (
            <>
              <div className="space-y-1.5">
                <Label>{t("smimeCert")}</Label>
                <textarea
                  value={smimeCertPem}
                  onChange={(e) => setSmimeCertPem(e.target.value)}
                  rows={4}
                  placeholder="-----BEGIN CERTIFICATE-----"
                  className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("smimePrivateKey")}</Label>
                <textarea
                  value={smimeKeyPem}
                  onChange={(e) => setSmimeKeyPem(e.target.value)}
                  rows={4}
                  placeholder="-----BEGIN PRIVATE KEY-----"
                  className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                />
              </div>
            </>
          ) : (
            <>
              <div className="space-y-1.5">
                <Label>{t("smimeP12")}</Label>
                <textarea
                  value={smimeP12B64}
                  onChange={(e) => setSmimeP12B64(e.target.value)}
                  rows={3}
                  placeholder={t("smimeP12Placeholder")}
                  className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t("smimeP12Password")}</Label>
                <Input
                  type="password"
                  value={smimeP12Password}
                  onChange={(e) => setSmimeP12Password(e.target.value)}
                  className="text-sm"
                />
              </div>
            </>
          )}
          <Button type="button" size="sm" onClick={onImportSmime} disabled={smimeImporting}>
            {smimeImporting ? t("smimeImporting") : t("smimeImport")}
          </Button>
        </div>
      )}

      {smime && smime.has_cert && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            {smime.email && <>{smime.email} · </>}
            {t("smimeFingerprint")}: <span className="font-mono">{smime.fingerprint}</span>
          </p>
          {smime.issuer && <p className="text-xs text-muted-foreground">{t("smimeIssuer")}: {smime.issuer}</p>}
          {smime.not_after && (
            <p className="text-xs text-muted-foreground">
              {t("smimeExpires")}: {new Date(smime.not_after).toLocaleDateString()}
            </p>
          )}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-muted-foreground hover:text-destructive"
            onClick={onDeleteSmime}
          >
            {t("smimeDelete")}
          </Button>
        </div>
      )}

      {/* Certificate keyring: imported certs for external recipients */}
      <div className="space-y-2 border-t pt-3">
        <p className="text-sm font-medium">{t("smimeKeyring")}</p>
        <div className="space-y-1.5">
          <Label>{t("smimeKeyringEmail")}</Label>
          <Input
            value={smimeKeyringEmail}
            onChange={(e) => setSmimeKeyringEmail(e.target.value)}
            placeholder="alice@example.com"
            className="text-sm"
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("smimeKeyringCert")}</Label>
          <textarea
            value={smimeKeyringCert}
            onChange={(e) => setSmimeKeyringCert(e.target.value)}
            rows={4}
            placeholder="-----BEGIN CERTIFICATE-----"
            className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs shadow-sm"
          />
        </div>
        <Button
          type="button"
          size="sm"
          onClick={onImportSmimeCert}
          disabled={smimeKeyringImporting}
        >
          {smimeKeyringImporting ? t("smimeImporting") : t("smimeImport")}
        </Button>
        {smimeCerts === null && <p className="text-xs text-muted-foreground">{t("loading")}</p>}
        {smimeCerts !== null && smimeCerts.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("smimeKeyringEmpty")}</p>
        )}
        <ul className="space-y-1.5">
          {smimeCerts?.map((c) => (
            <li key={c.id} className="flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{c.email}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">
                  {c.fingerprint.slice(0, 40)}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => onDeleteSmimeCert(c.id)}
                disabled={smimeKeyringDeleting === c.id}
              >
                {smimeKeyringDeleting === c.id ? t("pgpDeleting") : t("webhookDelete")}
              </Button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
