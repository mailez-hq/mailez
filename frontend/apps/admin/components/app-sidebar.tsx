"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { Logo } from "@/components/logo";
import { logout } from "@/lib/api";
import type { Me } from "@/lib/api";

const navItems = [
  { href: "/domains", key: "domains", roles: ["admin"] },
  { href: "/users", key: "users", roles: ["admin", "manager"] },
  { href: "/aliases", key: "aliases", roles: ["admin", "manager"] },
  { href: "/relays", key: "relays", roles: ["admin"] },
  { href: "/fetches", key: "fetches", roles: ["admin"] },
  { href: "/tokens", key: "tokens", roles: ["admin"] },
  { href: "/anon-aliases", key: "anonAliases", roles: ["admin", "manager", "user"] },
  { href: "/audit", key: "audit", roles: ["admin"] },
  { href: "/config", key: "config", roles: ["admin"] },
];

export function AppSidebar({ me }: { me: Me }) {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const router = useRouter();
  const role = me.global_admin ? "admin" : me.manager ? "manager" : "user";
  const nav = navItems.filter((item) => item.roles.includes(role));

  async function onLogout() {
    await logout();
    router.push("/");
    router.refresh();
  }

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-white dark:bg-zinc-900">
      <div className="flex items-center justify-between px-4 py-4">
        <Link href={nav[0]?.href || "/"} className="flex items-center gap-2 text-lg font-semibold">
          <Logo className="size-9" />
          mailez
        </Link>
        <LocaleSwitcher />
      </div>
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
              {t(item.key)}
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
          {t("logout")}
        </Button>
      </div>
    </aside>
  );
}
