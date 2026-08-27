"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Download, Eye, Plus, Search, Trash2 } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Pagination } from "@/components/pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  archiveExportUrl, archiveMessage, archiveMessages, archiveSettings,
  deleteArchiveMessage, reviewArchiveMessage, saveArchiveSettings,
} from "@/lib/api";
import type { ArchiveSettings, ArchivedMessage } from "@/lib/types";

type Filters = {
  direction: string;
  domain: string;
  from: string;
  to: string;
  q: string;
  date_from: string;
  date_to: string;
  reviewed: string;
};

const emptyFilters: Filters = {
  direction: "", domain: "", from: "", to: "", q: "", date_from: "", date_to: "", reviewed: "",
};

function fmtTime(s: string | null | undefined) {
  if (!s) return "—";
  const d = new Date(s);
  return isNaN(d.getTime()) ? s : d.toLocaleString();
}

function fmtSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export default function ArchivePage() {
  const t = useTranslations("archive");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // settings
  const [global, setGlobal] = useState<ArchiveSettings>({
    id: 0, domain: "", enabled: true, capture_inbound: true, capture_outbound: true,
    retention_days: 0, updated_at: "",
  });
  const [domains, setDomains] = useState<ArchiveSettings[]>([]);
  const [newDomain, setNewDomain] = useState("");
  const [domainOverrides, setDomainOverrides] = useState<Record<string, ArchiveSettings>>({});

  // search
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [applied, setApplied] = useState<Filters>(emptyFilters);
  const [rows, setRows] = useState<ArchivedMessage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);

  // detail
  const [detail, setDetail] = useState<ArchivedMessage | null>(null);
  const [preview, setPreview] = useState("");
  const [note, setNote] = useState("");

  const loadSettings = useCallback(async () => {
    try {
      const res = await archiveSettings();
      setGlobal(res.global);
      setDomains(res.domains);
      const map: Record<string, ArchiveSettings> = {};
      for (const d of res.domains) map[d.domain] = d;
      setDomainOverrides(map);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  const loadMessages = useCallback(async (f: Filters, p: number, limit: number) => {
    try {
      const params: Record<string, string> = {};
      if (f.direction) params.direction = f.direction;
      if (f.domain) params.domain = f.domain;
      if (f.from) params.from = f.from;
      if (f.to) params.to = f.to;
      if (f.q) params.q = f.q;
      if (f.date_from) params.date_from = f.date_from;
      if (f.date_to) params.date_to = f.date_to;
      if (f.reviewed) params.reviewed = f.reviewed;
      const res = await archiveMessages(params, p, limit);
      setRows(res.data);
      setTotal(res.total);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "search failed");
    }
  }, []);

  useEffect(() => { loadSettings(); }, [loadSettings]);
  useEffect(() => { loadMessages(applied, page, pageSize); }, [applied, page, pageSize, loadMessages]);

  async function saveGlobal() {
    setError(""); setNotice("");
    try {
      await saveArchiveSettings({
        domain: "", enabled: global.enabled,
        capture_inbound: global.capture_inbound,
        capture_outbound: global.capture_outbound,
        retention_days: Number(global.retention_days) || 0,
      });
      setNotice(t("saved"));
      loadSettings();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    }
  }

  async function saveOverride(domain: string) {
    setError(""); setNotice("");
    const o = domainOverrides[domain];
    if (!o) return;
    try {
      await saveArchiveSettings({
        domain,
        enabled: o.enabled,
        capture_inbound: o.capture_inbound,
        capture_outbound: o.capture_outbound,
        retention_days: Number(o.retention_days) || 0,
      });
      setNotice(t("saved"));
      loadSettings();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    }
  }

  function addOverride() {
    const d = newDomain.trim().toLowerCase();
    if (!d || domainOverrides[d]) return;
    setDomainOverrides({
      ...domainOverrides,
      [d]: { id: 0, domain: d, enabled: global.enabled, capture_inbound: true, capture_outbound: true, retention_days: 0, updated_at: "" },
    });
    setNewDomain("");
  }

  function onSearch() {
    setPage(1);
    setApplied(filters);
  }

  async function openDetail(row: ArchivedMessage) {
    setDetail(row);
    setNote(row.review_note || "");
    setPreview("");
    try {
      const res = await archiveMessage(row.id);
      setDetail(res.message);
      setPreview(res.preview);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  async function markReviewed(reviewed: boolean) {
    if (!detail) return;
    try {
      const updated = await reviewArchiveMessage(detail.id, reviewed, note);
      setDetail(updated);
      setRows((rs) => rs.map((r) => (r.id === updated.id ? updated : r)));
      setNotice(reviewed ? t("reviewed") : t("reopened"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    }
  }

  async function removeMessage(row: ArchivedMessage) {
    if (!confirm(t("deleteConfirm", { subject: row.subject || row.id }))) return;
    try {
      await deleteArchiveMessage(row.id);
      setRows((rs) => rs.filter((r) => r.id !== row.id));
      setTotal((n) => Math.max(0, n - 1));
      setDetail(null);
      setNotice(t("deleted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  function exportMbox() {
    const params: Record<string, string> = {};
    if (applied.direction) params.direction = applied.direction;
    if (applied.domain) params.domain = applied.domain;
    if (applied.from) params.from = applied.from;
    if (applied.to) params.to = applied.to;
    if (applied.q) params.q = applied.q;
    window.location.href = archiveExportUrl(params);
  }

  const filterInputs: Array<{ key: keyof Filters; type?: string; phKey: string }> = [
    { key: "q", phKey: "phSubject" },
    { key: "from", phKey: "phFrom" },
    { key: "to", phKey: "phTo" },
    { key: "date_from", type: "date", phKey: "phDateFrom" },
    { key: "date_to", type: "date", phKey: "phDateTo" },
  ];

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")} />
      {error && <p className="text-sm text-red-600">{error}</p>}
      {notice && <p className="text-sm text-emerald-600">{notice}</p>}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("policyTitle")}</CardTitle>
          <CardDescription>{t("policyDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <Label>{t("enabled")}</Label>
                <Switch checked={global.enabled} onCheckedChange={(v) => setGlobal({ ...global, enabled: !!v })} />
              </div>
              <div className="flex items-center justify-between gap-3">
                <Label>{t("captureInbound")}</Label>
                <Switch checked={global.capture_inbound} onCheckedChange={(v) => setGlobal({ ...global, capture_inbound: !!v })} />
              </div>
              <div className="flex items-center justify-between gap-3">
                <Label>{t("captureOutbound")}</Label>
                <Switch checked={global.capture_outbound} onCheckedChange={(v) => setGlobal({ ...global, capture_outbound: !!v })} />
              </div>
              <div className="flex items-center gap-3">
                <Label className="shrink-0">{t("retentionDays")}</Label>
                <Input
                  type="number" min={0}
                  value={global.retention_days}
                  onChange={(e) => setGlobal({ ...global, retention_days: Number(e.target.value) || 0 })}
                  className="w-32"
                />
              </div>
              <Button onClick={saveGlobal}>{t("savePolicy")}</Button>
            </div>

            <div className="space-y-3">
              <Label>{t("domainOverrides")}</Label>
              <div className="flex gap-2">
                <Input
                  placeholder={t("phDomain")}
                  value={newDomain}
                  onChange={(e) => setNewDomain(e.target.value)}
                  className="max-w-56"
                />
                <Button variant="outline" onClick={addOverride} disabled={!newDomain.trim()}>
                  <Plus className="size-4" /> {t("addOverride")}
                </Button>
              </div>
              {domains.map((d) => {
                const o = domainOverrides[d.domain];
                return (
                  <div key={d.domain} className="rounded-md border p-3">
                    <div className="mb-2 flex items-center justify-between">
                      <span className="text-sm font-medium">{d.domain}</span>
                      <Badge variant={o?.enabled ? "secondary" : "destructive"}>
                        {o?.enabled ? t("on") : t("off")}
                      </Badge>
                    </div>
                    {o && (
                      <div className="grid grid-cols-2 gap-x-4 gap-y-2">
                        <div className="flex items-center justify-between gap-2">
                          <Label className="text-xs">{t("captureInbound")}</Label>
                          <Switch checked={o.capture_inbound} onCheckedChange={(v) => setDomainOverrides({ ...domainOverrides, [o.domain]: { ...o, capture_inbound: !!v } })} />
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <Label className="text-xs">{t("captureOutbound")}</Label>
                          <Switch checked={o.capture_outbound} onCheckedChange={(v) => setDomainOverrides({ ...domainOverrides, [o.domain]: { ...o, capture_outbound: !!v } })} />
                        </div>
                        <div className="col-span-2 flex items-center gap-3">
                          <Label className="shrink-0 text-xs">{t("retentionDays")}</Label>
                          <Input
                            type="number" min={0}
                            value={o.retention_days}
                            onChange={(e) => setDomainOverrides({ ...domainOverrides, [o.domain]: { ...o, retention_days: Number(e.target.value) || 0 } })}
                            className="w-28"
                          />
                          <Button size="sm" variant="outline" onClick={() => saveOverride(o.domain)}>
                            {t("saveOverride")}
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("searchTitle")}</CardTitle>
          <CardDescription>{t("searchDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-end gap-2">
            <div>
              <Label className="text-xs">{t("direction")}</Label>
              <select
                value={filters.direction}
                onChange={(e) => setFilters({ ...filters, direction: e.target.value })}
                className="mt-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
              >
                <option value="">{t("all")}</option>
                <option value="inbound">{t("inbound")}</option>
                <option value="outbound">{t("outbound")}</option>
              </select>
            </div>
            <div>
              <Label className="text-xs">{t("domain")}</Label>
              <Input
                placeholder={t("phDomain")}
                value={filters.domain}
                onChange={(e) => setFilters({ ...filters, domain: e.target.value })}
                className="mt-1 w-40"
              />
            </div>
            <div>
              <Label className="text-xs">{t("reviewed")}</Label>
              <select
                value={filters.reviewed}
                onChange={(e) => setFilters({ ...filters, reviewed: e.target.value })}
                className="mt-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
              >
                <option value="">{t("all")}</option>
                <option value="true">{t("reviewedYes")}</option>
                <option value="false">{t("reviewedNo")}</option>
              </select>
            </div>
            {filterInputs.map((f) => (
              <div key={f.key}>
                <Label className="text-xs">{t(f.key)}</Label>
                <Input
                  type={f.type || "text"}
                  placeholder={t(f.phKey)}
                  value={filters[f.key]}
                  onChange={(e) => setFilters({ ...filters, [f.key]: e.target.value })}
                  className="mt-1 w-44"
                />
              </div>
            ))}
            <div className="flex gap-2">
              <Button onClick={onSearch}>
                <Search className="size-4" /> {t("search")}
              </Button>
              <Button variant="outline" onClick={exportMbox}>
                <Download className="size-4" /> {t("export")}
              </Button>
            </div>
          </div>

          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("colDate")}</TableHead>
                <TableHead>{t("colDir")}</TableHead>
                <TableHead>{t("colDomain")}</TableHead>
                <TableHead>{t("colFrom")}</TableHead>
                <TableHead>{t("colTo")}</TableHead>
                <TableHead>{t("colSubject")}</TableHead>
                <TableHead>{t("colSize")}</TableHead>
                <TableHead>{t("colReviewed")}</TableHead>
                <TableHead>{t("actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="whitespace-nowrap text-xs">{fmtTime(r.archived_at)}</TableCell>
                  <TableCell>
                    <Badge variant={r.direction === "inbound" ? "secondary" : "default"}>
                      {r.direction === "inbound" ? t("inbound") : t("outbound")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{r.domain || "—"}</TableCell>
                  <TableCell className="max-w-52 truncate text-xs" title={r.from || r.envelope_from}>
                    {r.from || r.envelope_from || "—"}
                  </TableCell>
                  <TableCell className="max-w-52 truncate text-xs" title={r.to || r.envelope_to}>
                    {r.to || r.envelope_to || "—"}
                  </TableCell>
                  <TableCell className="max-w-64 truncate text-xs">{r.subject || "（无主题）"}</TableCell>
                  <TableCell className="text-xs">{fmtSize(r.size)}</TableCell>
                  <TableCell>
                    {r.reviewed ? (
                      <Badge className="bg-emerald-600 text-white">{t("reviewedYes")}</Badge>
                    ) : (
                      <Badge variant="outline">{t("reviewedNo")}</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button size="icon-sm" variant="ghost" title={t("view")} onClick={() => openDetail(r)}>
                        <Eye className="size-4" />
                      </Button>
                      <Button size="icon-sm" variant="ghost" title={t("delete")} className="text-destructive" onClick={() => removeMessage(r)}>
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={9} className="text-center text-muted-foreground">{t("noItems")}</TableCell>
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

      <Dialog open={!!detail} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent className="max-w-3xl">
          {detail && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8">
                  {detail.subject || "（无主题）"}
                </DialogTitle>
                <DialogDescription>
                  <span className="text-xs text-muted-foreground">
                    {detail.direction === "inbound" ? t("inbound") : t("outbound")} · {detail.domain || "—"} · {fmtTime(detail.date)}
                  </span>
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-3 text-sm">
                <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                  <span className="text-muted-foreground">{t("from")}</span>
                  <span className="break-all">{detail.from || detail.envelope_from || "—"}</span>
                  <span className="text-muted-foreground">{t("to")}</span>
                  <span className="break-all">{detail.to || detail.envelope_to || "—"}</span>
                  {detail.cc && (
                    <>
                      <span className="text-muted-foreground">Cc</span>
                      <span className="break-all">{detail.cc}</span>
                    </>
                  )}
                  <span className="text-muted-foreground">{t("colDate")}</span>
                  <span>{fmtTime(detail.date)}</span>
                  <span className="text-muted-foreground">{t("colSize")}</span>
                  <span>{fmtSize(detail.size)}</span>
                  <span className="text-muted-foreground">{t("messageId")}</span>
                  <span className="break-all font-mono text-xs">{detail.message_id || "—"}</span>
                  <span className="text-muted-foreground">{t("expiresAt")}</span>
                  <span>{fmtTime(detail.expires_at)}</span>
                </div>
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/40 p-3 text-xs">
                  {preview || t("noPreview")}
                </pre>
                <div className="flex items-center gap-2">
                  <Label className="shrink-0">{t("reviewNote")}</Label>
                  <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("phNote")} />
                </div>
                <div className="flex flex-wrap gap-2">
                  <a
                    href={`/api/v1/archive/messages/${detail.id}/raw`}
                    download
                    className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-border bg-background px-2.5 text-sm font-medium whitespace-nowrap text-foreground transition-all hover:bg-muted dark:border-input dark:bg-input/30 dark:hover:bg-input/50"
                  >
                    <Download className="size-4" /> {t("downloadEml")}
                  </a>
                  {!detail.reviewed ? (
                    <Button onClick={() => markReviewed(true)}>{t("markReviewed")}</Button>
                  ) : (
                    <Button variant="outline" onClick={() => markReviewed(false)}>{t("reopen")}</Button>
                  )}
                  {detail.reviewed && detail.reviewed_by && (
                    <span className="self-center text-xs text-muted-foreground">
                      {t("reviewedBy", { user: detail.reviewed_by, at: fmtTime(detail.reviewed_at) })}
                    </span>
                  )}
                </div>
              </div>
              <DialogFooter>
                <Button variant="ghost" onClick={() => setDetail(null)}>{t("close")}</Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
