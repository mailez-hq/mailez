"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import {
  DEFAULT_PREFS,
  applyPreferences,
  readPreferences,
  resolveTheme,
  writePreferences,
  type Density,
  type Preferences,
  type Theme,
} from "@/lib/preferences";

type PreferencesContextValue = {
  prefs: Preferences;
  theme: Theme;
  density: Density;
  setTheme: (theme: Theme) => void;
  setDensity: (density: Density) => void;
  resolvedDark: boolean;
};

const PreferencesContext = createContext<PreferencesContextValue | null>(null);

export function PreferencesProvider({ children }: { children: React.ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(DEFAULT_PREFS);
  const [resolvedDark, setResolvedDark] = useState(false);

  useEffect(() => {
    const initial = readPreferences();
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

  const value = useMemo(
    () => ({
      prefs,
      theme: prefs.theme,
      density: prefs.density,
      setTheme,
      setDensity,
      resolvedDark,
    }),
    [prefs, setTheme, setDensity, resolvedDark],
  );

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>;
}

export function usePreferences() {
  const ctx = useContext(PreferencesContext);
  if (!ctx) throw new Error("usePreferences must be used within PreferencesProvider");
  return ctx;
}
