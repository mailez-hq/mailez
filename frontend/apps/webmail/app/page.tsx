"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { MailView } from "@/components/mail-view";
import { login, me, type Me } from "@/lib/api";

export default function Home() {
  const t = useTranslations("login");
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [pw, setPw] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const check = useCallback(() => {
    me().then(setUser).catch(() => setUser(null)).finally(() => setLoading(false));
  }, []);

  useEffect(check, [check]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await login(email, pw);
      setUser(await me());
    } catch (err) {
      setError(err instanceof Error ? err.message : t("error"));
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-primary/20 border-t-primary" />
        <span className="sr-only">{t("title")}</span>
      </div>
    );
  }

  if (user) {
    return <MailView me={user} />;
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="absolute top-4 right-4">
        <LocaleSwitcher />
      </div>
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <span className="text-3xl font-extrabold tracking-tight">
            mail
            <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
              ez
            </span>
          </span>
          <CardTitle className="text-base font-semibold">{t("title")}</CardTitle>
          <CardDescription>{t("description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">{t("email")}</Label>
              <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pw">{t("password")}</Label>
              <Input id="pw" type="password" value={pw} onChange={(e) => setPw(e.target.value)} required />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={busy}>
              {busy ? t("submitting") : t("submit")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
