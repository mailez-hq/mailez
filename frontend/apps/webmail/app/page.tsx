"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Globe, LayoutDashboard, Mail, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LocaleSwitcher } from "@/components/locale-switcher";
import {
  ApiError, login, loginTotp, me, serverSettings,
  type BrandingConfig, type ServerSettings,
} from "@/lib/api";
import { readLastFolder, readPreferences } from "@/lib/preferences";

// Where to send a signed-in user: the deep link they asked for (?next=), else
// the workspace dashboard (unless they opted out), else their last-visited
// folder / inbox. next is only trusted when it stays inside the /mail area,
// so the login page can't be used as an open redirect.
function mailboxTarget(): string {
  try {
    const next = new URLSearchParams(window.location.search).get("next");
    if (
      next &&
      (next === "/mail" || next.startsWith("/mail/")) &&
      !next.startsWith("//") &&
      !next.includes("\\")
    ) {
      return next;
    }
  } catch {
    // ignore malformed query
  }
  if (readPreferences().landing === "home") return "/home";
  const last = readLastFolder();
  return last ? `/mail/${encodeURIComponent(last)}` : "/mail/Inbox";
}

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
  const [settings, setSettings] = useState<ServerSettings | null>(null);

  // Enterprise branding from the server (admin console). Empty fields fall
  // back to the built-in Mailez brand below.
  const brand: BrandingConfig = settings?.branding ?? {};
  const brandTitle = brand.title?.trim() || "Mailez";
  const brandSubtitle = brand.subtitle?.trim() || t("subtitle");
  const features: { icon: typeof Globe; text: string }[] = [
    { icon: ShieldCheck, text: brand.feature1?.trim() || t("featSecurity") },
    { icon: LayoutDashboard, text: brand.feature2?.trim() || t("featWorkspace") },
    { icon: Globe, text: brand.feature3?.trim() || t("featAnywhere") },
  ].filter((f) => f.text);

  const check = useCallback(() => {
    // Only probe authentication on deep links (?next=) where an already
    // signed-in visitor should be sent straight into the mailbox. A plain
    // visit to the sign-in page skips the probe: /sso/me would 401 and the
    // browser would log it as console noise.
    if (!new URLSearchParams(window.location.search).has("next")) {
      setLoading(false);
      return;
    }
    me()
      .then(() => router.replace(mailboxTarget()))
      .catch(() => {
        // not authenticated: stay on the sign-in form
      })
      .finally(() => setLoading(false));
  }, [router]);

  useEffect(check, [check]);

  useEffect(() => {
    serverSettings()
      .then(setSettings)
      .catch(() => setSettings(null));
  }, []);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const res = await login(email, pw);
      if (res.totp_required && res.pending_token) {
        setPendingToken(res.pending_token);
      } else {
        router.replace(mailboxTarget());
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
      router.replace(mailboxTarget());
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
    <div className="flex min-h-screen">
      {/* Brand panel: hero image / gradient showcase, hidden on small screens */}
      <div className="relative hidden w-1/2 flex-col justify-between overflow-hidden bg-gradient-to-br from-[#2F8E6C] to-[#2E6E8E] p-10 text-white lg:flex xl:w-[55%]">
        {brand.hero_url ? (
          <>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={brand.hero_url}
              alt=""
              className="absolute inset-0 size-full object-cover"
            />
            <div className="absolute inset-0 bg-black/45" />
          </>
        ) : (
          <>
            <Mail className="pointer-events-none absolute -top-12 -right-12 size-72 text-white/10" />
            <div className="pointer-events-none absolute -bottom-24 -left-24 size-72 rounded-full bg-white/5" />
          </>
        )}
        <div className="relative flex items-center gap-3">
          {brand.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={brand.logo_url} alt={brandTitle} className="h-10 w-auto" />
          ) : (
            <span className="text-2xl font-extrabold tracking-tight">Mailez</span>
          )}
          <div className="leading-tight">
            <p className="text-xl font-bold">{brandTitle}</p>
            {brandSubtitle && <p className="text-sm text-white/80">{brandSubtitle}</p>}
          </div>
        </div>
        <div className="relative space-y-8">
          <h1 className="max-w-md text-4xl leading-tight font-bold">
            {brand.tagline?.trim() || t("tagline")}
          </h1>
          <ul className="space-y-3">
            {features.map(({ icon: Icon, text }) => (
              <li key={text} className="flex items-center gap-3 text-white/85">
                <Icon className="size-5 shrink-0" />
                {text}
              </li>
            ))}
          </ul>
        </div>
        <p className="relative text-sm text-white/60">
          {brand.copyright?.trim() || `© ${new Date().getFullYear()} Mailez`}
        </p>
      </div>

      {/* Sign-in form panel */}
      <div className="relative flex min-w-0 flex-1 justify-center p-4">
        <div className="absolute top-4 right-4">
          <LocaleSwitcher />
        </div>
        <div className="w-full max-w-sm pt-[7vh]">
          {/* Compact brand header on small screens (brand panel is hidden) */}
          <div className="mb-5 flex items-center justify-center gap-2.5 lg:hidden">
            {brand.logo_url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={brand.logo_url} alt={brandTitle} className="h-8 w-auto" />
            ) : (
              <span className="text-2xl font-extrabold tracking-tight">
                Mail
                <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
                  ez
                </span>
              </span>
            )}
            <div className="text-left leading-tight">
              <p className="text-base font-bold">{brandTitle}</p>
              {brandSubtitle && <p className="text-xs text-muted-foreground">{brandSubtitle}</p>}
            </div>
          </div>
          <Card>
            <CardHeader className="items-center text-center">
              {brand.logo_url ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={brand.logo_url} alt={brandTitle} className="h-10 w-auto" />
              ) : (
                <span className="text-3xl font-extrabold tracking-tight">
                  Mail
                  <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
                    ez
                  </span>
                </span>
              )}
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
          {settings && (
            <details className="mt-4 rounded-lg border border-border p-3 text-xs text-muted-foreground">
              <summary className="cursor-pointer font-medium text-foreground">
                {t("mailSettings")}
              </summary>
              <p className="mt-1">{t("mailSettingsHint")}</p>
              <ul className="mt-2 space-y-1">
                <li>
                  {t("smtpServer")}：<code>{settings.hostname}</code>
                  <span className="ml-1">（{t("serverPorts", { ports: `${settings.smtp.ssl}/${settings.smtp.submission}` })}）</span>
                </li>
                <li>
                  {t("pop3Server")}：<code>{settings.hostname}</code>
                  <span className="ml-1">（{t("serverPorts", { ports: `${settings.pop3.plain}/${settings.pop3.ssl}` })}）</span>
                </li>
                <li>
                  {t("imapServer")}：<code>{settings.hostname}</code>
                  <span className="ml-1">（{t("serverPorts", { ports: `${settings.imap.plain}/${settings.imap.ssl}` })}）</span>
                </li>
              </ul>
            </details>
          )}
        </div>
      </div>
    </div>
  );
}
