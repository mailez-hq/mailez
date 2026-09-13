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
import {
  api, apiDelete, apiPost, apiPut, domainDkim, domainDnsRecords, generateDomainDkim,
  type DnsWizardView,
} from "@/lib/api";
import type { Alternative, DkimInfo, Domain, Page } from "@/lib/types";

const fmtBytes = (n: number) =>
  n > 0 ? `${Math.round((n / 1e9) * 10) / 10} GB` : "unlimited";

export default function DomainsPage() {
  const t = useTranslations("domains");
  const ct = useTranslations("common");
  const [domains, setDomains] = useState<Domain[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [open, setOpen] = useState(false);
  // Page-level error (load/domain delete) renders outside the modal; the
  // edit dialog (form + alternatives + DKIM) reports through formError.
  const [error, setError] = useState("");
  const [formError, setFormError] = useState("");
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

  // DNS setup wizard per domain.
  const [wizard, setWizard] = useState<DnsWizardView | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [wizardLoading, setWizardLoading] = useState(false);
  const [copiedId, setCopiedId] = useState("");

  const load = useCallback(async () => {
    try {
      const res = await api<Page<Domain>>(`/domains?page=${page}&limit=${pageSize}`);
      setDomains(res.data);
      setTotal(res.total);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, [page, pageSize]);

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
      setFormError(err instanceof Error ? err.message : "save failed");
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
      setFormError(err instanceof Error ? err.message : "add failed");
    }
  }

  async function removeAlternative(a: Alternative) {
    if (!confirm(t("deleteAltConfirm", { name: a.name }))) return;
    try {
      await apiDelete(`/alternatives/${encodeURIComponent(a.name)}`);
      if (editTarget) loadAlternatives(editTarget.name);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "delete failed");
    }
  }

  async function generateDkim() {
    if (!editTarget) return;
    try {
      setDkim(await generateDomainDkim(editTarget.name));
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "generate failed");
    }
  }

  async function copyRecord() {
    if (!dkim) return;
    await navigator.clipboard.writeText(dkim.record);
    setDkimCopied(true);
  }

  async function openWizard(d: Domain) {
    setWizardOpen(true);
    setWizardLoading(true);
    setWizard(null);
    setCopiedId("");
    try {
      setWizard(await domainDnsRecords(d.name));
    } catch (err) {
      setError(err instanceof Error ? err.message : "load failed");
      setWizardOpen(false);
    } finally {
      setWizardLoading(false);
    }
  }

  async function copyWizardValue(id: string, value: string) {
    if (!value) return;
    await navigator.clipboard.writeText(value);
    setCopiedId(id);
  }

  return (
    <div className="space-y-4">
      <PageHeader title={t("title")} description={t("desc")}>
        <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (v) setFormError(""); }}>
          <DialogTrigger render={<Button><Plus />{t("new")}</Button>} />
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
                          <Button variant="ghost" size="sm" onClick={() => removeAlternative(a)}>{ct("delete")}</Button>
                        </div>
                      ))}
                      {alternatives.length === 0 && (
                        <p className="text-sm text-muted-foreground">{t("noAlternatives")}</p>
                      )}
                      <form onSubmit={addAlternative} className="flex items-center gap-2">
                        <Input value={altName} onChange={(e) => setAltName(e.target.value)} placeholder="alt.example.com" required />
                        <Button type="submit" variant="outline" size="sm">{ct("add")}</Button>
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
                              <code className="flex-1 break-all rounded-md bg-muted px-3 py-2 text-xs">
                                {dkim.record}
                              </code>
                              <Button type="button" variant="outline" size="sm" onClick={copyRecord}>
                                {dkimCopied ? t("dkimCopied") : ct("copy")}
                              </Button>
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label>{t("dkimPublicKey")}</Label>
                            <code className="block break-all rounded-md bg-muted px-3 py-2 text-xs">
                              {dkim.public_key}
                            </code>
                          </div>
                        </>
                      ) : (
                        <p className="text-sm text-muted-foreground">{t("dkimHint")}</p>
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

              {formError && <p className="text-sm text-red-600">{formError}</p>}
              <DialogFooter>
                <Button type="submit">{editTarget ? ct("edit") : ct("create")}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </PageHeader>

      {/* DNS setup wizard: expected records with live verification. */}
      <Dialog open={wizardOpen} onOpenChange={setWizardOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("wizardTitle", { name: wizard?.domain || "\u2026" })}</DialogTitle>
          </DialogHeader>
          {wizardLoading && <p className="text-sm text-muted-foreground">{ct("loading")}</p>}
          {wizard && (
            <>
              <p className="text-sm text-muted-foreground">
                {t("wizardDesc", { hostname: wizard.hostname })}
              </p>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("wizardRecord")}</TableHead>
                    <TableHead>{t("wizardName")}</TableHead>
                    <TableHead>{t("wizardValue")}</TableHead>
                    <TableHead>{t("wizardStatus")}</TableHead>
                    <TableHead className="w-10" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {wizard.records.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell>
                        <Badge variant="outline">{r.type}</Badge>{" "}
                        <span className="text-xs">{t(`wizardIds.${r.id}`)}</span>
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {r.name === "@" ? wizard.domain : `${r.name}.${wizard.domain}`}
                      </TableCell>
                      <TableCell className="max-w-64">
                        {r.value ? (
                          <code className="block break-all text-xs" title={r.value}>
                            {r.value.length > 80 ? r.value.slice(0, 77) + "\u2026" : r.value}
                          </code>
                        ) : (
                          <span className="text-xs text-muted-foreground">\u2014</span>
                        )}
                        {r.detail && <p className="text-xs text-muted-foreground">{r.detail}</p>}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            r.status === "ok" ? "default" : r.status === "missing" ? "secondary" : "destructive"
                          }
                        >
                          {t(`wizardStatus_${r.status}`)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {r.value && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => copyWizardValue(r.id, r.value)}
                            title={ct("copy")}
                          >
                            {copiedId === r.id ? t("wizardCopied") : ct("copy")}
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          )}
        </DialogContent>
      </Dialog>

      {error && <p className="text-sm text-red-600">{error}</p>}

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
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {domains.map((d) => (
                <TableRow key={d.name}>
                  <TableCell className="font-medium">{d.name}</TableCell>
                  <TableCell>{d.max_users < 0 ? "∞" : d.max_users}</TableCell>
                  <TableCell>{d.max_aliases < 0 ? "∞" : d.max_aliases}</TableCell>
                  <TableCell>{fmtBytes(d.max_quota_bytes)}</TableCell>
                  <TableCell>
                    {d.signup_enabled ? (
                      <Badge variant="default">{t("open")}</Badge>
                    ) : (
                      <Badge variant="secondary">{t("closed")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="w-10">
                    <div className="flex items-center gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openWizard(d)} title={t("wizard")}>
                        DNS
                      </Button>
                      <RowActions onEdit={() => openEdit(d)} onDelete={() => remove(d)} />
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {domains.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground">{ct("noItems")}</TableCell>
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
