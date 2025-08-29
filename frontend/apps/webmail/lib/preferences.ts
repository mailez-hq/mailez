export type Theme = "light" | "dark" | "system";
export type Density = "compact" | "cozy" | "relaxed";

// Accent is the brand color family applied via data-accent on <html>. The
// default "blue" (stone cyan) is the default theme; "green" is the legacy
// mailez brand teal. Alternatives provide additional theme families.
export type Accent = "blue" | "green" | "purple" | "orange" | "rose";
export type ReaderFontSize = "sm" | "md" | "lg" | "xl";
export type ReadingPaneWidth = "narrow" | "md" | "wide";
// Conversation view groups the message list by thread; turning
// it off falls back to one row per message.
export type ConversationPref = boolean;
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
  conversation: boolean;
  // autoSignature appends the personal signature to new compose automatically.
  autoSignature: boolean;
  // collapseReplyQuote folds the quoted original into a "…" row in reply
  // compose; off shows the full quote expanded.
  collapseReplyQuote: boolean;
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
  conversation: true,
  autoSignature: true,
  collapseReplyQuote: true,
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
      conversation: parsed.conversation !== false,
      autoSignature: parsed.autoSignature !== false,
      collapseReplyQuote: parsed.collapseReplyQuote !== false,
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
// used by mainstream mail clients).
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

// The theme-relevant slice of preferences, mirrored into a cookie so the
// server render can paint the correct dark class / density / accent on
// <html> in the initial HTML — no bootstrap script, no theme flash.
// "system" is stored as the last RESOLVED scheme (sys:dark / sys:light):
// the server cannot query matchMedia, and refreshing the value on every
// visit keeps it in sync with OS scheme changes.
export const THEME_COOKIE = "mailez.theme";

export type ThemeCookie = {
  theme: Theme; // "light" | "dark" | "system" (system = sys:<resolved>)
  dark: boolean; // resolved scheme, what SSR actually needs
  density: Density;
  accent: Accent;
};

export function parseThemeCookie(raw: string | undefined | null): ThemeCookie | null {
  if (!raw) return null;
  const fields = new Map<string, string>();
  for (const part of raw.split("&")) {
    const eq = part.indexOf("=");
    if (eq > 0) fields.set(part.slice(0, eq), part.slice(eq + 1));
  }
  const t = fields.get("t");
  if (t !== "dark" && t !== "light" && t !== "sys:dark" && t !== "sys:light") return null;
  const d = fields.get("d");
  const density = d === "compact" || d === "relaxed" ? d : "cozy";
  const a = fields.get("a");
  const accent = a === "green" || a === "purple" || a === "orange" || a === "rose" ? a : "blue";
  return {
    theme: t === "sys:dark" || t === "sys:light" ? "system" : t,
    dark: t === "dark" || t === "sys:dark",
    density,
    accent,
  };
}

export function writeThemeCookie(prefs: Preferences) {
  try {
    const t =
      prefs.theme === "system" ? `sys:${resolveTheme("system")}` : prefs.theme;
    document.cookie = `${THEME_COOKIE}=t=${t}&d=${prefs.density}&a=${prefs.accent}; path=/; max-age=31536000; samesite=lax`;
  } catch {
    // cookie unavailable; SSR falls back to defaults for one load
  }
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
  // Keep the SSR mirror fresh so the next full load paints this exact
  // theme server-side (this is also the "system" resolved-value refresh).
  writeThemeCookie(prefs);
}
