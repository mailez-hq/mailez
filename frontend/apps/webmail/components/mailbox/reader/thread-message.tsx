"use client";

import { useMemo } from "react";
import {
  ChevronDown,
  ChevronUp,
  Loader2,
  Reply,
  ReplyAll,
  Send,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import type { MailMessage } from "@/lib/api";
import { AttachmentCard } from "@/components/mailbox/reader/attachment-card";
import { QuoteBlock } from "@/components/mailbox/reader/quote-block";
import { useMounted } from "@/components/mailbox/reader/use-mounted";
import {
  blockRemoteImages, hasRemoteImages,
} from "@/components/mailbox/reader/remote-images";
import {
  fmtFullDate, fmtShort, getSnippet, parseBody,
} from "@/components/mailbox/reader/body";
import { foldHtmlQuotes } from "@/components/mailbox/reader/html-quotes";
import { Highlight } from "@/components/mailbox/highlight";
import { sanitizeMailHTML } from "@/lib/sanitize";
import { cn } from "@/lib/utils";
import { InvitationBanner } from "@/components/mailbox/reader/invitation-banner";

export function ThreadMessage({
  message,
  isExpanded,
  loading,
  onToggleExpand,
  onClose,
  detailsOpen,
  onToggleDetails,
  expandedQuotes,
  onToggleQuote,
  highlightTerms,
  remoteLoaded,
  onReply,
  onReplyAll,
  onForward,
  actionRowExtra,
  belowActions,
}: {
  message: MailMessage;
  isExpanded: boolean;
  loading?: boolean;
  onToggleExpand: () => void;
  onClose: () => void;
  detailsOpen: boolean;
  onToggleDetails: () => void;
  expandedQuotes: Set<number>;
  onToggleQuote: (i: number) => void;
  highlightTerms?: string[];
  remoteLoaded: boolean;
  onReply: () => void;
  onReplyAll: () => void;
  onForward: () => void;
  /** Extra controls appended after the forward button (thread-level quick
   * reply / AI summary on the newest member). */
  actionRowExtra?: React.ReactNode;
  /** Optional content rendered below the action row (the quick reply box). */
  belowActions?: React.ReactNode;
}) {
  const t = useTranslations("mail");
  const mounted = useMounted();
  const sender = message.from[0];
  const senderName = sender?.name || sender?.email || "?";

  const segments = useMemo(
    () => parseBody(message.text_body || ""),
    [message.text_body],
  );
  const remoteImages = useMemo(
    () => hasRemoteImages(message.html_body),
    [message.html_body],
  );
  const htmlBody = useMemo(() => {
    if (!message.html_body) return "";
    const clean = sanitizeMailHTML(message.html_body);
    const guarded =
      remoteImages && !remoteLoaded
        ? blockRemoteImages(clean)
        : clean;
    // Fold quoted history into <details> - the engine turns "> " lines into
    // blockquotes, which would otherwise re-render the whole thread history
    // inside every member.
    return foldHtmlQuotes(guarded, t("quotedText"));
  }, [message.html_body, remoteImages, remoteLoaded, t]);

  if (!isExpanded) {
    return (
      <button
        onClick={onToggleExpand}
        className="flex w-full items-start gap-3 border-b border-border py-3 text-left transition-colors hover:bg-muted/40 last:border-b-0"
      >
        <span
          className={cn(
            "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
            "bg-accent/60 text-accent-foreground",
          )}
        >
          {senderName.charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="truncate text-sm font-medium">{senderName}</span>
            <span className="shrink-0 text-[11px] text-muted-foreground">
              {fmtShort(message.date)}
            </span>
          </div>
          <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
            <Highlight text={getSnippet(message.text_body || "")} terms={highlightTerms} />
          </p>
          {message.attachments && message.attachments.length > 0 && (
            <div className="mt-1 flex items-center gap-1 text-[11px] text-muted-foreground">
              <span>📎</span>
              <span>
                {t("attachments", { count: message.attachments.length })}
              </span>
            </div>
          )}
        </div>
        {/* Same footprint as the expanded-state collapse button (28x28, top
            aligned) so toggling a thread message does not make the arrow
            jump horizontally or vertically. */}
        <span className="flex size-7 shrink-0 items-center justify-center">
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </span>
      </button>
    );
  }

  return (
    <div className="border-b border-border py-3 last:border-b-0">
      {/* Sender header */}
      <div className="mb-3 flex items-start gap-3">
        <span
          className={cn(
            "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
            "bg-accent text-accent-foreground",
          )}
        >
          {senderName.charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{senderName}</p>
              <button
                onClick={onToggleDetails}
                className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
              >
                {sender?.email}
                {detailsOpen ? (
                  <ChevronUp className="size-3" />
                ) : (
                  <ChevronDown className="size-3" />
                )}
              </button>
            </div>
            <span className="shrink-0 text-xs text-muted-foreground">
              {fmtFullDate(message.date)}
            </span>
          </div>
        </div>
        {/* Collapse control: up arrow (not X) so it reads as "collapse" rather
            than close/delete. Plain button on purpose: the shared Button has an
            active:translate-y-px press effect that makes the icon drift on
            click. */}
        <button
          type="button"
          onClick={onClose}
          title={t("collapseMessage")}
          aria-label={t("collapseMessage")}
          className="flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <ChevronUp className="size-3.5" />
        </button>
      </div>

      {message.invitation && message.invitation.method === "REQUEST" && (
        <InvitationBanner invitation={message.invitation} />
      )}

      {/* Expanded details */}
      {detailsOpen && (
        <div className="mb-3 space-y-0.5 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
          <p>
            <span className="font-medium text-foreground/80">From: </span>
            {sender?.name ? `${sender.name} <${sender.email}>` : sender?.email}
          </p>
          <p>
            <span className="font-medium text-foreground/80">To: </span>
            {message.to.map((a) => a.name || a.email).join(", ") || "—"}
          </p>
          {message.cc && message.cc.length > 0 && (
            <p>
              <span className="font-medium text-foreground/80">Cc: </span>
              {message.cc.map((a) => a.name || a.email).join(", ")}
            </p>
          )}
          <p>
            <span className="font-medium text-foreground/80">Date: </span>
            {fmtFullDate(message.date)}
          </p>
        </div>
      )}

      {/* Message body: HTML preferred, plain text as fallback */}
      <div className="mail-body">
        {mounted && htmlBody ? (
          <div dangerouslySetInnerHTML={{ __html: htmlBody }} />
        ) : segments.length === 0 && loading ? (
          <p className="flex items-center gap-1.5 text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("loading")}
          </p>
        ) : segments.length === 0 ? (
          <p className="text-muted-foreground">{t("noTextBody")}</p>
        ) : (
          segments.map((seg, i) =>
            seg.type === "p" ? (
              <p key={i} className="text-sm leading-6">
                {seg.lines.map((l, j) => (
                  <span key={j}>
                    <Highlight text={l} terms={highlightTerms} />
                    {j < seg.lines.length - 1 && <br />}
                  </span>
                ))}
              </p>
            ) : (
              <QuoteBlock
                key={i}
                lines={seg.lines}
                expanded={expandedQuotes.has(i)}
                onToggle={() => onToggleQuote(i)}
              />
            ),
          )
        )}
      </div>

      {/* Attachments */}
      {message.attachments && message.attachments.length > 0 && (
        <div className="mt-4 border-t border-border pt-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            {t("attachments", { count: message.attachments.length })}
          </p>
          <div className="flex flex-wrap gap-2">
            {message.attachments.map((a, i) => (
              <AttachmentCard key={i} attachment={a} />
            ))}
          </div>
        </div>
      )}

      {/* Action buttons below message */}
      <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-border pt-3">
        <Button size="sm" variant="outline" onClick={onReply}>
          <Reply className="size-3.5" />
          {t("reply")}
        </Button>
        <Button size="sm" variant="outline" onClick={onReplyAll}>
          <ReplyAll className="size-3.5" />
          {t("replyAll")}
        </Button>
        <Button size="sm" variant="outline" onClick={onForward}>
          <Send className="size-3.5" />
          {t("forward")}
        </Button>
        {actionRowExtra}
      </div>
      {belowActions}
    </div>
  );
}
