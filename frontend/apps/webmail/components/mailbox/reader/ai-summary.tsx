"use client";

import {
  ChevronDown,
  ChevronUp,
  Info,
  RotateCw,
  Sparkles,
  ThumbsDown,
  ThumbsUp,
  X,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type { Dispatch, SetStateAction } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// AiSummary is the collapsible AI summary panel toggled from the message
// action row; it also carries the regenerate and helpful/not-helpful controls.
export function AiSummary({
  setSummaryOpen,
  summarizing,
  summary,
  summaryCollapsed,
  setSummaryCollapsed,
  feedback,
  setFeedback,
  onSummarize,
}: {
  setSummaryOpen: Dispatch<SetStateAction<boolean>>;
  summarizing: boolean;
  summary: string;
  summaryCollapsed: boolean;
  setSummaryCollapsed: Dispatch<SetStateAction<boolean>>;
  feedback: "up" | "down" | null;
  setFeedback: Dispatch<SetStateAction<"up" | "down" | null>>;
  onSummarize: () => void;
}) {
  const t = useTranslations("mail");
  return (
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
                onClick={() => setSummaryOpen(false)}
                title={t("collapseQuote")}
                className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
              >
                <X className="size-3.5" />
              </button>
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
  );
}
