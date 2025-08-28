"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { MailShell } from "@/components/mailbox/mail-shell";
import { MailStoreProvider } from "@/components/mailbox/mail-store";
import { me, type Me } from "@/lib/api";
import { readLastFolder } from "@/lib/preferences";

// AppLayout guards the signed-in area — the /home workspace and the /mail
// segment alike — and keeps the MailStoreProvider + MailShell chrome mounted
// across transitions, so the sidebar, drawers and compose state survive
// navigation between home and mail.
export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    me()
      .then((u) => {
        setUser(u);
        // A bare /mail visit restores the last-visited folder (or the inbox);
        // explicit /mail/[folder] deep links and /home are left untouched.
        const segs = window.location.pathname.split("/").filter(Boolean);
        if (segs.length === 1 && segs[0] === "mail") {
          const last = readLastFolder();
          router.replace(last ? `/mail/${encodeURIComponent(last)}` : "/mail/Inbox");
        }
      })
      .catch(() => {
        // Keep the deep link so a signed-out visitor returns to the folder or
        // message they asked for after signing in.
        router.replace(`/?next=${encodeURIComponent(window.location.pathname + window.location.search)}`);
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

  // children hold the segment content: the workspace dashboard on /home and
  // MailView (via the /mail layout) on /mail. MailStoreProvider owns all
  // mailbox state while the segments stay thin presentational layers.
  return (
    <MailStoreProvider me={user}>
      <MailShell>{children}</MailShell>
    </MailStoreProvider>
  );
}
