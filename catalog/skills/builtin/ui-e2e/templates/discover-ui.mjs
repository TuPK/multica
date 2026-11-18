#!/usr/bin/env node
/**
 * Scans a React/TSX source tree and prints a UI feature inventory as Markdown:
 * screens (files with a default or named component), navigation labels, buttons,
 * form fields (label, placeholder, id), headings, dialog titles, empty/error
 * states and toast/alert texts. Dependency-free; run it from the project root:
 *
 *   node ~/.claude/skills/ui-e2e/templates/discover-ui.mjs <srcDir> > ui-inventory.md
 *
 * The output is the input for the coverage matrix: every row must end up with a
 * test (see coverage-check.mjs) or a reasoned gap. Strings are heuristics; read
 * the source for anything the regexes miss.
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const root = process.argv[2];
if (!root) {
  console.error("usage: discover-ui.mjs <srcDir>");
  process.exit(1);
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name.startsWith(".") || name === "dist") continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(tsx|jsx)$/.test(name) && !/\.test\.|\.spec\./.test(name)) out.push(p);
  }
  return out;
}

const clean = (s) => s.replace(/\s+/g, " ").replace(/\{[^}]*\}/g, "").trim();
const uniq = (arr) => [...new Set(arr.filter(Boolean))];

const patterns = {
  button: /<Button[^>]*>([\s\S]*?)<\/Button>/g,
  heading: /<h[1-3][^>]*>([\s\S]*?)<\/h[1-3]>/g,
  cardTitle: /<CardTitle[^>]*>([\s\S]*?)<\/CardTitle>/g,
  dialogTitle: /<DialogTitle[^>]*>([\s\S]*?)<\/DialogTitle>/g,
  dialogPropTitle: /<ConfirmReasonDialog[\s\S]*?title="([^"]+)"/g,
  confirmLabel: /confirmLabel="([^"]+)"/g,
  placeholder: /placeholder="([^"]+)"/g,
  ariaLabel: /aria-label="([^"]+)"/g,
  emptyTitle: /<EmptyState[^>]*title="([^"]+)"/g,
  errorTitle: /<ErrorState[^>]*title="([^"]+)"/g,
  errorTitleAttr: /title="(Failed to[^"]+)"/g,
  alertText: /role="alert"[^>]*>\s*([^<{][^<]*)</g,
  labelFor: /<label[^>]*htmlFor="([^"]+)"[^>]*>([\s\S]*?)<\/label>/g,
  navLabel: /label:\s*"([^"]+)"/g,
  optionLabel: /<option[^>]*value="([^"]*)"[^>]*>([^<]+)<\/option>/g,
  selectId: /<select[^>]*id="([^"]+)"/g,
  inputId: /<Input[^>]*id="([^"]+)"/g,
  textareaId: /<Textarea[^>]*id="([^"]+)"/g,
};

const files = walk(root).sort();
const lines = [`# UI inventory (${relative(process.cwd(), root) || root})`, ""];
for (const file of files) {
  const src = readFileSync(file, "utf8");
  const hasComponent = /export\s+(default\s+)?function\s+[A-Z]|export\s+const\s+[A-Z]\w*\s*[:=]/.test(src);
  if (!hasComponent) continue;
  const rel = relative(root, file);
  const grab = (re, f = (x) => x) => uniq([...src.matchAll(re)].map((m) => f(m[1] ?? m[0]))).map(clean).filter(Boolean);
  const sections = [
    ["Buttons", grab(patterns.button)],
    ["Headings", [...grab(patterns.heading), ...grab(patterns.cardTitle)]],
    ["Dialogs", [...grab(patterns.dialogTitle), ...grab(patterns.dialogPropTitle), ...grab(patterns.confirmLabel)]],
    ["Form fields", [
      ...grab(patterns.labelFor, (m) => m).map((x) => x),
      ...grab(patterns.placeholder).map((p) => `placeholder: ${p}`),
      ...grab(patterns.selectId).map((i) => `select#${i}`),
      ...grab(patterns.inputId).map((i) => `input#${i}`),
      ...grab(patterns.textareaId).map((i) => `textarea#${i}`),
      ...grab(patterns.optionLabel, (m) => m).map((x) => `option: ${x}`),
    ]],
    ["States", [
      ...grab(patterns.emptyTitle).map((t) => `empty: ${t}`),
      ...grab(patterns.errorTitle).map((t) => `error: ${t}`),
      ...grab(patterns.errorTitleAttr).map((t) => `error: ${t}`),
      ...grab(patterns.alertText).map((t) => `alert: ${t}`),
    ]],
    ["Navigation labels", grab(patterns.navLabel)],
    ["Aria labels", grab(patterns.ariaLabel).map((a) => `aria: ${a}`)],
  ];
  const nonEmpty = sections.filter(([, items]) => items.length > 0);
  if (nonEmpty.length === 0) continue;
  lines.push(`## ${rel}`, "");
  for (const [title, items] of nonEmpty) {
    lines.push(`**${title}**`);
    for (const item of items) lines.push(`- ${item}`);
    lines.push("");
  }
}
console.log(lines.join("\n"));
