"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { me, type Me } from "@/lib/api";

// CalendarLayout guards /calendar: the built-in calendar shares the webmail
// session but lives outside the three-pane /mail shell.
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
  return <div className="h-screen">{children}</div>;
}

