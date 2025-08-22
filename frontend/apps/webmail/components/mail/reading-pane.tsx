"use client";

import { useMemo, useState } from "react";
import {
  Archive,
  ArrowLeft,
  ChevronDown,
  ChevronUp,
  File,
  FileArchive,
  FileAudio,
  FileText,
  FileVideo,
  Image as ImageIcon,
  Info,
  Loader2,
  MessagesSquare,
  RotateCw,
  Reply,
  ReplyAll,
  Send,
  Sparkles,
  Star,
  ThumbsDown,
  ThumbsUp,
  Trash2,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import type { MailAttachment, MailMessage, MailThread } from "@/lib/api";
import { cn } from "@/lib/utils";

type Segment = { type: "p" | "quote"; lines: string[] };

function parseBody(text: string): Segment[] {
  const segs: Segment[] = [];
  let para: string[] | null = null;
  let quote: string[] | null = null;
  const flushPara = () => {
    if (para) {
      segs.push({ type: "p", lines: para });
      para = null;
    }
  };
  const flushQuote = () => {
    if (quote) {
      segs.push({ type: "quote", lines: quote });
      quote = null;
    }
  };
  for (const raw of text.split(/\r?\n/)) {
    const m = raw.match(/^>\s?(.*)$/);
    if (m) {
      flushPara();
      if (!quote) quote = [];
      quote.push(m[1] || "");
    } else if (raw.trim() === "") {
      flushPara();
      flushQuote();
    } else {
      flushQuote();
      if (!para) para = [];
      para.push(raw);
    }
  }
  flushPara();
  flushQuote();
  return segs;
}

function hasRemoteImages(html?: string) {
  return /<img\b[^>]*\bsrc="https?:\/\//i.test(html || "");
}

function blockRemoteImages(html: string) {
  return html.replace(
    /(<img\b[^>]*\bsrc)="(https?:\/\/[^"]+)"/gi,
    '$1="about:blank" data-remote-src="$2"',
  );
}

function fmtFullDate(d: string) {
  const date = new Date(d);
  return Number.isNaN(date.getTime())
    ? d
    : date.toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
}

function fmtShort(d: string) {
  const date = new Date(d);
  if (Number.isNaN(date.getTime())) return "";
  const now = new Date();
  if (date.toDateString() === now.toDateString())
    return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (date.getFullYear() === now.getFullYear())
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function fmtSize(n: number) {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.round(n / 1024)} KB`;
  return `${n} B`;
}

function attachmentIcon(a: MailAttachment) {
  const ct = a.content_type;
  const cls = "size-4 shrink-0";
  if (ct.startsWith("image/")) return <ImageIcon className={cls} />;
  if (ct.startsWith("audio/")) return <FileAudio className={cls} />;
  if (ct.startsWith("video/")) return <FileVideo className={cls} />;
  if (ct.includes("zip") || ct.includes("rar") || ct.includes("tar") || ct.includes("7z"))
    return <FileArchive className={cls} />;
  if (ct.includes("pdf") || ct.startsWith("text/")) return <FileText className={cls} />;
  return <File className={cls} />;
}

export function ReadingPane({
  detail,
  aiEnabled,
  summary,
  summarizing,
  onSummarize,
  onReply,
  onReplyAll,
  onForward,
  onArchive,
  onDelete,
  onStar,
  onBack,
  thread,
  threadOpen,
  threadLoading,
  onToggleThread,
  onSelectThread,
}: {
  detail: MailMessage;
  aiEnabled: boolean;
  summary: string;
  summarizing: boolean;
  onSummarize: () => void;
  onReply: () => void;
  onReplyAll: () => void;
  onForward: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onStar: () => void;
  onBack: () => void;
  thread: MailThread | null;
  threadOpen: boolean;
  threadLoading: boolean;
  onToggleThread: () => void;
  onSelectThread: (uid: number) => void;
}) {
  const t = useTranslations("mail");
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [expandedQuotes, setExpandedQuotes] = useState<Set<number>>(new Set());
  const [remoteLoaded, setRemoteLoaded] = useState(false);
  const [summaryCollapsed, setSummaryCollapsed] = useState(false);
  const [feedback, setFeedback] = useState<"up" | "down" | null>(null);

  const segments = useMemo(
    () => parseBody(detail.text_body || ""),
    [detail.text_body],
  );
  const remoteImages = useMemo(
    () => hasRemoteImages(detail.html_body),
    [detail.html_body],
  );
  const htmlBody = useMemo(() => {
    if (!detail.html_body) return "";
    return remoteImages && !remoteLoaded
      ? blockRemoteImages(detail.html_body)
      : detail.html_body;
  }, [detail.html_body, remoteImages, remoteLoaded]);

  const starred = detail.flags.includes("\\Flagged");
  const sender = detail.from[0];
  const senderName = sender?.name || sender?.email || "?";

  return (
    <main className="mail-scroll min-w-0 flex-1 overflow-y-auto bg-background">
      <article className="mx-auto max-w-3xl p-4 md:p-6">
        {/* mobile back */}
        <div className="mb-2 md:hidden">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft className="size-4" />
            {t("back")}
          </Button>
        </div>

        <h1 className="break-words text-xl font-semibold tracking-tight md:text-2xl">
          {detail.subject || t("noSubject")}
        </h1>

        <div className="mt-3 flex items-center gap-2">
          <span
            className={cn(
              "flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
              "bg-accent text-accent-foreground",
            )}
          >
            {senderName.charAt(0).toUpperCase()}
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">{senderName}</p>
            <button
              onClick={() => setDetailsOpen((v) => !v)}
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
          <span className="ml-auto shrink-0 text-xs text-muted-foreground">
            {fmtFullDate(detail.date)}
          </span>
        </div>

        {detailsOpen && (
          <div className="mt-2 space-y-0.5 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
            <p>
              <span className="font-medium text-foreground/80">To: </span>
              {detail.to.map((a) => a.name || a.email).join(", ") || "—"}
            </p>
          </div>
        )}

        <div className="mt-3 flex flex-wrap items-center gap-1">
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
          <Button
            size="sm"
            variant="ghost"
            onClick={onArchive}
            title={t("archive")}
          >
            <Archive className="size-3.5" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={onStar}
            title={starred ? t("unstar") : t("star")}
            className={cn(starred && "text-[#C9A227]")}
          >
            <Star className={cn("size-3.5", starred && "fill-current")} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={onDelete}
            className="text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="size-3.5" />
          </Button>
        </div>

        {/* conversation walker */}
        {detail.thread_count && detail.thread_count > 1 && (
          <div className="mt-4 overflow-hidden rounded-lg border border-border">
            <button
              type="button"
              onClick={onToggleThread}
              className="flex w-full items-center justify-between gap-2 px-3 py-2 text-xs font-medium transition-colors hover:bg-muted/60"
            >
              <span className="flex items-center gap-1.5">
                <MessagesSquare className="size-3.5 text-muted-foreground" />
                {t("threadCount", { count: detail.thread_count })}
              </span>
              {threadLoading ? (
                <Loader2 className="size-3 animate-spin text-muted-foreground" />
              ) : threadOpen ? (
                <ChevronUp className="size-3.5 text-muted-foreground" />
              ) : (
                <ChevronDown className="size-3.5 text-muted-foreground" />
              )}
            </button>
            {threadOpen && (
              <div className="divide-y divide-border border-t border-border">
                {thread?.messages.map((m) => (
                  <button
                    key={m.uid}
                    type="button"
                    onClick={() => onSelectThread(m.uid)}
                    className={cn(
                      "flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition-colors hover:bg-muted/60",
                      m.uid === detail.uid && "bg-accent",
                    )}
                  >
                    <span
                      className={cn(
                        "truncate",
                        m.uid === detail.uid
                          ? "font-medium text-accent-foreground"
                          : "text-foreground",
                      )}
                    >
                      {m.from[0]?.name || m.from[0]?.email || "?"}
                    </span>
                    <span className="ml-auto shrink-0 text-muted-foreground">
                      {fmtShort(m.date)}
                    </span>
                  </button>
                ))}
                {threadLoading && (
                  <p className="p-3 text-xs text-muted-foreground">{t("loading")}</p>
                )}
              </div>
            )}
          </div>
        )}

        {/* AI summary — 石青 is the AI accent per the design spec */}
        {aiEnabled && (
          <div className="mt-4 rounded-lg border border-ai/30 bg-ai/10 p-3">
            <div className="flex items-center gap-2">
              <span className="flex items-center gap-1 rounded bg-ai px-1.5 py-0.5 text-[11px] font-semibold text-ai-foreground">
                <Sparkles className="size-3" />
                AI
              </span>
              {summary ? (
                <>
                  <button
                    onClick={() => setSummaryCollapsed((v) => !v)}
                    className="rounded p-0.5 text-muted-foreground hover:text-foreground"
                    title={summaryCollapsed ? t("expandQuote") : t("collapseQuote")}
                  >
                    {summaryCollapsed ? (
                      <ChevronDown className="size-3.5" />
                    ) : (
                      <ChevronUp className="size-3.5" />
                    )}
                  </button>
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={onSummarize}
                    disabled={summarizing}
                  >
                    <RotateCw className="size-3" />
                    {t("regenerate")}
                  </Button>
                  <div className="ml-auto flex items-center gap-0.5">
                    <button
                      onClick={() => setFeedback(feedback === "up" ? null : "up")}
                      title={t("helpful")}
                      className={cn(
                        "rounded p-1 transition-colors",
                        feedback === "up"
                          ? "text-primary"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <ThumbsUp className="size-3.5" />
                    </button>
                    <button
                      onClick={() => setFeedback(feedback === "down" ? null : "down")}
                      title={t("notHelpful")}
                      className={cn(
                        "rounded p-1 transition-colors",
                        feedback === "down"
                          ? "text-destructive"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <ThumbsDown className="size-3.5" />
                    </button>
                  </div>
                </>
              ) : !summarizing ? (
                <Button size="xs" variant="outline" onClick={onSummarize}>
                  {t("summarize")}
                </Button>
              ) : (
                <span className="text-xs text-muted-foreground">{t("summarizing")}</span>
              )}
            </div>
            {summary && !summaryCollapsed && (
              <>
                <p className="mt-2 text-sm leading-6 whitespace-pre-wrap">{summary}</p>
                <p className="mt-2 flex items-center gap-1 text-[11px] text-muted-foreground">
                  <Info className="size-3" />
                  {t("aiGenerated")}
                </p>
                {feedback && (
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    {t("feedbackThanks")}
                  </p>
                )}
              </>
            )}
          </div>
        )}

        {/* remote image policy */}
        {remoteImages && !remoteLoaded && (
          <div className="mt-4 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
            <span>{t("remoteImages")}</span>
            <Button size="xs" variant="outline" onClick={() => setRemoteLoaded(true)}>
              {t("loadRemoteImages")}
            </Button>
          </div>
        )}

        <div className="mail-body mt-5">
          {segments.length === 0 && !htmlBody && (
            <p className="text-muted-foreground">{t("noTextBody")}</p>
          )}
          {segments.map((seg, i) =>
            seg.type === "p" ? (
              <p key={i}>
                {seg.lines.map((l, j) => (
                  <span key={j}>
                    {l}
                    {j < seg.lines.length - 1 && <br />}
                  </span>
                ))}
              </p>
            ) : (
              <QuoteBlock
                key={i}
                index={i}
                lines={seg.lines}
                expanded={expandedQuotes.has(i)}
                onToggle={() =>
                  setExpandedQuotes((prev) => {
                    const next = new Set(prev);
                    if (next.has(i)) next.delete(i);
                    else next.add(i);
                    return next;
                  })
                }
              />
            ),
          )}

          {htmlBody && (
            <div className="mt-6 border-t border-border pt-4">
              <p className="mb-2 text-xs text-muted-foreground">{t("htmlVersion")}</p>
              <div className="mail-body" dangerouslySetInnerHTML={{ __html: htmlBody }} />
            </div>
          )}
        </div>

        {detail.attachments && detail.attachments.length > 0 && (
          <div className="mt-6 border-t border-border pt-4">
            <p className="mb-2 text-xs font-medium text-muted-foreground">
              {t("attachments", { count: detail.attachments.length })}
            </p>
            <div className="flex flex-wrap gap-2">
              {detail.attachments.map((a, i) => (
                <AttachmentCard key={i} attachment={a} />
              ))}
            </div>
          </div>
        )}
      </article>
    </main>
  );
}

function QuoteBlock({
  index,
  lines,
  expanded,
  onToggle,
}: {
  index: number;
  lines: string[];
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useTranslations("mail");
  const collapsed = !expanded && lines.length > 3;
  return (
    <div className="my-1">
      <blockquote
        className={cn(
          "relative overflow-hidden rounded-r-md",
          collapsed && "max-h-20 after:pointer-events-none after:absolute after:inset-x-0 after:bottom-0 after:h-8 after:bg-gradient-to-t after:from-background",
        )}
      >
        {lines.map((l, j) => (
          <p key={j}>{l || <br />}</p>
        ))}
      </blockquote>
      {lines.length > 3 && (
        <button
          onClick={onToggle}
          className="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          {expanded ? (
            <>
              <ChevronUp className="size-3" />
              {t("collapseQuote")}
            </>
          ) : (
            <>
              <ChevronDown className="size-3" />
              {t("expandQuote")} · {t("quoteLines", { count: lines.length })}
            </>
          )}
        </button>
      )}
      <span className="sr-only">{index}</span>
    </div>
  );
}

function AttachmentCard({ attachment }: { attachment: MailAttachment }) {
  const isImage = attachment.content_type.startsWith("image/") && attachment.data;
  return (
    <div className="w-52 rounded-lg border border-border p-2 transition-colors hover:bg-muted/50">
      {isImage && (
        <img
          src={`data:${attachment.content_type};base64,${attachment.data}`}
          alt={attachment.filename}
          className="mb-2 max-h-28 w-full rounded-md object-cover"
        />
      )}
      <a
        href={`data:${attachment.content_type};base64,${attachment.data}`}
        download={attachment.filename}
        className="flex items-center gap-2"
      >
        {attachmentIcon(attachment)}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-xs font-medium">{attachment.filename}</span>
          <span className="block text-[11px] text-muted-foreground">
            {fmtSize(attachment.size)}
          </span>
        </span>
        <Archive className="size-3.5 shrink-0 text-muted-foreground" />
      </a>
    </div>
  );
}
