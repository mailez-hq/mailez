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
    return <div className="flex min-h-screen items-center justify-center text-sm text-zinc-500">Loading...</div>;
  }

  if (!user) return null;

  return (
    <div className="flex min-h-screen">
      <AppSidebar me={user} />
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  );
}
