/** Line diff for the Hub update review. Pure, so the dialog only renders its result. */

export type HubDiffLine =
  | { kind: "same"; text: string }
  | { kind: "added"; text: string }
  | { kind: "removed"; text: string };

/** Above this many cells the LCS table is too big to build; the diff falls back to whole-block changes. */
const MAX_LCS_CELLS = 4_000_000;

export interface HubLineDiff {
  lines: HubDiffLine[];
  /** True when line matching was skipped, so unchanged lines show as removed then added. */
  approximate: boolean;
}

/** Splits text into lines, ignoring the newline that ends the last line. */
export function splitHubLines(text: string | null): string[] {
  if (text === null || text === "") return [];
  const lines = text.split("\n").map((line) => (line.endsWith("\r") ? line.slice(0, -1) : line));
  if (text.endsWith("\n")) lines.pop();
  return lines;
}

/**
 * Unified line diff. Common head and tail are trimmed first, so the table only
 * covers the part that changed. A null side is an added or removed file.
 */
export function diffHubLines(oldText: string | null, newText: string | null): HubLineDiff {
  const oldLines = splitHubLines(oldText);
  const newLines = splitHubLines(newText);

  let head = 0;
  while (head < oldLines.length && head < newLines.length && oldLines[head] === newLines[head]) head += 1;
  let tail = 0;
  while (
    tail < oldLines.length - head &&
    tail < newLines.length - head &&
    oldLines[oldLines.length - 1 - tail] === newLines[newLines.length - 1 - tail]
  ) {
    tail += 1;
  }

  const lines: HubDiffLine[] = oldLines.slice(0, head).map((text) => ({ kind: "same", text }));
  const oldMid = oldLines.slice(head, oldLines.length - tail);
  const newMid = newLines.slice(head, newLines.length - tail);

  let approximate = false;
  if (oldMid.length * newMid.length > MAX_LCS_CELLS) {
    approximate = true;
    for (const text of oldMid) lines.push({ kind: "removed", text });
    for (const text of newMid) lines.push({ kind: "added", text });
  } else {
    lines.push(...lcsMiddle(oldMid, newMid));
  }

  for (const text of oldLines.slice(oldLines.length - tail)) lines.push({ kind: "same", text });
  return { lines, approximate };
}

function lcsMiddle(a: string[], b: string[]): HubDiffLine[] {
  const n = a.length;
  const m = b.length;
  const width = m + 1;
  // table[i * width + j] = LCS length of a[i..] and b[j..].
  const table = new Uint32Array((n + 1) * width);
  for (let i = n - 1; i >= 0; i -= 1) {
    for (let j = m - 1; j >= 0; j -= 1) {
      table[i * width + j] =
        a[i] === b[j]
          ? table[(i + 1) * width + j + 1]! + 1
          : Math.max(table[(i + 1) * width + j]!, table[i * width + j + 1]!);
    }
  }

  const out: HubDiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: "same", text: a[i]! });
      i += 1;
      j += 1;
    } else if (table[(i + 1) * width + j]! >= table[i * width + j + 1]!) {
      out.push({ kind: "removed", text: a[i]! });
      i += 1;
    } else {
      out.push({ kind: "added", text: b[j]! });
      j += 1;
    }
  }
  while (i < n) out.push({ kind: "removed", text: a[i++]! });
  while (j < m) out.push({ kind: "added", text: b[j++]! });
  return out;
}

export type HubDiffRow =
  | { type: "line"; line: HubDiffLine }
  | { type: "gap"; count: number };

/**
 * Keeps changed lines with `context` unchanged lines on each side and folds the
 * rest into gap rows, so a one-line edit in a long file stays readable.
 */
export function collapseHubDiff(lines: HubDiffLine[], context = 3): HubDiffRow[] {
  const keep = lines.map(() => false);
  lines.forEach((line, index) => {
    if (line.kind === "same") return;
    for (let k = Math.max(0, index - context); k <= Math.min(lines.length - 1, index + context); k += 1) {
      keep[k] = true;
    }
  });

  const rows: HubDiffRow[] = [];
  let hidden = 0;
  lines.forEach((line, index) => {
    if (keep[index]) {
      if (hidden > 0) rows.push({ type: "gap", count: hidden });
      hidden = 0;
      rows.push({ type: "line", line });
    } else {
      hidden += 1;
    }
  });
  if (hidden > 0) rows.push({ type: "gap", count: hidden });
  return rows;
}
