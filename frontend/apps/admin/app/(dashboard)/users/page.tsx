"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Pagination } from "@/components/pagination";
import { RowActions } from "@/components/row-actions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardHeader, CardTitle,
} from "@/components/ui/card";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { api, apiDelete, apiPost, apiPut } from "@/lib/api";
import type { Page, User } from "@/lib/types";

const fmtBytes = (n: number) => `${Math.round(n / 1e6) / 1000} GB`;

// backend serializes time.Time as RFC3339; date inputs need yyyy-mm-dd
const toDateInput = (s: string) => (s && !s.startsWith("0001") ? s.slice(0, 10) : "");

export default function UsersPage() {
  const t = useTranslations("users");
  const ct = useTranslations("common");
  const [users, setUsers] = useState<User[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayedName, setDisplayedName] = useState("");
  const [quota, setQuota] = useState(1000000000);
  const [globalAdmin, setGlobalAdmin] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [enableImap, setEnableImap] = useState(true);
  const [enablePop, setEnablePop] = useState(true);
  const [allowSpoofing, setAllowSpoofing] = useState(false);
  const [forwardEnabled, setForwardEnabled] = useState(false);
  const [forwardDestination, setForwardDestination] = useState("");
  const [forwardKeep, setForwardKeep] = useState(true);
  const [replyEnabled, setReplyEnabled] = useState(false);
  const [replySubject, setReplySubject] = useState("");
  const [replyBody, setReplyBody] = useState("");
  const [replyStartdate, setReplyStartdate] = useState("");
  const [replyEnddate, setReplyEnddate] = useState("");
  const [spamEnabled, setSpamEnabled] = useState(true);
  const [spamMarkAsRead, setSpamMarkAsRead] = useState(true);
  const [spamThreshold, setSpamThreshold] = useState(80);

  const load = useCallback(async () => {
    try {
      const res = await api<Page<User>>(`/users?page=${page}&limit=${pageSize}`);
      setUsers(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);

  function openEdit(u: User) {
    setEditTarget(u);
    setEmail(u.email);
    setDisplayedName(u.displayed_name);
    setQuota(u.quota_bytes);
    setGlobalAdmin(u.global_admin);
    setEnabled(u.enabled);
    setEnableImap(u.enable_imap);
    setEnablePop(u.enable_pop);
    setAllowSpoofing(u.allow_spoofing);
    setForwardEnabled(u.forward_enabled);
    setForwardDestination(u.forward_destination);
    setForwardKeep(u.forward_keep);
    setReplyEnabled(u.reply_enabled);
    setReplySubject(u.reply_subject);
    setReplyBody(u.reply_body);
    setReplyStartdate(toDateInput(u.reply_startdate));
    setReplyEnddate(toDateInput(u.reply_enddate));
    setSpamEnabled(u.spam_enabled);
    setSpamMarkAsRead(u.spam_mark_as_read);
    setSpamThreshold(u.spam_threshold);
    setPassword("");
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    try {
      const body = {
        password,
        quota_bytes: quota,
        global_admin: globalAdmin,
        enabled,
        displayed_name: displayedName,
        enable_imap: enableImap,
        enable_pop: enablePop,
        allow_spoofing: allowSpoofing,
        forward_enabled: forwardEnabled,
        forward_destination: forwardDestination,
        forward_keep: forwardKeep,
        reply_enabled: replyEnabled,
        reply_subject: replySubject,
        reply_body: replyBody,
        reply_startdate: replyStartdate,
        reply_enddate: replyEnddate,
        spam_enabled: spamEnabled,
        spam_mark_as_read: spamMarkAsRead,
        spam_threshold: spamThreshold,
      };
      if (editTarget) {
        await apiPut(`/users/${encodeURIComponent(editTarget.email)}`, body);
      } else {
        await apiPost("/users", { email, ...body });
      }
      setOpen(false);
      setEditTarget(null);
      setEmail(""); setPassword(""); setDisplayedName("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(u: User) {
    if (!confirm(t("deleteConfirm", { email: u.email }))) return;
    try {
      await apiDelete(`/users/${encodeURIComponent(u.email)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button><Plus />{t("new")}</Button>} />
          <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
            <form onSubmit={save} className="space-y-4">
              <DialogHeader>
                <DialogTitle>{editTarget ? t("edit") : t("new")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>{t("email")}</Label>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" required disabled={!!editTarget} />
              </div>
              <div className="space-y-2">
                <Label>{editTarget ? t("keepPassword") : t("password")}</Label>
                <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required={!editTarget} />
              </div>
              <div className="space-y-2">
                <Label>{t("displayedName")}</Label>
                <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>{t("quota")}</Label>
                <Input type="number" value={quota} onChange={(e) => setQuota(Number(e.target.value))} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("globalAdmin")}</Label>
                <Switch checked={globalAdmin} onCheckedChange={setGlobalAdmin} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("enabled")}</Label>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </div>

              <div className="border-t pt-4">
                <h2 className="mb-2 text-sm font-medium">{t("services")}</h2>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label>{t("enableImap")}</Label>
                    <Switch checked={enableImap} onCheckedChange={setEnableImap} />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label>{t("enablePop")}</Label>
                    <Switch checked={enablePop} onCheckedChange={setEnablePop} />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label>{t("allowSpoofing")}</Label>
                    <Switch checked={allowSpoofing} onCheckedChange={setAllowSpoofing} />
                  </div>
                </div>
              </div>

              <div className="border-t pt-4">
                <h2 className="mb-2 text-sm font-medium">{t("forwarding")}</h2>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label>{t("forwardMail")}</Label>
                    <Switch checked={forwardEnabled} onCheckedChange={setForwardEnabled} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("forwardTo")}</Label>
                    <Input
                      value={forwardDestination}
                      onChange={(e) => setForwardDestination(e.target.value)}
                      disabled={!forwardEnabled}
                    />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label>{t("forwardKeep")}</Label>
                    <Switch checked={forwardKeep} onCheckedChange={setForwardKeep} disabled={!forwardEnabled} />
                  </div>
                </div>
              </div>

              <div className="border-t pt-4">
                <h2 className="mb-2 text-sm font-medium">{t("autoReply")}</h2>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label>{t("enableAutoReply")}</Label>
                    <Switch checked={replyEnabled} onCheckedChange={setReplyEnabled} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("replySubject")}</Label>
                    <Input value={replySubject} onChange={(e) => setReplySubject(e.target.value)} disabled={!replyEnabled} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("replyBody")}</Label>
                    <textarea
                      value={replyBody}
                      onChange={(e) => setReplyBody(e.target.value)}
                      disabled={!replyEnabled}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      rows={3}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label>{t("startDate")}</Label>
                      <Input type="date" value={replyStartdate} onChange={(e) => setReplyStartdate(e.target.value)} disabled={!replyEnabled} />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("endDate")}</Label>
                      <Input type="date" value={replyEnddate} onChange={(e) => setReplyEnddate(e.target.value)} disabled={!replyEnabled} />
                    </div>
                  </div>
                </div>
              </div>

              <div className="border-t pt-4">
                <h2 className="mb-2 text-sm font-medium">{t("spamFilter")}</h2>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label>{t("enableSpam")}</Label>
                    <Switch checked={spamEnabled} onCheckedChange={setSpamEnabled} />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label>{t("markAsRead")}</Label>
                    <Switch checked={spamMarkAsRead} onCheckedChange={setSpamMarkAsRead} disabled={!spamEnabled} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("threshold")}</Label>
                    <Input
                      type="number"
                      min={0}
                      max={100}
                      value={spamThreshold}
                      onChange={(e) => setSpamThreshold(Number(e.target.value))}
                      disabled={!spamEnabled}
                    />
                  </div>
                </div>
              </div>

              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">{editTarget ? ct("edit") : ct("create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </PageHeader>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("accounts")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("email")}</TableHead>
                <TableHead>{t("name")}</TableHead>
                <TableHead>{t("quota")}</TableHead>
                <TableHead>{t("role")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((u) => (
                <TableRow key={u.email}>
                  <TableCell className="font-medium">{u.email}</TableCell>
                  <TableCell>{u.displayed_name}</TableCell>
                  <TableCell>{fmtBytes(u.quota_bytes)}</TableCell>
                  <TableCell>
                    {u.global_admin ? (
                      <Badge variant="default">{t("admin")}</Badge>
                    ) : (
                      <Badge variant="secondary">{t("user")}</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {u.enabled ? (
                      <Badge variant="outline">{t("enabled")}</Badge>
                    ) : (
                      <Badge variant="destructive">{t("disabled")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="w-10">
                    <RowActions onEdit={() => openEdit(u)} onDelete={() => remove(u)} />
                  </TableCell>
                </TableRow>
              ))}
              {users.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground">{ct("noItems")}</TableCell>
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
