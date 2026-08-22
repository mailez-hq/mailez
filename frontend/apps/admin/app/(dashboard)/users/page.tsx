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
import { api, apiDelete, apiPost, apiPut } from "@/lib/api";
import type { User } from "@/lib/types";

const fmtBytes = (n: number) => `${Math.round(n / 1e6) / 1000} GB`;

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayedName, setDisplayedName] = useState("");
  const [quota, setQuota] = useState(1000000000);
  const [globalAdmin, setGlobalAdmin] = useState(false);
  const [enabled, setEnabled] = useState(true);

  const load = useCallback(async () => {
    try {
      setUsers(await api<User[]>("/users"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  function openEdit(u: User) {
    setEditTarget(u);
    setEmail(u.email);
    setDisplayedName(u.displayed_name);
    setQuota(u.quota_bytes);
    setGlobalAdmin(u.global_admin);
    setEnabled(u.enabled);
    setPassword("");
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    try {
      if (editTarget) {
        await apiPut(`/users/${encodeURIComponent(editTarget.email)}`, {
          password, quota_bytes: quota, global_admin: globalAdmin, enabled, displayed_name: displayedName,
        });
      } else {
        await apiPost("/users", {
          email, password, displayed_name: displayedName, quota_bytes: quota,
          global_admin: globalAdmin, enabled,
        });
      }
      setOpen(false);
      setEditTarget(null);
      setEmail(""); setPassword("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    }
  }

  async function remove(u: User) {
    if (!confirm(`Delete user ${u.email}?`)) return;
    try {
      await apiDelete(`/users/${encodeURIComponent(u.email)}`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Users</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>New user</Button>
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={create} className="space-y-4">
              <DialogHeader><DialogTitle>New user</DialogTitle></DialogHeader>
              <div className="space-y-2">
                <Label>Email</Label>
                <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" required />
              </div>
              <div className="space-y-2">
                <Label>Password</Label>
                <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label>Displayed name</Label>
                <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>Quota (bytes)</Label>
                <Input type="number" value={quota} onChange={(e) => setQuota(Number(e.target.value))} />
              </div>
              <div className="flex items-center justify-between">
                <Label>Global admin</Label>
                <Switch checked={globalAdmin} onCheckedChange={setGlobalAdmin} />
              </div>
              <div className="flex items-center justify-between">
                <Label>Enabled</Label>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <DialogFooter>
                <Button type="submit">{editTarget ? "Save" : "Create"}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Mail accounts</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Quota</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((u) => (
                <TableRow key={u.email}>
                  <TableCell className="font-medium">{u.email}</TableCell>
                  <TableCell>{u.displayed_name}</TableCell>
                  <TableCell>{fmtBytes(u.quota_bytes)}</TableCell>
                  <TableCell>{u.global_admin ? "Admin" : "User"}</TableCell>
                  <TableCell>{u.enabled ? "Enabled" : "Disabled"}</TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(u)}>Edit</Button>
                      <Button variant="ghost" size="sm" onClick={() => remove(u)}>Delete</Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {users.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-zinc-400">No users</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
