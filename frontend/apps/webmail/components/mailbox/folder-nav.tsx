"use client";

import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import {
  Archive,
  CalendarClock,
  ChevronDown,
  ChevronRight,
  ChevronsUpDown,
  ExternalLink,
  FileText,
  Inbox,
  LogOut,
  Menu,
  Plus,
  Search,
  Send,
  ShieldAlert,
  Trash2,
  Filter,
  Folder as FolderIcon,
  FolderPlus,
  MoreVertical,
  PenLine,
  Settings,
  Settings2,
  Share2,
  Users,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { Button, buttonVariants } from "@/components/ui/button";
import type { MailAccount, MailDelegation } from "@/lib/api";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Logo } from "@/components/logo";
import { FolderACLDialog } from "@/components/mailbox/folder-acl-dialog";
import { LocaleSwitcher } from "@/components/locale-switcher";
import {
  SYSTEM_FOLDERS,
  buildFolderTree,
  folderLabel,
  type FolderNode,
} from "@/components/mailbox/folder-tree";
import { labelColor, type SavedSearch } from "@/components/mailbox/mail-utils";
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
  JUNK: <ShieldAlert className="size-4" />,
  SPAM: <ShieldAlert className="size-4" />,
};

// A folder-management dialog: create/rename take a name input, delete/clear ask
// for confirmation.
type FolderDialog =
  | { mode: "create"; value: string }
  | { mode: "rename"; name: string; value: string }
  | { mode: "delete"; name: string }
  | { mode: "clear"; name: string };

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
  accountList,
  activeAccount,
  delegateList,
  activeDelegate,
  onSwitchAccount,
  onSwitchDelegate,
  onManageAccounts,
  onSelect,
  onSelectLabel,
  onManageLabels,
  onDeleteLabel,
  onSelectSavedSearch,
  onRemoveSavedSearch,
  onMoveToFolder,
  onScheduled,
  onCreateFolder,
  onRenameFolder,
  onDeleteFolder,
  onClearFolder,
  onCompose,
  onSettings,
  onContacts,
  onSieve,
  onLogout,
  onClose,
}: {
  folders: string[];
  unseen?: Record<string, number>;
  labels?: string[];
  labelColors?: Record<string, string>;
  activeLabel?: string;
  savedSearches?: SavedSearch[];
  current: string;
  email: string;
  quotaBytes?: number;
  quotaUsed?: number;
  open: boolean;
  accountList?: MailAccount[];
  activeAccount?: number | null;
  delegateList?: MailDelegation[];
  activeDelegate?: string | null;
  onSwitchAccount?: (id: number | null) => void;
  onSwitchDelegate?: (email: string | null) => void;
  onManageAccounts?: () => void;
  onSelect: (folder: string) => void;
  onSelectLabel: (label: string) => void;
  onManageLabels?: () => void;
  onDeleteLabel?: (label: string) => void;
  onSelectSavedSearch: (item: SavedSearch) => void;
  onRemoveSavedSearch: (id: number) => void;
  onMoveToFolder: (folder: string, uid: number) => void;
  onScheduled: () => void;
  onCreateFolder: (name: string) => void;
  onRenameFolder: (name: string, newName: string) => void;
  onDeleteFolder: (name: string) => void;
  onClearFolder: (name: string) => void;
  onCompose: () => void;
  onSettings: () => void;
  onContacts: () => void;
  onSieve: () => void;
  onLogout: () => void;
  onClose: () => void;
}) {
  const t = useTranslations("mail");
  const folderTree = buildFolderTree(folders);
  const [dialog, setDialog] = useState<FolderDialog | null>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [confirmLabel, setConfirmLabel] = useState<string | null>(null);
  const [aclFor, setAclFor] = useState<string | null>(null);
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const [nameInput, setNameInput] = useState("");
  const [parentInput, setParentInput] = useState("");
  const nameRef = useRef<HTMLInputElement>(null);
  // Folded parent folders; an empty set means everything is expanded. The
  // currently active folder's ancestors are always expanded regardless.
  const [folded, setFolded] = useState<Set<string>>(new Set());
  const activeExternal = accountList?.find((a) => a.id === activeAccount) ?? null;
  const currentAccountEmail = activeDelegate ?? (activeExternal ? activeExternal.email : email);

  // Close the account switcher on Escape / outside click.
  useEffect(() => {
    if (!accountMenuOpen) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setAccountMenuOpen(false);
    const onDown = (e: MouseEvent) => {
      if (!(e.target instanceof Element) || !e.target.closest("[data-account-menu]")) {
        setAccountMenuOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onDown);
    };
  }, [accountMenuOpen]);
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

  const openCreate = () => {
    setNameInput("");
    setParentInput("");
    setDialog({ mode: "create", value: "" });
    requestAnimationFrame(() => nameRef.current?.focus());
  };
  const openRename = (name: string) => {
    // Prefill the parent (path prefix) and the leaf name so renaming a nested
    // folder only needs the last segment changed.
    const idx = name.lastIndexOf("/");
    const parent = idx >= 0 ? name.slice(0, idx) : "";
    const leaf = idx >= 0 ? name.slice(idx + 1) : name;
    setParentInput(parent);
    setNameInput(leaf);
    setDialog({ mode: "rename", name, value: leaf });
    setMenuFor(null);
    requestAnimationFrame(() => nameRef.current?.focus());
  };
  // Renaming may move the folder under another parent; it cannot be moved
  // under itself or one of its descendants.
  const parentOptions = folders.filter(
    (f) => dialog?.mode !== "rename" || (f !== dialog.name && !f.startsWith(`${dialog.name}/`)),
  );
  const submitName = (e: React.FormEvent) => {
    e.preventDefault();
    const value = nameInput.trim();
    if (!value) return;
    const parent = parentInput.trim();
    const fullName = parent ? `${parent}/${value}` : value;
    if (dialog?.mode === "create") {
      onCreateFolder(fullName);
    } else if (dialog?.mode === "rename") {
      onRenameFolder(dialog.name, fullName);
    }
    setDialog(null);
  };
  const confirmDestructive = () => {
    if (!dialog) return;
    if (dialog.mode === "delete") onDeleteFolder(dialog.name);
    if (dialog.mode === "clear") onClearFolder(dialog.name);
    setDialog(null);
  };

  // renderFolderNodes renders the folder tree recursively: each row keeps the
  // selection / drag-drop / "..." menu behaviour, parent folders get an expand
  // chevron, and children are indented. The active folder's ancestors stay
  // expanded regardless of the folded set.
  const renderFolderNodes = (nodes: FolderNode[]): ReactNode =>
    nodes.map((node) => {
      const active = node.name === current;
      const system = SYSTEM_FOLDERS.has(node.name.toUpperCase());
      const icon =
        FOLDER_ICON[node.name.toUpperCase()] || <FolderIcon className="size-4" />;
      const hasChildren = node.children.length > 0;
      // A parent is only foldable when it is not an ancestor of the active
      // folder; the active path is always kept visible.
      const isFolded =
        folded.has(node.name) && !current.startsWith(`${node.name}/`);
      const showChildren = hasChildren && !isFolded;

      const toggleFold = () => {
        if (!hasChildren) return;
        setFolded((prev) => {
          const next = new Set(prev);
          if (next.has(node.name)) next.delete(node.name);
          else next.add(node.name);
          return next;
        });
      };

      return (
        <Fragment key={node.name}>
          <div className="group relative flex w-full items-center rounded-lg transition-colors">
            {/* No leading spacer on leaf rows: folders, labels and saved
                searches all align flush with the section header. Only a
                parent folder gets the expand/collapse chevron. */}
            {hasChildren && (
              <button
                onClick={toggleFold}
                title={isFolded ? t("expandFolder") : t("collapseFolder")}
                className="flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:text-foreground"
              >
                {isFolded ? (
                  <ChevronRight className="size-3" />
                ) : (
                  <ChevronDown className="size-3" />
                )}
              </button>
            )}
            <button
              onClick={() => {
                onSelect(node.name);
                onClose();
              }}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
              }}
              onDrop={(e) => {
                e.preventDefault();
                const uid = Number(e.dataTransfer.getData("text/plain"));
                if (uid) onMoveToFolder(node.name, uid);
              }}
              className={cn(
                "flex min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
                active
                  ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                  : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
              )}
            >
              <span className="shrink-0 opacity-70">{icon}</span>
              <span className="truncate">{folderLabel(t, node.label)}</span>
              {unseen?.[node.name] != null && unseen[node.name] > 0 && (
                <span className="ml-auto shrink-0 rounded-full bg-primary/15 px-1.5 py-px text-[10px] font-medium text-primary">
                  {unseen[node.name]}
                </span>
              )}
            </button>
            {!system && (
              <div className="absolute right-1 top-1/2 z-30 -translate-y-1/2">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className={cn(
                    "size-6 rounded-md",
                    menuFor === node.name
                      ? "opacity-100"
                      : "opacity-0 group-hover:opacity-100",
                  )}
                  onClick={() => setMenuFor(menuFor === node.name ? null : node.name)}
                >
                  <MoreVertical className="size-3.5" />
                  <span className="sr-only">{t("menu")}</span>
                </Button>
                {menuFor === node.name && (
                  <div className="absolute right-0 top-full z-30 w-40 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                    <button
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                      onClick={() => {
                        setMenuFor(null);
                        setAclFor(node.name);
                      }}
                    >
                      <Share2 className="size-3.5" />
                      {t("shareFolder")}
                    </button>
                    <button
                      className="flex w-full items-center rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                      onClick={() => openRename(node.name)}
                    >
                      {t("renameFolder")}
                    </button>
                    <button
                      className="flex w-full items-center rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                      onClick={() => {
                        setMenuFor(null);
                        setDialog({ mode: "clear", name: node.name });
                      }}
                    >
                      {t("clearFolder")}
                    </button>
                    <button
                      className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-destructive transition-colors hover:bg-muted"
                      onClick={() => {
                        setMenuFor(null);
                        setDialog({ mode: "delete", name: node.name });
                      }}
                    >
                      {t("deleteFolder")}
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>
          {showChildren && (
            <div className="pl-6">{renderFolderNodes(node.children)}</div>
          )}
        </Fragment>
      );
    });

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
          {/* Logo navigates straight into the mailbox instead of "/" (the
              sign-in route), so clicking it never flashes the login page or
              re-runs the auth check. */}
          <Link
            href="/mail/Inbox"
            onClick={onClose}
            className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80"
          >
            <Logo />
            Mailez Webmail
          </Link>
          <Button variant="ghost" size="sm" onClick={onClose} className="lg:hidden">
            <Menu className="size-4" />
            <span className="sr-only">{t("menu")}</span>
          </Button>
        </div>

        {/* account switcher: own mailbox + delegated mailboxes + external accounts */}
        {onSwitchAccount && onManageAccounts && (
          <div className="relative px-3 pb-2" data-account-menu>
            <button
              onClick={() => setAccountMenuOpen((o) => !o)}
              className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm transition-colors hover:bg-sidebar-accent/60"
              title={currentAccountEmail}
            >
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
                {currentAccountEmail.charAt(0).toUpperCase()}
              </span>
              <span className="min-w-0 flex-1 truncate text-left">{currentAccountEmail}</span>
              <ChevronsUpDown className="size-3.5 shrink-0 opacity-50" />
            </button>
            {accountMenuOpen && (
              <div className="absolute left-3 right-3 top-full z-30 mt-1 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                <button
                  onClick={() => {
                    onSwitchAccount(null);
                    setAccountMenuOpen(false);
                  }}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                    activeAccount === null && activeDelegate === null && "bg-muted font-medium",
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">{email}</span>
                  <span className="text-[10px] text-muted-foreground">{t("myAccount")}</span>
                </button>
                {delegateList && delegateList.length > 0 && (
                  <div className="mt-0.5 border-t border-border pt-0.5">
                    <p className="px-2 pb-1 pt-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
                      {t("delegatedMailboxes")}
                    </p>
                    {delegateList.map((d) => (
                      <button
                        key={d.id}
                        className={cn(
                          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                          activeDelegate === d.owner_email && "bg-muted font-medium",
                        )}
                        onClick={() => {
                          onSwitchDelegate?.(d.owner_email);
                          setAccountMenuOpen(false);
                        }}
                      >
                        <span className="min-w-0 flex-1 truncate">{d.owner_email}</span>
                        <span className="rounded bg-accent px-1.5 py-px text-[10px] text-accent-foreground">
                          {t("delegated")}
                        </span>
                      </button>
                    ))}
                  </div>
                )}
                {accountList?.map((a) => (
                  <button
                    key={a.id}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                      activeAccount === a.id && "bg-muted font-medium",
                    )}
                    onClick={() => {
                      onSwitchAccount(a.id);
                      setAccountMenuOpen(false);
                    }}
                  >
                    <span className="min-w-0 flex-1 truncate">{a.email}</span>
                    {!a.enabled && <span className="text-[10px] text-destructive">{t("accountOff")}</span>}
                  </button>
                ))}
                <button
                  className="mt-0.5 flex w-full items-center gap-2 rounded-md border-t border-border px-2 py-1.5 text-left text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onManageAccounts();
                  }}
                >
                  <Plus className="size-3.5" />
                  {t("manageAccounts")}
                </button>
              </div>
            )}
          </div>
        )}

        <div className="px-3 pb-2">
          <Button className="w-full" onClick={onCompose}>
            <PenLine className="size-4" />
            {t("write")}
          </Button>
        </div>

        <nav className="mail-scroll flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
          <div className="flex items-center justify-between pr-1">
            <p className="px-2.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
              {t("folders")}
            </p>
            <button
              onClick={openCreate}
              title={t("newFolder")}
              className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              <FolderPlus className="size-3.5" />
            </button>
          </div>
          {renderFolderNodes(folderTree)}
          <div className="mt-3 border-t border-sidebar-border pt-2">
            <button
              onClick={onScheduled}
              className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-sidebar-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground"
            >
              <CalendarClock className="size-4 shrink-0 opacity-70" />
              <span className="truncate">{t("scheduled")}</span>
            </button>
          </div>
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
                <div key={label} className="group relative flex w-full items-center">
                  <button
                    onClick={() => {
                      onSelectLabel(label);
                      onClose();
                    }}
                    className={cn(
                      "flex min-w-0 flex-1 items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
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
                  {onDeleteLabel && (
                    <div className="absolute right-1 top-1/2 z-30 -translate-y-1/2">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        title={t("delete")}
                        className={cn(
                          "size-6 rounded-md",
                          confirmLabel === label
                            ? "opacity-100"
                            : "opacity-0 group-hover:opacity-100",
                        )}
                        onClick={() =>
                          setConfirmLabel(confirmLabel === label ? null : label)
                        }
                      >
                        <Trash2 className="size-3.5" />
                        <span className="sr-only">{t("delete")}</span>
                      </Button>
                      {confirmLabel === label && (
                        <div className="absolute right-0 top-full z-30 mt-1 w-40 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                          <p className="px-2 py-1 text-xs text-muted-foreground">
                            {t("confirmDeleteLabel", { name: label })}
                          </p>
                          <div className="flex items-center gap-1">
                            <Button
                              size="xs"
                              variant="destructive"
                              className="flex-1"
                              onClick={() => {
                                onDeleteLabel(label);
                                setConfirmLabel(null);
                              }}
                            >
                              {t("delete")}
                            </Button>
                            <Button
                              size="xs"
                              variant="ghost"
                              onClick={() => setConfirmLabel(null)}
                            >
                              {t("cancel")}
                            </Button>
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
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
              {savedSearches.map((item) => (
                <div
                  key={item.id}
                  className="group flex w-full items-center gap-1 rounded-lg px-1 transition-colors hover:bg-sidebar-accent/60"
                >
                  <button
                    onClick={() => {
                      onSelectSavedSearch(item);
                      onClose();
                    }}
                    className="min-w-0 flex-1 rounded-lg px-1.5 py-1.5 text-left text-sm text-sidebar-foreground transition-colors hover:text-foreground"
                  >
                    <span className="flex items-center gap-2 truncate">
                      <Search className="size-3.5 shrink-0 opacity-60" />
                      <span className="truncate">{item.name}</span>
                    </span>
                  </button>
                  <button
                    onClick={() => onRemoveSavedSearch(item.id)}
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
          <div className="mb-1 flex items-center justify-center">
            <LocaleSwitcher />
          </div>
          {/* Divider between the language switcher and the user info /
              account controls below. */}
          <div className="mx-2 mb-1.5 border-t border-sidebar-border" />
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
            <Button variant="ghost" size="sm" onClick={onLogout} title={t("logout")}>
              <LogOut className="size-4" />
            </Button>
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

      {/* folder name dialog (create / rename) */}
      <Dialog open={dialog !== null && (dialog.mode === "create" || dialog.mode === "rename")} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "rename" ? t("renameFolder") : t("newFolder")}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={submitName} className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="folder-parent">{t("folderParent")}</Label>
              <select
                id="folder-parent"
                value={parentInput}
                onChange={(e) => setParentInput(e.target.value)}
                className="rounded-md border border-input bg-background px-2 py-1.5 text-sm"
              >
                <option value="">{t("folderParentRoot")}</option>
                {parentOptions.map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="folder-name" className="sr-only">
                {t("folderNamePlaceholder")}
              </Label>
              <Input
                id="folder-name"
                ref={nameRef}
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                placeholder={t("folderNamePlaceholder")}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialog(null)}>
                {t("cancel")}
              </Button>
              <Button type="submit" disabled={!nameInput.trim()}>
                {t("save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* confirm dialog (delete / clear) */}
      <Dialog open={dialog !== null && (dialog.mode === "delete" || dialog.mode === "clear")} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "delete" ? t("deleteFolder") : t("clearFolder")}
            </DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {dialog && dialog.mode !== "create"
              ? dialog.mode === "delete"
                ? t("confirmDeleteFolder", { name: dialog.name })
                : t("confirmClearFolder", { name: dialog.name })
              : ""}
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialog(null)}>
              {t("cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDestructive}>
              {t("confirmDelete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* folder sharing / ACL dialog */}
      <FolderACLDialog
        folder={aclFor ?? ""}
        open={aclFor !== null}
        onOpenChange={(o) => !o && setAclFor(null)}
      />
    </>
  );
}
