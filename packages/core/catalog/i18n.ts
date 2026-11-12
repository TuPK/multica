import { CATALOG_I18N } from "./generated/i18n";

export type CatalogKind = "squad" | "collection" | "pack" | "workflow";

export interface CatalogText {
  title: string;
  description?: string;
}

/**
 * The name and description of a catalog template for a locale. The catalog build refuses a template
 * that lacks any supported locale, so there is no fallback to another language: a missing entry is a
 * bug in the build, and this throws instead of showing the wrong language.
 */
export function catalogText(kind: CatalogKind, id: string, locale: string): CatalogText {
  const byLocale = CATALOG_I18N[`${kind}:${id}`];
  const text = byLocale?.[locale];
  if (!text) throw new Error(`no ${locale} text for ${kind} ${id}; run pnpm catalog:build`);
  return text;
}
