"use client";

import { useCallback, useEffect, useState } from "react";
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
import type { Domain } from "@/lib/types";

const fmtBytes = (n: number) =>
  n > 0 ? `${Math.round((n / 1e9) * 10) / 10} GB` : "unlimited";

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [maxUsers, setMaxUsers] = useState(-1);
  const [maxAliases, setMaxAliases] = useState(-1);

  const load = useCallback(async () => {
    try {
      setDomains(await api<Domain[]>("/domains"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      await apiPost("/domains", { name, max_users: maxUsers, max_aliases: maxAliases });
      setOpen(false);
      setName("");
      setMaxUsers(-1);
      setMaxAliases(-1);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "create failed");
    }
  }

  async function remove(d: Domain) {
    if (!confirm(`Delete domain ${d.name}?`)) return;
    try {
      await apiDelete(`/domains/${encodeURIComponent(d.name)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Domains</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>New domain</Button>
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader>
                <DialogTitle>New domain</DialogTitle>
              </DialogHeader>
              <div className="space-y-2">
                <Label>Domain name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" required />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Max users (-1 = unlimited)</Label>
                  <Input type="number" value={maxUsers} onChange={(e) => setMaxUsers(Number(e.target.value))} />
                </div>
                <div className="space-y-2">
                  <Label>Max aliases (-1 = unlimited)</Label>
                  <Input type="number" value={maxAliases} onChange={(e) => setMaxAliases(Number(e.target.value))} />
                </div>
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">Create</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Served domains</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Max users</TableHead>
                <TableHead>Max aliases</TableHead>
                <TableHead>Quota</TableHead>
                <TableHead className="w-20" />
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
                    <Button variant="ghost" size="sm" onClick={() => remove(d)}>Delete</Button>
                  </TableCell>
                </TableRow>
              ))}
              {domains.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-zinc-400">No domains</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
