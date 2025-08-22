export type Theme = "light" | "dark" | "system";
export type Density = "compact" | "cozy" | "relaxed";

export type Preferences = {
  theme: Theme;
  density: Density;
};

export const PREF_KEY = "mailez.prefs";

export const DEFAULT_PREFS: Preferences = {
  theme: "system",
  density: "cozy",
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

export function applyPreferences(prefs: Preferences) {
  applyTheme(prefs.theme);
  applyDensity(prefs.density);
}

/** Inline script injected into <head> to apply saved theme/density before paint. */
export const themeBootstrapScript = `try{var p=JSON.parse(localStorage.getItem("${PREF_KEY}")||"{}");var t=p.theme==="dark"||p.theme==="light"?p.theme:"system";var d=t==="dark"||(t==="system"&&matchMedia("(prefers-color-scheme: dark)").matches);document.documentElement.classList.toggle("dark",d);document.documentElement.dataset.density=p.density==="compact"||p.density==="relaxed"?p.density:"cozy"}catch(e){document.documentElement.dataset.density="cozy"}`;
