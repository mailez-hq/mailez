"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { me } from "@/lib/api";
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
    <div className="flex min-h-screen items-center justify-center p-4">
      <LoginForm />
    </div>
  );
}
