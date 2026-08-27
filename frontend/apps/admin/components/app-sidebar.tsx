"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  AtSign,
  LayoutDashboard,
  Download,
  EyeOff,
  Globe,
  KeyRound,
  LogOut,
  Megaphone,
  Archive,
  ShieldAlert,
  ScrollText,
  Server,
  Settings,
  ShieldCheck,
  Users,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { Logo } from "@/components/logo";
import { logout } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { Me } from "@/lib/api";

const navItems = [
  { href: "/overview", key: "overview", roles: ["admin", "manager", "user"], icon: LayoutDashboard },
  { href: "/domains", key: "domains", roles: ["admin"], icon: Globe },
  { href: "/users", key: "users", roles: ["admin", "manager"], icon: Users },
  { href: "/aliases", key: "aliases", roles: ["admin", "manager"], icon: AtSign },
  { href: "/relays", key: "relays", roles: ["admin"], icon: Server },
  { href: "/fetches", key: "fetches", roles: ["admin"], icon: Download },
  { href: "/tokens", key: "tokens", roles: ["admin"], icon: KeyRound },
  { href: "/announcement", key: "announcement", roles: ["admin"], icon: Megaphone },
  { href: "/anon-aliases", key: "anonAliases", roles: ["admin", "manager", "user"], icon: EyeOff },
  { href: "/archive", key: "archive", roles: ["admin"], icon: Archive },
  // Approvers (regular users listed on hold rules) also need the center.
  { href: "/dlp", key: "dlp", roles: ["admin", "user"], icon: ShieldAlert },
  { href: "/audit", key: "audit", roles: ["admin"], icon: ScrollText },
  { href: "/config", key: "config", roles: ["admin"], icon: Settings },
];

function roleLabel(me: Me) {
  if (me.global_admin) return "admin";
  if (me.manager) return "manager";
  return "user";
}

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
    <aside className="flex w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div className="flex items-center px-4 py-4">
        <Link
          href={nav[0]?.href || "/"}
          className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80"
        >
          <Logo className="size-9" />
          Mailez{" "}
          <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
            Admin
          </span>
        </Link>
      </div>

      <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-2">
        {nav.map((item) => {
          const active = pathname.startsWith(item.href);
          const Icon = item.icon;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "relative flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                active
                  ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                  : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
              )}
            >
              {active && (
                <span className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-sidebar-primary" />
              )}
              <Icon className={cn("size-4 shrink-0", active ? "text-sidebar-primary" : "opacity-70")} />
              <span className="truncate">{t(item.key)}</span>
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-sidebar-border p-2">
        <div className="flex items-center justify-center">
          <LocaleSwitcher />
        </div>
      </div>

      <div className="flex items-center gap-2 border-t border-sidebar-border px-3 py-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-sidebar-primary text-sm font-bold text-sidebar-primary-foreground">
          {(me.displayed_name || me.email).charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-1 truncate text-sm font-medium text-foreground">
            <span className="truncate">{me.displayed_name || me.email}</span>
            {me.global_admin && <ShieldCheck className="size-3.5 shrink-0 text-sidebar-primary" />}
          </p>
          <p className="truncate text-xs text-muted-foreground">{roleLabel(me)}</p>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onLogout}
          title={t("logout")}
          className="text-muted-foreground hover:text-destructive"
        >
          <LogOut className="size-4" />
        </Button>
      </div>
    </aside>
  );
}
