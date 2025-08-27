"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { EventDialog } from "@/components/calendar/event-dialog";
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

export function CalendarView() {
  const t = useTranslations("calendar");
  const [month, setMonth] = useState(() => startOfMonth(new Date()));
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<CalendarEvent | null>(null);
  const [defaultDate, setDefaultDate] = useState<Date | undefined>(undefined);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const from = new Date(month.getFullYear(), month.getMonth(), -6).toISOString();
      const to = new Date(month.getFullYear(), month.getMonth() + 1, 7).toISOString();
      const list = await calendarEvents(from, to);
      setEvents(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, [month]);

  useEffect(() => {
    load();
  }, [load]);

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

  const monthLabel = month.toLocaleDateString(undefined, { year: "numeric", month: "long" });

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <header className="flex shrink-0 items-center justify-between gap-2 border-b px-4 py-2.5">
        <div className="flex items-center gap-1.5">
          <Button variant="ghost" size="icon-sm" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))} title={t("prevMonth")}>
            <ChevronLeft className="size-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))} title={t("nextMonth")}>
            <ChevronRight className="size-4" />
          </Button>
          <h1 className="min-w-32 text-base font-semibold">{monthLabel}</h1>
          <Button variant="outline" size="sm" onClick={() => setMonth(startOfMonth(new Date()))}>
            {t("today")}
          </Button>
        </div>
        <Button size="sm" onClick={() => openNew(new Date())}>
          <Plus className="size-4" />
          {t("newEvent")}
        </Button>
      </header>
      {error && <p className="shrink-0 px-4 py-1.5 text-xs text-destructive">{error}</p>}
      {loading ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">{t("loading")}</div>
      ) : (
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
      )}
      <EventDialog
        open={dialogOpen}
        event={editing}
        defaultDate={defaultDate}
        onOpenChange={setDialogOpen}
        onSave={save}
        onDelete={editing ? del : undefined}
      />
    </div>
  );
}

