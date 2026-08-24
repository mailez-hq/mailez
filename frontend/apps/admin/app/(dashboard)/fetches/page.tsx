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
import type { Fetch, Page, User } from "@/lib/types";

export default function FetchesPage() {
  const t = useTranslations("fetches");
  const ct = useTranslations("common");
  const [fetches, setFetches] = useState<Fetch[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Fetch | null>(null);

  const [userEmail, setUserEmail] = useState("");
  const [protocol, setProtocol] = useState("imap");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(993);
  const [tls, setTls] = useState(true);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [folders, setFolders] = useState("");
  const [keep, setKeep] = useState(true);
  const [scan, setScan] = useState(true);
  const [invisible, setInvisible] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api<Page<Fetch>>(`/fetches?page=${page}&limit=${pageSize}`);
      setFetches(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    api<Page<User>>(`/users?page=1&limit=200`).then((r) => setUsers(r.data)).catch(() => {});
  }, []);

  function openEdit(f: Fetch) {
    setEditTarget(f);
    setUserEmail(f.user_email);
    setProtocol(f.protocol);
    setHost(f.host);
    setPort(f.port);
    setTls(f.tls);
    setUsername(f.username);
    setPassword("");
    setFolders(f.folders);
    setKeep(f.keep);
    setScan(f.scan);
    setInvisible(f.invisible);
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      const body = {
        user_email: userEmail, protocol, host, port, tls, username, password,
        folders, keep, scan, invisible,
      };
      if (editTarget) {
        await apiPut(`/fetches/${editTarget.id}`, body);
      } else {
        await apiPost("/fetches", body);
      }
      setOpen(false);
      setEditTarget(null);
      setUserEmail(""); setHost(""); setUsername(""); setPassword(""); setFolders("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(f: Fetch) {
    if (!confirm(t("deleteConfirm", { username: f.username, host: f.host }))) return;
    try {
      await apiDelete(`/fetches/${f.id}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  const status = (f: Fetch) =>
    f.error
      ? { label: t("error"), variant: "destructive" as const }
      : f.last_check
        ? { label: t("ok"), variant: "default" as const }
        : { label: t("never"), variant: "secondary" as const };

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button><Plus />{t("new")}</Button>} />
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader>
                <DialogTitle>{editTarget ? t("edit") : t("new")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>{t("user")}</Label>
                <select
                  value={userEmail}
                  onChange={(e) => setUserEmail(e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  required
                  disabled={!!editTarget}
                >
                  <option value="" disabled>{t("selectUser")}</option>
                  {users.map((u) => (
                    <option key={u.email} value={u.email}>{u.email}</option>
                  ))}
                </select>
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div className="space-y-2">
                  <Label>{t("protocol")}</Label>
                  <select
                    value={protocol}
                    onChange={(e) => {
                      const p = e.target.value;
                      setProtocol(p);
                      setPort(p === "pop3" ? 995 : 993);
                    }}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  >
                    <option value="imap">IMAP</option>
                    <option value="pop3">POP3</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label>{t("host")}</Label>
                  <Input value={host} onChange={(e) => setHost(e.target.value)} required />
                </div>
                <div className="space-y-2">
                  <Label>{t("port")}</Label>
                  <Input type="number" value={port} onChange={(e) => setPort(Number(e.target.value))} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("username")}</Label>
                <Input value={username} onChange={(e) => setUsername(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label>{editTarget ? t("password") : t("passwordNew")}</Label>
                <Input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required={!editTarget}
                />
              </div>
              <div className="space-y-2">
                <Label>{t("folders")}</Label>
                <Input value={folders} onChange={(e) => setFolders(e.target.value)} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("tls")}</Label>
                <Switch checked={tls} onCheckedChange={setTls} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("keep")}</Label>
                <Switch checked={keep} onCheckedChange={setKeep} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("scan")}</Label>
                <Switch checked={scan} onCheckedChange={setScan} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("invisible")}</Label>
                <Switch checked={invisible} onCheckedChange={setInvisible} />
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
                <TableHead>{t("source")}</TableHead>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {fetches.map((f) => (
                <TableRow key={f.id}>
                  <TableCell className="font-medium">{f.username}@{f.host}</TableCell>
                  <TableCell>{f.user_email}</TableCell>
                  <TableCell>
                    <Badge variant={status(f).variant}>{status(f).label}</Badge>
                  </TableCell>
                  <TableCell className="w-10">
                    <RowActions onEdit={() => openEdit(f)} onDelete={() => remove(f)} />
                  </TableCell>
                </TableRow>
              ))}
              {fetches.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-muted-foreground">{ct("noItems")}</TableCell>
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
