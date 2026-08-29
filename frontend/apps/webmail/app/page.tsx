"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Globe, LayoutDashboard, Mail, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LocaleSwitcher } from "@/components/locale-switcher";
import {
  ApiError, login, loginTotp, me, serverSettings,
  type BrandingConfig, type ServerSettings,
} from "@/lib/api";
import { readLastFolder, readPreferences } from "@/lib/preferences";
import { prefetchFolders } from "@/lib/folders-cache";

// Where to send a signed-in user: the deep link they asked for (?next=), else
// the workspace dashboard (unless they opted out), else their last-visited
// folder / inbox. next is only trusted when it stays inside the /mail area,
// so the login page can't be used as an open redirect.
function mailboxTarget(): string {
  try {
    const next = new URLSearchParams(window.location.search).get("next");
    if (
      next &&
      (next === "/mail" || next.startsWith("/mail/") || next === "/home") &&
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
  // Loading is only ever true while the deep-link auth probe runs, so the
  // initial value already accounts for plain visits (no ?next= → not loading).
  // window is guarded for SSR: client components also render on the server
  // for the initial HTML, where window is undefined.
  const [loading, setLoading] = useState(
    () =>
      typeof window !== "undefined" &&
      new URLSearchParams(window.location.search).has("next"),
  );
  const [loginId, setLoginId] = useState("");
  const [domain, setDomain] = useState("");
  const [pw, setPw] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingToken, setPendingToken] = useState("");
  const [pendingEmail, setPendingEmail] = useState("");
  const [code, setCode] = useState("");
  const [settings, setSettings] = useState<ServerSettings | null>(null);
  // ?expired=1 is set by the API layer when a 401 bounced the user here:
  // explain the kick instead of dropping them on a silent sign-in form.
  const [expired] = useState(
    () =>
      typeof window !== "undefined" &&
      new URLSearchParams(window.location.search).get("expired") === "1",
  );

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
      return;
    }
    me()
      .then((u) => {
        prefetchFolders(u.email);
        router.replace(mailboxTarget());
      })
      .catch(() => {
        // not authenticated: stay on the sign-in form
      })
      .finally(() => setLoading(false));
  }, [router]);

  useEffect(check, [check]);

  useEffect(() => {
    serverSettings()
      .then((s) => {
        setSettings(s);
        setDomain((d) => d || s.default_domain || s.domain || "");
      })
      .catch(() => setSettings(null));
  }, []);

  const domains = settings?.domains?.length ? settings.domains : [];
  const defaultDomain = settings?.default_domain || settings?.domain || "";

  // The sign-in box accepts just a username (single/multi domain): the
  // selected (or default) domain is appended here. A full address typed
  // directly always wins, matching how desktop clients log in.
  function resolveEmail(): string {
    const id = loginId.trim();
    if (!id) return "";
    if (id.includes("@")) return id.toLowerCase();
    return `${id}@${domain || defaultDomain}`.toLowerCase();
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const email = resolveEmail();
      if (!email) {
        setError(t("error"));
        setBusy(false);
        return;
      }
      const res = await login(email, pw);
      if (res.totp_required && res.pending_token) {
        setPendingToken(res.pending_token);
        setPendingEmail(email);
      } else {
        // Warm the folder-list cache while the router navigates, so the
        // sidebar's first paint already has the folders.
        prefetchFolders(email);
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
      prefetchFolders(pendingEmail);
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
    <div className="flex min-h-screen flex-col">
      {/* Top bar: enterprise logo + brand name, language switcher on the right */}
      <header className="border-b border-border/60">
        <div className="mx-auto flex w-full max-w-[1140px] flex-wrap items-center justify-between gap-3 px-5 py-4 md:px-10 min-[1440px]:max-w-[1320px]">
          <div className="flex items-center gap-3">
            {brand.logo_url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={brand.logo_url} alt={brandTitle} className="h-9 w-auto" />
            ) : (
              <span className="text-2xl font-extrabold tracking-tight">
                Mail
                <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
                  ez
                </span>
              </span>
            )}
            <div className="leading-tight">
              <p className="text-base font-bold">{brandTitle}</p>
              {brandSubtitle && <p className="text-xs text-muted-foreground">{brandSubtitle}</p>}
            </div>
          </div>
          <LocaleSwitcher />
        </div>
      </header>

      {/* Middle: left brand image + right sign-in card */}
      <main className="flex flex-1">
        <div className="mx-auto flex w-full max-w-[1140px] items-start justify-center gap-10 px-5 py-8 md:px-10 lg:py-12 min-[1440px]:max-w-[1320px]">
        {/* Left brand image, hidden on small screens */}
        <div className="relative hidden h-[540px] min-w-0 flex-1 overflow-hidden rounded-3xl text-white shadow-xl lg:block">
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
              <div className="absolute inset-0 bg-gradient-to-br from-[#2F8E6C] to-[#2E6E8E]" />
              <Mail className="pointer-events-none absolute -top-12 -right-12 size-72 text-white/10" />
              <div className="pointer-events-none absolute -bottom-24 -left-24 size-72 rounded-full bg-white/5" />
            </>
          )}
          <div className="absolute inset-0 flex flex-col justify-center space-y-8 p-8">
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
        </div>

        {/* Right sign-in form */}
        <div className="w-full max-w-sm shrink-0">
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
            </CardHeader>
            <CardContent>
          {expired && (
            <div
              role="status"
              className="mb-4 rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-300"
            >
              {t("sessionExpired")}
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
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? t("submitting") : t("verify")}
              </Button>
            </form>
          ) : (
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="login-id">{t("account")}</Label>
                <div className="flex items-center gap-1.5">
                  <Input
                    id="login-id"
                    type="text"
                    value={loginId}
                    onChange={(e) => setLoginId(e.target.value)}
                    placeholder={t("usernameHint")}
                    autoComplete="username"
                    className="min-w-0 flex-1"
                    required
                  />
                  {domains.length > 1 && (
                    <>
                      <span className="text-muted-foreground">@</span>
                      <select
                        aria-label={t("domainLabel")}
                        value={domain || defaultDomain}
                        onChange={(e) => setDomain(e.target.value)}
                        className="h-8 max-w-[180px] rounded-lg border border-input bg-background px-2 text-sm"
                      >
                        {domains.map((d) => (
                          <option key={d} value={d}>{d}</option>
                        ))}
                      </select>
                    </>
                  )}
                </div>
                {domains.length > 1 && (
                  <p className="text-xs text-muted-foreground">{t("domainHint")}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="pw">{t("password")}</Label>
                <Input id="pw" type="password" value={pw} onChange={(e) => setPw(e.target.value)} required />
              </div>
              <p className="text-right text-xs text-muted-foreground">{t("forgotPasswordHint")}</p>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? t("submitting") : t("submit")}
              </Button>
            </form>
          )}
            </CardContent>
          </Card>
          {/* Security notice, between the sign-in form and mail settings */}
          <div className="mt-4 rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-left text-xs leading-relaxed text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-300">
            <p>{t("noticeClassified", { product: brandTitle })}</p>
            <p>{t("noticeSecurity")}</p>
          </div>
          {settings && (
            <details open className="mt-4 rounded-lg border border-border p-3 text-xs text-muted-foreground">
              <summary className="cursor-pointer font-medium text-foreground">
                {t("mailSettings")}
              </summary>
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
      </main>

      {/* Bottom bar: centered copyright + contact */}
      <footer className="flex flex-col items-center gap-1 border-t border-border/60 px-5 py-4 text-center text-xs text-muted-foreground">
        <p>{brand.copyright?.trim() || `© ${new Date().getFullYear()} Mailez`}</p>
        {brand.contact?.trim() && <p>{brand.contact.trim()}</p>}
      </footer>
    </div>
  );
}
