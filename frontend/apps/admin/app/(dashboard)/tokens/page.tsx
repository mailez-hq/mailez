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
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { api, apiDelete, apiPost } from "@/lib/api";
import type { Token, User } from "@/lib/types";

type TokenResult = Token & { token: string };

export default function TokensPage() {
  const t = useTranslations("tokens");
  const [tokens, setTokens] = useState<Token[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [ip, setIp] = useState("");
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setTokens(await api<Token[]>("/tokens"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    api<User[]>("/users").then(setUsers).catch(() => {});
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await apiPost<TokenResult>("/tokens", { email, ip });
      setSecret(res.token);
      setCopied(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  async function remove(tok: Token) {
    if (!confirm(t("deleteConfirm", { email: tok.user_email }))) return;
    try {
      await apiDelete(`/tokens/${tok.id}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  async function copySecret() {
    await navigator.clipboard.writeText(secret);
    setCopied(true);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <Dialog
          open={open}
          onOpenChange={(v) => { setOpen(v); if (!v) setSecret(""); }}
        >
          <DialogTrigger render={<Button>{t("new")}</Button>} />
          <DialogContent>
            {secret ? (
              <div className="space-y-4">
                <DialogHeader>
                  <DialogTitle>{t("copyTitle")}</DialogTitle>
                </DialogHeader>
                <p className="text-sm text-zinc-500">{t("copyHint")}</p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 break-all rounded-md bg-zinc-100 px-3 py-2 text-sm dark:bg-zinc-800">
                    {secret}
                  </code>
                  <Button type="button" variant="outline" onClick={copySecret}>
                    {copied ? "✓" : t("common:copy")}
                  </Button>
                </div>
                <DialogFooter>
                  <Button onClick={() => { setOpen(false); setSecret(""); }}>
                    {t("done")}
                  </Button>
                </DialogFooter>
              </div>
            ) : (
              <form onSubmit={create} className="space-y-4">
                <DialogHeader>
                  <DialogTitle>{t("app")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-2">
                  <Label>{t("user")}</Label>
                  <select
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                    required
                  >
                    <option value="" disabled>{t("selectUser")}</option>
                    {users.map((u) => (
                      <option key={u.email} value={u.email}>{u.email}</option>
                    ))}
                  </select>
                </div>
                <div className="space-y-2">
                  <Label>{t("ip")}</Label>
                  <Input value={ip} onChange={(e) => setIp(e.target.value)} placeholder="1.2.3.4" />
                </div>
                {error && <p className="text-sm text-red-600">{error}</p>}
                <DialogFooter>
                  <Button type="submit">{t("common:create")}</Button>
                </DialogFooter>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("accounts")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("ip")}</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((tok) => (
                <TableRow key={tok.id}>
                  <TableCell className="font-medium">{tok.user_email}</TableCell>
                  <TableCell>{tok.ip || t("ipAny")}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => remove(tok)}>{t("common:delete")}</Button>
                  </TableCell>
                </TableRow>
              ))}
              {tokens.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="text-center text-zinc-400">{t("common:noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
