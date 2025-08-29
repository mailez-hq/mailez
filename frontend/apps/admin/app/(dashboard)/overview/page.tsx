"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  Archive, BadgeCheck, Database, FileStack, Globe, HardDrive, LifeBuoy, Mail, Server, ShieldAlert, Users,
} from "lucide-react";
import { adminOverview, type AdminOverview } from "@/lib/api";

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export default function OverviewPage() {
  const t = useTranslations("overview");
  const [data, setData] = useState<AdminOverview | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    adminOverview()
      .then(setData)
      .catch((e) => setError(e instanceof Error ? e.message : "load failed"));
  }, []);

  if (error) return <p className="text-sm text-destructive">{error}</p>;
  if (!data) return <p className="text-sm text-muted-foreground">{t("loading")}</p>;

  // mailezine is the only engine; the edition follows the license
  // (community = free single-node tier, dev = built-in unlimited,
  // enterprise = licensed). A loaded license is reported separately.
  const lic = data.license;
  const communityEdition = lic?.edition === "community";
  const engineLabel = t("engineMailezine");
  const dbLabel =
    data.db_driver === "mysql"
      ? t("dbMysql")
      : data.db_driver === "sqlite"
        ? t("dbSqlite")
        : data.db_driver || t("blobUnknown");
  const kvLabel =
    data.kv_backend === "tidb"
      ? t("kvTidb")
      : data.kv_backend === "pebble"
        ? t("kvPebble")
        : t("blobUnknown");
  const blobLabel =
    data.blob_backend === "minio"
      ? t("blobMinio")
      : data.blob_backend === "local"
        ? t("blobLocal")
        : t("blobUnknown");
  const licenseSub = () => {
    // The community edition is free: it never shows license info, even if
    // an enterprise license file happens to be mounted.
    if (communityEdition) return t("licenseFree");
    if (!lic) return `${t("licenseDev")} · ${t("licenseUnlimited")}`;
    const parts = [
      lic.max_mailboxes > 0
        ? t("licenseUsage", { used: lic.used, max: lic.max_mailboxes })
        : t("licenseUnlimited"),
      lic.service_valid
        ? lic.expires_at
          ? t("licenseServiceEnds", { date: lic.expires_at.slice(0, 10) })
          : ""
        : t("licenseServiceExpired"),
      lic.licensee ? t("licenseLicensee", { name: lic.licensee }) : "",
    ].filter(Boolean);
    return parts.join(" · ");
  };
  const serviceValue = data.service
    ? data.service.tier === "premium"
      ? t("servicePremium")
      : t("serviceStandard")
    : t("serviceNone");
  const serviceSub = data.service
    ? [
        data.service.valid
          ? data.service.expires_at
            ? t("serviceEnds", { date: data.service.expires_at.slice(0, 10) })
            : ""
          : t("serviceExpired"),
        data.service.licensee ? t("licenseLicensee", { name: data.service.licensee }) : "",
      ]
        .filter(Boolean)
        .join(" · ")
    : t("serviceHint");

  const cards = [
    {
      key: "license",
      value: communityEdition ? t("licenseCommunity") : t("licenseEnterprise"),
      sub: licenseSub(),
      icon: BadgeCheck,
    },
    { key: "service", value: serviceValue, sub: serviceSub, icon: LifeBuoy },
    { key: "users", value: `${data.users}`, sub: t("usersEnabled", { n: data.users_enabled }), icon: Users },
    { key: "domains", value: `${data.domains}`, sub: t("aliases", { n: data.aliases }), icon: Globe },
    { key: "orgContacts", value: `${data.org_contacts}`, sub: t("ldap"), icon: Database },
    { key: "pendingApprovals", value: `${data.pending_approvals}`, sub: t("dlp"), icon: ShieldAlert },
    { key: "archived", value: `${data.archived_messages}`, sub: t("compliance"), icon: Archive },
    { key: "drive", value: `${data.drive_files}`, sub: fmtBytes(data.drive_bytes), icon: FileStack },
    { key: "uploads", value: fmtBytes(data.upload_bytes), sub: t("largeAttachments"), icon: FileStack },
  ];

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">
          {communityEdition ? t("licenseCommunity") : t("licenseEnterprise")} · {engineLabel} ·{" "}
          {data.domain} · {data.hostname}
        </p>
      </div>
      {/* Infrastructure */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Server className="size-4" />
          {t("infrastructure")}
        </div>
        <div className="mt-3 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div className="flex items-center gap-2.5">
            <Mail className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">{t("mailEngine")}</p>
              <p className="mt-0.5 truncate text-sm font-medium">{engineLabel}</p>
            </div>
          </div>
          <div className="flex items-center gap-2.5">
            <Database className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">{t("controlDb")}</p>
              <p className="mt-0.5 truncate text-sm font-medium">{dbLabel}</p>
            </div>
          </div>
          <div className="flex items-center gap-2.5">
            <Database className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">{t("engineKv")}</p>
              <p className="mt-0.5 truncate text-sm font-medium">{kvLabel}</p>
            </div>
          </div>
          <div className="flex items-center gap-2.5">
            <HardDrive className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">{t("blobStore")}</p>
              <p className="mt-0.5 truncate text-sm font-medium">{blobLabel}</p>
            </div>
          </div>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {cards.map((card) => {
          const Icon = card.icon;
          return (
            <div key={card.key} className="rounded-xl border border-border bg-card p-4">
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Icon className="size-4" />
                {t(card.key)}
              </div>
              <p className="mt-2 text-2xl font-bold">{card.value}</p>
              <p className="mt-1 text-xs text-muted-foreground">{card.sub}</p>
            </div>
          );
        })}
      </div>
    </div>
  );
}
