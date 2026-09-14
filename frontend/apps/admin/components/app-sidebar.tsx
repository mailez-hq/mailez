"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  AtSign,
  BarChart3,
  HeartPulse,
  LayoutDashboard,
  Download,
  EyeOff,
  ExternalLink,
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
  UsersRound,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { Logo } from "@/components/logo";
import { useBrand } from "@/lib/use-brand";
import { HAS_OPTIONAL_MODULES, logout } from "@/lib/api";

// The webmail console is the deployment root (the gateway serves it at "/"
// and this console under "/admin"); local dev can override with
// NEXT_PUBLIC_WEBMAIL_URL (e.g. http://localhost:3001).
const WEBMAIL_URL = process.env.NEXT_PUBLIC_WEBMAIL_URL || "/";
import { cn } from "@/lib/utils";
import type { Me } from "@/lib/api";

type NavItem = {
  href: string;
  key: string;
  roles: string[];
  icon: typeof LayoutDashboard;
};

// Navigation is grouped by workflow so related sections stay together:
// monitoring first (what an admin opens daily), then the organization
// hierarchy, mail-flow services, compliance, and finally system-level
// settings at the bottom.
const navGroups: { key: string; items: NavItem[] }[] = [
  {
    key: "groupOverview",
    items: [
      { href: "/overview", key: "overview", roles: ["admin", "manager", "user"], icon: LayoutDashboard },
      { href: "/health", key: "health", roles: ["admin"], icon: HeartPulse },
      { href: "/reports", key: "reports", roles: ["admin"], icon: BarChart3 },
    ],
  },
  {
    key: "groupOrg",
    items: [
      { href: "/domains", key: "domains", roles: ["admin"], icon: Globe },
      { href: "/users", key: "users", roles: ["admin", "manager"], icon: Users },
      { href: "/groups", key: "groups", roles: ["admin", "manager"], icon: UsersRound },
      { href: "/aliases", key: "aliases", roles: ["admin", "manager"], icon: AtSign },
    ],
  },
  {
    key: "groupMail",
    items: [
      { href: "/relays", key: "relays", roles: ["admin"], icon: Server },
      { href: "/fetches", key: "fetches", roles: ["admin"], icon: Download },
      { href: "/tokens", key: "tokens", roles: ["admin"], icon: KeyRound },
      { href: "/anon-aliases", key: "anonAliases", roles: ["admin", "manager", "user"], icon: EyeOff },
    ],
  },
  {
    key: "groupCompliance",
    items: [
      // Approvers (regular users listed on hold rules) also need the center.
      { href: "/dlp", key: "dlp", roles: ["admin", "user"], icon: ShieldAlert },
      { href: "/archive", key: "archive", roles: ["admin"], icon: Archive },
      { href: "/audit", key: "audit", roles: ["admin"], icon: ScrollText },
    ],
  },
  {
    key: "groupSystem",
    items: [
      { href: "/announcement", key: "announcement", roles: ["admin"], icon: Megaphone },
      { href: "/config", key: "config", roles: ["admin"], icon: Settings },
    ],
  },
];

// Paid-edition-only sections must not surface in the community build (their
// CE routes render "not enabled" placeholders). Page-level placeholders still
// catch direct deep links; the module filter only hides the navigation.
const EE_NAV_HREFS: ReadonlySet<string> = new Set(["/announcement", "/archive", "/dlp", "/reports"]);

function roleLabel(me: Me) {
  if (me.global_admin) return "admin";
  if (me.manager) return "manager";
  return "user";
}

export function AppSidebar({ me }: { me: Me }) {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const router = useRouter();
  // White-label: branded deployments show the org name (and logo when
  // configured) instead of the built-in Mailez wordmark.
  const brand = useBrand();
  const role = me.global_admin ? "admin" : me.manager ? "manager" : "user";
  const visible = (item: NavItem) =>
    (HAS_OPTIONAL_MODULES || !EE_NAV_HREFS.has(item.href)) && item.roles.includes(role);
  const groups = navGroups
    .map((g) => ({ ...g, items: g.items.filter(visible) }))
    .filter((g) => g.items.length > 0);
  const firstHref = groups[0]?.items[0]?.href || "/overview";

  async function onLogout() {
    await logout();
    router.push("/");
    router.refresh();
  }

  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div className="flex items-center px-4 py-4">
        <Link
          href={firstHref}
          className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80"
        >
          {brand.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={brand.logo_url}
              alt={brand.title}
              className="size-9 shrink-0 rounded-lg object-contain"
            />
          ) : (
            <Logo className="size-9" />
          )}
          {brand.title || "Mailez"}{" "}
          {/* Theme primary (teal) rather than the old blue gradient, so the
              lockup follows light/dark with the rest of the console. */}
          <span className="text-primary">Admin</span>
        </Link>
      </div>

      <nav className="flex flex-1 flex-col overflow-y-auto px-2 pb-2">
        {groups.map((group, gi) => (
          <div key={group.key} className={gi === 0 ? "" : "mt-3"}>
            <p className="px-2.5 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground/80">
              {t(group.key)}
            </p>
            <div className="flex flex-col gap-0.5">
              {group.items.map((item) => {
                const active = pathname.startsWith(item.href);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn(
                      "relative flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                      active
                        ? // Same active language as the webmail sidebar: --sidebar-accent
                          // barely contrasts against --sidebar, so tint with
                          // --primary (which the data-accent families override)
                          // and anchor with an inset bar.
                          "bg-primary/20 font-medium text-primary shadow-[inset_2px_0_0_0_var(--primary)] hover:bg-primary/25"
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
            </div>
          </div>
        ))}
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
          onClick={() => window.open(WEBMAIL_URL, "_blank", "noopener,noreferrer")}
          title={t("webmail")}
          className="text-muted-foreground hover:text-foreground"
        >
          <ExternalLink className="size-4" />
        </Button>
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
