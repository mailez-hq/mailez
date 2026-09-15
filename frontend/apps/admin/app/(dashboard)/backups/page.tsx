"use client";

// Scheduled encrypted backups: configuration summary, run history, a
// manual trigger and per-run verification (re-read + decrypt the archive).
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { ArchiveRestore, Play, RefreshCw, ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  adminBackup, adminBackupRun, adminBackupVerify, type BackupRun, type BackupStatus,
} from "@/lib/api";
import { cn } from "@/lib/utils";

export default function BackupsPage() {
  const t = useTranslations("backups");
  const ct = useTranslations("common");
  const [status, setStatus] = useState<BackupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<number | "run" | null>(null);
  const [verdict, setVerdict] = useState<Record<number, string>>({});

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setStatus(await adminBackup());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const runNow = async () => {
    setBusy("run");
    setError("");
    try {
      await adminBackupRun();
      // Give the run a moment, then refresh the history.
      await new Promise((r) => setTimeout(r, 1500));
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "trigger failed");
    } finally {
      setBusy(null);
    }
  };

  const verify = async (id: number) => {
    setBusy(id);
    try {
      const res = await adminBackupVerify(id);
      setVerdict((v) => ({ ...v, [id]: t("verifyOk", { entries: res.entries }) }));
    } catch (e) {
      setVerdict((v) => ({ ...v, [id]: e instanceof Error ? e.message : t("verifyFailed") }));
    } finally {
      setBusy(null);
    }
  };

  const fmtTime = (s?: string) => {
    if (!s) return "—";
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString();
  };
  const fmtSize = (n: number) =>
    n > 0 ? `${(n / 1024 / 1024).toFixed(1)} MB` : "—";

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("description")}>
        <Button onClick={load} disabled={loading}>
          <RefreshCw className={cn(loading && "animate-spin")} />
          {loading ? ct("loading") : t("rerun")}
        </Button>
        <Button onClick={runNow} disabled={busy === "run" || !status?.configured || !status?.key_set}>
          <Play />
          {busy === "run" ? ct("loading") : t("runNow")}
        </Button>
      </PageHeader>
      {error && <p className="text-sm text-red-600">{error}</p>}

      {status && (
        <div className="grid gap-3 sm:grid-cols-4">
          <Card><CardContent className="py-4">
            <p className="text-sm font-medium">{t("configured")}</p>
            <p className="text-lg font-bold">
              <Badge variant={status.configured ? "secondary" : "destructive"}>
                {status.configured ? t("yes") : t("no")}
              </Badge>
            </p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-sm font-medium">{t("target")}</p>
            <p className="truncate text-sm font-mono text-muted-foreground" title={status.target}>
              {status.target || "—"}
            </p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-sm font-medium">{t("keySet")}</p>
            <p className="text-lg font-bold">
              <Badge variant={status.key_set ? "secondary" : "destructive"}>
                {status.key_set ? t("yes") : t("no")}
              </Badge>
            </p>
          </CardContent></Card>
          <Card><CardContent className="py-4">
            <p className="text-sm font-medium">{t("schedule")}</p>
            <p className="text-sm text-muted-foreground">
              {t("dailyAt", { hour: status.hour })} · {t("keepN", { n: status.keep })}
            </p>
          </CardContent></Card>
        </div>
      )}

      {!status?.configured && (
        <Card><CardContent className="py-4 text-sm text-muted-foreground">
          {t("notConfigured")}
        </CardContent></Card>
      )}
      {status?.configured && !status.key_set && (
        <Card><CardContent className="py-4 text-sm text-amber-600">
          {t("noKey")}
        </CardContent></Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("runs")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("started")}</TableHead>
                <TableHead>{t("finished")}</TableHead>
                <TableHead>{t("statusCol")}</TableHead>
                <TableHead>{t("size")}</TableHead>
                <TableHead>{t("detail")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(status?.runs || []).map((r: BackupRun) => (
                <TableRow key={r.id}>
                  <TableCell className="whitespace-nowrap">{fmtTime(r.started_at)}</TableCell>
                  <TableCell className="whitespace-nowrap">{fmtTime(r.finished_at)}</TableCell>
                  <TableCell>
                    <Badge variant={r.ok ? "secondary" : "destructive"}>
                      {r.ok ? t("ok") : t("failed")}
                    </Badge>
                  </TableCell>
                  <TableCell>{fmtSize(r.size)}</TableCell>
                  <TableCell className="max-w-56 truncate font-mono text-xs text-muted-foreground" title={r.detail}>
                    {verdict[r.id] || r.detail || "—"}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!r.ok || busy === r.id}
                      onClick={() => verify(r.id)}
                    >
                      <ShieldCheck className="size-4" />
                      {busy === r.id ? ct("loading") : t("verify")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {(status?.runs || []).length === 0 && !loading && (
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
      <p className="flex items-center gap-2 text-xs text-muted-foreground">
        <ArchiveRestore className="size-4" />
        {t("scopeNote")}
      </p>
    </div>
  );
}
