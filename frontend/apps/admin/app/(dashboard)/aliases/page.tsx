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
import type { Alias, Page } from "@/lib/types";

export default function AliasesPage() {
  const t = useTranslations("aliases");
  const ct = useTranslations("common");
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Alias | null>(null);

  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [destination, setDestination] = useState("");
  const [wildcard, setWildcard] = useState(false);
  const [disabled, setDisabled] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api<Page<Alias>>(`/aliases?page=${page}&limit=${pageSize}`);
      setAliases(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

  useEffect(() => { load(); }, [load]);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      const body = { email, name, destination, wildcard };
      if (editTarget) {
        await apiPut(`/aliases/${encodeURIComponent(editTarget.email)}`, {
          name,
          destination,
          wildcard,
          disabled,
        });
      } else {
        await apiPost("/aliases", body);
      }
      setOpen(false);
      setEditTarget(null);
      setEmail(""); setName(""); setDestination(""); setWildcard(false); setDisabled(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  function openEdit(a: Alias) {
    setEditTarget(a);
    setEmail(a.email);
    setName(a.name);
    setDestination(a.destination);
    setWildcard(a.wildcard);
    setDisabled(a.disabled);
    setOpen(true);
  }

  async function remove(a: Alias) {
    if (!confirm(t("deleteConfirm", { email: a.email }))) return;
    try {
      await apiDelete(`/aliases/${encodeURIComponent(a.email)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger
            render={(
              <Button onClick={() => { setEditTarget(null); setEmail(""); setName(""); setDestination(""); setWildcard(false); setDisabled(false); }}>
                <Plus />{t("new")}
              </Button>
            )}
          />
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader><DialogTitle>{editTarget ? t("edit") : t("new")}</DialogTitle></DialogHeader>
              <div className="space-y-2">
                <Label>{t("address")}</Label>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="info@example.com" required disabled={!!editTarget} />
              </div>
              <div className="space-y-2">
                <Label>{t("name")}</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Info Desk" />
              </div>
              <div className="space-y-2">
                <Label>{t("destination")}</Label>
                <Input
                  value={destination}
                  onChange={(e) => setDestination(e.target.value)}
                  placeholder="user@example.com"
                  required={!editTarget}
                />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("wildcard")}</Label>
                <Switch checked={wildcard} onCheckedChange={setWildcard} />
              </div>
              {editTarget && (
                <div className="flex items-center justify-between">
                  <Label>{t("disabled")}</Label>
                  <Switch checked={disabled} onCheckedChange={setDisabled} />
                </div>
              )}
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>{ct("cancel")}</Button>
                <Button type="submit">{editTarget ? t("save") : ct("create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </PageHeader>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("forwardingRules")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("alias")}</TableHead>
                <TableHead>{t("destination")}</TableHead>
                <TableHead>{t("wildcard")}</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {aliases.map((a) => (
                <TableRow key={a.email}>
                  <TableCell className="font-medium">{a.email}</TableCell>
                  <TableCell>{a.destination}</TableCell>
                  <TableCell>
                    {a.wildcard ? (
                      <Badge variant="default">{t("yes")}</Badge>
                    ) : (
                      <Badge variant="secondary">{t("no")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="w-10">
                    <RowActions onEdit={() => openEdit(a)} onDelete={() => remove(a)} />
                  </TableCell>
                </TableRow>
              ))}
              {aliases.length === 0 && (
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
