"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  Archive, BadgeCheck, Database, FileStack, Globe, ShieldAlert, Users,
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

  const cards = [
    {
      key: "license",
      value:
        data.license?.edition === "enterprise"
          ? t("licenseEnterprise")
          : t("licenseDev"),
      sub: data.license
        ? [
            data.license.max_mailboxes > 0
              ? t("licenseUsage", { used: data.license.used, max: data.license.max_mailboxes })
              : t("licenseUnlimited"),
            data.license.valid
              ? data.license.expires_at
                ? t("licenseExpires", { date: data.license.expires_at.slice(0, 10) })
                : ""
              : t("licenseExpired"),
            data.license.licensee ? t("licenseLicensee", { name: data.license.licensee }) : "",
          ]
            .filter(Boolean)
            .join(" · ")
        : "",
      icon: BadgeCheck,
    },
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
          {data.engine} · {data.domain} · {data.hostname}
        </p>
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
