"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { MailView } from "@/components/mailbox/mail-view";
import { MailStoreProvider } from "@/components/mailbox/mail-store";
import { me, type Me } from "@/lib/api";
import { readLastFolder } from "@/lib/preferences";

// MailLayout guards the /mail area and keeps MailView mounted across folder /
// message transitions so the message list never rebuilds or flashes. The URL
// is the single source for folder & opened uid — MailView reads usePathname.
export default function MailLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    me()
      .then((u) => {
        setUser(u);
        // A bare /mail visit restores the last-visited folder (or the inbox);
        // explicit /mail/[folder] deep links are left untouched.
        const segs = window.location.pathname.split("/").filter(Boolean);
        if (segs.length <= 1) {
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

  // children hold the leaf route (folder / message) pages, which are kept as
  // thin stubs; MailView renders the full three-pane UI driven by the URL and
  // stays mounted so the message list never rebuilds. MailStoreProvider owns
  // all mailbox state while MailView stays a thin presentational shell.
  return (
    <MailStoreProvider me={user}>
      <MailView />
    </MailStoreProvider>
  );
}
