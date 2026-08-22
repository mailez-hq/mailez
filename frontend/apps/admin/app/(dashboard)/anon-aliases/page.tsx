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
import {
  anonAliases, anonmailDomains, createAnonAlias, deleteAnonAlias,
} from "@/lib/api";
import type { Alias } from "@/lib/types";

export default function AnonAliasesPage() {
  const t = useTranslations("anonAliases");
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [domains, setDomains] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [domain, setDomain] = useState("");
  const [displayName, setDisplayName] = useState("");

  const load = useCallback(async () => {
    try {
      setAliases(await anonAliases());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    anonmailDomains().then((d) => {
      setDomains(d);
      if (d.length > 0) setDomain(d[0]);
    }).catch(() => setDomains([]));
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      await createAnonAlias(domain, displayName);
      setOpen(false);
      setDisplayName("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  async function remove(a: Alias) {
    if (!confirm(t("deleteConfirm", { email: a.email }))) return;
    try {
      await deleteAnonAlias(a.email);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  const domainOf = (email: string) => email.slice(email.indexOf("@") + 1);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button disabled={domains.length === 0}>{t("new")}</Button>} />
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader>
                <DialogTitle>{t("new")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>{t("domain")}</Label>
                <select
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                  required
                >
                  {domains.map((d) => (
                    <option key={d} value={d}>{d}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-2">
                <Label>{t("displayName")}</Label>
                <Input
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="my-shop"
                  required
                />
              </div>
              <p className="text-sm text-zinc-500">
                {t("hint", { example: `${displayName || "my-shop"}.xxxx@${domain}` })}
              </p>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">{t("common:create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("yours")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("alias")}</TableHead>
                <TableHead>{t("deliversTo")}</TableHead>
                <TableHead>{t("domainCol")}</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {aliases.map((a) => (
                <TableRow key={a.email}>
                  <TableCell className="font-medium">{a.email}</TableCell>
                  <TableCell>{a.destination}</TableCell>
                  <TableCell>{domainOf(a.email)}</TableCell>
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
