"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { ApiError, login, loginTotp, me } from "@/lib/api";

// The root route is the sign-in page. An already-authenticated visitor is
// sent straight into the mailbox; after a successful login we route into it.
export default function Home() {
  const t = useTranslations("login");
  const loginError = (err: unknown) =>
    err instanceof ApiError && err.code === "rate_limited"
      ? t("rateLimited")
      : err instanceof Error
        ? err.message
        : t("error");
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [pw, setPw] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingToken, setPendingToken] = useState("");
  const [code, setCode] = useState("");

  const check = useCallback(() => {
    me()
      .then(() => router.replace("/mail/Inbox"))
      .catch(() => {
        // not authenticated: stay on the sign-in form
      })
      .finally(() => setLoading(false));
  }, [router]);

  useEffect(check, [check]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const res = await login(email, pw);
      if (res.totp_required && res.pending_token) {
        setPendingToken(res.pending_token);
      } else {
        router.replace("/mail/Inbox");
      }
    } catch (err) {
      setError(loginError(err));
    } finally {
      setBusy(false);
    }
  }

  async function onSubmitTotp(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await loginTotp(pendingToken, code);
      router.replace("/mail/Inbox");
    } catch (err) {
      setError(loginError(err));
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
          {pendingToken ? (
            <form onSubmit={onSubmitTotp} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="code">{t("totp")}</Label>
                <Input
                  id="code"
                  inputMode="numeric"
                  autoFocus
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="123456"
                  required
                />
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? t("submitting") : t("verify")}
              </Button>
            </form>
          ) : (
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
          )}
        </CardContent>
      </Card>
    </div>
  );
}
