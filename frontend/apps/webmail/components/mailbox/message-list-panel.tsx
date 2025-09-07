"use client";

import { useMemo, useState } from "react";
import { Archive, Bookmark, Check, ChevronDown, Inbox, MailCheck, Plus, RefreshCw, SearchX, ShieldCheck, SlidersHorizontal, Sparkles, Tag, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
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
  spamFolder,
  onBulkRelease,
  onLoadMore,
  error,
  className,
  folders,
  onMoveToFolder,
  onSaveSearch,
  category,
  onCategoryChange,
  aiPriorityEnabled,
  prioritizing,
  priorityOn,
  categories,
  onTogglePriority,
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
  spamFolder?: string;
  onBulkRelease?: () => void;
  onLoadMore: () => void;
  error: string;
  className?: string;
  folders: string[];
  onMoveToFolder: (destination: string) => void;
  onSaveSearch: () => void;
  category: string;
  onCategoryChange: (category: string) => void;
  aiPriorityEnabled: boolean;
  prioritizing: boolean;
  priorityOn: boolean;
  categories?: Record<string, string>;
  onTogglePriority: () => void;
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
  const [labelPopoverOpen, setLabelPopoverOpen] = useState(false);
  const [bulkLabelName, setBulkLabelName] = useState("");
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
          {showCategoryFilter && (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button size="xs" variant="outline" className="shrink-0">
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
          <select
            value={`${sortBy}-${sortDir}`}
            onChange={(e) => onChangeSort(e.target.value)}
            title={t("sortLabel")}
            className="h-7 cursor-pointer rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
          >
            <option value="date-desc">{t("sortNewest")}</option>
            <option value="date-asc">{t("sortOldest")}</option>
            {/* Option values must stay canonical "<field>-<dir>": the select
                is controlled by `${sortBy}-${sortDir}`, and a bare "size"
                value would match nothing and visibly refuse to switch. */}
            <option value="from-asc">{t("sortFrom")}</option>
            <option value="subject-asc">{t("sortSubject")}</option>
            <option value="size-asc">{t("sortSize")}</option>
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
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onMarkAllRead}
            title={t("markAllRead")}
            className="shrink-0"
          >
            <MailCheck className="size-4" />
          </Button>
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
          {aiPriorityEnabled && (
            <Button
              size="xs"
              variant={priorityOn ? "default" : "outline"}
              onClick={onTogglePriority}
              disabled={prioritizing}
              className="ml-auto shrink-0"
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

      {/* Quarantine (junk) banner: explains the folder and offers one-tap
          release of the selected messages back to the inbox. */}
      {folder === spamFolder && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
          <ShieldCheck className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1">{t("quarantineHint")}</span>
          {onBulkRelease && selectedUids.size > 0 && (
            <Button size="xs" variant="outline" onClick={onBulkRelease}>
              <Inbox className="size-3" />
              {t("release")}
            </Button>
          )}
        </div>
      )}

      {selectedUids.size > 0 && (
        <div className="flex flex-wrap items-center gap-1 border-b border-border px-2 py-1.5">
          <span className="mr-1 shrink-0 whitespace-nowrap text-xs text-muted-foreground">
            {t("selected", { count: selectedUids.size })}
          </span>
          {onBulkRelease && folder === spamFolder && (
            <Button size="xs" variant="outline" onClick={onBulkRelease}>
              <Inbox className="size-3" />
              {t("release")}
            </Button>
          )}
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
        <div className="relative min-h-0 flex-1">
          <VirtualList
            items={shown}
            rowHeight={rowHeight}
            scrollKey={`${folder}-${searching}`}
            onEndReached={onLoadMore}
            onPullRefresh={onRefresh}
            // IMAP UIDs are only unique per mailbox: search-all results mix
            // folders, so two different messages can share a uid. Qualify the
            // key with the source folder when the row carries one.
            getKey={(m) => (m.folder ? `${m.folder}/${m.uid}` : m.uid)}
            className={cn(
              "h-full",
              loading && messages.length > 0 && "pointer-events-none opacity-50 transition-opacity",
            )}
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
          {/* In-place refresh feedback: when rows already exist, dim them and
              float a small spinner pill instead of swapping to skeletons. */}
          {loading && messages.length > 0 && (
            <div className="pointer-events-none absolute inset-x-0 top-2 flex justify-center">
              <span className="flex items-center gap-1.5 rounded-full border border-border bg-background/90 px-2.5 py-1 text-xs text-muted-foreground shadow-sm">
                <RefreshCw className="size-3 animate-spin" />
                {t("loading")}
              </span>
            </div>
          )}
        </div>
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
