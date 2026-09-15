"use client";

// Active IP bans from the ban engine: one row per banned source address,
// with the auth surface that triggered it and a manual lift action.
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Ban, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { adminBanLift, adminBans, type BanRecord } from "@/lib/api";
import { cn } from "@/lib/utils";

export default function BansPage() {
  const t = useTranslations("bans");
  const ct = useTranslations("common");
  const [bans, setBans] = useState<BanRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [lifting, setLifting] = useState<number | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setBans(await adminBans());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const lift = async (id: number) => {
    setLifting(id);
    setError("");
    try {
      await adminBanLift(id);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("liftFailed"));
    } finally {
      setLifting(null);
    }
  };

  const fmtTime = (s: string) => {
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString();
  };

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("description")}>
        <Button onClick={load} disabled={loading}>
          <RefreshCw className={cn(loading && "animate-spin")} />
          {loading ? ct("loading") : t("rerun")}
        </Button>
      </PageHeader>
      {error && <p className="text-sm text-red-600">{error}</p>}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("ip")}</TableHead>
                <TableHead>{t("surface")}</TableHead>
                <TableHead>{t("failed")}</TableHead>
                <TableHead>{t("bannedAt")}</TableHead>
                <TableHead>{t("until")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {bans.map((b) => (
                <TableRow key={b.id}>
                  <TableCell className="font-mono">{b.ip}</TableCell>
                  <TableCell>{b.surface}</TableCell>
                  <TableCell>{b.failed}</TableCell>
                  <TableCell className="whitespace-nowrap">{fmtTime(b.created_at)}</TableCell>
                  <TableCell className="whitespace-nowrap">{fmtTime(b.until)}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={lifting === b.id}
                      onClick={() => lift(b.id)}
                    >
                      <Ban className="size-4" />
                      {lifting === b.id ? t("lifting") : t("lift")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {bans.length === 0 && !loading && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground">
                    {t("empty")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
