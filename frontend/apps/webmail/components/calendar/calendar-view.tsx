"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus, Share2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { EventDialog } from "@/components/calendar/event-dialog";
import { ShareDialog } from "@/components/calendar/share-dialog";
import {
  calendarEventCreate,
  calendarEventDelete,
  calendarEventUpdate,
  calendarEvents,
  type CalendarEvent,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const WEEKDAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];

function startOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1);
}

function addDays(d: Date, n: number): Date {
  const out = new Date(d);
  out.setDate(out.getDate() + n);
  return out;
}

// weekStart returns the Sunday of the week containing d (grids start Sunday,
// matching the month view).
function weekStart(d: Date): Date {
  const out = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  out.setDate(out.getDate() - out.getDay());
  return out;
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

// dayEvents groups events by their local calendar day (all-day events use
// their date; timed events use their local start date).
function dayEvents(events: CalendarEvent[]): Map<string, CalendarEvent[]> {
  const map = new Map<string, CalendarEvent[]>();
  for (const ev of events) {
    const d = new Date(ev.start);
    if (Number.isNaN(d.getTime())) continue;
    const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
    const list = map.get(key) || [];
    list.push(ev);
    map.set(key, list);
  }
  return map;
}

export function CalendarView({ initialEvent }: { initialEvent?: CalendarEvent | null }) {
  const t = useTranslations("calendar");
  const [month, setMonth] = useState(() => startOfMonth(new Date()));
  const [view, setView] = useState<"month" | "week">("month");
  // Week view anchor: any date; the visible week is the one containing it.
  const [weekAnchor, setWeekAnchor] = useState(() => new Date());
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<CalendarEvent | null>(null);
  const [defaultDate, setDefaultDate] = useState<Date | undefined>(undefined);
  const [shareOpen, setShareOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      let from: Date;
      let to: Date;
      if (view === "week") {
        const start = weekStart(weekAnchor);
        from = addDays(start, -1);
        to = addDays(start, 7);
      } else {
        from = new Date(month.getFullYear(), month.getMonth(), -6);
        to = new Date(month.getFullYear(), month.getMonth() + 1, 7);
      }
      const list = await calendarEvents(from.toISOString(), to.toISOString());
      setEvents(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, [month, view, weekAnchor]);

  useEffect(() => {
    load();
  }, [load]);

  // When the drawer is opened from the workspace home with a focused event
  // (clicking a row in "today's schedule"), jump to its month and pop the
  // event dialog immediately. Consumed once on mount.
  useEffect(() => {
    if (!initialEvent) return;
    const d = new Date(initialEvent.start);
    if (!Number.isNaN(d.getTime())) {
      setMonth(startOfMonth(d));
      setWeekAnchor(d);
    }
    setEditing(initialEvent);
    setDefaultDate(undefined);
    setDialogOpen(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const grid = useMemo(() => {
    const first = startOfMonth(month);
    const lead = first.getDay(); // Sunday-start grid
    const daysInMonth = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate();
    const cells: Date[] = [];
    for (let i = 0; i < lead; i++) cells.push(addDays(first, i - lead));
    for (let i = 0; i < daysInMonth; i++) cells.push(addDays(first, i));
    const tail = cells.length % 7 === 0 ? 0 : 7 - (cells.length % 7);
    const last = new Date(month.getFullYear(), month.getMonth() + 1, 0);
    for (let i = 1; i <= tail; i++) cells.push(addDays(last, i));
    return cells;
  }, [month]);

  const byDay = useMemo(() => dayEvents(events), [events]);
  const today = new Date();

  const openNew = (d?: Date) => {
    setEditing(null);
    setDefaultDate(d);
    setDialogOpen(true);
  };

  const openEdit = (ev: CalendarEvent) => {
    // Read-only shared calendars open the dialog in view mode only.
    setEditing(ev);
    setDefaultDate(undefined);
    setDialogOpen(true);
  };

  const save = async (input: Parameters<typeof calendarEventCreate>[0]) => {
    if (editing) {
      const updated = await calendarEventUpdate(editing.id, input);
      setEvents((es) => es.map((e) => (e.id === updated.id ? updated : e)));
    } else {
      const created = await calendarEventCreate(input);
      setEvents((es) => [...es, created]);
    }
  };

  const del = async (id: number) => {
    await calendarEventDelete(id);
    setEvents((es) => es.filter((e) => e.id !== id));
  };

  const weekStartDate = weekStart(weekAnchor);
  const weekEndDate = addDays(weekStartDate, 6);
  const title =
    view === "month"
      ? month.toLocaleDateString(undefined, { year: "numeric", month: "long" })
      : weekStartDate.getFullYear() === weekEndDate.getFullYear()
        ? `${weekStartDate.toLocaleDateString(undefined, { month: "short", day: "numeric" })} – ${weekEndDate.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`
        : `${weekStartDate.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })} – ${weekEndDate.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`;

  const prev = () => {
    if (view === "week") setWeekAnchor(addDays(weekStartDate, -7));
    else setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1));
  };
  const next = () => {
    if (view === "week") setWeekAnchor(addDays(weekStartDate, 7));
    else setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1));
  };
  const goToday = () => {
    if (view === "week") setWeekAnchor(new Date());
    else setMonth(startOfMonth(new Date()));
  };

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <header className="flex shrink-0 items-center justify-between gap-2 border-b px-4 py-2.5">
        <div className="flex items-center gap-1.5">
          <Button variant="ghost" size="icon-sm" onClick={prev} title={view === "week" ? t("prevWeek") : t("prevMonth")}>
            <ChevronLeft className="size-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={next} title={view === "week" ? t("nextWeek") : t("nextMonth")}>
            <ChevronRight className="size-4" />
          </Button>
          <h1 className="min-w-32 text-base font-semibold">{title}</h1>
          <Button variant="outline" size="sm" onClick={goToday}>
            {t("today")}
          </Button>
          <div className="flex rounded-lg border border-border bg-muted/40 p-0.5">
            {(["month", "week"] as const).map((v) => (
              <button
                key={v}
                type="button"
                onClick={() => setView(v)}
                className={cn(
                  "rounded-md px-2 py-1 text-xs transition-colors",
                  view === v ? "bg-background font-medium text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                )}
              >
                {v === "month" ? t("monthView") : t("weekView")}
              </button>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => setShareOpen(true)} title={t("shareCalendar")}>
            <Share2 className="size-4" />
            <span className="hidden sm:inline">{t("share")}</span>
          </Button>
          <Button size="sm" onClick={() => openNew(new Date())}>
            <Plus className="size-4" />
            {t("newEvent")}
          </Button>
        </div>
      </header>
      {error && <p className="shrink-0 px-4 py-1.5 text-xs text-destructive">{error}</p>}
      {loading ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">{t("loading")}</div>
      ) : view === "month" ? (
        <div className="grid min-h-0 flex-1 grid-cols-7 overflow-auto">
          {WEEKDAYS.map((d) => (
            <div key={d} className="border-b border-r border-border px-2 py-1.5 text-center text-[11px] font-medium text-muted-foreground uppercase last:border-r-0">
              {t(`weekday${d.charAt(0).toUpperCase()}${d.slice(1)}`)}
            </div>
          ))}
          {grid.map((d, i) => {
            const inMonth = d.getMonth() === month.getMonth();
            const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
            const dayList = byDay.get(key) || [];
            return (
              <div
                key={i}
                className={cn(
                  "group min-h-24 cursor-pointer border-b border-r border-border p-1 transition-colors hover:bg-muted/40 last:border-r-0",
                  !inMonth && "bg-muted/30",
                )}
                onClick={() => openNew(d)}
              >
                <div className="flex items-center justify-between">
                  <span
                    className={cn(
                      "flex size-6 items-center justify-center rounded-full text-xs",
                      sameDay(d, today) && "bg-primary font-semibold text-primary-foreground",
                      !inMonth && "text-muted-foreground",
                    )}
                  >
                    {d.getDate()}
                  </span>
                  <Plus className="size-3 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                </div>
                <div className="mt-1 space-y-0.5">
                  {dayList.slice(0, 3).map((ev) => (
                    <button
                      key={ev.id}
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        openEdit(ev);
                      }}
                      className={cn(
                        "block w-full truncate rounded px-1 py-0.5 text-left text-[11px] leading-4",
                        ev.all_day ? "bg-accent font-medium text-accent-foreground" : "bg-primary/10 text-primary",
                      )}
                      title={ev.summary}
                    >
                      {ev.owner_email && (
                        <span className="mr-1 rounded-sm bg-background/60 px-1 text-[9px] font-medium text-muted-foreground">
                          {ev.owner_email.split("@")[0]}
                        </span>
                      )}
                      {!ev.all_day && new Date(ev.start).getHours() !== 0 && (
                        <span className="mr-1 opacity-70">
                          {new Date(ev.start).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}
                        </span>
                      )}
                      {ev.summary || "(no title)"}
                    </button>
                  ))}
                  {dayList.length > 3 && (
                    <p className="px-1 text-[10px] text-muted-foreground">+{dayList.length - 3}</p>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-7 overflow-auto">
          {WEEKDAYS.map((d, i) => {
            const day = addDays(weekStartDate, i);
            const key = `${day.getFullYear()}-${day.getMonth()}-${day.getDate()}`;
            const dayList = byDay.get(key) || [];
            return (
              <div key={d} className="flex min-h-0 flex-col border-b border-r border-border last:border-r-0">
                <div className="shrink-0 border-b border-border px-1 py-1.5 text-center">
                  <p className="text-[11px] font-medium text-muted-foreground uppercase">
                    {t(`weekday${d.charAt(0).toUpperCase()}${d.slice(1)}`)}
                  </p>
                  <span
                    className={cn(
                      "inline-flex size-6 items-center justify-center rounded-full text-xs",
                      sameDay(day, today) && "bg-primary font-semibold text-primary-foreground",
                    )}
                  >
                    {day.getDate()}
                  </span>
                </div>
                <div
                  className="min-h-0 flex-1 cursor-pointer space-y-1 overflow-y-auto p-1"
                  onClick={() => openNew(day)}
                >
                  {dayList.map((ev) => (
                    <button
                      key={ev.id}
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        openEdit(ev);
                      }}
                      className={cn(
                        "block w-full truncate rounded px-1 py-1 text-left text-[11px] leading-4",
                        ev.all_day ? "bg-accent font-medium text-accent-foreground" : "bg-primary/10 text-primary",
                      )}
                      title={ev.summary}
                    >
                      {ev.owner_email && (
                        <span className="mr-1 rounded-sm bg-background/60 px-1 text-[9px] font-medium text-muted-foreground">
                          {ev.owner_email.split("@")[0]}
                        </span>
                      )}
                      {!ev.all_day && (
                        <span className="mr-1 opacity-70">
                          {new Date(ev.start).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}
                        </span>
                      )}
                      {ev.summary || "(no title)"}
                      {ev.location && <span className="ml-1 opacity-60">· {ev.location}</span>}
                    </button>
                  ))}
                  {dayList.length === 0 && (
                    <p className="px-1 text-[10px] text-muted-foreground">+</p>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
      <EventDialog
        open={dialogOpen}
        event={editing}
        defaultDate={defaultDate}
        onOpenChange={setDialogOpen}
        onSave={save}
        onDelete={editing ? del : undefined}
        readOnly={!!editing?.read_only}
      />
      <ShareDialog open={shareOpen} onOpenChange={setShareOpen} />
    </div>
  );
}
