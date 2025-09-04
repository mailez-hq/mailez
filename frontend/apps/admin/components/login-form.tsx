"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, dashboardTarget, login, loginTotp, serverSettings } from "@/lib/api";

export function LoginForm() {
  const t = useTranslations("login");
  const loginError = (err: unknown) =>
    err instanceof ApiError && err.code === "rate_limited"
      ? t("rateLimited")
      : err instanceof Error
        ? err.message
        : t("error");
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [pw, setPw] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [pendingToken, setPendingToken] = useState("");
  const [code, setCode] = useState("");
  // Federated sign-in (optional module): the button renders only when the
  // backend mounts the OIDC routes; ?sso_error explains a failed round-trip.
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const [ssoError, setSsoError] = useState("");

  useEffect(() => {
    try {
      const reason = new URLSearchParams(window.location.search).get("sso_error");
      if (reason) setSsoError(reason);
    } catch {
      // ignore malformed query
    }
    serverSettings()
      .then((s) => setSsoEnabled(Boolean(s?.oidc?.enabled)))
      .catch(() => setSsoEnabled(false));
  }, []);

  function startSso() {
    window.location.href =
      "/api/v1/sso/oidc/start?next=" + encodeURIComponent(dashboardTarget());
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await login(email, pw);
      if (res.totp_required && res.pending_token) {
        setPendingToken(res.pending_token);
      } else {
        // Overview is the landing page; ?next= (session-expiry bounce)
        // sends the admin back to where they were instead.
        router.push(dashboardTarget());
        router.refresh();
      }
    } catch (err) {
      setError(loginError(err));
    } finally {
      setLoading(false);
    }
  }

  async function onSubmitTotp(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await loginTotp(pendingToken, code);
      router.push(dashboardTarget());
      router.refresh();
    } catch (err) {
      setError(loginError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card className="w-full max-w-sm shadow-sm">
      <CardHeader>
        <CardTitle className="text-xl">{t("title")}</CardTitle>
        <CardDescription>{t("description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {ssoError && (
          <div
            role="alert"
            className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
          >
            {t("ssoError", { reason: ssoError })}
          </div>
        )}
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
            {error && <p className="text-sm text-red-600">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? t("submitting") : t("verify")}
            </Button>
          </form>
        ) : (
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">{t("email")}</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pw">{t("password")}</Label>
              <Input
                id="pw"
                type="password"
                value={pw}
                onChange={(e) => setPw(e.target.value)}
                required
              />
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? t("submitting") : t("submit")}
            </Button>
            {ssoEnabled && (
              <Button type="button" variant="outline" className="w-full" onClick={startSso}>
                {t("ssoButton")}
              </Button>
            )}
          </form>
        )}
      </CardContent>
    </Card>
  );
}
