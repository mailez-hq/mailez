"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { auditLogs } from "@/lib/api";
import type { AuditLog } from "@/lib/types";

export default function AuditPage() {
  const t = useTranslations("audit");
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setLogs(await auditLogs());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const fmtTime = (s: string) => {
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString();
  };

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">{t("title")}</h1>
      {error && <p className="text-sm text-red-600">{error}</p>}
      <Card>
        <CardHeader><CardTitle className="text-base">{t("recent")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("time")}</TableHead>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("method")}</TableHead>
                <TableHead>{t("path")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead>{t("ip")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="whitespace-nowrap">{fmtTime(l.created_at)}</TableCell>
                  <TableCell>{l.user}</TableCell>
                  <TableCell>{l.method}</TableCell>
                  <TableCell className="font-mono text-xs">{l.path}</TableCell>
                  <TableCell>{l.status}</TableCell>
                  <TableCell>{l.ip}</TableCell>
                </TableRow>
              ))}
              {logs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-zinc-400">{t("noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
