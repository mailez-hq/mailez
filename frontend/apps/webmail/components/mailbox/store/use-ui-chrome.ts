import { useEffect, useRef, useState } from "react";

import type { CalendarEvent, MailMessage } from "@/lib/api";

export type CtxMenu = { x: number; y: number; message: MailMessage } | null;

/**
 * Workspace chrome: which dialog/drawer is open, the list-pane width and the
 * message context menu. Pure UI state with no mailbox semantics, so the
 * mailbox store stays focused on data and actions.
 */
export function useUiChrome() {
  // Deep links from the app sidebar (?open=settings / ?open=contacts) open
  // the matching dialog when the mailbox mounts. Seeded via lazy initializers
  // (guarded for SSR); the effect only consumes the query — no state writes.
  const isDeepLink = (name: string) =>
    typeof window !== "undefined" &&
    new URLSearchParams(window.location.search).get("open") === name;
  const [settingsOpen, setSettingsOpen] = useState(() => isDeepLink("settings"));
  // The settings section to land on when the dialog opens (e.g. "accounts"
  // from the sidebar account manager); defaults to the first section.
  const [settingsInitialSection, setSettingsInitialSection] = useState("appearance");
  const [contactsOpen, setContactsOpen] = useState(() => isDeepLink("contacts"));
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [sieveOpen, setSieveOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [calendarOpen, setCalendarOpen] = useState(false);
  // Event to focus when the calendar drawer opens (set by the workspace
  // home "today's schedule" list; consumed once by CalendarView).
  const [calendarFocus, setCalendarFocus] = useState<CalendarEvent | null>(null);
  const [driveOpen, setDriveOpen] = useState(false);
  const [listWidth, setListWidth] = useState(360);
  const [ctxMenu, setCtxMenu] = useState<CtxMenu>(null);
  const resizeRef = useRef<{ x: number; w: number } | null>(null);

  // Consume the deep-link query once after mount (no state writes here).
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("open")) {
      const url = new URL(window.location.href);
      url.searchParams.delete("open");
      window.history.replaceState(null, "", url.toString());
    }
  }, []);

  // openSettingsSection opens the settings dialog on a specific section.
  function openSettingsSection(section: string) {
    setSettingsInitialSection(section);
    setSettingsOpen(true);
  }

  function openContextMenu(e: React.MouseEvent, m: MailMessage) {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, message: m });
  }

  // ---- list width drag ----
  function onResizeStart(e: React.PointerEvent<HTMLDivElement>) {
    resizeRef.current = { x: e.clientX, w: listWidth };
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function onResizeMove(e: React.PointerEvent<HTMLDivElement>) {
    const r = resizeRef.current;
    if (!r) return;
    setListWidth(Math.min(480, Math.max(300, r.w + e.clientX - r.x)));
  }
  function onResizeEnd() {
    resizeRef.current = null;
  }

  return {
    settingsOpen,
    setSettingsOpen,
    settingsInitialSection,
    openSettingsSection,
    contactsOpen,
    setContactsOpen,
    paletteOpen,
    setPaletteOpen,
    shortcutsOpen,
    setShortcutsOpen,
    sieveOpen,
    setSieveOpen,
    sidebarOpen,
    setSidebarOpen,
    calendarOpen,
    setCalendarOpen,
    calendarFocus,
    setCalendarFocus,
    driveOpen,
    setDriveOpen,
    listWidth,
    setListWidth,
    ctxMenu,
    setCtxMenu,
    openContextMenu,
    onResizeStart,
    onResizeMove,
    onResizeEnd,
  };
}

export type UiChrome = ReturnType<typeof useUiChrome>;
