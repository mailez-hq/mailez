"use client";

import {
  Archive,
  ArrowLeft,
  Ban,
  BellRing,
  Languages,
  Loader2,
  PenLine,
  Printer,
  Star,
  Tag,
  Trash2,
  Undo2,
  Volume2,
  VolumeX,
} from "lucide-react";
import { useTranslations } from "next-intl";
import type { Dispatch, RefObject, SetStateAction } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Highlight } from "@/components/mailbox/highlight";
import { labelColor } from "@/components/mailbox/mail-utils";
import type { MailMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

// isSentFolder reports whether a mailbox path is the Sent Items folder.
function isSentFolder(name: string): boolean {
  return ["sent", "sent items", "sentitems", "已发送"].includes(name.toLowerCase().trim());
}

// snoozePreset computes the timestamp of the next preset snooze moment:
// 6pm today, 9am tomorrow, or 9am next Monday.
function snoozePreset(mode: "today" | "tomorrow" | "nextweek") {
  const d = new Date();
  if (mode === "today") {
    d.setHours(18, 0, 0, 0);
  } else if (mode === "tomorrow") {
    d.setDate(d.getDate() + 1);
    d.setHours(9, 0, 0, 0);
  } else {
    const diff = (8 - d.getDay()) % 7 || 7;
    d.setDate(d.getDate() + diff);
    d.setHours(9, 0, 0, 0);
  }
  return d.getTime();
}

// MessageHeader is the sticky subject/actions row of the reading pane: the
// subject line, the quick actions (star, recall, archive, spam, delete,
// translate, mute, print, snooze) and the labels/tags row with its editor.
export function MessageHeader({
  detail,
  folder,
  isDraft,
  starred,
  muted,
  snoozed,
  labels,
  labelColors,
  aiEnabled,
  highlightTerms,
  translating,
  showNotSpam,
  onBack,
  onStar,
  onEditDraft,
  onRecall,
  onArchive,
  onSpam,
  onDelete,
  onTranslate,
  onToggleMute,
  onPrint,
  onSnooze,
  onToggleLabel,
  snoozeOpen,
  setSnoozeOpen,
  snoozeCustom,
  setSnoozeCustom,
  labelOpen,
  setLabelOpen,
  newLabel,
  setNewLabel,
  labelMenuRef,
}: {
  detail: MailMessage;
  folder: string;
  isDraft: boolean;
  starred: boolean;
  muted: boolean;
  snoozed: boolean;
  labels: string[];
  labelColors?: Record<string, string>;
  aiEnabled: boolean;
  highlightTerms?: string[];
  translating: boolean;
  showNotSpam: boolean;
  onBack: () => void;
  onStar: () => void;
  onEditDraft: () => void;
  onRecall: (folder: string, uid: number) => Promise<boolean>;
  onArchive: () => void;
  onSpam: () => void;
  onDelete: () => void;
  onTranslate: () => void;
  onToggleMute: () => void;
  onPrint: () => void;
  onSnooze: (untilMs: number) => void;
  onToggleLabel: (label: string) => void;
  snoozeOpen: boolean;
  setSnoozeOpen: Dispatch<SetStateAction<boolean>>;
  snoozeCustom: string;
  setSnoozeCustom: Dispatch<SetStateAction<string>>;
  labelOpen: boolean;
  setLabelOpen: Dispatch<SetStateAction<boolean>>;
  newLabel: string;
  setNewLabel: Dispatch<SetStateAction<string>>;
  labelMenuRef: RefObject<HTMLDivElement | null>;
}) {
  const t = useTranslations("mail");
  return (
    <div className="sticky top-0 z-10 border-b border-border bg-card/95 px-4 py-3 backdrop-blur-sm md:px-6">
      {/* Mobile back */}
      <div className="mb-1.5 md:hidden">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="size-4" />
          {t("back")}
        </Button>
      </div>

      <div className="flex items-start gap-2">
        <h1 className="min-w-0 flex-1 break-words text-lg font-semibold tracking-tight md:text-xl">
          {detail.subject ? (
            <Highlight text={detail.subject} terms={highlightTerms} />
          ) : (
            t("noSubject")
          )}
        </h1>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            onClick={onStar}
            title={starred ? t("unstar") : t("star")}
            className={cn("size-8", starred && "text-[#C9A227]")}
          >
            <Star className={cn("size-4", starred && "fill-current")} />
          </Button>
          {isDraft && (
            <Button
              size="sm"
              variant="outline"
              onClick={onEditDraft}
              className="h-8 gap-1.5 px-2.5 text-xs"
            >
              <PenLine className="size-3.5" />
              {t("editDraft")}
            </Button>
          )}
          {isSentFolder(folder) && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => onRecall(detail.folder || folder, detail.uid)}
              title={t("recall")}
              className="size-8 text-muted-foreground hover:text-foreground"
            >
              <Undo2 className="size-4" />
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={onArchive}
            title={t("archive")}
            className="size-8"
          >
            <Archive className="size-4" />
          </Button>
          {!showNotSpam && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onSpam}
              title={t("spam")}
              className="size-8 text-muted-foreground hover:text-destructive"
            >
              <Ban className="size-4" />
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={onDelete}
            className="size-8 text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="size-4" />
          </Button>
          {aiEnabled && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onTranslate}
              title={t("translate")}
              className="size-8 text-muted-foreground"
            >
              {translating ? <Loader2 className="size-4 animate-spin" /> : <Languages className="size-4" />}
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={onToggleMute}
            title={muted ? t("unmute") : t("mute")}
            className={cn("size-8", muted && "text-primary")}
          >
            {muted ? <VolumeX className="size-4" /> : <Volume2 className="size-4" />}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={onPrint}
            title={t("print")}
            className="size-8 text-muted-foreground"
          >
            <Printer className="size-4" />
          </Button>
          <div className="relative">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setSnoozeOpen((v) => !v)}
              title={snoozed ? t("unsnooze") : t("snooze")}
              className={cn("size-8", snoozed && "text-primary")}
            >
              <BellRing className="size-4" />
            </Button>
            {snoozeOpen && (
              <div className="absolute top-full right-0 z-30 mt-1 w-56 rounded-lg border border-border bg-popover p-2 shadow-lg">
                <div className="space-y-1">
                  <button
                    type="button"
                    className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted"
                    onClick={() => {
                      onSnooze(snoozed ? 0 : snoozePreset("today"));
                      setSnoozeOpen(false);
                    }}
                  >
                    {snoozed ? t("unsnooze") : t("snoozeLaterToday")}
                  </button>
                  {!snoozed && (
                    <>
                      <button
                        type="button"
                        className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted"
                        onClick={() => {
                          onSnooze(snoozePreset("tomorrow"));
                          setSnoozeOpen(false);
                        }}
                      >
                        {t("snoozeTomorrow")}
                      </button>
                      <button
                        type="button"
                        className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted"
                        onClick={() => {
                          onSnooze(snoozePreset("nextweek"));
                          setSnoozeOpen(false);
                        }}
                      >
                        {t("snoozeNextWeek")}
                      </button>
                      <div className="flex items-center gap-1.5 border-t border-border pt-1.5">
                        <input
                          type="datetime-local"
                          value={snoozeCustom}
                          onChange={(e) => setSnoozeCustom(e.target.value)}
                          className="h-7 min-w-0 flex-1 rounded-md border border-input bg-background px-1.5 text-xs outline-none"
                        />
                        <Button
                          size="xs"
                          disabled={!snoozeCustom}
                          onClick={() => {
                            const t0 = new Date(snoozeCustom).getTime();
                            if (t0 > Date.now()) {
                              onSnooze(t0);
                              setSnoozeCustom("");
                              setSnoozeOpen(false);
                            }
                          }}
                        >
                          {t("snooze")}
                        </Button>
                      </div>
                    </>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Labels / tags */}
      <div className="mt-1.5 flex flex-wrap items-center gap-1">
        {muted && (
          <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            {t("mutedBadge")}
          </span>
        )}
        {labels
          .filter((l) => detail.flags.includes(l))
          .map((l) => {
            const color = labelColor(l, labelColors?.[l]);
            return (
              <span
                key={l}
                className="inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium"
                style={{
                  backgroundColor: `${color}1f`,
                  borderColor: `${color}59`,
                  color,
                }}
              >
                {l}
              </span>
            );
          })}
        <div className="relative" ref={labelMenuRef}>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setLabelOpen((v) => !v)}
            title={t("labels")}
            className="h-5 px-1.5 text-[11px]"
          >
            <Tag className="size-3" />
          </Button>
          {labelOpen && (
            <div className="absolute top-full left-0 z-20 mt-1 w-52 rounded-lg border border-border bg-popover p-2 shadow-lg">
              <div className="mb-2 flex flex-wrap gap-1">
                {labels.map((l) => {
                  const has = detail.flags.includes(l);
                  return (
                    <button
                      key={l}
                      type="button"
                      onClick={() => onToggleLabel(l)}
                      className={cn(
                        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                        has
                          ? "border-transparent font-medium"
                          : "border-border text-muted-foreground hover:bg-muted",
                      )}
                      style={
                        has
                          ? {
                              backgroundColor: `${labelColor(l, labelColors?.[l])}1f`,
                              color: labelColor(l, labelColors?.[l]),
                            }
                          : undefined
                      }
                    >
                      <span
                        className="size-1.5 rounded-full"
                        style={{ backgroundColor: labelColor(l, labelColors?.[l]) }}
                      />
                      {l}
                    </button>
                  );
                })}
              </div>
              <div className="flex gap-1">
                <Input
                  value={newLabel}
                  onChange={(e) => setNewLabel(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && newLabel.trim()) {
                      onToggleLabel(newLabel.trim());
                      setNewLabel("");
                      setLabelOpen(false);
                    }
                  }}
                  placeholder={t("newLabel")}
                  className="h-7 text-xs"
                />
                <Button
                  size="xs"
                  onClick={() => {
                    if (newLabel.trim()) {
                      onToggleLabel(newLabel.trim());
                      setNewLabel("");
                      setLabelOpen(false);
                    }
                  }}
                >
                  +
                </Button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
