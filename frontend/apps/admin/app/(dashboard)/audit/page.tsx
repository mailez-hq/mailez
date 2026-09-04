"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "@/components/page-header";
import { Pagination } from "@/components/pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { auditLogs } from "@/lib/api";
import type { AuditLog } from "@/lib/types";
import { AUDIT_EXPORT } from "@/modules/audit-export";

export default function AuditPage() {
  const t = useTranslations("audit");
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [total, setTotal] = useState(0);

  const load = useCallback(async () => {
    try {
      const res = await auditLogs(page, pageSize);
      setLogs(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);

  const fmtTime = (s: string) => {
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString();
  };

  const actionText = (l: AuditLog) => l.action || `${l.method} ${l.path}`;

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")} />
      {error && <p className="text-sm text-red-600">{error}</p>}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">{t("recent")}</CardTitle>
          {AUDIT_EXPORT && (
            <Button variant="outline" size="sm" onClick={() => { window.location.href = "/api/v1/audit/export"; }}>
              {t("export")}
            </Button>
          )}
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("time")}</TableHead>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("action")}</TableHead>
                <TableHead>{t("target")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead>{t("ip")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="whitespace-nowrap">{fmtTime(l.created_at)}</TableCell>
                  <TableCell>{l.user}</TableCell>
                  <TableCell className="font-mono text-xs" title={`${l.method} ${l.path}${l.detail ? `?${l.detail}` : ""}`}>
                    {actionText(l)}
                  </TableCell>
                  <TableCell className="max-w-48 truncate font-mono text-xs" title={l.target}>{l.target || "—"}</TableCell>
                  <TableCell>
                    <Badge variant={l.status >= 400 ? "destructive" : "secondary"}>
                      {l.status}
                    </Badge>
                  </TableCell>
                  <TableCell>{l.ip}</TableCell>
                </TableRow>
              ))}
              {logs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground">{t("noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <Pagination
            total={total}
            page={page}
            pageSize={pageSize}
            onPageChange={setPage}
            onPageSizeChange={(s) => { setPage(1); setPageSize(s); }}
          />
        </CardContent>
      </Card>
    </div>
  );
}
