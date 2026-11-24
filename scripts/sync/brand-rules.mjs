// Single source of the fork's display-name rule (docs/fork-branding.md): text people
// read says Delegora, identifiers keep multica. The merge drivers, the post-merge
// rebrand pass and `brand:check` all use these functions, so they cannot disagree.
//
// canon() maps both spellings to the upstream spelling so that a fork file and an
// upstream file can be compared and merged without brand noise. brand() is the
// reverse and applies only where the rule says display text.

/** Upstream spelling of the product name in prose. */
export const UPSTREAM_NAME = "Multica";
/** The fork's display name. */
export const FORK_NAME = "Delegora";

// A bare `Multica` is display text unless it is part of something that must keep the
// upstream spelling: an identifier or header (`X-Multica-Id`, `Multica-Workspace`,
// `MulticaFoo`, `Multica_x`), a path or domain (`/Multica`, `.Multica`), the dev app
// name `Multica Canary`, the licence name `Multica License`, or a copyright holder
// line. `@multica/core` is lowercase, so only `@Multica Bot`-style mentions reach here.
// The packaged desktop app is `Delegora.app`, so `/Multica.app` is display text too.
const DISPLAY = new RegExp(
  "(?<![-/.\\w©])(?<!© )(?<!Copyright \\d{4} )(?<!\\(c\\) )" +
    UPSTREAM_NAME +
    "(?![-\\w])(?! Canary)(?! License)" +
    "|(?<![-.\\w])" +
    UPSTREAM_NAME +
    "(?=\\.app\\b)",
  "g",
);

export const canon = (text) => text.split(FORK_NAME).join(UPSTREAM_NAME);
export const brand = (text) => text.replace(DISPLAY, FORK_NAME);

/** Surfaces a person reads; `brand:check` reports a bare `Multica` in these. */
export const DISPLAY_SURFACES = [
  /^packages\/views\/locales\/.+\.json$/,
  /^apps\/mobile\/locales\/.+\.json$/,
  /^apps\/web\/features\/landing\/i18n\/.+\.ts$/,
  /^README[^/]*\.md$/,
  /^SELF_HOSTING[^/]*\.md$/,
  /^apps\/docs\/content\/.+\.mdx?$/,
];

export const isDisplaySurface = (path) => DISPLAY_SURFACES.some((re) => re.test(path));

/** Bare `Multica` occurrences the rule says should read Delegora: [{line, text}]. */
export function bareUpstreamName(text) {
  const out = [];
  text.split("\n").forEach((line, i) => {
    if (brand(line) !== line) out.push({ line: i + 1, text: line.trim().slice(0, 160) });
  });
  return out;
}
