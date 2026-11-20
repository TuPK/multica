import { useSyncExternalStore } from "react";
import { vi } from "./vi";

export type Language = "en" | "vi";

export const LANGUAGES: readonly Language[] = ["en", "vi"];
export const DEFAULT_LANGUAGE: Language = "en";

const STORAGE_KEY = "platform-console.language";

// English text is the lookup key, so an untranslated string renders in English
// and the source keeps its readable literals.
const dictionaries: Record<Language, Record<string, string>> = {
  en: {},
  vi,
};

function readStoredLanguage(): Language {
  try {
    const stored = globalThis.localStorage?.getItem(STORAGE_KEY);
    if (stored === "en" || stored === "vi") return stored;
  } catch {
    // Storage can be blocked (private windows, disabled site data); use the default.
  }
  return DEFAULT_LANGUAGE;
}

let currentLanguage: Language = readStoredLanguage();
const listeners = new Set<() => void>();

export function getLanguage(): Language {
  return currentLanguage;
}

export function setLanguage(language: Language): void {
  if (language === currentLanguage) return;
  currentLanguage = language;
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, language);
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

/** Re-renders the caller when the language changes; returns the current language. */
export function useLanguage(): Language {
  return useSyncExternalStore(subscribe, getLanguage, getLanguage);
}

/**
 * Translates an English source string into the current language. Placeholders
 * {0}, {1}, ... in the source text are filled from the arguments, so a sentence
 * with values keeps one translatable key.
 */
export function t(text: string, ...args: unknown[]): string {
  const template = currentLanguage === "en" ? text : (dictionaries[currentLanguage][text] ?? text);
  if (args.length === 0) return template;
  return template.replace(/\{(\d+)\}/g, (match, index: string) =>
    index in args ? String(args[Number(index)]) : match,
  );
}
