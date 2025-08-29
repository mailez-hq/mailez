"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { BellRing, CalendarClock, FolderPlus, House } from "lucide-react";
import type { MailAccount, MailDelegation } from "@/lib/api";
import { FolderACLDialog } from "@/components/mailbox/folder-acl-dialog";
import { buildFolderTree } from "@/components/mailbox/folder-tree";
import type { SavedSearch } from "@/components/mailbox/mail-utils";
import { cn } from "@/lib/utils";
import { AccountMenu } from "./folder-nav/account-menu";
import { FolderDialogs } from "./folder-nav/folder-dialogs";
import { FolderTreeNode } from "./folder-nav/folder-tree-node";
import { LabelSection } from "./folder-nav/label-section";
import { SavedSearchesSection } from "./folder-nav/saved-searches-section";
import { SidebarFooter } from "./folder-nav/sidebar-footer";
import { SidebarHeader } from "./folder-nav/sidebar-header";
import type { FolderDialog } from "./folder-nav/shared";

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
  onSnoozed,
  onCreateFolder,
  onRenameFolder,
  onDeleteFolder,
  onClearFolder,
  onCompose,
  aiComposeEnabled,
  aiComposeBusy,
  onAiCompose,
  onSettings,
  onContacts,
  onSieve,
  onCalendar,
  onDrive,
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
  onSnoozed: () => void;
  onCreateFolder: (name: string) => void;
  onRenameFolder: (name: string, newName: string) => void;
  onDeleteFolder: (name: string) => void;
  onClearFolder: (name: string) => void;
  onCompose: () => void;
  aiComposeEnabled: boolean;
  aiComposeBusy: boolean;
  onAiCompose: () => void;
  onSettings: () => void;
  onContacts: () => void;
  onSieve: () => void;
  onCalendar: () => void;
  onDrive: () => void;
  onLogout: () => void;
  onClose: () => void;
}) {
  const t = useTranslations("mail");
  const pathname = usePathname();
  const onHome = pathname?.startsWith("/home") ?? false;
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
  const toggleFold = (name: string) => {
    setFolded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

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
        <SidebarHeader
          onClose={onClose}
          onCompose={onCompose}
          aiComposeEnabled={aiComposeEnabled}
          aiComposeBusy={aiComposeBusy}
          onAiCompose={onAiCompose}
        />

        {/* account switcher: own mailbox + delegated mailboxes + external accounts */}
        {onSwitchAccount && onManageAccounts && (
          <AccountMenu
            email={email}
            currentAccountEmail={currentAccountEmail}
            accountList={accountList}
            delegateList={delegateList}
            activeAccount={activeAccount}
            activeDelegate={activeDelegate}
            open={accountMenuOpen}
            onOpenChange={setAccountMenuOpen}
            onSwitchAccount={onSwitchAccount}
            onSwitchDelegate={onSwitchDelegate}
            onManageAccounts={onManageAccounts}
          />
        )}

        <nav className="mail-scroll flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
          <Link
            href="/home"
            onClick={onClose}
            className={cn(
              "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
              onHome
                ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
            )}
          >
            <House className="size-4 shrink-0 opacity-70" />
            {t("workspace")}
          </Link>
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
          <FolderTreeNode
            nodes={folderTree}
            current={current}
            unseen={unseen}
            folded={folded}
            onToggleFold={toggleFold}
            menuFor={menuFor}
            onMenuFor={setMenuFor}
            onSelect={onSelect}
            onClose={onClose}
            onMoveToFolder={onMoveToFolder}
            onShare={(name) => setAclFor(name)}
            onRename={openRename}
            onClear={(name) => setDialog({ mode: "clear", name })}
            onDelete={(name) => setDialog({ mode: "delete", name })}
          />
          <div className="mt-3 border-t border-sidebar-border pt-2">
            <button
              onClick={onScheduled}
              className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-sidebar-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground"
            >
              <CalendarClock className="size-4 shrink-0 opacity-70" />
              <span className="truncate">{t("scheduled")}</span>
            </button>
            <button
              onClick={onSnoozed}
              className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-sidebar-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground"
            >
              <BellRing className="size-4 shrink-0 opacity-70" />
              <span className="truncate">{t("snoozed")}</span>
            </button>
          </div>
          {labels && (
            <LabelSection
              labels={labels}
              labelColors={labelColors}
              activeLabel={activeLabel}
              confirmLabel={confirmLabel}
              onConfirmLabel={setConfirmLabel}
              onSelectLabel={onSelectLabel}
              onClose={onClose}
              onManageLabels={onManageLabels}
              onDeleteLabel={onDeleteLabel}
            />
          )}
          {savedSearches && savedSearches.length > 0 && (
            <SavedSearchesSection
              savedSearches={savedSearches}
              onSelectSavedSearch={onSelectSavedSearch}
              onRemoveSavedSearch={onRemoveSavedSearch}
              onClose={onClose}
            />
          )}
        </nav>

        <SidebarFooter
          quotaPercent={quotaPercent}
          quotaBarColor={quotaBarColor}
          onContacts={onContacts}
          onCalendar={onCalendar}
          onDrive={onDrive}
          onSettings={onSettings}
          onSieve={onSieve}
          onLogout={onLogout}
        />
      </aside>

      <FolderDialogs
        dialog={dialog}
        onClose={() => setDialog(null)}
        nameInput={nameInput}
        onNameInput={setNameInput}
        parentInput={parentInput}
        onParentInput={setParentInput}
        parentOptions={parentOptions}
        onSubmit={submitName}
        onConfirmDestructive={confirmDestructive}
        nameRef={nameRef}
      />

      {/* folder sharing / ACL dialog */}
      <FolderACLDialog
        folder={aclFor ?? ""}
        open={aclFor !== null}
        onOpenChange={(o) => !o && setAclFor(null)}
      />
    </>
  );
}
