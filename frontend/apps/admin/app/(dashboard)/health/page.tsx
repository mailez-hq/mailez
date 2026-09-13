"use client";

// Domain & system health center: live DNS and service checks per mail
// domain (MX/SPF/DMARC/DKIM/blacklist/autoconfig/MTA-STS) plus local
// system probes. Check ids map to localized labels and fix hints.
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { adminHealth, type HealthItem, type HealthReport } from "@/lib/api";
import { cn } from "@/lib/utils";

const DOMAIN_CHECK_IDS = [
  "mx", "spf", "dmarc", "dkim", "blacklist", "autoconfig", "mta_sts",
] as const;

const SYSTEM_CHECK_IDS = [
  "engine_imap", "engine_mta", "redis", "database", "disk", "memory", "cert",
] as const;

function statusColor(status: string) {
  switch (status) {
    case "ok": return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400";
    case "warn": return "bg-amber-500/15 text-amber-700 dark:text-amber-400";
    case "fail": return "bg-red-500/15 text-red-700 dark:text-red-400";
    default: return "bg-muted text-muted-foreground";
  }
}

export default function HealthPage() {
  const t = useTranslations("health");
  const ct = useTranslations("common");
  const [report, setReport] = useState<HealthReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setReport(await adminHealth());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const counts = { ok: 0, warn: 0, fail: 0, unknown: 0 };
  const tally = (items: HealthItem[]) => {
    for (const item of items) counts[item.status] = (counts[item.status] || 0) + 1;
  };
  (report?.domains || []).forEach((d) => tally(d.items));
  if (report) tally(report.system);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("title")}
        description={t("description", { hostname: report?.hostname || "…" })}
      >
        <Button onClick={load} disabled={loading}>
          <RefreshCw className={cn("size-4", loading && "animate-spin")} />
          {loading ? ct("loading") : t("rerun")}
        </Button>
      </PageHeader>

      {error && <p className="text-sm text-red-600">{error}</p>}

      {report && (
        <div className="grid gap-3 sm:grid-cols-4">
          <Card><CardContent className="py-4">
            <p className="text-2xl font-bold text-emerald-600">{counts.ok}</p>
            <p className="text-xs text-muted-foreground">{t("ok")}</p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-2xl font-bold text-amber-600">{counts.warn}</p>
            <p className="text-xs text-muted-foreground">{t("warn")}</p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-2xl font-bold text-red-600">{counts.fail}</p>
            <p className="text-xs text-muted-foreground">{t("fail")}</p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-2xl font-bold text-muted-foreground">{counts.unknown}</p>
            <p className="text-xs text-muted-foreground">{t("unknown")}</p>
          </CardContent></Card>
        </div>
      )}

      {(report?.domains || []).map((d) => (
        <Card key={d.domain}>
          <CardHeader>
            <CardTitle className="text-base">{d.domain}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("check")}</TableHead>
                  <TableHead>{t("statusHeader")}</TableHead>
                  <TableHead>{t("detail")}</TableHead>
                  <TableHead>{t("hint")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {DOMAIN_CHECK_IDS.map((id) => {
                  const item = d.items.find((i) => i.id === id);
                  if (!item) return null;
                  return (
                    <TableRow key={id}>
                      <TableCell className="font-medium">{t(`checks.${id}`)}</TableCell>
                      <TableCell>
                        <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium", statusColor(item.status))}>
                          {t(`status.${item.status}`)}
                        </span>
                      </TableCell>
                      <TableCell className="max-w-72 break-all font-mono text-xs text-muted-foreground">
                        {item.detail || "—"}
                      </TableCell>
                      <TableCell className="max-w-80 text-xs text-muted-foreground">
                        {item.status === "ok" ? "" : t(`hints.${id}`)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      ))}

      {report && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("systemTitle")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {SYSTEM_CHECK_IDS.map((id) => {
                const item = report.system.find((i) => i.id === id);
                if (!item) return null;
                return (
                  <div key={id} className="flex items-center justify-between rounded-lg border p-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{t(`system.${id}`)}</p>
                      <p className="truncate text-xs text-muted-foreground" title={item.detail}>
                        {item.detail || "—"}
                      </p>
                    </div>
                    <span className={cn("ml-3 shrink-0 inline-flex rounded-full px-2 py-0.5 text-xs font-medium", statusColor(item.status))}>
                      {t(`status.${item.status}`)}
                    </span>
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
