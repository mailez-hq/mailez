"use client";

import { ChevronDown, ChevronUp } from "lucide-react";
import { useTranslations } from "next-intl";

import { cn } from "@/lib/utils";

export function QuoteBlock({
  lines,
  expanded,
  onToggle,
}: {
  lines: string[];
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useTranslations("mail");
  // Conversation model: quoted history is collapsed by default however short it is
  // - the thread above already shows every member in full, so an in-message
  // quote is redundant until the reader asks for it.
  const collapsed = !expanded;
  return (
    <div className="my-1">
      <blockquote
        className={cn(
          "relative overflow-hidden rounded-r-md border-l-2 border-muted pl-3 text-muted-foreground",
          collapsed && "max-h-20 after:pointer-events-none after:absolute after:inset-x-0 after:bottom-0 after:h-8 after:bg-gradient-to-t after:from-card",
        )}
      >
        {lines.map((l, j) => (
          <p key={j} className="text-sm">{l || <br />}</p>
        ))}
      </blockquote>
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
    </div>
  );
}
