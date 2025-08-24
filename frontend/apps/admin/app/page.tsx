"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { me } from "@/lib/api";
import { Logo } from "@/components/logo";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { LoginForm } from "@/components/login-form";

// Home doubles as the SSO entry: an existing session cookie (e.g. from the
// webmail app on another port) is detected via /sso/me and redirected to the
// dashboard, so users don't see a login page after already signing in.
export default function Home() {
  const router = useRouter();
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    me()
      .then(() => router.replace("/domains"))
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
      <div className="absolute top-4 right-4">
        <LocaleSwitcher />
      </div>
      <div className="mb-6 flex items-center gap-3">
        <Logo className="size-11" />
        <div>
          <h1 className="text-2xl font-extrabold tracking-tight text-foreground">
            mailez
            <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
              admin
            </span>
          </h1>
        </div>
      </div>
      <LoginForm />
    </div>
  );
}
