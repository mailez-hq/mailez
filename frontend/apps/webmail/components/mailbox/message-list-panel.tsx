"use client";

import { Archive, Bookmark, FolderSearch, Inbox, Menu, RefreshCw, Search, SearchX, Sparkles, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePreferences } from "@/components/preferences-provider";
import type { MailMessage } from "@/lib/api";
import { cn } from "@/lib/utils";
import { MessageRow, ROW_HEIGHTS } from "@/components/mailbox/message-row";
import { VirtualList } from "@/components/mailbox/virtual-list";

const SKELETON_ROWS = 8;

export function MessageListPanel({
  folder,
  messages,
  total,
  searching,
  loading,
  query,
  onQueryChange,
  onSearch,
  onClearSearch,
  selectedUids,
  cursor,
  onOpen,
  onToggleSelect,
  onDelete,
  onStar,
  onArchive,
  openId,
  onBulkDelete,
  onBulkArchive,
  onBulkSpam,
  onBulkFlag,
  onLoadMore,
  searchInputRef,
  onMenu,
  error,
  className,
  folders,
  onMoveToFolder,
  onSaveSearch,
  searchAll,
  onToggleSearchAll,
  activeView,
  onSelectView,
  aiSearchEnabled,
  aiPriorityEnabled,
  prioritizing,
  priorityOn,
  categories,
  onTogglePriority,
  aiSearching,
  onAiSearch,
  refreshing,
  onRefresh,
  highlightTerms,
  onContextMenu,
}: {
  folder: string;
  messages: MailMessage[];
  total: number;
  searching: boolean;
  loading: boolean;
  query: string;
  onQueryChange: (q: string) => void;
  onSearch: () => void;
  onClearSearch: () => void;
  selectedUids: Set<number>;
  cursor: number;
  onOpen: (m: MailMessage) => void;
  onToggleSelect: (m: MailMessage) => void;
  onDelete: (m: MailMessage) => void;
  onStar: (m: MailMessage) => void;
  onArchive: (m: MailMessage) => void;
  openId?: string;
  onBulkDelete: () => void;
  onBulkArchive: () => void;
  onBulkSpam: () => void;
  onBulkFlag: (flag: string, value: boolean) => void;
  onLoadMore: () => void;
  searchInputRef: React.RefObject<HTMLInputElement | null>;
  onMenu: () => void;
  error: string;
  className?: string;
  folders: string[];
  onMoveToFolder: (destination: string) => void;
  onSaveSearch: () => void;
  searchAll: boolean;
  onToggleSearchAll: () => void;
  activeView: string;
  onSelectView: (view: string) => void;
  aiSearchEnabled: boolean;
  aiPriorityEnabled: boolean;
  prioritizing: boolean;
  priorityOn: boolean;
  categories?: Record<string, string>;
  onTogglePriority: () => void;
  aiSearching: boolean;
  onAiSearch: () => void;
  refreshing: boolean;
  onRefresh: () => void;
  highlightTerms?: string[];
  onContextMenu?: (e: React.MouseEvent, message: MailMessage) => void;
}) {
  const t = useTranslations("mail");
  const { density } = usePreferences();
  const rowHeight = ROW_HEIGHTS[density];
  const folderLabel = (name: string) => {
    const key = `folder${name.charAt(0).toUpperCase()}${name.slice(1).toLowerCase()}`;
    return t.has(key) ? t(key) : name;
  };

  return (
    <div className={cn("flex min-w-0 flex-col border-r border-border bg-card", className)}>
      <div className="border-b border-border p-2">
        <div className="flex items-center gap-1.5">
          <Button variant="ghost" size="icon-sm" className="shrink-0 md:hidden" onClick={onMenu}>
            <Menu className="size-4" />
            <span className="sr-only">{t("menu")}</span>
          </Button>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              onSearch();
            }}
            className="relative flex-1"
          >
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              ref={searchInputRef}
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  onClearSearch();
                  e.currentTarget.blur();
                }
              }}
              placeholder={t("searchPlaceholder", { folder: folderLabel(folder) })}
              className="h-8 pl-8 pr-12"
            />
            {query.trim() !== "" ? (
              <button
                type="button"
                onClick={onClearSearch}
                title={t("clearSearch")}
                className="absolute top-1/2 right-2 -translate-y-1/2 rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <X className="size-3.5" />
              </button>
            ) : (
              <kbd className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 rounded border border-border px-1.5 py-0.5 font-sans text-[10px] text-muted-foreground">
                /
              </kbd>
            )}
          </form>
          {searching && query.trim() !== "" && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onSaveSearch}
              title={t("saveSearch")}
              className="shrink-0"
            >
              <Bookmark className="size-4" />
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onToggleSearchAll}
            title={t("searchAll")}
            className={cn("shrink-0", searchAll && "text-primary")}
          >
            <FolderSearch className="size-4" />
          </Button>
          {aiSearchEnabled && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => onAiSearch()}
              disabled={aiSearching}
              title={t("aiSearch")}
              className="shrink-0"
            >
              <Sparkles className={cn("size-4", aiSearching && "animate-pulse")} />
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onRefresh}
            disabled={refreshing}
            title={t("refresh")}
            className="shrink-0"
          >
            <RefreshCw className={cn("size-4", refreshing && "animate-spin")} />
          </Button>
        </div>
        <div className="mt-1.5 flex items-center gap-1">
          {["all", "unread", "flagged", "attachment"].map((v) => (
            <button
              key={v}
              type="button"
              onClick={() => onSelectView(v)}
              className={cn(
                "rounded-full px-2 py-0.5 text-[11px] transition-colors",
                activeView === v
                  ? "bg-accent font-medium text-accent-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              {t(`view${v.charAt(0).toUpperCase()}${v.slice(1)}`)}
            </button>
          ))}
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          <span className="hidden text-[10px] text-muted-foreground sm:inline">{t("syntaxHint")}</span>
          <div className="flex flex-wrap items-center gap-1">
            {["from:", "to:", "subject:", "has:attachment", "before:", "after:"].map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => onQueryChange(query ? `${query} ${s}` : s)}
                className="rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                {s}
              </button>
            ))}
          </div>
          {aiPriorityEnabled && (
            <Button
              size="xs"
              variant={priorityOn ? "default" : "outline"}
              onClick={onTogglePriority}
              disabled={prioritizing}
              className="ml-auto"
            >
              <Sparkles className="size-3" />
              {prioritizing ? t("prioritizing") : t("aiPriority")}
            </Button>
          )}
        </div>
      </div>

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-3 py-1.5 text-xs text-destructive">
          {error}
        </div>
      )}

      {selectedUids.size > 0 && (
        <div className="flex items-center gap-1 border-b border-border px-2 py-1.5">
          <span className="mr-1 text-xs text-muted-foreground">
            {t("selected", { count: selectedUids.size })}
          </span>
          <Button size="xs" variant="outline" onClick={onBulkDelete}>
            <Trash2 className="size-3" />
            {t("delete")}
          </Button>
          <Button size="xs" variant="outline" onClick={onBulkArchive}>
            <Archive className="size-3" />
            {t("archive")}
          </Button>
          <Button size="xs" variant="outline" onClick={onBulkSpam}>
            <Inbox className="size-3" />
            {t("spam")}
          </Button>
          <Button size="xs" variant="outline" onClick={() => onBulkFlag("\\Seen", true)}>
            {t("read")}
          </Button>
          <Button size="xs" variant="outline" onClick={() => onBulkFlag("\\Seen", false)}>
            {t("unread")}
          </Button>
          <select
            value=""
            onChange={(e) => {
              if (e.target.value) onMoveToFolder(e.target.value);
            }}
            title={t("moveTo")}
            className="h-6 rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
          >
            <option value="">{t("moveTo")}…</option>
            {folders
              .filter((f) => f !== folder && !/^(trash|drafts)$/i.test(f))
              .map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
          </select>
        </div>
      )}

      {loading && messages.length === 0 ? (
        <div className="flex-1">
          {Array.from({ length: SKELETON_ROWS }).map((_, i) => (
            <div key={i} className="flex items-center gap-2 border-b border-border px-3 py-2.5">
              <div className="size-7 animate-pulse rounded-full bg-muted" />
              <div className="flex-1 space-y-1.5">
                <div className="h-2.5 w-1/3 animate-pulse rounded bg-muted" />
                <div className="h-2.5 w-2/3 animate-pulse rounded bg-muted/70" />
              </div>
            </div>
          ))}
        </div>
      ) : messages.length === 0 ? (
        <div className="flex flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
          {searching ? t("noMatches") : t("noMessages")}
        </div>
      ) : (
        <VirtualList
          items={messages}
          rowHeight={rowHeight}
          scrollKey={`${folder}-${searching}`}
          onEndReached={onLoadMore}
          onPullRefresh={onRefresh}
          getKey={(m) => m.uid}
          className="flex-1"
          renderRow={(m, i) => (
            <MessageRow
              message={m}
              index={i}
              density={density}
              category={categories?.[String(m.uid)]}
              selected={m.id === openId && !!openId}
              selectedInBulk={selectedUids.has(m.uid)}
              cursorActive={i === cursor && !searching}
              onOpen={() => onOpen(m)}
              onToggleSelect={() => onToggleSelect(m)}
              onDelete={() => onDelete(m)}
              onArchive={() => onArchive(m)}
              onStar={() => onStar(m)}
              highlightTerms={highlightTerms}
              onContextMenu={onContextMenu}
            />
          )}
        />
      )}

      {!searching && !loading && messages.length > 0 && messages.length < total && (
        <button
          onClick={onLoadMore}
          className="w-full border-t border-border p-2 text-xs text-muted-foreground transition-colors hover:bg-muted"
        >
          {t("loadMore", { count: total - messages.length })}
        </button>
      )}

      {searching && (
        <button
          onClick={onClearSearch}
          className="flex w-full items-center justify-center gap-1 border-t border-border p-2 text-xs text-muted-foreground transition-colors hover:bg-muted"
        >
          <SearchX className="size-3" />
          {t("search")} ✕
        </button>
      )}
    </div>
  );
}
