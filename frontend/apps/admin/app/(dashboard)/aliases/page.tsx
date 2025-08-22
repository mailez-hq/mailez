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
import { Switch } from "@/components/ui/switch";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { api, apiDelete, apiPost } from "@/lib/api";
import type { Alias } from "@/lib/types";

export default function AliasesPage() {
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
    if (!confirm(`Delete alias ${a.email}?`)) return;
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
        <h1 className="text-2xl font-semibold">Aliases</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>New alias</Button>
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader><DialogTitle>New alias</DialogTitle></DialogHeader>
              <div className="space-y-2">
                <Label>Alias address</Label>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="info@example.com" required />
              </div>
              <div className="space-y-2">
                <Label>Destination (comma separated)</Label>
                <Input value={destination} onChange={(e) => setDestination(e.target.value)} placeholder="user@example.com" required />
              </div>
              <div className="flex items-center justify-between">
                <Label>Wildcard</Label>
                <Switch checked={wildcard} onCheckedChange={setWildcard} />
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter><Button type="submit">Create</Button></DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Forwarding rules</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Alias</TableHead>
                <TableHead>Destination</TableHead>
                <TableHead>Wildcard</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {aliases.map((a) => (
                <TableRow key={a.email}>
                  <TableCell className="font-medium">{a.email}</TableCell>
                  <TableCell>{a.destination}</TableCell>
                  <TableCell>{a.wildcard ? "Yes" : "No"}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => remove(a)}>Delete</Button>
                  </TableCell>
                </TableRow>
              ))}
              {aliases.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-zinc-400">No aliases</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
