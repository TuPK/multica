import { vi, type Dict } from "./vi";
import { en } from "./en";

export type Lang = "vi" | "en";

export const defaultLang: Lang = "vi";

const dictionaries: Record<Lang, Dict> = { vi, en };

export function getDict(lang: Lang): Dict {
  return dictionaries[lang];
}

// Vietnamese is served at the site root, other languages under their own prefix.
export function pathFor(lang: Lang): string {
  return lang === defaultLang ? "/" : `/${lang}/`;
}
