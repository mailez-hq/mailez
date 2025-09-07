"use client";

// AppHeader is the full-width top bar of the signed-in app (modern-style
// chrome). Left: hamburger (mobile drawer toggle) + the brand, which links
// back to the workspace. Right: settings gear and the account avatar whose
// menu carries the sign-out action — the single canonical exit point, so the
// sidebar footer no longer needs one. Rendered by MailShell above the sidebar
// and the /home and /mail content alike.

import Link from "next/link";
import { useTranslations } from "next-intl";
import { FolderSearch, LogOut, Menu, Search, Settings, SlidersHorizontal, Sparkles, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Logo } from "@/components/logo";
import { useBrand } from "@/lib/use-brand";
import { cn } from "@/lib/utils";

export function AppHeader({
  email,
  displayName,
  query,
  onQueryChange,
  onSearchSubmit,
  onClearSearch,
  searchInputRef,
  searchAll,
  onToggleSearchAll,
  aiSearchEnabled,
  aiSearching,
  onAiSearch,
  onOpenAdvanced,
  onMenu,
  onSettings,
  onLogout,
}: {
  email: string;
  displayName?: string;
  query: string;
  onQueryChange: (q: string) => void;
  onSearchSubmit: () => void;
  onClearSearch: () => void;
  searchInputRef: React.RefObject<HTMLInputElement | null>;
  searchAll: boolean;
  onToggleSearchAll: () => void;
  aiSearchEnabled: boolean;
  aiSearching: boolean;
  onAiSearch: () => void;
  onOpenAdvanced: () => void;
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

      {/* Search pill: the modern-style centerpiece of the header. Enter runs
          the search, Escape clears and blurs, "/" (handled by the global
          hotkeys) focuses this input via searchInputRef. Capped at max-w-2xl
          and centered between the brand and the account controls. */}
      <div className="mx-2 flex min-w-0 flex-1 justify-center">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onSearchSubmit();
          }}
          className="flex h-9 w-full max-w-2xl items-center gap-2 rounded-full bg-muted/60 px-4 transition-colors focus-within:bg-background focus-within:ring-1 focus-within:ring-ring"
        >
        <Search className="pointer-events-none size-4 shrink-0 text-muted-foreground" />
        <input
          ref={searchInputRef}
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              onClearSearch();
              e.currentTarget.blur();
            }
          }}
          placeholder={t("searchMail")}
          className="h-full w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          aria-label={t("searchMail")}
        />
        {/* Search-scope and modes, lifted from the list toolbar (mainstream webmail keeps
            these attached to the search field): all-folders toggle, AI mode,
            advanced builder. */}
        <button
          type="button"
          onClick={onToggleSearchAll}
          title={t("searchAll")}
          aria-pressed={searchAll}
          className={cn(
            "rounded p-1 transition-colors",
            searchAll
              ? "bg-primary/10 text-primary"
              : "text-muted-foreground hover:bg-muted hover:text-foreground",
          )}
        >
          <FolderSearch className="size-4" />
        </button>
        {aiSearchEnabled && (
          <button
            type="button"
            onClick={onAiSearch}
            disabled={aiSearching}
            title={t("aiSearch")}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <Sparkles className={cn("size-4", aiSearching && "animate-pulse")} />
          </button>
        )}
        <button
          type="button"
          onClick={onOpenAdvanced}
          title={t("advancedSearch")}
          className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <SlidersHorizontal className="size-4" />
        </button>
        {query.trim() !== "" ? (
          <button
            type="button"
            onClick={onClearSearch}
            title={t("clearSearch")}
            className="rounded p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        ) : (
          <kbd className="pointer-events-none rounded border border-border px-1.5 py-0.5 font-sans text-[10px] text-muted-foreground">
            /
          </kbd>
        )}
        </form>
      </div>

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
            {/* GroupLabel requires a Group parent (Base UI throws without
                one in production builds), so label + actions live in a
                group. */}
            <DropdownMenuGroup>
              <DropdownMenuLabel className="max-w-52 truncate">{label}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onClick={onLogout}>
                <LogOut className="size-4" />
                {t("logout")}
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
