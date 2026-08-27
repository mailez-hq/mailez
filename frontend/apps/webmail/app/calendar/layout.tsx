"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { me, type Me } from "@/lib/api";
import { AppSidebar } from "@/components/app-sidebar";

// CalendarLayout guards /calendar and renders it inside the shared
// application frame (AppSidebar) so the calendar is one click away from the
// mailbox instead of a bare full-screen page.
export default function CalendarLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    me()
      .then(setUser)
      .catch(() => {
        router.replace(`/?next=${encodeURIComponent("/calendar")}`);
      })
      .finally(() => setLoading(false));
  }, [router]);

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-primary/20 border-t-primary" />
        <span className="sr-only">Loading</span>
      </div>
    );
  }
  if (!user) return null;
  const quotaPercent =
    user.quota_bytes && user.quota_bytes > 0 && user.quota_bytes_used != null
      ? Math.round((user.quota_bytes_used / user.quota_bytes) * 100)
      : null;
  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <AppSidebar email={user.email} quotaPercent={quotaPercent} />
      <div className="min-w-0 min-h-0 flex-1">{children}</div>
    </div>
  );
}
