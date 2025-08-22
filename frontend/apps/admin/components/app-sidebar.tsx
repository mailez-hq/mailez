"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { logout } from "@/lib/api";
import type { Me } from "@/lib/api";

const nav = [
  { href: "/domains", label: "Domains" },
  { href: "/users", label: "Users" },
  { href: "/aliases", label: "Aliases" },
];

export function AppSidebar({ me }: { me: Me }) {
  const pathname = usePathname();
  const router = useRouter();

  async function onLogout() {
    await logout();
    router.push("/");
    router.refresh();
  }

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-white dark:bg-zinc-900">
      <Link href="/domains" className="flex items-center gap-2 px-4 py-4">
        <span className="text-lg font-semibold">mailess</span>
      </Link>
      <nav className="flex flex-1 flex-col gap-1 px-2 py-2">
        {nav.map((item) => {
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                active
                  ? "bg-zinc-100 text-zinc-900 dark:bg-zinc-800 dark:text-zinc-50"
                  : "text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800"
              }`}
            >
              {item.label}
            </Link>
          );
        })}
      </nav>
      <div className="flex items-center justify-between border-t px-4 py-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">
            {me.displayed_name || me.email}
          </p>
          <p className="truncate text-xs text-zinc-500">{me.email}</p>
        </div>
        <Button variant="ghost" size="sm" onClick={onLogout}>
          Logout
        </Button>
      </div>
    </aside>
  );
}
