"use client";

import { useState } from "react";
import { CalendarDays, CalendarX, Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { inviteRespond } from "@/lib/api";
import type { MailInvitation } from "@/lib/api";

// InvitationBanner renders the three iTIP message kinds:
// - REQUEST: meeting details plus accept / tentative / decline (sends a REPLY);
// - REPLY:   the organizer-side RSVP notice (who answered, with which status);
// - CANCEL:  the meeting is off, with a one-tap calendar cleanup.
export function InvitationBanner({ invitation }: { invitation: MailInvitation }) {
  const t = useTranslations("mail");
  const [busy, setBusy] = useState<"" | "accept" | "tentative" | "decline" | "cancel">("");
  const [done, setDone] = useState("");
  const [error, setError] = useState("");

  async function respond(action: "accept" | "decline" | "tentative" | "cancel") {
    setBusy(action);
    setError("");
    setDone("");
    try {
      await inviteRespond(invitation.ics, action);
      setDone(
        action === "accept" ? t("inviteAccepted")
        : action === "tentative" ? t("inviteTentative")
        : action === "cancel" ? t("inviteRemoved")
        : t("inviteDeclined"),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : t("inviteFailed"));
    } finally {
      setBusy("");
    }
  }

  const fmtWhen = () => {
    if (!invitation.start) return "";
    const s = new Date(invitation.start);
    if (Number.isNaN(s.getTime())) return invitation.start;
    if (!invitation.end) return s.toLocaleString();
    const e = new Date(invitation.end);
    if (Number.isNaN(e.getTime())) return s.toLocaleString();
    return `${s.toLocaleString()} – ${e.toLocaleString()}`;
  };

  const statusLabel = (status?: string) =>
    status === "accepted" || status === "accept" ? t("inviteStatusAccepted")
    : status === "declined" || status === "decline" ? t("inviteStatusDeclined")
    : status === "tentative" ? t("inviteStatusTentative")
    : status;

  const cancelled = invitation.method === "CANCEL";
  const isReply = invitation.method === "REPLY";

  return (
    <div className="mb-3 rounded-lg border border-border bg-accent/40 p-3">
      <div className="flex flex-wrap items-center gap-2">
        {cancelled ? (
          <CalendarX className="size-4 shrink-0 text-destructive" />
        ) : (
          <CalendarDays className="size-4 shrink-0 text-primary" />
        )}
        <div className="min-w-0 flex-1">
          <p className={`text-sm font-medium ${cancelled ? "line-through" : ""}`}>
            {invitation.summary || t("inviteMeeting")}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {fmtWhen()}
            {invitation.location ? ` · ${invitation.location}` : ""}
          </p>
          {invitation.organizer && (
            <p className="truncate text-xs text-muted-foreground">
              {t("inviteOrganizer")}: {invitation.organizer}
            </p>
          )}
        </div>
      </div>
      {cancelled ? (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <p className="text-xs text-muted-foreground">{t("inviteCancelledNote")}</p>
        </div>
      ) : isReply ? (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <p className="text-xs text-muted-foreground">
            {invitation.reply_attendee
              ? t("inviteReplyFrom", { name: invitation.reply_attendee })
              : t("inviteReplyReceived")}
            {invitation.reply_status && (
              <span className="ml-1 font-medium text-foreground">{statusLabel(invitation.reply_status)}</span>
            )}
          </p>
        </div>
      ) : (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button size="sm" onClick={() => respond("accept")} disabled={!!busy || !!done}>
            {busy === "accept" ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("inviteAccept")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => respond("tentative")} disabled={!!busy || !!done}>
            {busy === "tentative" ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("inviteTentative")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => respond("decline")} disabled={!!busy || !!done} className="text-destructive hover:text-destructive">
            {busy === "decline" ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("inviteDecline")}
          </Button>
          {done && <span className="text-xs text-emerald-600">{done}</span>}
          {error && <span className="text-xs text-destructive">{error}</span>}
        </div>
      )}
      {cancelled && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => respond("cancel")} disabled={!!busy || !!done}>
            {busy === "cancel" ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t("inviteRemove")}
          </Button>
          {done && <span className="text-xs text-emerald-600">{done}</span>}
          {error && <span className="text-xs text-destructive">{error}</span>}
        </div>
      )}
    </div>
  );
}
