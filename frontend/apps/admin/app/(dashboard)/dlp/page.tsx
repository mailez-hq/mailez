"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { CheckCircle2, Eye, Pencil, Plus, Trash2, XCircle } from "lucide-react";
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
  createDlpRule, decideApproval, deleteDlpRule, dlpApproval, dlpApprovals,
  dlpRules, me, updateDlpRule,
} from "@/lib/api";
import type { DlpRule, PendingApproval } from "@/lib/types";

const emptyRule: Partial<DlpRule> = {
  name: "", enabled: true, pattern: "", is_regex: false, scope: "all",
  action: "block", severity: "medium", approvers: "", hold_hours: 48, note: "",
};

function fmtTime(s: string | null | undefined) {
  if (!s) return "—";
  const d = new Date(s);
  return isNaN(d.getTime()) ? s : d.toLocaleString();
}

export default function DlpPage() {
  const t = useTranslations("dlp");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [rules, setRules] = useState<DlpRule[]>([]);
  const [editing, setEditing] = useState<Partial<DlpRule> | null>(null);
  const [saving, setSaving] = useState(false);

  const [approvals, setApprovals] = useState<PendingApproval[]>([]);
  const [status, setStatus] = useState("pending");
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [detail, setDetail] = useState<PendingApproval | null>(null);
  const [preview, setPreview] = useState("");
  const [reason, setReason] = useState("");
  const [deciding, setDeciding] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);

  const loadRules = useCallback(async () => {
    try {
      const res = await dlpRules();
      setRules(res.data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  const loadApprovals = useCallback(async () => {
    try {
      const res = await dlpApprovals(status, page, pageSize);
      setApprovals(res.data);
      setTotal(res.total);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [status, page, pageSize]);

  useEffect(() => { loadRules(); }, [loadRules]);
  useEffect(() => { loadApprovals(); }, [loadApprovals]);
  useEffect(() => {
    me().then((m) => setIsAdmin(!!m.global_admin)).catch(() => {});
  }, []);

  async function saveRule() {
    if (!editing) return;
    setError(""); setNotice(""); setSaving(true);
    try {
      if (editing.id) {
        await updateDlpRule(editing.id, editing);
      } else {
        await createDlpRule(editing);
      }
      setNotice(t("ruleSaved"));
      setEditing(null);
      loadRules();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  async function removeRule(r: DlpRule) {
    if (!confirm(t("ruleDeleteConfirm", { name: r.name }))) return;
    try {
      await deleteDlpRule(r.id);
      setRules((rs) => rs.filter((x) => x.id !== r.id));
      setNotice(t("ruleDeleted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  async function openDetail(p: PendingApproval) {
    setDetail(p);
    setReason("");
    setPreview("");
    try {
      const res = await dlpApproval(p.id);
      setDetail(res.approval);
      setPreview(res.preview);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  async function decide(decision: "approve" | "reject") {
    if (!detail) return;
    setDeciding(true); setError(""); setNotice("");
    try {
      const updated = await decideApproval(detail.id, decision, reason);
      setDetail(updated);
      setApprovals((rs) => rs.filter((x) => x.id !== updated.id));
      setTotal((n) => Math.max(0, n - 1));
      setNotice(decision === "approve" ? t("approvedMsg") : t("rejectedMsg"));
      if (decision === "reject") setDetail(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "decision failed");
    } finally {
      setDeciding(false);
    }
  }

  const sev = (s: string) => {
    if (s === "high") return <Badge variant="destructive">{t("sevHigh")}</Badge>;
    if (s === "low") return <Badge variant="secondary">{t("sevLow")}</Badge>;
    return <Badge>{t("sevMedium")}</Badge>;
  };

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")} />
      {error && <p className="text-sm text-red-600">{error}</p>}
      {notice && <p className="text-sm text-emerald-600">{notice}</p>}

      {isAdmin && <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle className="text-base">{t("rulesTitle")}</CardTitle>
            <CardDescription>{t("rulesDesc")}</CardDescription>
          </div>
          <Button onClick={() => setEditing({ ...emptyRule })}>
            <Plus className="size-4" /> {t("addRule")}
          </Button>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("colName")}</TableHead>
                <TableHead>{t("colPattern")}</TableHead>
                <TableHead>{t("colScope")}</TableHead>
                <TableHead>{t("colAction")}</TableHead>
                <TableHead>{t("colSeverity")}</TableHead>
                <TableHead>{t("colApprovers")}</TableHead>
                <TableHead>{t("colEnabled")}</TableHead>
                <TableHead>{t("actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="text-sm font-medium">{r.name}</TableCell>
                  <TableCell className="max-w-56 truncate font-mono text-xs" title={r.pattern}>
                    {r.is_regex ? `/ ${r.pattern} /` : r.pattern}
                  </TableCell>
                  <TableCell className="text-xs">{r.scope === "all" ? t("scopeAll") : r.scope}</TableCell>
                  <TableCell>
                    <Badge variant={r.action === "block" ? "destructive" : "default"}>
                      {r.action === "block" ? t("actionBlock") : t("actionHold")}
                    </Badge>
                  </TableCell>
                  <TableCell>{sev(r.severity)}</TableCell>
                  <TableCell className="max-w-40 truncate text-xs" title={r.approvers}>
                    {r.action === "hold" ? (r.approvers || "—") : "—"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={r.enabled ? "secondary" : "outline"}>
                      {r.enabled ? t("on") : t("off")}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button size="icon-sm" variant="ghost" title={t("edit")} onClick={() => setEditing({ ...r })}>
                        <Pencil className="size-4" />
                      </Button>
                      <Button size="icon-sm" variant="ghost" title={t("delete")} className="text-destructive" onClick={() => removeRule(r)}>
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {rules.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground">{t("noRules")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("approvalsTitle")}</CardTitle>
          <CardDescription>{t("approvalsDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex gap-2">
            {["pending", "approved", "rejected"].map((s) => (
              <Button
                key={s}
                size="sm"
                variant={status === s ? "default" : "outline"}
                onClick={() => { setStatus(s); setPage(1); }}
              >
                {s === "pending" ? t("pending") : s === "approved" ? t("approved") : t("rejected")}
              </Button>
            ))}
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("colSubject")}</TableHead>
                <TableHead>{t("colSender")}</TableHead>
                <TableHead>{t("colRecipients")}</TableHead>
                <TableHead>{t("colRule")}</TableHead>
                <TableHead>{t("colSeverity")}</TableHead>
                <TableHead>{t("colCreated")}</TableHead>
                <TableHead>{t("colExpires")}</TableHead>
                <TableHead>{t("actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {approvals.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="max-w-56 truncate text-xs" title={p.subject}>{p.subject || "（无主题）"}</TableCell>
                  <TableCell className="text-xs">{p.from}</TableCell>
                  <TableCell className="max-w-52 truncate text-xs" title={p.recipients}>{p.recipients}</TableCell>
                  <TableCell className="text-xs">{p.rule_name}</TableCell>
                  <TableCell>{sev(p.severity)}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs">{fmtTime(p.created_at)}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs">{fmtTime(p.expires_at)}</TableCell>
                  <TableCell>
                    <Button size="icon-sm" variant="ghost" title={t("view")} onClick={() => openDetail(p)}>
                      <Eye className="size-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {approvals.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground">{t("noApprovals")}</TableCell>
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

      {/* rule editor */}
      <Dialog open={!!editing} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{editing?.id ? t("editRule") : t("addRule")}</DialogTitle>
            <DialogDescription>{t("ruleFormDesc")}</DialogDescription>
          </DialogHeader>
          {editing && (
            <div className="grid gap-3 text-sm">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label>{t("colName")}</Label>
                  <Input value={editing.name || ""} onChange={(e) => setEditing({ ...editing, name: e.target.value })} placeholder={t("phName")} />
                </div>
                <div>
                  <Label>{t("colSeverity")}</Label>
                  <select
                    value={editing.severity || "medium"}
                    onChange={(e) => setEditing({ ...editing, severity: e.target.value })}
                    className="mt-1 w-full rounded-md border border-input bg-background px-2 py-1.5"
                  >
                    <option value="low">{t("sevLow")}</option>
                    <option value="medium">{t("sevMedium")}</option>
                    <option value="high">{t("sevHigh")}</option>
                  </select>
                </div>
              </div>
              <div>
                <Label>{t("colPattern")}</Label>
                <div className="flex gap-2">
                  <Input value={editing.pattern || ""} onChange={(e) => setEditing({ ...editing, pattern: e.target.value })} placeholder={t("phPattern")} className="flex-1 font-mono" />
                  <label className="flex items-center gap-1.5 text-xs">
                    <Switch checked={!!editing.is_regex} onCheckedChange={(v) => setEditing({ ...editing, is_regex: !!v })} />
                    {t("regex")}
                  </label>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label>{t("colAction")}</Label>
                  <select
                    value={editing.action || "block"}
                    onChange={(e) => setEditing({ ...editing, action: e.target.value as "block" | "hold" })}
                    className="mt-1 w-full rounded-md border border-input bg-background px-2 py-1.5"
                  >
                    <option value="block">{t("actionBlock")}</option>
                    <option value="hold">{t("actionHold")}</option>
                  </select>
                </div>
                <div>
                  <Label>{t("colScope")}</Label>
                  <select
                    value={editing.scope || "all"}
                    onChange={(e) => setEditing({ ...editing, scope: e.target.value })}
                    className="mt-1 w-full rounded-md border border-input bg-background px-2 py-1.5"
                  >
                    <option value="all">{t("scopeAll")}</option>
                    <option value="domain:example.com">example.com</option>
                    <option value="domain:other.com">other.com</option>
                  </select>
                </div>
              </div>
              {editing.action === "hold" && (
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <Label>{t("colApprovers")}</Label>
                    <Input value={editing.approvers || ""} onChange={(e) => setEditing({ ...editing, approvers: e.target.value })} placeholder="approver@example.com" />
                  </div>
                  <div>
                    <Label>{t("holdHours")}</Label>
                    <Input
                      type="number" min={1}
                      value={editing.hold_hours || 48}
                      onChange={(e) => setEditing({ ...editing, hold_hours: Number(e.target.value) || 48 })}
                    />
                  </div>
                </div>
              )}
              <div>
                <Label>{t("note")}</Label>
                <Input value={editing.note || ""} onChange={(e) => setEditing({ ...editing, note: e.target.value })} placeholder={t("phNote")} />
              </div>
              <div className="flex items-center justify-between">
                <label className="flex items-center gap-2 text-sm">
                  <Switch checked={!!editing.enabled} onCheckedChange={(v) => setEditing({ ...editing, enabled: !!v })} />
                  {t("colEnabled")}
                </label>
              </div>
              <DialogFooter>
                <Button variant="ghost" onClick={() => setEditing(null)}>{t("cancel")}</Button>
                <Button onClick={saveRule} disabled={saving}>
                  {saving ? t("saving") : t("save")}
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* approval detail */}
      <Dialog open={!!detail} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent className="max-w-3xl">
          {detail && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8">{detail.subject || "（无主题）"}</DialogTitle>
                <DialogDescription>
                  {t("hitRule", { rule: detail.rule_name })} · {fmtTime(detail.created_at)}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-3 text-sm">
                <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                  <span className="text-muted-foreground">{t("colSender")}</span>
                  <span className="break-all">{detail.from}</span>
                  <span className="text-muted-foreground">{t("colRecipients")}</span>
                  <span className="break-all">{detail.recipients}</span>
                  <span className="text-muted-foreground">{t("colExpires")}</span>
                  <span>{fmtTime(detail.expires_at)}</span>
                </div>
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/40 p-3 text-xs">
                  {preview || t("noPreview")}
                </pre>
                {detail.status !== "pending" ? (
                  <div className="text-sm">
                    <Badge variant={detail.status === "approved" ? "secondary" : "destructive"}>
                      {detail.status === "approved" ? t("approved") : t("rejected")}
                    </Badge>
                    <span className="ml-2 text-muted-foreground">
                      {t("decidedBy", { user: detail.approver, at: fmtTime(detail.decision_at) })}
                    </span>
                    {detail.reason && <p className="mt-1">{t("reason")}: {detail.reason}</p>}
                  </div>
                ) : (
                  <>
                    <div>
                      <Label>{t("reason")}</Label>
                      <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("phReason")} />
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button onClick={() => decide("approve")} disabled={deciding}>
                        <CheckCircle2 className="size-4" /> {t("approve")}
                      </Button>
                      <Button variant="destructive" onClick={() => decide("reject")} disabled={deciding}>
                        <XCircle className="size-4" /> {t("reject")}
                      </Button>
                    </div>
                  </>
                )}
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
