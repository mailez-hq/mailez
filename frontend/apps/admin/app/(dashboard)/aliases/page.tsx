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
import { api, apiDelete, apiPost } from "@/lib/api";
import type { Alias } from "@/lib/types";

export default function AliasesPage() {
  const t = useTranslations("aliases");
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);

  const [email, setEmail] = useState("");
  const [destination, setDestination] = useState("");
  const [wildcard, setWildcard] = useState(false);

  const load = useCallback(async () => {
    try {
      setAliases(await api<Alias[]>("/aliases"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      await apiPost("/aliases", { email, destination, wildcard });
      setOpen(false);
      setEmail(""); setDestination(""); setWildcard(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
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
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button>{t("new")}</Button>} />
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader><DialogTitle>{t("new")}</DialogTitle></DialogHeader>
              <div className="space-y-2">
                <Label>{t("address")}</Label>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="info@example.com" required />
              </div>
              <div className="space-y-2">
                <Label>{t("destination")}</Label>
                <Input value={destination} onChange={(e) => setDestination(e.target.value)} placeholder="user@example.com" required />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("wildcard")}</Label>
                <Switch checked={wildcard} onCheckedChange={setWildcard} />
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter><Button type="submit">{t("common:create")}</Button></DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("forwardingRules")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("alias")}</TableHead>
                <TableHead>{t("destination")}</TableHead>
                <TableHead>{t("wildcard")}</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {aliases.map((a) => (
                <TableRow key={a.email}>
                  <TableCell className="font-medium">{a.email}</TableCell>
                  <TableCell>{a.destination}</TableCell>
                  <TableCell>{a.wildcard ? t("yes") : t("no")}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => remove(a)}>{t("common:delete")}</Button>
                  </TableCell>
                </TableRow>
              ))}
              {aliases.length === 0 && (
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
