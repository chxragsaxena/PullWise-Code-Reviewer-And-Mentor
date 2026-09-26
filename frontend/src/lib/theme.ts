export type Theme = "light" | "dark";

const STORAGE_KEY = "pullwise-theme";

/** The user's saved choice, or null to follow the system setting. */
export function getStoredTheme(): Theme | null {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    return null; // storage can be blocked (private windows, site settings)
  }
}

export function storeTheme(theme: Theme): void {
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // The choice still applies for this page view.
  }
}

export function systemTheme(): Theme {
  return window.matchMedia?.("(prefers-color-scheme: dark)")?.matches ? "dark" : "light";
}

/** Sets the theme on <html>. null removes the override so CSS follows the system. */
export function applyTheme(theme: Theme | null): void {
  if (theme) document.documentElement.dataset.theme = theme;
  else delete document.documentElement.dataset.theme;
}

/** Call once before the first render to avoid a flash of the wrong theme. */
export function initTheme(): void {
  applyTheme(getStoredTheme());
}
