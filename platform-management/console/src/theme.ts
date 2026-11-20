import { useSyncExternalStore } from "react";

export const THEMES = ["blue-white", "light", "dark"] as const;
export type Theme = (typeof THEMES)[number];
export const DEFAULT_THEME: Theme = "blue-white";

const STORAGE_KEY = "platform-console.theme";

function readStoredTheme(): Theme {
  try {
    const stored = globalThis.localStorage?.getItem(STORAGE_KEY);
    if (stored === "blue-white" || stored === "light" || stored === "dark") return stored;
  } catch {
    // Storage can be blocked; fall back to the default palette.
  }
  return DEFAULT_THEME;
}

let currentTheme: Theme = DEFAULT_THEME;
const listeners = new Set<() => void>();

export function getTheme(): Theme {
  return currentTheme;
}

// The palette is a data attribute on the root element; styles.css defines the
// three palettes against it.
function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute("data-theme", theme);
}

/** Applies the stored theme before the first render so the page never flashes the default. */
export function initTheme(): void {
  currentTheme = readStoredTheme();
  applyTheme(currentTheme);
}

export function setTheme(theme: Theme): void {
  if (theme === currentTheme) return;
  currentTheme = theme;
  applyTheme(theme);
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, theme);
  } catch {
    // The choice still applies for this page view when it cannot be stored.
  }
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useTheme(): Theme {
  return useSyncExternalStore(subscribe, getTheme, getTheme);
}
