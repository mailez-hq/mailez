"use client";

import { Fragment, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import {
  Archive,
  ChevronDown,
  ChevronRight,
  FileText,
  Folder as FolderIcon,
  Inbox,
  MoreVertical,
  Send,
  Share2,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  isSystemFolder,
  folderLabel,
  type FolderNode,
} from "@/components/mailbox/folder-tree";
import { cn } from "@/lib/utils";

const FOLDER_ICON: Record<string, ReactNode> = {
  INBOX: <Inbox className="size-4" />,
  SENT: <Send className="size-4" />,
  DRAFTS: <FileText className="size-4" />,
  TRASH: <Trash2 className="size-4" />,
  ARCHIVE: <Archive className="size-4" />,
  JUNK: <ShieldAlert className="size-4" />,
  SPAM: <ShieldAlert className="size-4" />,
};

// Renders the folder tree recursively: each row keeps the selection /
// drag-drop / "..." menu behaviour, parent folders get an expand chevron, and
// children are indented. The active folder's ancestors stay expanded
// regardless of the folded set.
export function FolderTreeNode({
  nodes,
  current,
  unseen,
  folded,
  onToggleFold,
  menuFor,
  onMenuFor,
  onSelect,
  onClose,
  onMoveToFolder,
  onShare,
  onRename,
  onClear,
  onDelete,
}: {
  nodes: FolderNode[];
  current: string;
  unseen?: Record<string, number>;
  folded: Set<string>;
  onToggleFold: (name: string) => void;
  menuFor: string | null;
  onMenuFor: (name: string | null) => void;
  onSelect: (folder: string) => void;
  onClose: () => void;
  onMoveToFolder: (folder: string, uid: number) => void;
  onShare: (name: string) => void;
  onRename: (name: string) => void;
  onClear: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const t = useTranslations("mail");

  return (
    <Fragment>
      {nodes.map((node) => {
        const active = node.name === current;
        const system = isSystemFolder(node.name);
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
          onToggleFold(node.name);
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
                    onClick={() =>
                      onMenuFor(menuFor === node.name ? null : node.name)
                    }
                  >
                    <MoreVertical className="size-3.5" />
                    <span className="sr-only">{t("menu")}</span>
                  </Button>
                  {menuFor === node.name && (
                    <div className="absolute right-0 top-full z-30 w-40 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg">
                      <button
                        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                        onClick={() => {
                          onMenuFor(null);
                          onShare(node.name);
                        }}
                      >
                        <Share2 className="size-3.5" />
                        {t("shareFolder")}
                      </button>
                      <button
                        className="flex w-full items-center rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                        onClick={() => onRename(node.name)}
                      >
                        {t("renameFolder")}
                      </button>
                      <button
                        className="flex w-full items-center rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                        onClick={() => {
                          onMenuFor(null);
                          onClear(node.name);
                        }}
                      >
                        {t("clearFolder")}
                      </button>
                      <button
                        className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-destructive transition-colors hover:bg-muted"
                        onClick={() => {
                          onMenuFor(null);
                          onDelete(node.name);
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
              <div className="pl-6">
                <FolderTreeNode
                  nodes={node.children}
                  current={current}
                  unseen={unseen}
                  folded={folded}
                  onToggleFold={onToggleFold}
                  menuFor={menuFor}
                  onMenuFor={onMenuFor}
                  onSelect={onSelect}
                  onClose={onClose}
                  onMoveToFolder={onMoveToFolder}
                  onShare={onShare}
                  onRename={onRename}
                  onClear={onClear}
                  onDelete={onDelete}
                />
              </div>
            )}
          </Fragment>
        );
      })}
    </Fragment>
  );
}
