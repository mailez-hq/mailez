"use client";

// Traffic report: per-day inbound/outbound volumes sampled from the
// engine's Prometheus counters. Hand-rolled SVG bars keep the deployment
// air-gap-safe (no chart library, no CDN).
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { adminTraffic, type TrafficDay } from "@/lib/api";
import { cn } from "@/lib/utils";

const RANGES = [7, 14, 30] as const;

function Bars({ days, label }: { days: TrafficDay[]; label: (d: TrafficDay) => number }) {
  const values = days.map(label);
  const max = Math.max(1, ...values);
  return (
    <div className="flex h-32 items-end gap-1" aria-hidden>
      {days.map((d, i) => {
        const h = Math.round((values[i] / max) * 100);
        return (
          <div key={d.date} className="group relative flex-1" title={`${d.date}: ${values[i]}`}>
            <div
              className="w-full rounded-t bg-primary/70 transition-colors group-hover:bg-primary"
              style={{ height: `${Math.max(h, 2)}%` }}
            />
          </div>
        );
      })}
    </div>
  );
}

export default function ReportsPage() {
  const t = useTranslations("reports");
  const ct = useTranslations("common");
  const [days, setDays] = useState<number>(14);
  const [data, setData] = useState<{ days: TrafficDay[]; latest_queue_depth: number; sampling: boolean } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async (n: number) => {
    setLoading(true);
    setError("");
    try {
      setData(await adminTraffic(n));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(days); }, [load, days]);

  const rows = data?.days || [];
  const sum = (fn: (d: TrafficDay) => number) => rows.reduce((a, d) => a + fn(d), 0);

  return (
    <div className="space-y-6">
      <PageHeader title={t("title")} description={t("description")}>
        <div className="flex items-center gap-2">
          {RANGES.map((n) => (
            <Button
              key={n}
              variant={n === days ? "default" : "outline"}
              size="sm"
              onClick={() => setDays(n)}
            >
              {n}d
            </Button>
          ))}
          <Button size="sm" variant="outline" onClick={() => load(days)} disabled={loading}>
            <RefreshCw className={cn("size-4", loading && "animate-spin")} />
          </Button>
        </div>
      </PageHeader>

      {error && <p className="text-sm text-red-600">{error}</p>}
      {data && !data.sampling && <p className="text-sm text-amber-600">{t("samplingOff")}</p>}
      {loading && !data && <p className="text-sm text-muted-foreground">{ct("loading")}</p>}

      {rows.length > 0 && (
        <>
          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("inbound")}</CardTitle>
                <CardDescription>{t("inboundTotal", { n: sum((d) => d.in_accepted) })}</CardDescription>
              </CardHeader>
              <CardContent>
                <Bars days={rows} label={(d) => d.in_accepted} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("outbound")}</CardTitle>
                <CardDescription>{t("outboundTotal", { n: sum((d) => d.out_delivered) })}</CardDescription>
              </CardHeader>
              <CardContent>
                <Bars days={rows} label={(d) => d.out_delivered} />
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("daily")}</CardTitle>
              <CardDescription>{t("queue", { n: data?.latest_queue_depth ?? 0 })}</CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("date")}</TableHead>
                    <TableHead>{t("accepted")}</TableHead>
                    <TableHead>{t("rejected")}</TableHead>
                    <TableHead>{t("deferredIn")}</TableHead>
                    <TableHead>{t("delivered")}</TableHead>
                    <TableHead>{t("bounced")}</TableHead>
                    <TableHead>{t("deferredOut")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {[...rows].reverse().map((d) => (
                    <TableRow key={d.date}>
                      <TableCell className="font-mono text-xs">{d.date}</TableCell>
                      <TableCell>{d.in_accepted}</TableCell>
                      <TableCell>{d.in_rejected}</TableCell>
                      <TableCell>{d.in_deferred}</TableCell>
                      <TableCell>{d.out_delivered}</TableCell>
                      <TableCell>{d.out_bounced}</TableCell>
                      <TableCell>{d.out_deferred}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}
      {!loading && rows.length === 0 && !error && (
        <p className="text-sm text-muted-foreground">{t("noData")}</p>
      )}
    </div>
  );
}
