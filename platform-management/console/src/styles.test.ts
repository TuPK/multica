// @vitest-environment node
import { describe, it, expect } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const stylesPath = resolve(here, "styles.css");

// Tailwind only generates classes it finds in scanned sources. The shared UI
// primitives (Dialog's centring classes, Table's overflow wrapper) live in
// packages/ui, so the console stylesheet must scan that directory.
describe("console stylesheet", () => {
  it("scans the shared UI package so its primitive classes are generated", () => {
    const css = readFileSync(stylesPath, "utf8");
    const match = css.match(/@source\s+"([^"]+)"/);
    expect(match).not.toBeNull();

    const scanned = resolve(dirname(stylesPath), match![1]!);
    expect(existsSync(resolve(scanned, "components/ui/dialog.tsx"))).toBe(true);
  });
});
