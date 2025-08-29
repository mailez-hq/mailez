"use client";

import { memo } from "react";
import { Archive, Paperclip, Star, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { MailMessage } from "@/lib/api";
import type { Density } from "@/lib/preferences";
import { Highlight } from "@/components/mailbox/highlight";
import { isUserLabel, labelColor } from "@/components/mailbox/mail-utils";
import { useMailStore } from "@/components/mailbox/mail-store";
import { cn } from "@/lib/utils";

export const ROW_HEIGHTS: Record<Density, number> = {
  compact: 36,
  cozy: 44,
  relaxed: 52,
};

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
  if (Number.isNaN(date.getTime())) return "";
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
  category,
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
  category?: string;
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
  const { labelColors } = useMailStore();
  // Conversation-view rows carry thread-level aggregates: any member unread /
  // starred flips the whole row, and the sender line lists the participants.
  const unread = message.thread_unread ?? !message.flags.includes("\\Seen");
  const starred = message.thread_flagged ?? message.flags.includes("\\Flagged");
  const recalled = message.flags.includes("$RecallSent");
  const sender =
    message.thread_senders && message.thread_senders.length > 0
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
          ? "bg-accent shadow-[inset_3px_0_0_0_var(--primary)]"
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
          {message.flags
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
          {message.thread_count && message.thread_count > 1 && (
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
