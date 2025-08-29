"use client";

import { useMemo, useState } from "react";
import { Archive, Bookmark, Check, ChevronDown, Ellipsis, FolderSearch, Inbox, MailCheck, Menu, Plus, RefreshCw, Search, SearchX, SlidersHorizontal, Sparkles, Tag, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { usePreferences } from "@/components/preferences-provider";
  import type { MailMessage, MailSearchSpec } from "@/lib/api";
import { cn } from "@/lib/utils";
import { MessageRow, ROW_HEIGHTS } from "@/components/mailbox/message-row";
import { SearchBuilderDialog } from "@/components/mailbox/search-builder-dialog";
import { VirtualList } from "@/components/mailbox/virtual-list";
import { buildFolderTree, flattenTree, folderLabel } from "@/components/mailbox/folder-tree";

const SKELETON_ROWS = 8;

export function MessageListPanel({
  folder,
  messages,
  displayMessages,
  total,
  searching,
  loading,
  query,
  searchSpec,
  onQueryChange,
  onApplySpec,
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
  onSaveSearchSpec,
  searchAll,
  onToggleSearchAll,
  category,
  onCategoryChange,
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
  sortBy,
  sortDir,
  onChangeSort,
  onBulkLabel,
  labels,
  onMarkAllRead,
}: {
  folder: string;
  messages: MailMessage[];
  displayMessages: MailMessage[];
  total: number;
  searching: boolean;
  loading: boolean;
  query: string;
  searchSpec: MailSearchSpec | null;
  onQueryChange: (q: string) => void;
  onApplySpec: (spec: MailSearchSpec) => void;
  onSearch: (q?: string) => void;
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
  onSaveSearchSpec: (name: string, spec: MailSearchSpec) => void;
  searchAll: boolean;
  onToggleSearchAll: () => void;
  category: string;
  onCategoryChange: (category: string) => void;
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
  sortBy: string;
  sortDir: string;
  onChangeSort: (key: string) => void;
  onBulkLabel: (label: string) => void;
  labels?: string[];
  onMarkAllRead: () => void;
}) {
  const t = useTranslations("mail");
  const { density } = usePreferences();
  const rowHeight = ROW_HEIGHTS[density];
  const [builderOpen, setBuilderOpen] = useState(false);
  const [labelPopoverOpen, setLabelPopoverOpen] = useState(false);
  const [bulkLabelName, setBulkLabelName] = useState("");
  const activeFilterCount = useMemo(() => {
    if (!searchSpec) return 0;
    return Object.values(searchSpec).filter((v) =>
      Array.isArray(v) ? v.length > 0 : Boolean(v),
    ).length;
  }, [searchSpec]);
  // Category pills filter the loaded page client-side (the backend still
  // returns every row; classification is a lightweight per-row tag).
  const categoryOf = (m: MailMessage) => categories?.[String(m.uid)] ?? m.category;
  const filtered = category ? displayMessages.filter((m) => categoryOf(m) === category) : displayMessages;
  const shown = filtered;
  // Only show category pills for categories that actually appear in the
  // loaded page, so the filter row doesn't advertise dead categories.
  const presentCategories = useMemo(() => {
    const found = new Set<string>();
    for (const m of displayMessages) {
      const c = categoryOf(m);
      if (c) found.add(c);
    }
    return ["work", "social", "newsletter", "shopping", "finance", "other"].filter((c) =>
      found.has(c),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [displayMessages, categories]);
  // "其他" alone is not a useful filter (filtering it equals showing all), so
  // the category row only appears when a real category exists on the page;
  // "其他" is still offered when it mixes with real categories.
  const meaningfulCategories = presentCategories.filter((c) => c !== "other");
  const hasOther = presentCategories.includes("other");
  const showCategoryFilter = meaningfulCategories.length > 0;
  const categoryOptions = hasOther
    ? [...meaningfulCategories, "other"]
    : meaningfulCategories;
  const categoryLabel = (c: string) =>
    c === "" ? t("categoryAll") : t(`category${c.charAt(0).toUpperCase()}${c.slice(1)}`);

  return (
    <div className={cn("flex min-h-0 min-w-0 flex-col border-r border-border bg-card", className)}>
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
              placeholder={t("searchPlaceholder", { folder: folderLabel(t, folder) })}
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
          <select
            value={sortDir === "asc" && sortBy === "date" ? "date-asc" : `${sortBy}-${sortDir}`}
            onChange={(e) => onChangeSort(e.target.value)}
            title={t("sortLabel")}
            className="h-7 rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
          >
            <option value="date-desc">{t("sortNewest")}</option>
            <option value="date-asc">{t("sortOldest")}</option>
            <option value="from">{t("sortFrom")}</option>
            <option value="subject">{t("sortSubject")}</option>
            <option value="size">{t("sortSize")}</option>
          </select>
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
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  title={t("more")}
                  className={cn("relative shrink-0", activeFilterCount > 0 && "text-primary")}
                >
                  <Ellipsis className="size-4" />
                  {activeFilterCount > 0 && (
                    <span className="absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full bg-primary text-[9px] font-semibold text-primary-foreground">
                      {activeFilterCount}
                    </span>
                  )}
                </Button>
              }
            />
            <DropdownMenuContent align="end">
              {searching && query.trim() !== "" && (
                <DropdownMenuItem onClick={onSaveSearch}>
                  <Bookmark className="size-4" />
                  {t("saveSearch")}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem onClick={() => setBuilderOpen(true)}>
                <SlidersHorizontal className="size-4" />
                {t("advancedSearch")}
                {activeFilterCount > 0 && (
                  <span className="ml-auto rounded-full bg-primary px-1.5 text-[10px] font-semibold text-primary-foreground">
                    {activeFilterCount}
                  </span>
                )}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onToggleSearchAll}>
                <FolderSearch className="size-4" />
                {t("searchAll")}
                {searchAll && <Check className="ml-auto size-4" />}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onMarkAllRead}>
                <MailCheck className="size-4" />
                {t("markAllRead")}
              </DropdownMenuItem>
              {aiSearchEnabled && (
                <DropdownMenuItem onClick={() => onAiSearch()} disabled={aiSearching}>
                  <Sparkles className={cn("size-4", aiSearching && "animate-pulse")} />
                  {t("aiSearch")}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {(showCategoryFilter || aiPriorityEnabled) && (
          <div className="mt-1.5 flex items-center gap-1">
            {showCategoryFilter && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button size="xs" variant="outline">
                      <span className="max-w-32 truncate">{categoryLabel(category)}</span>
                      <ChevronDown className="size-3" />
                    </Button>
                  }
                />
                <DropdownMenuContent align="start">
                  <DropdownMenuItem onClick={() => onCategoryChange("")}>
                    {t("categoryAll")}
                    {category === "" && <Check className="ml-auto size-4" />}
                  </DropdownMenuItem>
                  {categoryOptions.map((c) => (
                    <DropdownMenuItem key={c} onClick={() => onCategoryChange(c)}>
                      {categoryLabel(c)}
                      {category === c && <Check className="ml-auto size-4" />}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
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
        )}
      </div>

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-3 py-1.5 text-xs text-destructive">
          {error}
        </div>
      )}

      {selectedUids.size > 0 && (
        <div className="flex flex-wrap items-center gap-1 border-b border-border px-2 py-1.5">
          <span className="mr-1 shrink-0 whitespace-nowrap text-xs text-muted-foreground">
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
          <div className="relative">
            <Button size="xs" variant="outline" onClick={() => setLabelPopoverOpen((v) => !v)}>
              <Tag className="size-3" />
              {t("bulkLabel")}
            </Button>
            {labelPopoverOpen && (
              <div className="absolute left-0 top-full z-30 mt-1 w-52 rounded-lg border border-border bg-popover p-1.5 text-sm shadow-lg">
                <div className="max-h-36 space-y-0.5 overflow-y-auto">
                  {(labels || []).map((l) => (
                    <button
                      key={l}
                      type="button"
                      onClick={() => {
                        onBulkLabel(l);
                        setLabelPopoverOpen(false);
                      }}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition-colors hover:bg-muted"
                    >
                      <span className="size-2 shrink-0 rounded-full bg-muted-foreground/40" />
                      <span className="truncate">{l}</span>
                    </button>
                  ))}
                </div>
                <div className="mt-1 flex items-center gap-1 border-t border-border pt-1">
                  <input
                    value={bulkLabelName}
                    onChange={(e) => setBulkLabelName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && bulkLabelName.trim()) {
                        onBulkLabel(bulkLabelName.trim());
                        setBulkLabelName("");
                        setLabelPopoverOpen(false);
                      }
                    }}
                    placeholder={t("newLabel")}
                    className="h-7 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <Button
                    size="xs"
                    onClick={() => {
                      if (bulkLabelName.trim()) {
                        onBulkLabel(bulkLabelName.trim());
                        setBulkLabelName("");
                        setLabelPopoverOpen(false);
                      }
                    }}
                  >
                    <Plus className="size-3" />
                  </Button>
                </div>
              </div>
            )}
          </div>
          <select
            value=""
            onChange={(e) => {
              if (e.target.value) onMoveToFolder(e.target.value);
            }}
            title={t("moveTo")}
            className="h-6 rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
          >
            <option value="">{t("moveTo")}…</option>
            {flattenTree(buildFolderTree(folders), (leaf) => folderLabel(t, leaf))
              .filter((o) => o.value !== folder && !/^(trash|drafts)$/i.test(o.value))
              .map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
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
      ) : shown.length === 0 ? (
        <div className="flex flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
          {searching || category ? t("noMatches") : t("noMessages")}
        </div>
      ) : (
        <VirtualList
          items={shown}
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
              category={categories?.[String(m.uid)] ?? m.category}
              selected={m.id === openId && !!openId}
              selectedInBulk={selectedUids.has(m.uid)}
              cursorActive={i === cursor && !searching}
              // Handlers pass straight through (no per-row closures) so
              // MessageRow's memoization sees stable identities.
              onOpen={onOpen}
              onToggleSelect={onToggleSelect}
              onDelete={onDelete}
              onArchive={onArchive}
              onStar={onStar}
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

      <SearchBuilderDialog
        open={builderOpen}
        onOpenChange={setBuilderOpen}
        onApply={onApplySpec}
        onSave={onSaveSearchSpec}
      />
    </div>
  );
}
