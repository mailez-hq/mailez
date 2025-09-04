"use client";

import { BadgeCheck } from "lucide-react";
import type { AdminOverview } from "@/lib/api";

// License card for the admin overview, shipped with this extended module
// set (the backend merges the license snapshot into the overview response
// only when this module set is baked in). The default module set renders
// nothing here, so strings are kept local instead of in shared messages.
type LicensedOverview = AdminOverview & {
  license?: {
    edition: string;
    licensee?: string;
    max_mailboxes: number;
    used: number;
    expires_at?: string;
    valid: boolean;
    service_valid: boolean;
    required: boolean;
    features?: string[];
  };
};

export function OverviewExtras({ data }: { data: AdminOverview }) {
  const lic = (data as LicensedOverview).license;
  if (!lic) return null;

  // The built-in dev license ("no file mounted, unrestricted") is not the
  // commercial edition — label it as the development tier.
  const editionLabel =
    lic.edition === "enterprise"
      ? "企业版 / Enterprise"
      : lic.edition === "dev"
        ? "开发版 / Development"
        : "社区版 / Community";

  const sub =
    lic.edition === "enterprise" || lic.edition === "dev"
      ? [
          lic.max_mailboxes > 0
            ? `${lic.used}/${lic.max_mailboxes} 邮箱 mailboxes`
            : "不限 / Unlimited",
          lic.service_valid
            ? lic.expires_at
              ? `服务至 / service until ${lic.expires_at.slice(0, 10)}`
              : ""
            : "服务已到期 / service expired",
          lic.licensee ? `${lic.licensee}` : "",
        ]
          .filter(Boolean)
          .join(" · ")
      : "开源免费，无需授权 / Open source · no license required";

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <BadgeCheck className="size-4" />
        授权 / License
      </div>
      <p className="mt-2 text-2xl font-bold">{editionLabel}</p>
      <p className="mt-1 text-xs text-muted-foreground">{sub}</p>
    </div>
  );
}
