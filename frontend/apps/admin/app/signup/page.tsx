"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Logo } from "@/components/logo";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { signup, signupDomains } from "@/lib/api";
import type { SignupDomain } from "@/lib/types";

export default function SignupPage() {
  const t = useTranslations("signup");
  const [domains, setDomains] = useState<SignupDomain[]>([]);
  const [domain, setDomain] = useState("");
  const [local, setLocal] = useState("");
  const [pw, setPw] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  useEffect(() => {
    signupDomains()
      .then((d) => {
        setDomains(d);
        if (d.length > 0) setDomain(d[0].name);
      })
      .catch(() => setDomains([]));
  }, []);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await signup(`${local}@${domain}`, pw);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("submit"));
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gradient-to-b from-background to-muted/50 p-4">
        <Card className="w-full max-w-sm shadow-sm">
          <CardHeader>
            <CardTitle className="text-xl">{t("success")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Link href="/">
              <Button className="w-full">{t("goToSignin")}</Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center bg-gradient-to-b from-background to-muted/50 p-4">
      <div className="absolute bottom-4 left-4">
        <LocaleSwitcher />
      </div>
      <div className="mb-6 flex items-center gap-3">
        <Logo className="size-11" />
        <h1 className="text-2xl font-extrabold tracking-tight text-foreground">
          Mailez{" "}
          <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
            Admin
          </span>
        </h1>
      </div>
      <Card className="w-full max-w-sm shadow-sm">
        <CardHeader>
          <CardTitle className="text-xl">{t("title")}</CardTitle>
          <CardDescription>{t("description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="domain">{t("domain")}</Label>
              {domains.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("noOpenDomains")}</p>
              ) : (
                <select
                  id="domain"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  {domains.map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({d.user_count}/{d.max_users < 0 ? "∞" : d.max_users})
                    </option>
                  ))}
                </select>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">{t("emailAddress")}</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="email"
                  value={local}
                  onChange={(e) => setLocal(e.target.value)}
                  placeholder="user"
                  required
                  disabled={domains.length === 0}
                />
                <span className="text-sm text-muted-foreground">@{domain}</span>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="pw">{t("password")}</Label>
              <Input
                id="pw"
                type="password"
                value={pw}
                onChange={(e) => setPw(e.target.value)}
                required
                disabled={domains.length === 0}
              />
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            <Button type="submit" className="w-full" disabled={busy || domains.length === 0}>
              {busy ? t("submitting") : t("submit")}
            </Button>
            <p className="text-center text-sm text-muted-foreground">
              <Link href="/" className="hover:underline">
                {t("goToSignin")}
              </Link>
            </p>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
