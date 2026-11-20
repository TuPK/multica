// @vitest-environment node
import { describe, it, expect } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const stylesPath = resolve(here, "styles.css");
const indexHtmlPath = resolve(here, "../index.html");
const tokensPath = resolve(here, "../../../packages/ui/styles/tokens.css");

// Tailwind only generates classes it finds in scanned sources. The shared UI
// primitives (Dialog's centring classes, Select, Tabs, Button) live in
// packages/ui, so the console stylesheet must scan that directory.
describe("console stylesheet and real Astryx Neutral integration", () => {
  it("scans the shared UI package so its primitive classes are generated", () => {
    const css = readFileSync(stylesPath, "utf8");
    const match = css.match(/@source\s+"([^"]+)"/);
    expect(match).not.toBeNull();

    const scanned = resolve(dirname(stylesPath), match![1]!);
    expect(existsSync(resolve(scanned, "components/ui/dialog.tsx"))).toBe(true);
  });

  it("sets data-astryx-theme='neutral' on the console HTML root", () => {
    const html = readFileSync(indexHtmlPath, "utf8");
    expect(html).toMatch(/<html[^>]*data-astryx-theme="neutral"/);
  });

  it("declares @layer cascade order above @import tailwindcss and imports real Astryx stylesheets", () => {
    const css = readFileSync(stylesPath, "utf8");
    const layerIdx = css.indexOf(
      "@layer reset, theme, base, astryx-base, astryx-theme, components, utilities;",
    );
    const tailwindIdx = css.indexOf('@import "tailwindcss";');
    expect(layerIdx).toBeGreaterThanOrEqual(0);
    expect(tailwindIdx).toBeGreaterThan(layerIdx);

    expect(css).toContain('@import "@astryxdesign/core/reset.css";');
    expect(css).toContain('@import "@astryxdesign/core/astryx.css";');
    expect(css).toContain('@import "@astryxdesign/theme-neutral/theme.css";');
    expect(css).toContain('@import "@astryxdesign/charts/charts.css";');
  });

  it("bridges @multica/ui variables to live Astryx tokens and keeps packages/ui free of hand-copied Astryx tokens", () => {
    const css = readFileSync(stylesPath, "utf8");
    expect(css).toContain("--app-shell: var(--color-background-body);");
    expect(css).toContain("--surface: var(--color-background-surface);");
    expect(css).toContain("--foreground: var(--color-text-primary);");
    expect(css).toContain("--muted-foreground: var(--color-text-secondary);");

    const tokensCss = readFileSync(tokensPath, "utf8");
    expect(tokensCss).not.toContain(".astryx-neutral");
  });
});

describe("theme palettes", () => {
  it("defines blue-white, light and dark palettes against the data-theme attribute", () => {
    const css = readFileSync(stylesPath, "utf8");
    expect(css).toContain('[data-astryx-theme="neutral"]:not([data-theme="light"]):not([data-theme="dark"])');
    expect(css).toContain('[data-astryx-theme="neutral"][data-theme="light"]');
    expect(css).toContain('[data-astryx-theme="neutral"][data-theme="dark"]');
  });

  it("writes explicit values instead of light-dark(), which Lightning CSS mangles inside custom properties", () => {
    const css = readFileSync(stylesPath, "utf8");
    expect(css).not.toContain("light-dark(");
  });
});
