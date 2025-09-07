"use client";

// AppHeader is the full-width top bar of the signed-in app (modern-style
// chrome). Left: hamburger (mobile drawer toggle) + the brand, which links
// back to the workspace. Right: settings gear and the account avatar whose
// menu carries the sign-out action — the single canonical exit point, so the
// sidebar footer no longer needs one. Rendered by MailShell above the sidebar
// and the /home and /mail content alike.

import Link from "next/link";
import { useTranslations } from "next-intl";
import { LogOut, Menu, Settings } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Logo } from "@/components/logo";
import { useBrand } from "@/lib/use-brand";

export function AppHeader({
  email,
  displayName,
  onMenu,
  onSettings,
  onLogout,
}: {
  email: string;
  displayName?: string;
  onMenu: () => void;
  onSettings: () => void;
  onLogout: () => void;
}) {
  const t = useTranslations("mail");
  // White-label: branded deployments show the org name (and logo when
  // configured) instead of the built-in Mailez wordmark.
  const brand = useBrand();
  const label = displayName?.trim() || email;
  const initial = (label || "?").charAt(0).toUpperCase();

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background px-3">
      {/* Mobile drawer toggle; on lg+ the sidebar is always visible. */}
      <Button
        variant="ghost"
        size="icon-sm"
        className="shrink-0 lg:hidden"
        onClick={onMenu}
        title={t("menu")}
      >
        <Menu className="size-4" />
        <span className="sr-only">{t("menu")}</span>
      </Button>

      {/* Brand navigates straight into the workspace instead of "/" (the
          sign-in route), so clicking it never flashes the login page. */}
      <Link
        href="/home"
        className="flex min-w-0 items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80"
      >
        {brand.logo_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={brand.logo_url}
            alt={brand.title}
            className="size-8 shrink-0 rounded-lg object-contain"
          />
        ) : (
          <Logo />
        )}
        <span className="truncate">{brand.title || "Mailez Webmail"}</span>
      </Link>

      <div className="ml-auto flex shrink-0 items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onSettings}
          title={t("settings")}
          aria-label={t("settings")}
        >
          <Settings className="size-4" />
        </Button>

        {/* Account menu: the avatar opens identity options with sign-out at
            the bottom, mirroring mainstream webmail clients. */}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                className="rounded-full"
                title={label}
                aria-label={t("myAccount")}
              >
                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground">
                  {initial}
                </span>
              </Button>
            }
          />
          <DropdownMenuContent align="end">
            <DropdownMenuLabel className="max-w-52 truncate">{label}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={onLogout}>
              <LogOut className="size-4" />
              {t("logout")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
