export const WIKI_HASH_PREFIX = "#wiki/";

const WIKI_LINK = /\[\[([^\]|\n]+?)(?:\|([^\]\n]+?))?\]\]/g;

/**
 * Wiki pages cite each other as `[[slug|Label]]` or `[[slug]]`. Markdown does
 * not know that syntax, so it would show as raw brackets. This turns each one
 * into an ordinary link to `#wiki/<slug>`, which the panel handles itself.
 */
export function wikiLinksToMarkdown(content: string): string {
  return content.replace(WIKI_LINK, (_match, target: string, label?: string) => {
    const slug = target.trim();
    const text = (label ?? slug.split("/").pop() ?? slug).trim().replace(/[[\]]/g, "");
    return `[${text}](${WIKI_HASH_PREFIX}${encodeURI(slug)})`;
  });
}

/** The slug a `#wiki/<slug>` href points at, or null for any other href. */
export function wikiSlugFromHref(href: string | null | undefined): string | null {
  if (!href || !href.startsWith(WIKI_HASH_PREFIX)) return null;
  try {
    return decodeURI(href.slice(WIKI_HASH_PREFIX.length));
  } catch {
    return null;
  }
}
