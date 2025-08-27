"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Check, ChevronLeft, ChevronRight, Plus, Search, UserPlus, X } from "lucide-react";
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
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { api, apiDelete, apiPost, apiPut } from "@/lib/api";
import type { Alias, Page, User } from "@/lib/types";

type MemberDraft = { email: string; name: string };

const emptyMember = (): MemberDraft => ({ email: "", name: "" });

export default function GroupsPage() {
  const t = useTranslations("groups");
  const ct = useTranslations("common");
  const [groups, setGroups] = useState<Alias[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Alias | null>(null);

  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [members, setMembers] = useState<MemberDraft[]>([emptyMember()]);
  const [formError, setFormError] = useState("");
  const [userQuery, setUserQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [userPage, setUserPage] = useState(1);
  const [userTotal, setUserTotal] = useState(0);

  const load = useCallback(async () => {
    try {
      const res = await api<Page<Alias>>(`/aliases?group=true&page=${page}&limit=${pageSize}`);
      setGroups(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);

  // Internal user picker: server-side search (debounced) + pagination.
  useEffect(() => {
    const id = setTimeout(() => setDebouncedQuery(userQuery), 300);
    return () => clearTimeout(id);
  }, [userQuery]);

  const loadUsers = useCallback(async () => {
    try {
      const qs = new URLSearchParams({ page: String(userPage), limit: "50" });
      if (debouncedQuery.trim()) qs.set("q", debouncedQuery.trim());
      const res = await api<Page<User>>(`/users?${qs}`);
      setUsers(res.data);
      setUserTotal(res.total);
    } catch {
      // keep manual entry as fallback
    }
  }, [debouncedQuery, userPage]);

  useEffect(() => { loadUsers(); }, [loadUsers]);

  function openCreate() {
    setEditTarget(null);
    setEmail("");
    setName("");
    setMembers([emptyMember()]);
    setFormError("");
    setOpen(true);
  }

  function openEdit(g: Alias) {
    setEditTarget(g);
    setEmail(g.email);
    setName(g.name);
    setMembers(
      g.members && g.members.length > 0
        ? g.members.map((m) => ({ email: m.email, name: m.name || "" }))
        : [emptyMember()],
    );
    setFormError("");
    setOpen(true);
  }

  function setMember(index: number, patch: Partial<MemberDraft>) {
    setMembers((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)));
  }

  function addMember() {
    setMembers((prev) => [...prev, emptyMember()]);
  }

  function removeMember(index: number) {
    setMembers((prev) => prev.filter((_, i) => i !== index));
  }

  function appendUser(u: User) {
    if (members.some((m) => m.email.toLowerCase() === u.email.toLowerCase())) return;
    setMembers((prev) => [...prev, { email: u.email, name: u.displayed_name || "" }]);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    const clean = members
      .map((m) => ({ email: m.email.trim(), name: m.name.trim() }))
      .filter((m) => m.email !== "");
    if (!editTarget && !email.trim()) {
      setFormError(t("emailRequired"));
      return;
    }
    if (clean.length === 0) {
      setFormError(t("memberRequired"));
      return;
    }
    try {
      if (editTarget) {
        await apiPut(`/aliases/${encodeURIComponent(editTarget.email)}`, {
          name,
          members: clean,
        });
      } else {
        await apiPost("/aliases", {
          email: email.trim(),
          name,
          members: clean,
        });
      }
      setOpen(false);
      setEditTarget(null);
      load();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(g: Alias) {
    if (!confirm(t("deleteConfirm", { email: g.email }))) return;
    try {
      await apiDelete(`/aliases/${encodeURIComponent(g.email)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button onClick={openCreate}><Plus />{t("new")}</Button>} />
          <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
            <form onSubmit={save} className="space-y-4">
              <DialogHeader>
                <DialogTitle>{editTarget ? t("edit") : t("new")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>{t("address")}</Label>
                <Input
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="team@example.com"
                  required
                  disabled={!!editTarget}
                />
              </div>
              <div className="space-y-2">
                <Label>{t("name")}</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Sales Team" />
              </div>

              <div className="space-y-2">
                <Label>{t("members")}</Label>
                <div className="space-y-2">
                  {members.map((m, i) => (
                    <div key={i} className="flex items-start gap-2">
                      <Input
                        value={m.email}
                        onChange={(e) => setMember(i, { email: e.target.value })}
                        placeholder={t("memberPlaceholder")}
                        className="flex-1"
                      />
                      <Input
                        value={m.name}
                        onChange={(e) => setMember(i, { name: e.target.value })}
                        placeholder={t("memberName")}
                        className="hidden w-40 sm:block"
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => removeMember(i)}
                        disabled={members.length === 1}
                        className="text-muted-foreground hover:text-destructive"
                        aria-label={ct("delete")}
                      >
                        <X />
                      </Button>
                    </div>
                  ))}
                </div>
                <Button type="button" variant="outline" size="sm" onClick={addMember}>
                  <Plus />{t("addMember")}
                </Button>
              </div>

              <div className="space-y-2 border-t pt-3">
                <Label>{t("internalUsers")}</Label>
                <p className="text-xs text-muted-foreground">{t("memberHint")}</p>
                <div className="relative">
                  <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={userQuery}
                    onChange={(e) => { setUserQuery(e.target.value); setUserPage(1); }}
                    placeholder={t("userSearchPlaceholder")}
                    className="pl-8"
                  />
                </div>
                <div className="max-h-40 overflow-y-auto rounded-md border">
                  {users.map((u) => {
                    const added = members.some((m) => m.email.toLowerCase() === u.email.toLowerCase());
                    return (
                      <button
                        key={u.email}
                        type="button"
                        onClick={() => appendUser(u)}
                        disabled={added}
                        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-muted disabled:opacity-60"
                      >
                        {added
                          ? <Check className="size-3.5 shrink-0 text-primary" />
                          : <UserPlus className="size-3.5 shrink-0 text-muted-foreground" />}
                        <span className="truncate">{u.displayed_name || u.email}</span>
                        <span className="ml-auto truncate text-xs text-muted-foreground">{u.email}</span>
                      </button>
                    );
                  })}
                  {users.length === 0 && (
                    <p className="px-3 py-2 text-xs text-muted-foreground">{t("noMatch")}</p>
                  )}
                </div>
                {userTotal > 0 && (
                  <div className="flex items-center justify-between pt-1">
                    <span className="text-xs text-muted-foreground">
                      {(userPage - 1) * 50 + 1}-{Math.min(userTotal, userPage * 50)} / {userTotal}
                    </span>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={userPage <= 1}
                        onClick={() => setUserPage(userPage - 1)}
                        aria-label={ct("prev")}
                      >
                        <ChevronLeft />
                      </Button>
                      <span className="min-w-10 text-center text-xs text-muted-foreground">
                        {userPage} / {Math.max(1, Math.ceil(userTotal / 50))}
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={userPage >= Math.max(1, Math.ceil(userTotal / 50))}
                        onClick={() => setUserPage(userPage + 1)}
                        aria-label={ct("next")}
                      >
                        <ChevronRight />
                      </Button>
                    </div>
                  </div>
                )}
              </div>

              {formError && <p className="text-sm text-red-600">{formError}</p>}
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>{ct("cancel")}</Button>
                <Button type="submit">{ct("save")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </PageHeader>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("title")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("name")}</TableHead>
                <TableHead>{t("address")}</TableHead>
                <TableHead>{t("members")}</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((g) => (
                <TableRow key={g.email}>
                  <TableCell className="font-medium">{g.name || g.email}</TableCell>
                  <TableCell>{g.email}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{t("memberCount", { n: g.members?.length || 0 })}</Badge>
                  </TableCell>
                  <TableCell className="w-10">
                    <RowActions onEdit={() => openEdit(g)} onDelete={() => remove(g)} />
                  </TableCell>
                </TableRow>
              ))}
              {groups.length === 0 && (
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
      {error && <p className="text-sm text-red-600">{error}</p>}
    </div>
  );
}
