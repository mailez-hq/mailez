"use client";

import { Inbox as InboxIcon, PenLine, Search, Send, Settings, Users } from "lucide-react";
import type { PaletteAction } from "@/components/palette/command-palette";
import type { Density, Theme } from "@/lib/preferences";

// Dependencies the mailbox needs to build the global command palette actions.
export interface UsePaletteOptions {
  t: (key: string) => string;
  tp: (key: string) => string;
  ts: (key: string) => string;
  openCompose: () => void;
  focusSearch: () => void;
  selectFolder: (folder: string) => void;
  openSettings: () => void;
  openContacts: () => void;
  theme: Theme;
  setTheme: (theme: Theme) => void;
  density: Density;
  setDensity: (density: Density) => void;
}

// usePaletteActions builds the command palette's static action list. It is kept
// separate from MailView to keep the mailbox component lean; these actions only
// shell out to mailbox handlers via the options bag.
export function usePaletteActions(o: UsePaletteOptions): PaletteAction[] {
  const { t, tp, ts } = o;
  return [
    {
      id: "write",
      label: t("write"),
      icon: <PenLine className="size-4" />,
      keywords: "new compose mail",
      run: () => o.openCompose(),
    },
    {
      id: "search",
      label: t("search"),
      icon: <Search className="size-4" />,
      keywords: "find",
      run: () => o.focusSearch(),
    },
    {
      id: "inbox",
      label: t("folderInbox"),
      icon: <InboxIcon className="size-4" />,
      keywords: "inbox",
      run: () => o.selectFolder("INBOX"),
    },
    {
      id: "sent",
      label: t("folderSent"),
      icon: <Send className="size-4" />,
      keywords: "sent",
      run: () => o.selectFolder("Sent"),
    },
    {
      id: "settings",
      label: t("settings"),
      icon: <Settings className="size-4" />,
      run: () => o.openSettings(),
    },
    {
      id: "contacts",
      label: t("contacts"),
      icon: <Users className="size-4" />,
      run: () => o.openContacts(),
    },
    {
      id: "theme",
      label: tp("theme"),
      icon: <PenLine className="size-4" />,
      run: () => o.setTheme(o.theme === "dark" ? "light" : "dark"),
    },
    {
      id: "density",
      label: `${tp("density")}: ${
        { compact: ts("densityCompact"), cozy: ts("densityCozy"), relaxed: ts("densityRelaxed") }[o.density]
      }`,
      icon: <PenLine className="size-4" />,
      run: () => o.setDensity(o.density === "compact" ? "cozy" : o.density === "cozy" ? "relaxed" : "compact"),
    },
  ];
}