export type Theme = "light" | "dark" | "system";
export type Density = "compact" | "cozy" | "relaxed";

// Accent is the brand color family applied via data-accent on <html>. The
// default "blue" (stone cyan) is the default theme; "green" is the legacy
// mailez brand teal. Alternatives provide additional theme families.
export type Accent = "blue" | "green" | "purple" | "orange" | "rose";
export type ReaderFontSize = "sm" | "md" | "lg" | "xl";
export type ReadingPaneWidth = "narrow" | "md" | "wide";
// Where a signed-in user lands: the workspace dashboard or straight into
// the mailbox (their last folder / inbox).
export type Landing = "home" | "inbox";

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
  // Compose spellcheck (browser built-in on the editor).
  spellcheck: boolean;
  // Reading pane typography / width presets.
  readerFont: ReaderFontSize;
  paneWidth: ReadingPaneWidth;
  landing: Landing;
};

export const PREF_KEY = "mailez.prefs";

export const DEFAULT_PREFS: Preferences = {
  theme: "system",
  density: "cozy",
  accent: "blue",
  ai: { enabled: true, summary: true, draft: true, priority: true, search: true },
  notifications: true,
  undoSendSeconds: 5,
  spellcheck: true,
  readerFont: "md",
  paneWidth: "md",
  landing: "home",
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
      spellcheck: parsed.spellcheck !== false,
      readerFont: ["sm", "lg", "xl"].includes(parsed.readerFont ?? "")
        ? (parsed.readerFont as ReaderFontSize)
        : "md",
      paneWidth: ["narrow", "wide"].includes(parsed.paneWidth ?? "")
        ? (parsed.paneWidth as ReadingPaneWidth)
        : "md",
      landing: parsed.landing === "inbox" ? "inbox" : "home",
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

// Last-visited mailbox folder, used to restore the user's position after
// login / app open (the "inbox is home, but home is where you left off" rule
// used by Gmail / Outlook / Thunderbird).
export const LAST_FOLDER_KEY = "mailez.lastFolder";

export function readLastFolder(): string | null {
  if (typeof window === "undefined") return null;
  try {
    const v = window.localStorage.getItem(LAST_FOLDER_KEY);
    if (typeof v !== "string" || v.length === 0 || v.length > 200) return null;
    return v;
  } catch {
    return null;
  }
}

export function writeLastFolder(folder: string) {
  try {
    window.localStorage.setItem(LAST_FOLDER_KEY, folder);
  } catch {
    // storage unavailable (private mode etc.); position just isn't persisted
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
