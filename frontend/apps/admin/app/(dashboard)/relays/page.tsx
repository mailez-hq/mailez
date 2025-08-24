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
import { api, apiDelete, apiPost, apiPut } from "@/lib/api";
import type { Relay } from "@/lib/types";

export default function RelaysPage() {
  const t = useTranslations("relays");
  const ct = useTranslations("common");
  const [relays, setRelays] = useState<Relay[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Relay | null>(null);
  const [name, setName] = useState("");
  const [smtp, setSmtp] = useState("");

  const load = useCallback(async () => {
    try {
      setRelays(await api<Relay[]>("/relays"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  function openEdit(r: Relay) {
    setEditTarget(r);
    setName(r.name);
    setSmtp(r.smtp);
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      if (editTarget) {
        await apiPut(`/relays/${encodeURIComponent(editTarget.name)}`, { smtp });
      } else {
        await apiPost("/relays", { name, smtp });
      }
      setOpen(false);
      setEditTarget(null);
      setName(""); setSmtp("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(r: Relay) {
    if (!confirm(t("deleteConfirm", { name: r.name }))) return;
    try {
      await apiDelete(`/relays/${encodeURIComponent(r.name)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

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
                <Label>{t("name")}</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="example.com"
                  required
                  disabled={!!editTarget}
                />
              </div>
              <div className="space-y-2">
                <Label>{t("smtp")}</Label>
                <Input
                  value={smtp}
                  onChange={(e) => setSmtp(e.target.value)}
                  placeholder="smtp.example.com"
                />
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">{editTarget ? ct("edit") : ct("create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("served")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("name")}</TableHead>
                <TableHead>{t("smtp")}</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {relays.map((r) => (
                <TableRow key={r.name}>
                  <TableCell className="font-medium">{r.name}</TableCell>
                  <TableCell>{r.smtp || t("smtpDefault")}</TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(r)}>{ct("edit")}</Button>
                      <Button variant="ghost" size="sm" onClick={() => remove(r)}>{ct("delete")}</Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {relays.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="text-center text-zinc-400">{ct("noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
