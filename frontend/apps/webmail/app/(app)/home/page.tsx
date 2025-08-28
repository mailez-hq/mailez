"use client";

import { useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import {
  BellRing,
  CalendarDays,
  FileText,
  HardDrive,
  Inbox,
  ListTodo,
  Megaphone,
  ShieldCheck,
  Sparkles,
  Users,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useMailStore } from "@/components/mailbox/mail-store";
import {
  calendarEvents,
  contacts,
  driveTree,
  mailAnnouncement,
  mailSnoozed,
  type CalendarEvent,
  type DriveEntry,
  type MailAnnouncement,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// Small human-readable size (mirrors the drive view).
function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function greetingKey(h: number): string {
  return h < 12 ? "greetingMorning" : h < 18 ? "greetingAfternoon" : "greetingEvening";
}

const MAX_LIST_ROWS = 5;

// One clickable overview tile: a colored icon chip, a big number and a
// destination. `tone` is a tailwind pair like "bg-sky-500/10 text-sky-600".
function StatCard({
  icon: Icon,
  label,
  value,
  caption,
  onClick,
  tone,
}: {
  icon: typeof Inbox;
  label: string;
  value: number;
  caption?: string;
  onClick: () => void;
  tone: string;
}) {
  return (
    <Card
      className="cursor-pointer transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md"
      onClick={onClick}
    >
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <span className={cn("flex size-9 items-center justify-center rounded-lg", tone)}>
          <Icon className="size-4" />
        </span>
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tracking-tight">{value}</div>
        {caption && <p className="mt-1 text-xs text-muted-foreground">{caption}</p>}
      </CardContent>
    </Card>
  );
}

function CardIcon({ icon: Icon }: { icon: typeof Inbox }) {
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
      <Icon className="size-4" />
    </span>
  );
}

// The workspace dashboard inside the shared mail shell: a gradient greeting
// banner, color-coded overview tiles and refined info cards. Identity, unread
// counts and quota come from the mounted MailStore; the rest is fetched
// locally and degrades to empty states.
export default function WorkspacePage() {
  const t = useTranslations("home");
  const locale = useLocale();
  const router = useRouter();
  const { me, unseen, setContactsOpen, setCalendarOpen, openSnoozed } = useMailStore();
  const [todoCount, setTodoCount] = useState(0);
  const [contactCount, setContactCount] = useState(0);
  const [events, setEvents] = useState<CalendarEvent[] | null>(null);
  const [files, setFiles] = useState<DriveEntry[] | null>(null);
  // "loading" | null (none active) | the published announcement.
  const [announcement, setAnnouncement] = useState<MailAnnouncement | null | "loading">("loading");
  const [logins, setLogins] = useState<RecentLogin[] | null>(null);

  useEffect(() => {
    mailSnoozed()
      .then((l) => setTodoCount(l.length))
      .catch(() => setTodoCount(0));
    contacts()
      .then((c) => setContactCount(c.length))
      .catch(() => setContactCount(0));
    mailAnnouncement()
      .then((a) => setAnnouncement(a ?? null))
      .catch(() => setAnnouncement(null));
    const now = new Date();
    const from = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const to = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
    calendarEvents(from.toISOString(), to.toISOString())
      .then(setEvents)
      .catch(() => setEvents([]));
    driveTree()
      .then((entries) => {
        const recent = entries
          .filter((e) => !e.is_dir && !e.trashed)
          .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
          .slice(0, MAX_LIST_ROWS);
        setFiles(recent);
      })
      .catch(() => setFiles([]));
  }, []);

  const name = me?.displayed_name || me?.email || "";
  const quota =
    me && typeof me.quota_bytes === "number" && me.quota_bytes > 0
      ? { used: me.quota_bytes_used ?? 0, total: me.quota_bytes }
      : null;
  const unseenMap: Record<string, number> = unseen;
  const inboxUnread = unseenMap["INBOX"] ?? unseenMap["Inbox"] ?? 0;
  const totalUnread = Object.values(unseenMap).reduce((a, b) => a + (b || 0), 0);
  const quotaPercent = quota ? Math.min(100, Math.round((quota.used / quota.total) * 100)) : 0;

  const openTodo = () => {
    openSnoozed();
    router.push("/mail/Inbox");
  };

  const initial = (name || "?").charAt(0).toUpperCase();
  const dateLine = new Date().toLocaleDateString(locale, {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  return (
    <div className="h-full overflow-y-auto bg-[radial-gradient(ellipse_at_top,rgba(46,133,85,0.07),transparent_55%)] dark:bg-[radial-gradient(ellipse_at_top,rgba(37,194,160,0.08),transparent_55%)]">
      <div className="w-full space-y-6 p-6">
        {/* Greeting banner */}
        <div className="relative overflow-hidden rounded-2xl bg-gradient-to-br from-primary/10 via-primary/5 to-background ring-1 ring-foreground/5">
          <Sparkles className="pointer-events-none absolute -right-4 -top-4 size-32 text-primary/10" />
          <div className="flex flex-wrap items-center gap-4 p-6">
            <div className="flex min-w-0 items-center gap-4">
              <span className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-primary/70 text-lg font-bold text-primary-foreground shadow-sm">
                {initial}
              </span>
              <div className="min-w-0">
                <h1 className="truncate text-2xl font-bold tracking-tight">
                  {t(greetingKey(new Date().getHours()), { name })}
                </h1>
                <p className="mt-0.5 text-sm text-muted-foreground">{dateLine}</p>
              </div>
            </div>
          </div>
        </div>

        {/* Overview tiles */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            icon={Inbox}
            label={t("unread")}
            value={inboxUnread}
            caption={t("unreadTotal", { count: totalUnread })}
            onClick={() => router.push("/mail/Inbox")}
            tone="bg-sky-500/10 text-sky-600 dark:text-sky-400"
          />
          <StatCard
            icon={ListTodo}
            label={t("todoMail")}
            value={todoCount}
            onClick={openTodo}
            tone="bg-amber-500/10 text-amber-600 dark:text-amber-400"
          />
          <StatCard
            icon={Users}
            label={t("contacts")}
            value={contactCount}
            onClick={() => setContactsOpen(true)}
            tone="bg-violet-500/10 text-violet-600 dark:text-violet-400"
          />
          <StatCard
            icon={BellRing}
            label={t("reminders")}
            value={events?.length ?? 0}
            onClick={() => setCalendarOpen(true)}
            tone="bg-rose-500/10 text-rose-600 dark:text-rose-400"
          />
        </div>

        {/* Info cards */}
        <div className="grid gap-4 lg:grid-cols-2">
          {/* Announcement */}
          <Card>
            <CardHeader className="flex flex-row items-center gap-2 pb-2">
              <CardIcon icon={Megaphone} />
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("announcements")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {announcement === "loading" ? null : announcement === null ? (
                <div className="flex items-center gap-2 py-2 text-sm text-muted-foreground">
                  <Megaphone className="size-4 opacity-40" />
                  {t("noAnnouncements")}
                </div>
              ) : (
                <div className="rounded-xl border border-primary/10 bg-primary/5 p-3">
                  <p className="flex items-center gap-2 font-medium">
                    <span className="size-1.5 rounded-full bg-primary" />
                    {announcement.subject}
                  </p>
                  {announcement.body && (
                    <p className="mt-1.5 whitespace-pre-wrap text-sm text-muted-foreground">
                      {announcement.body}
                    </p>
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Storage */}
          {quota && (
            <Card>
              <CardHeader className="flex flex-row items-center gap-2 pb-2">
                <CardIcon icon={HardDrive} />
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {t("storage")}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex items-baseline gap-1.5">
                  <span className="text-3xl font-bold tracking-tight">{quotaPercent}%</span>
                  <span className="text-xs text-muted-foreground">
                    {t("storageUsed", { used: fmtSize(quota.used), total: fmtSize(quota.total) })}
                  </span>
                </div>
                <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-muted">
                  <div
                    className={cn(
                      "h-full rounded-full bg-gradient-to-r from-primary to-primary/60 transition-all",
                      quotaPercent >= 90 && "from-destructive to-destructive/60",
                    )}
                    style={{ width: `${Math.max(quotaPercent, 2)}%` }}
                  />
                </div>
              </CardContent>
            </Card>
          )}

          {/* Today's schedule */}
          <Card>
            <CardHeader className="flex flex-row items-center gap-2 pb-2">
              <CardIcon icon={CalendarDays} />
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("todayEvents")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {events === null ? null : events.length === 0 ? (
                <div className="flex items-center gap-2 py-2 text-sm text-muted-foreground">
                  <CalendarDays className="size-4 opacity-40" />
                  {t("noEvents")}
                </div>
              ) : (
                <ul className="space-y-2">
                  {events.slice(0, MAX_LIST_ROWS).map((ev) => (
                    <li
                      key={ev.id}
                      className="flex items-center gap-3 rounded-lg px-2 py-1.5 transition-colors hover:bg-muted/60"
                    >
                      <span className="shrink-0 rounded-md bg-primary/10 px-1.5 py-0.5 font-mono text-[11px] font-medium text-primary">
                        {ev.all_day
                          ? t("allDay")
                          : new Date(ev.start).toLocaleTimeString([], {
                              hour: "2-digit",
                              minute: "2-digit",
                            })}
                      </span>
                      <span className="truncate text-sm">{ev.summary}</span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          {/* Recent files */}
          <Card>
            <CardHeader className="flex flex-row items-center gap-2 pb-2">
              <CardIcon icon={FileText} />
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {t("recentFiles")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {files === null ? null : files.length === 0 ? (
                <div className="flex items-center gap-2 py-2 text-sm text-muted-foreground">
                  <FileText className="size-4 opacity-40" />
                  {t("noFiles")}
                </div>
              ) : (
                <ul className="grid gap-x-8 gap-y-1 md:grid-cols-2">
                  {files.map((f) => (
                    <li
                      key={f.id}
                      className="flex items-center gap-3 rounded-lg px-2 py-1.5 transition-colors hover:bg-muted/60"
                    >
                      <FileText className="size-4 shrink-0 text-muted-foreground" />
                      <span className="truncate text-sm">{f.name}</span>
                      <span className="ml-auto shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                        {fmtSize(f.size)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
