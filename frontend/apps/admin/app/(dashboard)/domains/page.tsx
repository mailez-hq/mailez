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
import { api, apiDelete, apiPost, apiPut, domainDkim, generateDomainDkim } from "@/lib/api";
import type { Alternative, DkimInfo, Domain } from "@/lib/types";

const fmtBytes = (n: number) =>
  n > 0 ? `${Math.round((n / 1e9) * 10) / 10} GB` : "unlimited";

export default function DomainsPage() {
  const t = useTranslations("domains");
  const [domains, setDomains] = useState<Domain[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [editTarget, setEditTarget] = useState<Domain | null>(null);

  const [name, setName] = useState("");
  const [maxUsers, setMaxUsers] = useState(-1);
  const [maxAliases, setMaxAliases] = useState(-1);
  const [maxQuota, setMaxQuota] = useState(0);
  const [signupEnabled, setSignupEnabled] = useState(false);
  const [anonmailEnabled, setAnonmailEnabled] = useState(false);
  const [comment, setComment] = useState("");

  const [alternatives, setAlternatives] = useState<Alternative[]>([]);
  const [altName, setAltName] = useState("");

  const [dkim, setDkim] = useState<DkimInfo | null>(null);
  const [dkimCopied, setDkimCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setDomains(await api<Domain[]>("/domains"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const loadAlternatives = useCallback(async (domain: string) => {
    try {
      setAlternatives(await api<Alternative[]>(`/alternatives?domain=${encodeURIComponent(domain)}`));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  const loadDkim = useCallback(async (domain: string) => {
    try {
      setDkim(await domainDkim(domain));
    } catch {
      setDkim(null);
    }
  }, []);

  function openEdit(d: Domain) {
    setEditTarget(d);
    setName(d.name);
    setMaxUsers(d.max_users);
    setMaxAliases(d.max_aliases);
    setMaxQuota(d.max_quota_bytes);
    setSignupEnabled(d.signup_enabled);
    setAnonmailEnabled(d.anonmail_enabled);
    setComment(d.comment);
    setAltName("");
    setDkimCopied(false);
    loadAlternatives(d.name);
    loadDkim(d.name);
    setOpen(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    try {
      if (editTarget) {
        await apiPut(`/domains/${encodeURIComponent(editTarget.name)}`, {
          max_users: maxUsers,
          max_aliases: maxAliases,
          max_quota_bytes: maxQuota,
          signup_enabled: signupEnabled,
          anonmail_enabled: anonmailEnabled,
          comment,
        });
      } else {
        await apiPost("/domains", {
          name, max_users: maxUsers, max_aliases: maxAliases,
          max_quota_bytes: maxQuota, signup_enabled: signupEnabled,
          anonmail_enabled: anonmailEnabled, comment,
        });
      }
      setOpen(false);
      setEditTarget(null);
      setName(""); setMaxUsers(-1); setMaxAliases(-1); setMaxQuota(0);
      setSignupEnabled(false); setAnonmailEnabled(false); setComment("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(d: Domain) {
    if (!confirm(t("deleteConfirm", { name: d.name }))) return;
    try {
      await apiDelete(`/domains/${encodeURIComponent(d.name)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  async function addAlternative(e: React.FormEvent) {
    e.preventDefault();
    if (!editTarget) return;
    try {
      await apiPost("/alternatives", { name: altName, domain_name: editTarget.name });
      setAltName("");
      loadAlternatives(editTarget.name);
    } catch (err) {
      setError(err instanceof Error ? err.message : "add failed");
    }
  }

  async function removeAlternative(a: Alternative) {
    if (!confirm(t("deleteAltConfirm", { name: a.name }))) return;
    try {
      await apiDelete(`/alternatives/${encodeURIComponent(a.name)}`);
      if (editTarget) loadAlternatives(editTarget.name);
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  async function generateDkim() {
    if (!editTarget) return;
    try {
      setDkim(await generateDomainDkim(editTarget.name));
    } catch (err) {
      setError(err instanceof Error ? err.message : "generate failed");
    }
  }

  async function copyRecord() {
    if (!dkim) return;
    await navigator.clipboard.writeText(dkim.record);
    setDkimCopied(true);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button>{t("new")}</Button>} />
          <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
            <form onSubmit={save} className="space-y-4">
              <DialogHeader>
                <DialogTitle>{editTarget ? t("edit") : t("new")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>{t("name")}</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" required disabled={!!editTarget} />
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div className="space-y-2">
                  <Label>{t("maxUsers")}</Label>
                  <Input type="number" value={maxUsers} onChange={(e) => setMaxUsers(Number(e.target.value))} />
                </div>
                <div className="space-y-2">
                  <Label>{t("maxAliases")}</Label>
                  <Input type="number" value={maxAliases} onChange={(e) => setMaxAliases(Number(e.target.value))} />
                </div>
                <div className="space-y-2">
                  <Label>{t("maxQuota")}</Label>
                  <Input type="number" value={maxQuota} onChange={(e) => setMaxQuota(Number(e.target.value))} />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("allowSignup")}</Label>
                <Switch checked={signupEnabled} onCheckedChange={setSignupEnabled} />
              </div>
              <div className="flex items-center justify-between">
                <Label>{t("enableAnonmail")}</Label>
                <Switch checked={anonmailEnabled} onCheckedChange={setAnonmailEnabled} />
              </div>
              <div className="space-y-2">
                <Label>{t("comment")}</Label>
                <Input value={comment} onChange={(e) => setComment(e.target.value)} />
              </div>

              {editTarget && (
                <>
                  <div className="border-t pt-4">
                    <h2 className="mb-2 text-sm font-medium">{t("alternatives")}</h2>
                    <div className="space-y-2">
                      {alternatives.map((a) => (
                        <div key={a.name} className="flex items-center justify-between rounded-md border px-3 py-2">
                          <span className="text-sm">{a.name}</span>
                          <Button variant="ghost" size="sm" onClick={() => removeAlternative(a)}>{t("common:delete")}</Button>
                        </div>
                      ))}
                      {alternatives.length === 0 && (
                        <p className="text-sm text-zinc-400">{t("noAlternatives")}</p>
                      )}
                      <form onSubmit={addAlternative} className="flex items-center gap-2">
                        <Input value={altName} onChange={(e) => setAltName(e.target.value)} placeholder="alt.example.com" required />
                        <Button type="submit" variant="outline" size="sm">{t("common:add")}</Button>
                      </form>
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h2 className="mb-2 text-sm font-medium">{t("dkim")}</h2>
                    <div className="space-y-3">
                      <div className="flex items-center justify-between">
                        <Label>{t("dkimStatus")}</Label>
                        <span className="text-sm">{dkim?.enabled ? t("dkimEnabled") : t("dkimDisabled")}</span>
                      </div>
                      {dkim ? (
                        <>
                          <div className="space-y-2">
                            <Label>{t("dkimRecord")}</Label>
                            <div className="flex items-center gap-2">
                              <code className="flex-1 break-all rounded-md bg-zinc-100 px-3 py-2 text-xs dark:bg-zinc-800">
                                {dkim.record}
                              </code>
                              <Button type="button" variant="outline" size="sm" onClick={copyRecord}>
                                {dkimCopied ? t("dkimCopied") : t("common:copy")}
                              </Button>
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label>{t("dkimPublicKey")}</Label>
                            <code className="block break-all rounded-md bg-zinc-100 px-3 py-2 text-xs dark:bg-zinc-800">
                              {dkim.public_key}
                            </code>
                          </div>
                        </>
                      ) : (
                        <p className="text-sm text-zinc-500">{t("dkimHint")}</p>
                      )}
                      <div className="flex gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={generateDkim}>
                          {dkim ? t("dkimRotate") : t("dkimGenerate")}
                        </Button>
                      </div>
                    </div>
                  </div>
                </>
              )}

              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">{editTarget ? t("common:edit") : t("common:create")}</Button>
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
                <TableHead>{t("maxUsers")}</TableHead>
                <TableHead>{t("maxAliases")}</TableHead>
                <TableHead>{t("quota")}</TableHead>
                <TableHead>{t("signup")}</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {domains.map((d) => (
                <TableRow key={d.name}>
                  <TableCell className="font-medium">{d.name}</TableCell>
                  <TableCell>{d.max_users < 0 ? "∞" : d.max_users}</TableCell>
                  <TableCell>{d.max_aliases < 0 ? "∞" : d.max_aliases}</TableCell>
                  <TableCell>{fmtBytes(d.max_quota_bytes)}</TableCell>
                  <TableCell>{d.signup_enabled ? t("open") : t("closed")}</TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(d)}>{t("common:edit")}</Button>
                      <Button variant="ghost" size="sm" onClick={() => remove(d)}>{t("common:delete")}</Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {domains.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-zinc-400">{t("common:noItems")}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
