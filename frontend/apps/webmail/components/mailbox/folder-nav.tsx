"use client";

import {
  Archive,
  ExternalLink,
  FileText,
  Inbox,
  Menu,
  Search,
  Send,
  Star,
  Trash2,
  Filter,
  Folder as FolderIcon,
  PenLine,
  Settings,
  Settings2,
  Users,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { Button, buttonVariants } from "@/components/ui/button";
import { Logo } from "@/components/logo";
import { labelColor } from "@/components/mailbox/mail-utils";
import { cn } from "@/lib/utils";

// The admin console lives behind the same nginx front in deployments (/admin);
// local dev can override with NEXT_PUBLIC_ADMIN_URL (e.g. http://localhost:3000).
const ADMIN_URL = process.env.NEXT_PUBLIC_ADMIN_URL || "/admin";

const FOLDER_ICON: Record<string, React.ReactNode> = {
  INBOX: <Inbox className="size-4" />,
  SENT: <Send className="size-4" />,
  DRAFTS: <FileText className="size-4" />,
  TRASH: <Trash2 className="size-4" />,
  ARCHIVE: <Archive className="size-4" />,
  JUNK: <Star className="size-4" />,
  SPAM: <Star className="size-4" />,
};

// Common folders always sort before custom ones; INBOX is pinned first.
const FOLDER_ORDER = ["INBOX", "Sent", "Drafts", "Trash", "Archive", "Junk", "Spam"];

function sortFolders(folders: string[]): string[] {
  return [...folders].sort((a, b) => {
    const ia = FOLDER_ORDER.indexOf(a.toUpperCase());
    const ib = FOLDER_ORDER.indexOf(b.toUpperCase());
    const ra = ia === -1 ? Number.MAX_SAFE_INTEGER : ia;
    const rb = ib === -1 ? Number.MAX_SAFE_INTEGER : ib;
    if (ra !== rb) return ra - rb;
    return a.localeCompare(b);
  });
}

function folderLabel(t: ReturnType<typeof useTranslations<"mail">>, name: string) {
  const key = `folder${name.charAt(0).toUpperCase()}${name.slice(1).toLowerCase()}` as const;
  // fall back to the raw IMAP name for custom folders
  return t.has(key) ? t(key) : name;
}

export function FolderNav({
  folders,
  unseen,
  labels,
  labelColors,
  activeLabel,
  savedSearches,
  current,
  email,
  quotaBytes,
  quotaUsed,
  open,
  onSelect,
  onSelectLabel,
  onManageLabels,
  onSelectSavedSearch,
  onRemoveSavedSearch,
  onMoveToFolder,
  onCompose,
  onSettings,
  onContacts,
  onSieve,
  onClose,
}: {
  folders: string[];
  unseen?: Record<string, number>;
  labels?: string[];
  labelColors?: Record<string, string>;
  activeLabel?: string;
  savedSearches?: string[];
  current: string;
  email: string;
  quotaBytes?: number;
  quotaUsed?: number;
  open: boolean;
  onSelect: (folder: string) => void;
  onSelectLabel: (label: string) => void;
  onManageLabels?: () => void;
  onSelectSavedSearch: (query: string) => void;
  onRemoveSavedSearch: (query: string) => void;
  onMoveToFolder: (folder: string, uid: number) => void;
  onCompose: () => void;
  onSettings: () => void;
  onContacts: () => void;
  onSieve: () => void;
  onClose: () => void;
}) {
  const t = useTranslations("mail");
  const orderedFolders = sortFolders(folders);
  const quotaPercent =
    typeof quotaBytes === "number" &&
    typeof quotaUsed === "number" &&
    quotaBytes > 0
      ? Math.min(100, Math.round((quotaUsed / quotaBytes) * 100))
      : null;
  const quotaBarColor =
    quotaPercent === null
      ? "bg-primary"
      : quotaPercent >= 90
        ? "bg-destructive"
        : quotaPercent >= 70
          ? "bg-[#C9A227]"
          : "bg-primary";

  return (
    <>
      {/* mobile overlay */}
      <div
        className={cn(
          "fixed inset-0 z-40 bg-black/20 transition-opacity lg:hidden",
          open ? "opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={onClose}
      />
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 flex w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-transform duration-200 lg:static lg:z-auto lg:translate-x-0",
          open ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="flex h-12 items-center justify-between px-3">
          <Link href="/" className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80">
            <Logo />
            mail
            <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
              ez
            </span>
          </Link>
          <Button variant="ghost" size="sm" onClick={onClose} className="lg:hidden">
            <Menu className="size-4" />
            <span className="sr-only">{t("menu")}</span>
          </Button>
        </div>

        <div className="px-3 pb-2">
          <Button className="w-full" onClick={onCompose}>
            <PenLine className="size-4" />
            {t("write")}
          </Button>
        </div>

        <nav className="mail-scroll flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
          {orderedFolders.map((f) => {
            const active = f === current;
            const icon =
              FOLDER_ICON[f.toUpperCase()] || <FolderIcon className="size-4" />;
            return (
              <button
                key={f}
                onClick={() => {
                  onSelect(f);
                  onClose();
                }}
                onDragOver={(e) => {
                  e.preventDefault();
                  e.dataTransfer.dropEffect = "move";
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  const uid = Number(e.dataTransfer.getData("text/plain"));
                  if (uid) onMoveToFolder(f, uid);
                }}
                className={cn(
                  "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                  active
                    ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                    : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
                )}
              >
                <span className="shrink-0 opacity-70">{icon}</span>
                <span className="truncate">{folderLabel(t, f)}</span>
                {unseen?.[f] != null && unseen[f] > 0 && (
                  <span className="ml-auto shrink-0 rounded-full bg-primary/15 px-1.5 py-px text-[10px] font-medium text-primary">
                    {unseen[f]}
                  </span>
                )}
              </button>
            );
          })}
          {labels && (
            <div className="mt-3 border-t border-sidebar-border pt-2">
              <div className="flex items-center justify-between pr-1">
                <p className="px-2.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
                  {t("labels")}
                </p>
                {onManageLabels && (
                  <button
                    onClick={onManageLabels}
                    title={t("manageLabels")}
                    className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
                  >
                    <Settings2 className="size-3.5" />
                  </button>
                )}
              </div>
              {labels.map((label) => (
                <button
                  key={label}
                  onClick={() => {
                    onSelectLabel(label);
                    onClose();
                  }}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                    activeLabel === label
                      ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                      : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
                  )}
                >
                  <span
                    className="size-2 shrink-0 rounded-full"
                    style={{ backgroundColor: labelColor(label, labelColors?.[label]) }}
                  />
                  <span className="truncate">{label}</span>
                </button>
              ))}
              {labels.length === 0 && onManageLabels && (
                <button
                  onClick={onManageLabels}
                  className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground"
                >
                  <span className="size-2 shrink-0 rounded-full border border-border" />
                  {t("newLabel")}
                </button>
              )}
            </div>
          )}
          {savedSearches && savedSearches.length > 0 && (
            <div className="mt-3 border-t border-sidebar-border pt-2">
              <p className="px-2.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
                {t("savedSearches")}
              </p>
              {savedSearches.map((q) => (
                <div
                  key={q}
                  className="group flex w-full items-center gap-1 rounded-lg px-1 transition-colors hover:bg-sidebar-accent/60"
                >
                  <button
                    onClick={() => {
                      onSelectSavedSearch(q);
                      onClose();
                    }}
                    className="min-w-0 flex-1 rounded-lg px-1.5 py-1.5 text-left text-sm text-sidebar-foreground transition-colors hover:text-foreground"
                  >
                    <span className="flex items-center gap-2 truncate">
                      <Search className="size-3.5 shrink-0 opacity-60" />
                      <span className="truncate">{q}</span>
                    </span>
                  </button>
                  <button
                    onClick={() => onRemoveSavedSearch(q)}
                    title={t("delete")}
                    className="rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
                  >
                    <X className="size-3.5" />
                  </button>
                </div>
              ))}
            </div>
          )}
        </nav>

        <div className="border-t border-sidebar-border p-2">
          <div className="mb-1 flex items-center justify-center gap-0.5">
            <Button variant="ghost" size="sm" onClick={onContacts} title={t("contacts")}>
              <Users className="size-4" />
            </Button>
            <Button variant="ghost" size="sm" onClick={onSieve} title={t("filterRules")}>
              <Filter className="size-4" />
            </Button>
            <Button variant="ghost" size="sm" onClick={onSettings} title={t("settings")}>
              <Settings className="size-4" />
            </Button>
            <a
              href={ADMIN_URL}
              target="_blank"
              rel="noreferrer"
              title={t("adminConsole")}
              className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
            >
              <ExternalLink className="size-4" />
            </a>
          </div>
          <p className="truncate px-2 text-center text-xs text-muted-foreground">{email}</p>
          {quotaPercent !== null && (
            <div className="mt-1.5 px-3">
              <div className="h-1 w-full overflow-hidden rounded-full bg-sidebar-accent">
                <div
                  className={cn("h-full rounded-full transition-all", quotaBarColor)}
                  style={{ width: `${quotaPercent}%` }}
                />
              </div>
              <p className="mt-1 text-center text-[10px] text-muted-foreground">
                {t("quotaUsed", { percent: quotaPercent })}
              </p>
            </div>
          )}
        </div>
      </aside>
    </>
  );
}
