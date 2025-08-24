export type Theme = "light" | "dark" | "system";
export type Density = "compact" | "cozy" | "relaxed";

// Accent is the brand color family applied via data-accent on <html>. The
// default "blue" (stone cyan) is the default theme; "green" is the legacy
// mailez brand teal. Alternatives provide additional theme families.
export type Accent = "blue" | "green" | "purple" | "orange" | "rose";

// Per-feature AI toggles. A feature switch is only effective while the master
// switch is on; the backend additionally gates everything behind ai/status.
export type AiPrefs = {
  enabled: boolean;
  summary: boolean;
  draft: boolean;
  priority: boolean;
  search: boolean;
};

export type Preferences = {
  theme: Theme;
  density: Density;
  accent: Accent;
  ai: AiPrefs;
  notifications: boolean;
  undoSendSeconds: number;
};

export const PREF_KEY = "mailez.prefs";

export const DEFAULT_PREFS: Preferences = {
  theme: "system",
  density: "cozy",
  accent: "blue",
  ai: { enabled: true, summary: true, draft: true, priority: true, search: true },
  notifications: true,
  undoSendSeconds: 5,
};

export function readPreferences(): Preferences {
  if (typeof window === "undefined") return DEFAULT_PREFS;
  try {
    const raw = window.localStorage.getItem(PREF_KEY);
    if (!raw) return DEFAULT_PREFS;
    const parsed = JSON.parse(raw) as Partial<Preferences>;
    return {
      theme: parsed.theme === "light" || parsed.theme === "dark" ? parsed.theme : "system",
      density:
        parsed.density === "compact" || parsed.density === "relaxed"
          ? parsed.density
          : "cozy",
      accent: ["green", "purple", "orange", "rose"].includes(parsed.accent ?? "")
        ? (parsed.accent as Accent)
        : "blue",
      ai: {
        enabled: parsed.ai?.enabled !== false,
        summary: parsed.ai?.summary !== false,
        draft: parsed.ai?.draft !== false,
        priority: parsed.ai?.priority !== false,
        search: parsed.ai?.search !== false,
      },
      notifications: parsed.notifications !== false,
      undoSendSeconds: [0, 5, 10, 20, 30].includes(parsed.undoSendSeconds ?? 5)
        ? (parsed.undoSendSeconds ?? 5)
        : 5,
    };
  } catch {
    return DEFAULT_PREFS;
  }
}

export function writePreferences(prefs: Preferences) {
  try {
    window.localStorage.setItem(PREF_KEY, JSON.stringify(prefs));
  } catch {
    // storage unavailable (private mode etc.); theme still applies for this session
  }
}

export function resolveTheme(theme: Theme): "light" | "dark" {
  if (theme === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return theme;
}

export function applyTheme(theme: Theme) {
  const resolved = resolveTheme(theme);
  document.documentElement.classList.toggle("dark", resolved === "dark");
}

export function applyDensity(density: Density) {
  document.documentElement.dataset.density = density;
}

export function applyAccent(accent: Accent) {
  document.documentElement.dataset.accent = accent;
}

export function applyPreferences(prefs: Preferences) {
  applyTheme(prefs.theme);
  applyDensity(prefs.density);
  applyAccent(prefs.accent);
}

/** Inline script injected into <head> to apply saved theme/density/accent before paint. */
export const themeBootstrapScript = `try{var p=JSON.parse(localStorage.getItem("${PREF_KEY}")||"{}");var t=p.theme==="dark"||p.theme==="light"?p.theme:"system";var d=t==="dark"||(t==="system"&&matchMedia("(prefers-color-scheme: dark)").matches);document.documentElement.classList.toggle("dark",d);document.documentElement.dataset.density=p.density==="compact"||p.density==="relaxed"?p.density:"cozy";document.documentElement.dataset.accent=["green","purple","orange","rose"].indexOf(p.accent)>=0?p.accent:"blue"}catch(e){document.documentElement.dataset.density="cozy";document.documentElement.dataset.accent="blue"}`;
