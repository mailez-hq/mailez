"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { dashboardTarget, me } from "@/lib/api";
import { Logo } from "@/components/logo";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { LoginForm } from "@/components/login-form";
import { useBrand } from "@/lib/use-brand";

// Home doubles as the SSO entry: an existing session cookie (e.g. from the
// webmail app on another port) is detected via /sso/me and redirected to the
// dashboard, so users don't see a login page after already signing in.
export default function Home() {
  const t = useTranslations("login");
  const router = useRouter();
  const [checking, setChecking] = useState(true);
  // White-label: branded deployments show the org name (and logo when
  // configured) instead of the built-in Mailez wordmark.
  const brand = useBrand();
  // ?expired=1 is set by the API layer when a 401 bounced the user here:
  // explain the kick instead of dropping them on a silent sign-in form.
  const [expired] = useState(
    () =>
      typeof window !== "undefined" &&
      new URLSearchParams(window.location.search).get("expired") === "1",
  );

  useEffect(() => {
    me()
      .then(() => router.replace(dashboardTarget()))
      .catch(() => {})
      .finally(() => setChecking(false));
  }, [router]);

  if (checking) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">…</p>
      </div>
    );
  }

  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center bg-gradient-to-b from-background to-muted/50 p-4">
      <div className="absolute bottom-4 left-4">
        <LocaleSwitcher />
      </div>
      <div className="mb-6 flex items-center gap-3">
        {brand.logo_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={brand.logo_url}
            alt={brand.title}
            className="size-11 shrink-0 rounded-lg object-contain"
          />
        ) : (
          <Logo className="size-11" />
        )}
        <div>
          <h1 className="text-2xl font-extrabold tracking-tight text-foreground">
            {brand.title || "Mailez"}{" "}
            <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
              Admin
            </span>
          </h1>
        </div>
      </div>
      <LoginForm />
      {expired && (
        <div
          role="status"
          className="mt-4 w-full max-w-sm rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm text-amber-800"
        >
          {t("sessionExpired")}
        </div>
      )}
    </div>
  );
}
