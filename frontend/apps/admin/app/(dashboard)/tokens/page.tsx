"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Pagination } from "@/components/pagination";
import { RowActions } from "@/components/row-actions";
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
import type { Page, Token, User } from "@/lib/types";

type TokenResult = Token & { token: string };

export default function TokensPage() {
  const t = useTranslations("tokens");
  const ct = useTranslations("common");
  const [tokens, setTokens] = useState<Token[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [users, setUsers] = useState<User[]>([]);
  // Page-level error (load/delete) renders outside the modal: a delete that
  // fails with the dialog closed must stay visible. Form errors use
  // formError inside the dialog.
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [ip, setIp] = useState("");
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api<Page<Token>>(`/tokens?page=${page}&limit=${pageSize}`);
      setTokens(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    api<Page<User>>(`/users?page=1&limit=200`).then((r) => setUsers(r.data)).catch(() => {});
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await apiPost<TokenResult>("/tokens", { email, ip });
      setSecret(res.token);
      setCopied(false);
      load();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "create failed");
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
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog
          open={open}
          onOpenChange={(v) => { setOpen(v); if (!v) setSecret(""); if (v) setFormError(""); }}
        >
          <DialogTrigger render={<Button><Plus />{t("new")}</Button>} />
          <DialogContent>
            {secret ? (
              <div className="space-y-4">
                <DialogHeader>
                  <DialogTitle>{t("copyTitle")}</DialogTitle>
                </DialogHeader>
                <p className="text-sm text-muted-foreground">{t("copyHint")}</p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 break-all rounded-md bg-muted px-3 py-2 text-sm">
                    {secret}
                  </code>
                  <Button type="button" variant="outline" onClick={copySecret}>
                    {copied ? "✓" : ct("copy")}
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
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
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
                {formError && <p className="text-sm text-red-600">{formError}</p>}
                <DialogFooter>
                  <Button type="submit">{ct("create")}</Button>
                </DialogFooter>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </PageHeader>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <Card>
        <CardHeader><CardTitle className="text-base">{t("accounts")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("user")}</TableHead>
                <TableHead>{t("ip")}</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((tok) => (
                <TableRow key={tok.id}>
                  <TableCell className="font-medium">{tok.user_email}</TableCell>
                  <TableCell>{tok.ip || t("ipAny")}</TableCell>
                  <TableCell className="w-10">
                    <RowActions onDelete={() => remove(tok)} />
                  </TableCell>
                </TableRow>
              ))}
              {tokens.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="text-center text-muted-foreground">{ct("noItems")}</TableCell>
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
