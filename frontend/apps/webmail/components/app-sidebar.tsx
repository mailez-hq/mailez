"use client";

import { CalendarDays, ExternalLink, Filter, Inbox, LogOut, Settings, Users } from "lucide-react";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Button, buttonVariants } from "@/components/ui/button";
import { Logo } from "@/components/logo";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { cn } from "@/lib/utils";

const ADMIN_URL = process.env.NEXT_PUBLIC_ADMIN_URL || "/admin";

// AppSidebar is the compact application frame shared by routes outside the
// three-pane mailbox (calendar today): brand, primary navigation, and the
// same bottom user cluster as the mailbox sidebar so every surface feels like
// one product.
export function AppSidebar({ email, quotaPercent }: { email: string; quotaPercent?: number | null }) {
  const t = useTranslations("mail");
  const pathname = usePathname();
  const router = useRouter();

  const nav = [
    { href: "/mail/Inbox", icon: Inbox, label: t("mailbox"), active: pathname.startsWith("/mail") },
    { href: "/calendar", icon: CalendarDays, label: t("calendar"), active: pathname.startsWith("/calendar") },
  ];

  const logout = async () => {
    await fetch("/api/v1/sso/logout", { method: "POST" }).catch(() => {});
    router.replace("/");
  };

  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div className="flex h-12 items-center px-3">
        <Link href="/mail/Inbox" className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80">
          <Logo />
          Mailez Webmail
        </Link>
      </div>

      <nav className="mail-scroll flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
        {nav.map((n) => (
          <Link
            key={n.href}
            href={n.href}
            className={cn(
              "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
              n.active
                ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
            )}
          >
            <n.icon className="size-4 shrink-0 opacity-70" />
            <span className="truncate">{n.label}</span>
          </Link>
        ))}
      </nav>

      <div className="border-t border-sidebar-border p-2">
        <div className="mb-1 flex items-center justify-center">
          <LocaleSwitcher />
        </div>
        <div className="mx-2 mb-1.5 border-t border-sidebar-border" />
        <div className="mb-1 flex items-center justify-center gap-0.5">
          <Link href="/mail/Inbox?open=contacts" className={cn(buttonVariants({ variant: "ghost", size: "sm" }))} title={t("contacts")}>
            <Users className="size-4" />
          </Link>
          <Link href="/mail/Inbox?open=settings" className={cn(buttonVariants({ variant: "ghost", size: "sm" }))} title={t("settings")}>
            <Settings className="size-4" />
          </Link>
          <a href={ADMIN_URL} target="_blank" rel="noreferrer" title={t("adminConsole")} className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}>
            <ExternalLink className="size-4" />
          </a>
          <Button variant="ghost" size="sm" onClick={logout} title={t("logout")}>
            <LogOut className="size-4" />
          </Button>
        </div>
        <p className="truncate px-2 text-center text-xs text-muted-foreground">{email}</p>
        {typeof quotaPercent === "number" && quotaPercent > 0 && (
          <div className="mt-1.5 px-3">
            <div className="h-1 w-full overflow-hidden rounded-full bg-sidebar-accent">
              <div
                className={cn(
                  "h-full rounded-full",
                  quotaPercent >= 90 ? "bg-destructive" : quotaPercent >= 70 ? "bg-[#C9A227]" : "bg-primary",
                )}
                style={{ width: `${Math.min(100, quotaPercent)}%` }}
              />
            </div>
            <p className="mt-1 text-center text-[10px] text-muted-foreground">
              {t("quotaUsed", { percent: quotaPercent })}
            </p>
          </div>
        )}
      </div>
    </aside>
  );
}

