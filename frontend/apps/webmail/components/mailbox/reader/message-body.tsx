"use client";

import { Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { RefObject } from "react";

import { Highlight } from "@/components/mailbox/highlight";
import { QuoteBlock } from "@/components/mailbox/reader/quote-block";
import { fmtFullDate } from "@/components/mailbox/reader/body";
import type { Segment } from "@/components/mailbox/reader/body";
import type { MailMessage } from "@/lib/api";

// MessageBody renders the sanitized HTML body (preferred) or the parsed
// plain-text fallback, plus the burn-after-read watermark. The parent keeps
// bodyRef here because it observes text selections on this exact element to
// offer "reply with this quote".
export function MessageBody({
  mounted,
  htmlBody,
  segments,
  detailLoading,
  fontPx,
  burnRevealed,
  detail,
  meEmail,
  highlightTerms,
  expandedQuotes,
  onToggleQuote,
  bodyRef,
}: {
  mounted: boolean;
  htmlBody: string;
  segments: Segment[];
  detailLoading: boolean;
  fontPx: number;
  burnRevealed: boolean;
  detail: MailMessage;
  meEmail: string;
  highlightTerms?: string[];
  expandedQuotes: Set<number>;
  onToggleQuote: (i: number) => void;
  bodyRef: RefObject<HTMLDivElement | null>;
}) {
  const t = useTranslations("mail");
  return (
    <div className="mail-body" ref={bodyRef}>
      {(burnRevealed || detail.flags.includes("$BurnRead")) && (
        <div className="pointer-events-none fixed bottom-3 right-3 z-40 rounded bg-black/70 px-2 py-1 text-[10px] text-white">
          {meEmail} · {fmtFullDate(new Date().toISOString())}
        </div>
      )}
      {mounted && htmlBody ? (
        <div dangerouslySetInnerHTML={{ __html: htmlBody }} style={{fontSize: fontPx}} />
      ) : segments.length === 0 && detailLoading ? (
        <p className="flex items-center gap-1.5 text-muted-foreground">
          <Loader2 className="size-3.5 animate-spin" />
          {t("loading")}
        </p>
      ) : segments.length === 0 ? (
        <p className="text-muted-foreground">{t("noTextBody")}</p>
      ) : (
        segments.map((seg, i) =>
          seg.type === "p" ? (
            <p key={i} className="leading-6" style={{fontSize: fontPx}}>
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
  );
}
