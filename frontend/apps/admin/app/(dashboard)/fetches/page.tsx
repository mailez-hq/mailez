"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
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
import type { Fetch, User } from "@/lib/types";

export default function FetchesPage() {
  const t = useTranslations("fetches");
  const [fetches, setFetches] = useState<Fetch[]>([]);
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
      setFetches(await api<Fetch[]>("/fetches"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    api<User[]>("/users").then(setUsers).catch(() => {});
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
    f.error ? t("error") : f.last_check ? t("ok") : t("never");

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button>{t("new")}</Button>} />
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
                  className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
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
                    className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
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
                <Button type="submit">{editTarget ? t("common:edit") : t("common:create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("accounts")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("source")}</TableHead>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {fetches.map((f) => (
                <TableRow key={f.id}>
                  <TableCell className="font-medium">{f.username}@{f.host}</TableCell>
                  <TableCell>{f.user_email}</TableCell>
                  <TableCell>{status(f)}</TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(f)}>{t("common:edit")}</Button>
                      <Button variant="ghost" size="sm" onClick={() => remove(f)}>{t("common:delete")}</Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {fetches.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-zinc-400">{t("common:noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
