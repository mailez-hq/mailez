"use client";

import { memo } from "react";
import { Archive, Paperclip, Star, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { MailMessage } from "@/lib/api";
import type { Density } from "@/lib/preferences";
import { Highlight } from "@/components/mailbox/highlight";
import { isUserLabel, labelColor } from "@/components/mailbox/mail-utils";
import { useLabelColors } from "@/components/mailbox/mail-store";
import { cn } from "@/lib/utils";

export const ROW_HEIGHTS: Record<Density, number> = {
  compact: 36,
  cozy: 44,
  relaxed: 52,
};

// Row height with the preview line enabled. Compact density never shows the
// excerpt (minimal-style: density stays tight), cozy/relaxed grow one line.
export function rowHeightFor(density: Density, showPreview: boolean): number {
  if (!showPreview || density === "compact") return ROW_HEIGHTS[density];
  return ROW_HEIGHTS[density] + 16;
}

const AVATAR_COLORS = [
  "bg-accent text-accent-foreground",
  "bg-ai/15 text-ai",
  "bg-secondary text-secondary-foreground",
  "bg-primary/15 text-primary",
  "bg-[#E8E0D2] text-[#6B5D45] dark:bg-[#3A342A] dark:text-[#D8C9A8]",
];

function avatarColor(seed: string) {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  return AVATAR_COLORS[h % AVATAR_COLORS.length];
}

function fmtTime(d: string) {
  const date = new Date(d);
  // A message with no Date header carries the engine's zero time
  // ("0001-01-01T00:00:00Z"), which used to render as "1年1月1日" / "Jan 1,
  // 0001" in the list. Treat anything before the common era as "no date",
  // matching the reader's fmtShort/fmtFullDate guard.
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 100) return "";
  const now = new Date();
  const sameYear = date.getFullYear() === now.getFullYear();
  const sameDay = date.toDateString() === now.toDateString();
  if (sameDay) return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (sameYear) return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return date.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

// Memoized with the default shallow compare: rows take message-taking
// handlers (stable store callbacks passed straight through by the list), so
// an unchanged row skips re-rendering while the store churns elsewhere.
export const MessageRow = memo(function MessageRow({
  message,
  index,
  density,
  showPreview,
  category,
  conversation,
  selected,
  selectedInBulk,
  cursorActive,
  onOpen,
  onToggleSelect,
  onDelete,
  onArchive,
  onStar,
  highlightTerms,
  onContextMenu,
}: {
  message: MailMessage;
  index: number;
  density: Density;
  showPreview: boolean;
  category?: string;
  // Conversation rows carry thread-level aggregates (unread/starred/senders/
  // count). In flat mode each row is one individual message and must render
  // strictly its own state — showing a thread count or participant list on a
  // standalone message misrepresents what the row is.
  conversation: boolean;
  selected: boolean;
  selectedInBulk: boolean;
  cursorActive: boolean;
  onOpen: (m: MailMessage) => void;
  onToggleSelect: (m: MailMessage) => void;
  onDelete: (m: MailMessage) => void;
  onArchive: (m: MailMessage) => void;
  onStar: (m: MailMessage) => void;
  highlightTerms?: string[];
  onContextMenu?: (e: React.MouseEvent, message: MailMessage) => void;
}) {
  const t = useTranslations("mail");
  // The tiny label-colors context only: subscribing to the whole store here
  // would defeat the row memo.
  const labelColors = useLabelColors();
  // flags can arrive null for From-less/system mails (wire type says string[]
  // but older backend payloads carry null), so guard the array itself once.
  const flags = message.flags ?? [];
  // Thread aggregates only apply to conversation rows; a flat row is one
  // message and renders strictly its own flags and sender.
  const unread = conversation
    ? (message.thread_unread ?? !flags.includes("\\Seen"))
    : !flags.includes("\\Seen");
  const starred = conversation
    ? (message.thread_flagged ?? flags.includes("\\Flagged"))
    : flags.includes("\\Flagged");
  const recalled = flags.includes("$RecallSent");
  const sender =
    conversation && message.thread_senders && message.thread_senders.length > 0
      ? message.thread_senders.slice(0, 2).join(", ") +
        (message.thread_senders.length > 2 ? ` +${message.thread_senders.length - 2}` : "")
      // from is an array once the backend contract fix lands; older payloads
      // could carry null for From-less mails (e.g. system notices), so the
      // array itself needs the optional chain.
      : message.from?.[0]?.name || message.from?.[0]?.email || "?";
  const pad = density === "compact" ? "px-2" : "px-3";

  return (
    <div
      role="button"
      tabIndex={0}
      draggable
      onContextMenu={(e) => onContextMenu?.(e, message)}
      onDragStart={(e) => {
        e.dataTransfer.setData("text/plain", String(message.uid));
        e.dataTransfer.effectAllowed = "move";
      }}
      onClick={() => onOpen(message)}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Enter") onOpen(message);
      }}
      className={cn(
        "group flex h-full w-full cursor-pointer items-center gap-2 border-b border-border transition-colors",
        pad,
        selected
          ? // Selected must survive a glance: tint with the active primary at
            // a strength the light and dark backgrounds can both show (~20%
            // over white ≈ a clearly blue row; ~30% over the dark card), and
            // keep the inset primary bar as the orientation anchor. Plain
            // --accent was nearly invisible against the light list.
            "bg-primary/20 shadow-[inset_3px_0_0_0_var(--primary)] hover:bg-primary/25 dark:bg-primary/30 dark:hover:bg-primary/35"
          : selectedInBulk
            ? // Bulk-checked rows get a fainter tint of the same hue: visible
              // at a glance which rows the bulk bar will act on, without
              // competing with the opened row.
              "bg-primary/10 hover:bg-primary/15 dark:bg-primary/15 dark:hover:bg-primary/20"
            : cursorActive
              ? "bg-muted"
              : "hover:bg-muted/60",
      )}
    >
      <input
        type="checkbox"
        checked={selectedInBulk}
        onClick={(e) => e.stopPropagation()}
        onChange={() => onToggleSelect(message)}
        className="size-4 shrink-0 accent-[var(--primary)] sm:size-3.5"
        title={t("select")}
      />
      <span
        className={cn(
          "flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
          avatarColor(sender),
        )}
      >
        {sender.charAt(0).toUpperCase()}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1.5">
          {unread && <span className="size-1.5 shrink-0 rounded-full bg-primary" />}
          <span
            className={cn(
              "truncate text-[13px]",
              unread ? "font-semibold text-foreground" : "font-normal text-foreground/85",
            )}
          >
            <Highlight text={sender} terms={highlightTerms} />
          </span>
          <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
            {fmtTime(message.date)}
          </span>
        </div>
        <div className="flex min-w-0 items-center gap-1">
          <span
            className={cn(
              "truncate text-[13px]",
              unread ? "font-medium text-foreground" : "text-muted-foreground",
            )}
          >
            {message.subject ? (
              <Highlight text={message.subject} terms={highlightTerms} />
            ) : (
              t("noSubject")
            )}
          </span>
          {/* "其他" carries no signal in the list — only real categories get a pill. */}
          {category && category !== "other" && (
            <span className="shrink-0 rounded-full bg-muted px-1.5 py-px text-[10px] font-medium text-muted-foreground">
              {t.has(`category${category.charAt(0).toUpperCase()}${category.slice(1)}`)
                ? t(`category${category.charAt(0).toUpperCase()}${category.slice(1)}`)
                : category}
            </span>
          )}
          {recalled && (
            <span className="shrink-0 rounded-full bg-amber-100 px-1.5 py-px text-[10px] font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
              {t("recalled")}
            </span>
          )}
          {flags
            .filter((f) => isUserLabel(f) && f !== category)
            .slice(0, 2)
            .map((f) => {
              const color = labelColor(f, labelColors?.[f]);
              return (
                <span
                  key={f}
                  className="shrink-0 rounded-full px-1.5 py-px text-[10px] font-medium"
                  style={{ backgroundColor: `${color}1f`, color }}
                >
                  {f}
                </span>
              );
            })}
          {conversation && message.thread_count && message.thread_count > 1 && (
            <span
              title={t("threadCount", { count: message.thread_count })}
              className="shrink-0 rounded-full bg-muted px-1.5 py-px text-[10px] font-medium text-muted-foreground"
            >
              {message.thread_count}
            </span>
          )}
          {message.has_attachment && (
            <Paperclip className="size-3 shrink-0 text-muted-foreground" />
          )}
        </div>
        {/* Burn-after-read mail must never show a body excerpt in the list. */}
        {showPreview && density !== "compact" && !message.burn_after_minutes && !!message.preview && (
          <div className="truncate pt-0.5 text-xs leading-4 text-muted-foreground">
            <Highlight text={message.preview} terms={highlightTerms} />
          </div>
        )}
      </div>
      <div className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onArchive(message);
          }}
          title={t("archive")}
          className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
        >
          <Archive className="size-3.5" />
        </button>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onStar(message);
          }}
          title={starred ? t("unstar") : t("star")}
          className={cn(
            "rounded p-1 transition-colors",
            starred ? "text-[#C9A227]" : "text-muted-foreground hover:text-foreground",
          )}
        >
          <Star className={cn("size-3.5", starred && "fill-current")} />
        </button>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete(message);
          }}
          title={t("delete")}
          className="rounded p-1 text-muted-foreground transition-colors hover:text-destructive"
        >
          <Trash2 className="size-3.5" />
        </button>
      </div>
      <span className="sr-only">{index + 1}</span>
    </div>
  );
});
