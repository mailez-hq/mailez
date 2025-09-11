"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AppSidebar } from "@/components/app-sidebar";
import { me, type Me } from "@/lib/api";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    me()
      .then(setUser)
      .catch(() => router.replace("/"))
      .finally(() => setLoading(false));
  }, [router]);

  if (loading) {
    return <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">Loading...</div>;
  }

  if (!user) return null;

  return (
    // Fixed viewport height, not min-height: the main column scrolls
    // INTERNALLY (overflow-auto needs a bounded parent) so the sidebar —
    // with the locale switcher and user card at its bottom — stays fully
    // visible no matter how long the page content is.
    <div className="flex h-screen overflow-hidden">
      <AppSidebar me={user} />
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  );
}
