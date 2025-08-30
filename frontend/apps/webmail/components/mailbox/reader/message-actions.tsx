"use client";

import {
  Download,
  FileCode,
  MessageSquarePlus,
  Reply,
  ReplyAll,
  Send,
  ShieldCheck,
  Sparkles,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type { Dispatch, SetStateAction } from "react";

import { Button } from "@/components/ui/button";
import { buildFolderTree, flattenTree, folderLabel } from "@/components/mailbox/folder-tree";
import type { MailMessage } from "@/lib/api";

// MessageActions is the reply/forward/unsubscribe row under the message plus
// the raw-view/download/move-to controls. The AI summary button lives here
// too and toggles the summary panel rendered below the body.
export function MessageActions({
  showNotSpam,
  onNotSpam,
  detail,
  onUnsubscribe,
  onReply,
  onReplyAll,
  onForward,
  quickReplyOpen,
  setQuickReplyOpen,
  aiEnabled,
  aiLocked,
  summaryOpen,
  setSummaryOpen,
  summarizing,
  summary,
  onSummarize,
  onLoadRaw,
  onDownloadRaw,
  folders,
  onMoveToFolder,
}: {
  showNotSpam: boolean;
  onNotSpam: () => void;
  detail: MailMessage;
  onUnsubscribe: () => void;
  onReply: () => void;
  onReplyAll: () => void;
  onForward: () => void;
  quickReplyOpen: boolean;
  setQuickReplyOpen: Dispatch<SetStateAction<boolean>>;
  aiEnabled: boolean;
  /** Enterprise-only deployment: keep the AI button visible but locked. */
  aiLocked?: boolean;
  summaryOpen: boolean;
  setSummaryOpen: Dispatch<SetStateAction<boolean>>;
  summarizing: boolean;
  summary: string;
  onSummarize: () => void;
  onLoadRaw: () => void;
  onDownloadRaw: () => void;
  folders: string[];
  onMoveToFolder: (destination: string) => void;
}) {
  const t = useTranslations("mail");
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2">
      {showNotSpam && (
        <Button size="sm" variant="outline" onClick={onNotSpam}>
          <ShieldCheck className="size-3.5" />
          {t("notSpam")}
        </Button>
      )}
      {detail.unsubscribe_url && (
        <Button size="sm" variant="outline" onClick={onUnsubscribe} title={detail.unsubscribe_url}>
          <X className="size-3.5" />
          {t("unsubscribe")}
        </Button>
      )}
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
        variant={quickReplyOpen ? "secondary" : "outline"}
        onClick={() => setQuickReplyOpen((v) => !v)}
      >
        <MessageSquarePlus className="size-3.5" />
        {t("quickReply")}
      </Button>
      {aiEnabled ? (
        <Button
          size="sm"
          variant={summaryOpen || summarizing || summary ? "secondary" : "outline"}
          onClick={() => {
            if (summaryOpen) {
              setSummaryOpen(false);
            } else {
              setSummaryOpen(true);
              if (!summary) onSummarize();
            }
          }}
          disabled={summarizing}
        >
          <Sparkles className="size-3.5" />
          {summarizing ? t("summarizing") : summary ? t("aiSummary") : t("summarize")}
        </Button>
      ) : aiLocked ? (
        /* Community edition: the summary entry stays visible but locked so
           users can see what the enterprise edition adds. */
        <Button size="sm" variant="outline" className="opacity-60" disabled title={t("aiLockedTitle")}>
          <Sparkles className="size-3.5" />
          {t("summarize")}
        </Button>
      ) : null}
      <div className="ml-auto flex items-center gap-1">
        <Button size="sm" variant="ghost" onClick={onLoadRaw} title={t("viewRaw")}>
          <FileCode className="size-3.5" />
        </Button>
        <Button size="sm" variant="ghost" onClick={onDownloadRaw} title={t("downloadEml")}>
          <Download className="size-3.5" />
        </Button>
        <select
          value=""
          onChange={(e) => {
            if (e.target.value) onMoveToFolder(e.target.value);
          }}
          title={t("moveTo")}
          className="h-7 rounded-md border border-border bg-transparent px-1 text-xs text-muted-foreground outline-none focus-visible:border-ring"
        >
          <option value="">{t("moveTo")}…</option>
          {flattenTree(buildFolderTree(folders), (leaf) => folderLabel(t, leaf))
            .filter((o) => !/^(trash|drafts)$/i.test(o.value))
            .map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
        </select>
      </div>
    </div>
  );
}
