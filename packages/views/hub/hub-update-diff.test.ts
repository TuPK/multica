import { describe, expect, it } from "vitest";
import { collapseHubDiff, diffHubLines, splitHubLines } from "./hub-update-diff";

describe("splitHubLines", () => {
  it("returns no lines for empty or missing text", () => {
    expect(splitHubLines(null)).toEqual([]);
    expect(splitHubLines("")).toEqual([]);
  });

  it("drops the newline that ends the last line and strips CRLF endings", () => {
    expect(splitHubLines("a\r\nb\n")).toEqual(["a", "b"]);
    expect(splitHubLines("a\n\nb")).toEqual(["a", "", "b"]);
  });
});

describe("diffHubLines", () => {
  it("marks every line as unchanged for identical text", () => {
    const { lines, approximate } = diffHubLines("x\ny", "x\ny");
    expect(approximate).toBe(false);
    expect(lines.every((line) => line.kind === "same")).toBe(true);
  });

  it("reports a modified line as removed then added in place", () => {
    const { lines } = diffHubLines("one\ntwo\nthree", "one\n2\nthree");
    expect(lines).toEqual([
      { kind: "same", text: "one" },
      { kind: "removed", text: "two" },
      { kind: "added", text: "2" },
      { kind: "same", text: "three" },
    ]);
  });

  it("finds inserted and deleted lines in the middle of a file", () => {
    const { lines } = diffHubLines("a\nb\nc\nd", "a\nc\nx\nd");
    expect(lines.filter((line) => line.kind === "removed").map((line) => line.text)).toEqual(["b"]);
    expect(lines.filter((line) => line.kind === "added").map((line) => line.text)).toEqual(["x"]);
  });

  it("treats a null old side as an added file", () => {
    const { lines } = diffHubLines(null, "new\nfile");
    expect(lines).toEqual([
      { kind: "added", text: "new" },
      { kind: "added", text: "file" },
    ]);
  });

  it("treats a null new side as a removed file", () => {
    const { lines } = diffHubLines("gone", null);
    expect(lines).toEqual([{ kind: "removed", text: "gone" }]);
  });

  it("falls back to whole-block changes when the table would be too large", () => {
    const old = Array.from({ length: 2500 }, (_, i) => `old ${i}`).join("\n");
    const next = Array.from({ length: 2500 }, (_, i) => `new ${i}`).join("\n");
    const { lines, approximate } = diffHubLines(old, next);
    expect(approximate).toBe(true);
    expect(lines.filter((line) => line.kind === "removed")).toHaveLength(2500);
    expect(lines.filter((line) => line.kind === "added")).toHaveLength(2500);
  });
});

describe("collapseHubDiff", () => {
  it("keeps changes with context and folds the unchanged rest", () => {
    const old = Array.from({ length: 20 }, (_, i) => `line ${i}`).join("\n");
    const next = old.replace("line 10", "changed");
    const rows = collapseHubDiff(diffHubLines(old, next).lines, 2);
    const gaps = rows.filter((row) => row.type === "gap");
    expect(gaps).toEqual([
      { type: "gap", count: 8 },
      { type: "gap", count: 7 },
    ]);
    const changed = rows.filter((row) => row.type === "line" && row.line.kind !== "same");
    expect(changed).toHaveLength(2);
  });

  it("returns no rows for an empty diff", () => {
    expect(collapseHubDiff([])).toEqual([]);
  });
});
