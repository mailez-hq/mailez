"use client";

import { ChevronDown, ChevronUp } from "lucide-react";
import type { Dispatch, SetStateAction } from "react";

import { fmtFullDate } from "@/components/mailbox/reader/body";
import type { MailMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

// MessageMeta is the sender header row of the single-message view: avatar,
// sender name and email, the date, and the chevron that toggles the expanded
// details panel (MessageDetails).
export function MessageMeta({
  detail,
  senderName,
  sender,
  detailsOpen,
  setDetailsOpen,
}: {
  detail: MailMessage;
  senderName: string;
  sender: { name: string; email: string } | undefined;
  detailsOpen: boolean;
  setDetailsOpen: Dispatch<SetStateAction<boolean>>;
}) {
  return (
    <div className="mb-4 flex items-start gap-3">
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
          <span className="shrink-0 text-xs text-muted-foreground">
            {fmtFullDate(detail.date)}
          </span>
        </div>
      </div>
    </div>
  );
}

// MessageDetails is the expanded From/To/Cc/Date panel revealed by the
// sender row's chevron.
export function MessageDetails({
  detail,
  sender,
}: {
  detail: MailMessage;
  sender: { name: string; email: string } | undefined;
}) {
  return (
    <div className="mb-4 space-y-0.5 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
      <p>
        <span className="font-medium text-foreground/80">From: </span>
        {sender?.name ? `${sender.name} <${sender.email}>` : sender?.email}
      </p>
      <p>
        <span className="font-medium text-foreground/80">To: </span>
        {detail.to.map((a) => a.name || a.email).join(", ") || "—"}
      </p>
      {detail.cc && detail.cc.length > 0 && (
        <p>
          <span className="font-medium text-foreground/80">Cc: </span>
          {detail.cc.map((a) => a.name || a.email).join(", ")}
        </p>
      )}
      <p>
        <span className="font-medium text-foreground/80">Date: </span>
        {fmtFullDate(detail.date)}
      </p>
    </div>
  );
}
