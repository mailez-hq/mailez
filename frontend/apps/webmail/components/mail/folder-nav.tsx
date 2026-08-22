"use client";

import {
  Archive,
  FileText,
  Inbox,
  Menu,
  Send,
  Star,
  Trash2,
  Filter,
  Folder as FolderIcon,
  PenLine,
  Settings,
  Users,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

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
  current,
  email,
  open,
  onSelect,
  onCompose,
  onSettings,
  onContacts,
  onSieve,
  onClose,
}: {
  folders: string[];
  current: string;
  email: string;
  open: boolean;
  onSelect: (folder: string) => void;
  onCompose: () => void;
  onSettings: () => void;
  onContacts: () => void;
  onSieve: () => void;
  onClose: () => void;
}) {
  const t = useTranslations("mail");
  const orderedFolders = sortFolders(folders);

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
          <span className="text-lg font-extrabold tracking-tight text-foreground">
            mail
            <span className="bg-gradient-to-r from-[#2F8E6C] to-[#2E6E8E] bg-clip-text text-transparent">
              ez
            </span>
          </span>
          <Button variant="ghost" size="icon-sm" className="lg:hidden" onClick={onClose}>
            <Menu className="size-4" />
            <span className="sr-only">Menu</span>
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
                className={cn(
                  "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                  active
                    ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                    : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
                )}
              >
                <span className="shrink-0 opacity-70">{icon}</span>
                <span className="truncate">{folderLabel(t, f)}</span>
              </button>
            );
          })}
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
          </div>
          <p className="truncate px-2 text-center text-xs text-muted-foreground">{email}</p>
        </div>
      </aside>
    </>
  );
}
