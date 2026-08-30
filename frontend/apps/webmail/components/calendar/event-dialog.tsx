"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { inviteSend } from "@/lib/api";
import type { CalendarEvent } from "@/lib/api";

// localInput renders a RFC3339 time as a datetime-local input value.
function localInput(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// dateInput renders a RFC3339 time as a date input value (all-day events).
function dateInput(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function EventDialog({
  open,
  event,
  defaultDate,
  onOpenChange,
  onSave,
  onDelete,
  readOnly = false,
}: {
  open: boolean;
  event: CalendarEvent | null;
  defaultDate?: Date;
  onOpenChange: (v: boolean) => void;
  onSave: (input: {
    summary: string;
    location: string;
    description: string;
    all_day: boolean;
    start: string;
    end?: string;
    rrule: string;
    reminder_minutes: number;
  }) => Promise<void>;
  onDelete?: (id: number) => Promise<void>;
  readOnly?: boolean;
}) {
  const t = useTranslations("calendar");
  const [summary, setSummary] = useState("");
  const [location, setLocation] = useState("");
  const [description, setDescription] = useState("");
  const [allDay, setAllDay] = useState(false);
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [rrule, setRrule] = useState("");
  const [reminder, setReminder] = useState(0);
  const [sendInvite, setSendInvite] = useState(false);
  const [attendees, setAttendees] = useState("");
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  // Repopulate the form whenever the dialog opens (render-phase adjustment —
  // React-recommended over an effect, catches every open path).
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setError("");
      if (event) {
        setSummary(event.summary);
        setLocation(event.location || "");
        setDescription(event.description || "");
        setAllDay(event.all_day);
        setStart(event.all_day ? dateInput(event.start) : localInput(event.start));
        setEnd(event.end ? (event.all_day ? dateInput(event.end) : localInput(event.end)) : "");
        setRrule(event.rrule || "");
        setReminder(event.reminder_minutes || 0);
        setSendInvite(false);
        setAttendees("");
      } else {
        setSummary("");
        setLocation("");
        setDescription("");
        setAllDay(false);
        const d = defaultDate || new Date();
        const pad = (n: number) => String(n).padStart(2, "0");
        setStart(`${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`);
        setEnd("");
        setRrule("");
        setReminder(0);
        setSendInvite(false);
        setAttendees("");
      }
    }
  }

  const toISO = (v: string, allDay: boolean) => {
    if (allDay) {
      return new Date(`${v}T00:00:00`).toISOString();
    }
    return new Date(v).toISOString();
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (readOnly) return;
    if (!summary.trim() || !start) {
      setError(t("errorRequired"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      await onSave({
        summary: summary.trim(),
        location: location.trim(),
        description: description.trim(),
        all_day: allDay,
        start: toISO(start, allDay),
        end: end ? toISO(end, allDay) : undefined,
        rrule,
        reminder_minutes: reminder,
      });
      if (sendInvite && attendees.trim()) {
        const to = attendees
          .split(/[,，;；\s]+/)
          .map((s) => s.trim())
          .filter(Boolean);
        if (to.length > 0) {
          await inviteSend({
            to,
            summary: summary.trim(),
            location: location.trim(),
            description: description.trim(),
            start: toISO(start, allDay),
            end: end ? toISO(end, allDay) : undefined,
          });
        }
      }
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("errorSave"));
    } finally {
      setSaving(false);
    }
  };

  const del = async () => {
    if (readOnly) return;
    if (!event) return;
    setDeleting(true);
    setError("");
    try {
      await onDelete?.(event.id);
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("errorDelete"));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{event ? t("editEvent") : t("newEvent")}</DialogTitle>
          {readOnly && <p className="text-xs text-muted-foreground">{t("readOnlyCalendar")}</p>}
        </DialogHeader>
        <form onSubmit={submit} className="grid gap-3">
          {error && <p className="text-sm text-destructive">{error}</p>}
          <div className="grid gap-1.5">
            <Label htmlFor="ev-summary">{t("summary")}</Label>
            <Input id="ev-summary" value={summary} onChange={(e) => setSummary(e.target.value)} placeholder={t("summaryPlaceholder")} readOnly={readOnly} />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t("allDay")}</Label>
            <Switch checked={allDay} onCheckedChange={setAllDay} disabled={readOnly} />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div className="grid gap-1.5">
              <Label htmlFor="ev-start">{t("start")}</Label>
              <Input
                id="ev-start"
                type={allDay ? "date" : "datetime-local"}
                value={start}
                onChange={(e) => setStart(e.target.value)}
                readOnly={readOnly}
                required
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ev-end">{t("end")}</Label>
              <Input
                id="ev-end"
                type={allDay ? "date" : "datetime-local"}
                value={end}
                onChange={(e) => setEnd(e.target.value)}
                readOnly={readOnly}
              />
            </div>
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="ev-reminder">{t("reminder")}</Label>
            <select
              id="ev-reminder"
              value={reminder}
              onChange={(e) => setReminder(Number(e.target.value))}
              disabled={readOnly}
              className="h-8 rounded-md border border-border bg-transparent px-2 text-sm outline-none focus-visible:border-ring"
            >
              <option value={0}>{t("reminderNone")}</option>
              <option value={5}>{t("reminder5")}</option>
              <option value={15}>{t("reminder15")}</option>
              <option value={30}>{t("reminder30")}</option>
              <option value={60}>{t("reminder60")}</option>
              <option value={1440}>{t("reminder1d")}</option>
            </select>
          </div>
          <div className="rounded-md border border-border p-3">
            <div className="flex items-center justify-between">
              <Label>{t("sendInvite")}</Label>
              <Switch checked={sendInvite} onCheckedChange={setSendInvite} disabled={readOnly} />
            </div>
            {sendInvite && (
              <Input
                className="mt-2"
                value={attendees}
                onChange={(e) => setAttendees(e.target.value)}
                placeholder={t("attendeesPlaceholder")}
              />
            )}
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div className="grid gap-1.5">
              <Label htmlFor="ev-location">{t("location")}</Label>
              <Input id="ev-location" value={location} onChange={(e) => setLocation(e.target.value)} placeholder={t("locationPlaceholder")} readOnly={readOnly} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ev-rrule">{t("repeat")}</Label>
              <select
                id="ev-rrule"
                value={rrule}
                onChange={(e) => setRrule(e.target.value)}
                disabled={readOnly}
                className="rounded-md border border-input bg-background px-2 py-1.5 text-sm"
              >
                <option value="">{t("repeatNone")}</option>
                <option value="FREQ=DAILY">{t("repeatDaily")}</option>
                <option value="FREQ=WEEKLY">{t("repeatWeekly")}</option>
                <option value="FREQ=MONTHLY">{t("repeatMonthly")}</option>
                <option value="FREQ=YEARLY">{t("repeatYearly")}</option>
              </select>
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ev-desc">{t("description")}</Label>
            <textarea
              id="ev-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              readOnly={readOnly}
              rows={3}
              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
            />
          </div>
          <DialogFooter className="gap-2">
            {!readOnly && event && onDelete && (
              <Button type="button" variant="ghost" className="mr-auto text-destructive" onClick={del} disabled={deleting}>
                {deleting ? t("deleting") : t("delete")}
              </Button>
            )}
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("cancel")}
            </Button>
            {!readOnly && (
              <Button type="submit" disabled={saving}>
                {saving ? t("saving") : t("save")}
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
