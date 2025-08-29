"use client";

import { useEffect, useRef, type Dispatch, type RefObject, type SetStateAction } from "react";

import type { MailMessage } from "@/lib/api";

/** Snapshot of the mailbox state the hotkeys read (kept fresh by the store). */
export type HotkeyState = {
  messages: MailMessage[];
  /** The list exactly as rendered (pinned floated, snoozed dropped,
   * category filter applied) — hotkeys index THIS array so the highlighted
   * row and the acted-on row are always the same message. */
  shownMessages: MailMessage[];
  cursor: number;
  folder: string;
  searching: boolean;
  detail: MailMessage | null;
  composeOpen: boolean;
  settingsOpen: boolean;
  contactsOpen: boolean;
  paletteOpen: boolean;
  shortcutsOpen: boolean;
};

/** Actions the hotkeys invoke (stably mirrored by the store after each render). */
export type HotkeyApi = {
  openMessage: (m: MailMessage) => void;
  toggleSelect: (m: MailMessage) => void;
  removeMessage: (m: MailMessage) => void;
  toggleStar: (m: MailMessage) => void;
  setSeen: (m: MailMessage, value: boolean) => void;
  archiveMessage: (m: MailMessage) => void;
  spamMessage: (m: MailMessage) => void;
  openCompose: () => void;
  openThenReply: (kind: "reply" | "replyAll" | "forward") => void;
  selectFolder: (folder: string) => void;
  backToList: () => void;
};

/**
 * Global keyboard shortcuts. Reads state through snapshot refs (kept fresh by
 * the store after every render) so the listener never needs to re-subscribe.
 */
export function useMailHotkeys({
  stateRef,
  apiRef,
  searchRef,
  setPaletteOpen,
  setShortcutsOpen,
  setCursor,
}: {
  stateRef: RefObject<HotkeyState>;
  apiRef: RefObject<HotkeyApi>;
  searchRef: RefObject<HTMLInputElement | null>;
  setPaletteOpen: Dispatch<SetStateAction<boolean>>;
  setShortcutsOpen: Dispatch<SetStateAction<boolean>>;
  setCursor: Dispatch<SetStateAction<number>>;
}) {
  const pendingG = useRef(false);

  useEffect(() => {
    function isEditable(target: EventTarget | null) {
      if (!(target instanceof HTMLElement)) return false;
      const tag = target.tagName;
      return tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable;
    }

    function onKey(e: KeyboardEvent) {
      const s = stateRef.current;
      if (s.paletteOpen || s.shortcutsOpen || s.composeOpen || s.settingsOpen || s.contactsOpen) {
        return;
      }
      if (isEditable(e.target)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen(true);
        return;
      }
      if (mod) return;

      const api = apiRef.current;
      const m = s.shownMessages[s.cursor];

      if (e.shiftKey && e.key === "I") {
        if (m) api.setSeen(m, true);
        return;
      }
      if (e.shiftKey && e.key === "U") {
        if (m) api.setSeen(m, false);
        return;
      }
      if (e.key === "g") {
        pendingG.current = true;
        return;
      }
      if (pendingG.current) {
        pendingG.current = false;
        if (e.key === "i") api.selectFolder("Inbox");
        if (e.key === "s") api.selectFolder("Sent");
        return;
      }

      switch (e.key) {
        case "/":
          e.preventDefault();
          searchRef.current?.focus();
          break;
        case "?":
          setShortcutsOpen(true);
          break;
        case "n":
          api.openCompose();
          break;
        case "j":
          setCursor((c) => Math.min(c + 1, s.shownMessages.length - 1));
          break;
        case "k":
          setCursor((c) => Math.max(c - 1, 0));
          break;
        case "Enter":
        case "o":
          if (m) api.openMessage(m);
          break;
        case "x":
          if (m) api.toggleSelect(m);
          break;
        case "#":
          if (m) api.removeMessage(m);
          break;
        case "s":
          if (m) api.toggleStar(m);
          break;
        case "e":
          if (m) api.archiveMessage(m);
          break;
        case "!":
          if (m) api.spamMessage(m);
          break;
        case "r":
          api.openThenReply("reply");
          break;
        case "a":
          api.openThenReply("replyAll");
          break;
        case "f":
          api.openThenReply("forward");
          break;
        case "u":
          api.backToList();
          break;
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [stateRef, apiRef, searchRef, setPaletteOpen, setShortcutsOpen, setCursor]);
}
