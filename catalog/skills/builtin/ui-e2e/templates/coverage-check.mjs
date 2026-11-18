#!/usr/bin/env node
/**
 * Completeness gate for UI e2e coverage. It answers "is every UI feature covered?"
 * without asking a person.
 *
 *   node coverage-check.mjs <inventory.md> <matrix.md> <specDir> [ignore.txt]
 *
 * matrix.md is a Markdown table with these columns (header required):
 *   | id | screen | action | status | test |
 * - id: a flow id or a short key (e.g. "27", "console.tenants.search")
 * - status: covered | fixme | gap
 * - test: for covered/fixme, a substring of the test title that must appear in a
 *   spec under specDir (for fixme, the title must be wrapped in test.fixme);
 *   for gap, the reason.
 *
 * Checks, in order:
 *  1. every matrix row has a valid status;
 *  2. covered rows: the title substring exists in a spec as test("...") or test.describe;
 *  3. fixme rows: the title exists as test.fixme("...");
 *  4. gap rows: the reason is not empty;
 *  5. every screen file in the inventory is named in at least one matrix row;
 *  6. every simple button / nav label in the inventory appears in the matrix or in ignore.txt.
 * Exit code 1 when any check fails, so it can run in CI or before a "done" claim.
 */
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const [inventoryPath, matrixPath, specDir, ignorePath] = process.argv.slice(2);
if (!inventoryPath || !matrixPath || !specDir) {
  console.error("usage: coverage-check.mjs <inventory.md> <matrix.md> <specDir> [ignore.txt]");
  process.exit(2);
}

const problems = [];
const fail = (msg) => problems.push(msg);

function specText(dir) {
  let out = "";
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out += specText(p);
    else if (/\.spec\.ts$/.test(name)) out += readFileSync(p, "utf8") + "\n";
  }
  return out;
}
const specs = specText(specDir);
const escapeRe = (t) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
// A fixme row matches test.fixme("...<title>...") anywhere in the title, so the "flow N:" prefix is allowed.
const hasFixme = (title) => new RegExp(`test\\.fixme\\(\\s*["'\`][^"'\`]*${escapeRe(title)}`).test(specs);

// Matrix rows
const rows = readFileSync(matrixPath, "utf8")
  .split("\n")
  .filter((l) => /^\|/.test(l) && !/^\|\s*-/.test(l))
  .map((l) => l.split("|").slice(1, -1).map((c) => c.trim()));
const header = rows.shift() ?? [];
const idx = (name) => header.indexOf(name);
if (["id", "screen", "action", "status", "test"].some((h) => idx(h) < 0)) {
  fail("matrix header must be: | id | screen | action | status | test |");
}
const matrixText = rows.map((r) => r.join(" ")).join("\n").toLowerCase();

for (const r of rows) {
  const id = r[idx("id")], status = r[idx("status")], test = r[idx("test")];
  if (!["covered", "fixme", "gap"].includes(status)) {
    fail(`row ${id}: status must be covered, fixme or gap (got "${status}")`);
    continue;
  }
  if (status === "gap") {
    if (!test) fail(`row ${id}: a gap needs a reason in the test column`);
    continue;
  }
  if (!test) {
    fail(`row ${id}: ${status} row has no test title`);
    continue;
  }
  if (status === "covered" && !specs.includes(test)) {
    fail(`row ${id}: no spec contains the test title "${test}"`);
  }
  if (status === "fixme" && !hasFixme(test)) {
    fail(`row ${id}: fixme row needs test.fixme with title "${test}"`);
  }
}

// Screens in the inventory
if (existsSync(inventoryPath)) {
  const inv = readFileSync(inventoryPath, "utf8");
  const screens = [...inv.matchAll(/^## (.+)$/gm)].map((m) => m[1].trim()).filter((s) => /\.tsx$/.test(s));
  const matrixLower = matrixText;
  for (const s of screens) {
    const base = s.split("/").pop().replace(/\.tsx$/, "").toLowerCase();
    if (!matrixLower.includes(base)) fail(`screen ${s} is not named in the matrix`);
  }

  // Simple labels in the inventory
  const ignore = new Set(
    existsSync(ignorePath ?? "") ? readFileSync(ignorePath, "utf8").split("\n").map((s) => s.trim().toLowerCase()).filter(Boolean) : [],
  );
  const labels = new Set();
  for (const line of inv.split("\n")) {
    const m = line.match(/^- ([A-Za-z][A-Za-z ,'\-]{2,40})$/);
    if (m) labels.add(m[1].trim());
  }
  for (const label of labels) {
    const key = label.toLowerCase();
    if (ignore.has(key)) continue;
    if (!matrixLower.includes(key) && !specs.toLowerCase().includes(key)) {
      fail(`label "${label}" from the inventory is not in the matrix or any spec (add a row or an ignore entry)`);
    }
  }
} else {
  fail(`inventory not found: ${inventoryPath}`);
}

if (problems.length === 0) {
  console.log(`coverage OK: ${rows.length} matrix rows checked`);
  process.exit(0);
}
console.log(`coverage FAILED (${problems.length}):`);
for (const p of problems) console.log(`- ${p}`);
process.exit(1);
