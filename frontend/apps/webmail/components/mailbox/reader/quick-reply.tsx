"use client";

import { ChevronUp, Loader2, Send, Sparkles } from "lucide-react";
import { useTranslations } from "next-intl";
import type { Dispatch, SetStateAction } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// QuickReply is the inline composer opened by the action-bar button: smart
// reply suggestions, reply/reply-all scope toggle, and the send control.
export function QuickReply({
  replies,
  setQuickReplyText,
  quickReplyAll,
  setQuickReplyAll,
  aiEnabled,
  onLoadReplies,
  repliesLoading,
  setQuickReplyOpen,
  quickReplyText,
  onSend,
  quickSending,
}: {
  replies: string[];
  setQuickReplyText: Dispatch<SetStateAction<string>>;
  quickReplyAll: boolean;
  setQuickReplyAll: Dispatch<SetStateAction<boolean>>;
  aiEnabled: boolean;
  onLoadReplies: () => void;
  repliesLoading: boolean;
  setQuickReplyOpen: Dispatch<SetStateAction<boolean>>;
  quickReplyText: string;
  onSend: () => void;
  quickSending: boolean;
}) {
  const t = useTranslations("mail");
  return (
    <div className="mt-4 rounded-lg border border-border p-3">
      {replies.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {replies.map((r, i) => (
            <button
              key={i}
              type="button"
              onClick={() => setQuickReplyText(r)}
              className="rounded-full border border-ai/30 bg-ai/10 px-2.5 py-1 text-left text-[11px] text-ai transition-colors hover:bg-ai/20"
            >
              {r}
            </button>
          ))}
        </div>
      )}
      <div className="mb-2 flex items-center gap-2">
        <p className="text-xs font-medium text-muted-foreground">{t("quickReply")}</p>
        <button
          type="button"
          onClick={() => setQuickReplyAll((v) => !v)}
          className={cn(
            "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
            quickReplyAll
              ? "border-primary bg-accent font-medium text-accent-foreground"
              : "border-border text-muted-foreground hover:bg-muted",
          )}
        >
          {quickReplyAll ? t("replyAll") : t("reply")}
        </button>
        {aiEnabled && (
          <button
            type="button"
            onClick={() => onLoadReplies()}
            disabled={repliesLoading}
            title={t("smartReply")}
            className="flex items-center gap-1 rounded-full border border-ai/20 bg-ai/10 px-2 py-0.5 text-[11px] text-ai transition-colors hover:bg-ai/20"
          >
            {repliesLoading ? <Loader2 className="size-3 animate-spin" /> : <Sparkles className="size-3" />}
            {t("smartReply")}
          </button>
        )}
        <button
          type="button"
          onClick={() => setQuickReplyOpen(false)}
          title={t("collapseQuote")}
          className="ml-auto rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
        >
          <ChevronUp className="size-3.5" />
        </button>
      </div>
      <textarea
        value={quickReplyText}
        onChange={(e) => setQuickReplyText(e.target.value)}
        placeholder={t("quickReply")}
        rows={3}
        className="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <div className="mt-2 flex justify-end">
        <Button size="sm" onClick={onSend} disabled={!quickReplyText.trim() || quickSending}>
          {quickSending ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
          {t("send")}
        </Button>
      </div>
    </div>
  );
}
