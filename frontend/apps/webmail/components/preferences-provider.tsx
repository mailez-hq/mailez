"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import {
  DEFAULT_PREFS,
  applyPreferences,
  readPreferences,
  resolveTheme,
  writePreferences,
  type Accent,
  type AiPrefs,
  type Density,
  type Landing,
  type Preferences,
  type ReaderFontSize,
  type ReadingPaneWidth,
  type Theme,
} from "@/lib/preferences";

type PreferencesContextValue = {
  prefs: Preferences;
  theme: Theme;
  density: Density;
  accent: Accent;
  spellcheck: boolean;
  readerFont: ReaderFontSize;
  paneWidth: ReadingPaneWidth;
  conversation: boolean;
  autoSignature: boolean;
  collapseReplyQuote: boolean;
  landing: Landing;
  setTheme: (theme: Theme) => void;
  setDensity: (density: Density) => void;
  setAccent: (accent: Accent) => void;
  setAi: (ai: AiPrefs) => void;
  setNotifications: (enabled: boolean) => void;
  setUndoSend: (seconds: number) => void;
  setSpellcheck: (enabled: boolean) => void;
  setReaderFont: (size: ReaderFontSize) => void;
  setPaneWidth: (width: ReadingPaneWidth) => void;
  setConversation: (enabled: boolean) => void;
  setAutoSignature: (enabled: boolean) => void;
  setCollapseReplyQuote: (enabled: boolean) => void;
  setLanding: (landing: Landing) => void;
  resolvedDark: boolean;
};

const PreferencesContext = createContext<PreferencesContextValue | null>(null);

export function PreferencesProvider({ children }: { children: React.ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(DEFAULT_PREFS);
  const [resolvedDark, setResolvedDark] = useState(false);

  useEffect(() => {
    const initial = readPreferences();
    // Hydrating persisted preferences after mount is intentional: the server
    // render must use the defaults (no storage access on the server), so this
    // cannot be a lazy initializer.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPrefs(initial);
    applyPreferences(initial);
    setResolvedDark(resolveTheme(initial.theme) === "dark");

    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      setPrefs((p) => {
        if (p.theme === "system") {
          const dark = mq.matches;
          setResolvedDark(dark);
          document.documentElement.classList.toggle("dark", dark);
        }
        return p;
      });
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const update = useCallback((next: Preferences) => {
    setPrefs(next);
    writePreferences(next);
    applyPreferences(next);
    setResolvedDark(resolveTheme(next.theme) === "dark");
  }, []);

  const setTheme = useCallback(
    (theme: Theme) => update({ ...prefs, theme }),
    [prefs, update],
  );
  const setDensity = useCallback(
    (density: Density) => update({ ...prefs, density }),
    [prefs, update],
  );
  const setAccent = useCallback(
    (accent: Accent) => update({ ...prefs, accent }),
    [prefs, update],
  );
  const setAi = useCallback(
    (ai: AiPrefs) => update({ ...prefs, ai }),
    [prefs, update],
  );
  const setNotifications = useCallback(
    (notifications: boolean) => update({ ...prefs, notifications }),
    [prefs, update],
  );
  const setUndoSend = useCallback(
    (undoSendSeconds: number) => update({ ...prefs, undoSendSeconds }),
    [prefs, update],
  );
  const setSpellcheck = useCallback(
    (spellcheck: boolean) => update({ ...prefs, spellcheck }),
    [prefs, update],
  );
  const setReaderFont = useCallback(
    (readerFont: ReaderFontSize) => update({ ...prefs, readerFont }),
    [prefs, update],
  );
  const setPaneWidth = useCallback(
    (paneWidth: ReadingPaneWidth) => update({ ...prefs, paneWidth }),
    [prefs, update],
  );
  const setConversation = useCallback(
    (conversation: boolean) => update({ ...prefs, conversation }),
    [prefs, update],
  );
  const setAutoSignature = useCallback(
    (autoSignature: boolean) => update({ ...prefs, autoSignature }),
    [prefs, update],
  );
  const setCollapseReplyQuote = useCallback(
    (collapseReplyQuote: boolean) => update({ ...prefs, collapseReplyQuote }),
    [prefs, update],
  );
  const setLanding = useCallback(
    (landing: Landing) => update({ ...prefs, landing }),
    [prefs, update],
  );

  const value = useMemo(
    () => ({
      prefs,
      theme: prefs.theme,
      density: prefs.density,
      accent: prefs.accent,
      spellcheck: prefs.spellcheck,
      readerFont: prefs.readerFont,
      paneWidth: prefs.paneWidth,
      conversation: prefs.conversation,
      autoSignature: prefs.autoSignature,
      collapseReplyQuote: prefs.collapseReplyQuote,
      landing: prefs.landing,
      setTheme,
      setDensity,
      setAccent,
      setAi,
      setNotifications,
      setUndoSend,
      setSpellcheck,
      setReaderFont,
      setPaneWidth,
      setConversation,
      setAutoSignature,
      setCollapseReplyQuote,
      setLanding,
      resolvedDark,
    }),
    [prefs, setTheme, setDensity, setAccent, setAi, setNotifications, setUndoSend, setSpellcheck, setReaderFont, setPaneWidth, setConversation, setAutoSignature, setCollapseReplyQuote, setLanding, resolvedDark],
  );

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>;
}

export function usePreferences() {
  const ctx = useContext(PreferencesContext);
  if (!ctx) throw new Error("usePreferences must be used within PreferencesProvider");
  return ctx;
}
