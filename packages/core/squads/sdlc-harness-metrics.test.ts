// @vitest-environment node
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const script = fileURLToPath(new URL("./sdlc-skills/sdlc-harness/scripts/harness-metrics.mjs", import.meta.url));

const dirs: string[] = [];
afterEach(() => {
  for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

interface Fixture {
  id: string;
  status: string;
  runs?: { status: string }[];
  usage?: { total_input_tokens: number; total_output_tokens: number };
  moves?: { from?: string; to?: string; at: string }[];
  comments?: string[];
}

/** Writes the files `--from-dir` reads, in the shape the CLI prints. */
function dirOf(issues: Fixture[]) {
  const dir = mkdtempSync(join(tmpdir(), "harness-metrics-"));
  dirs.push(dir);
  mkdirSync(dir, { recursive: true });
  for (const i of issues) {
    const w = (suffix: string, value: unknown) => writeFileSync(join(dir, `${i.id}.${suffix}.json`), JSON.stringify(value));
    w("issue", { status: i.status });
    w("runs", i.runs ?? []);
    if (i.usage) w("usage", i.usage);
    w(
      "timeline",
      (i.moves ?? []).map((m) => ({ type: "activity", action: "status_changed", created_at: m.at, details: { from: m.from, to: m.to } })),
    );
    w("comments", (i.comments ?? []).map((content) => ({ content })));
  }
  return dir;
}

function metrics(dir: string, ...extra: string[]) {
  const res = spawnSync("node", [script, "--from-dir", dir, ...extra], { encoding: "utf8" });
  return { code: res.status, out: res.stdout, err: res.stderr };
}

const day = (h: number) => `2026-11-12T${String(h).padStart(2, "0")}:00:00Z`;

const ISSUES: Fixture[] = [
  {
    id: "A",
    status: "done",
    runs: [{ status: "completed" }, { status: "failed" }],
    usage: { total_input_tokens: 1000, total_output_tokens: 500 },
    moves: [
      { from: "todo", to: "in_progress", at: day(10) },
      { from: "in_progress", to: "in_review", at: day(12) },
      { from: "in_review", to: "done", at: day(14) },
    ],
  },
  {
    id: "B",
    status: "done",
    runs: [{ status: "completed" }, { status: "completed" }, { status: "completed" }],
    usage: { total_input_tokens: 2000, total_output_tokens: 1000 },
    moves: [
      { from: "todo", to: "in_progress", at: day(10) },
      { from: "in_progress", to: "in_review", at: day(12) },
      { from: "in_review", to: "in_progress", at: day(14) },
      { from: "in_progress", to: "done", at: day(20) },
    ],
    comments: ["BLOCKED\nBlocker-class: environment\nSymptom: `pnpm env:ensure` exited 1 (Ensure command)"],
  },
  { id: "C", status: "in_progress", comments: ["BLOCKED\nBlocker-class: tooling\nSymptom: check-harness H3: path does not exist: scripts/dev.sh"] },
  { id: "D", status: "done", runs: [{ status: "completed" }], moves: [{ at: day(9) }] },
];

describe("harness-metrics", () => {
  it("computes M2 to M7 from the platform's run, usage, timeline and comment data", () => {
    const { code, out } = metrics(dirOf(ISSUES));
    expect(code).toBe(0);
    const m = JSON.parse(out);
    expect(m.issues).toBe(4);
    expect(m.delivered).toBe(3);
    expect(m.M2_firstPassRate).toBeCloseTo(2 / 3, 3);
    expect(m.M3_runsPerDelivered).toEqual({ n: 3, median: 2, p90: 3 });
    expect(m.M3_failedRuns).toBe(1);
    expect(m.M4_tokensPerDelivered).toEqual({ n: 3, median: 1500, p90: 3000 });
    expect(m.M5_blocksByClass).toEqual({ environment: 1, tooling: 1 });
    expect(m.M5_blocksPerIssue).toBe(0.5);
    expect(m.M6_cycleHours).toEqual({ n: 2, median: 4, p90: 10 });
    expect(m.M7_harnessBlocks).toBe(2);
  });

  it("says which numbers are approximate instead of hiding it", () => {
    const m = JSON.parse(metrics(dirOf(ISSUES)).out);
    expect(m.approximate.join("\n")).toContain("no from/to");
    expect(m.approximate.join("\n")).toContain("token usage missing");
  });

  it("counts a move to an earlier status in --order as a return", () => {
    const dir = dirOf([{ id: "X", status: "done", moves: [{ from: "qa", to: "build", at: day(10) }, { from: "build", to: "done", at: day(12) }] }]);
    expect(JSON.parse(metrics(dir, "--order", "build,qa,done").out).M2_firstPassRate).toBe(0);
    expect(JSON.parse(metrics(dir, "--order", "qa,build,done").out).M2_firstPassRate).toBe(1);
  });

  it("reads a Blocker-class line written with markdown around it", () => {
    const dir = dirOf([
      { id: "M", status: "blocked", comments: ["**Blocker-class:** `delivery`\nSymptom: push refused"] },
      { id: "N", status: "blocked", comments: ["BLOCKED\n**Blocker-class:** environment"] },
    ]);
    expect(JSON.parse(metrics(dir).out).M5_blocksByClass).toEqual({ delivery: 1, environment: 1 });
  });

  it("counts block events from the timeline, not from comments that repeat one block", () => {
    const dir = dirOf([
      {
        id: "E",
        status: "done",
        moves: [
          { from: "in_progress", to: "blocked", at: day(10) },
          { from: "blocked", to: "in_progress", at: day(11) },
          { from: "in_progress", to: "blocked", at: day(12) },
        ],
        comments: ["BLOCKED\nBlocker-class: tooling", "routed\nBlocker-class: tooling", "unblock\nBlocker-class: tooling"],
      },
      { id: "F", status: "done" },
    ]);
    const m = JSON.parse(metrics(dir).out);
    expect(m.M5_blockEvents).toBe(2);
    expect(m.M5_blockEventsPerIssue).toBe(1);
    expect(m.M5_blocksByClass).toEqual({ tooling: 3 });
  });

  it("defaults M2 to the SDLC Auto order, so a return from qa counts", () => {
    const dir = dirOf([{ id: "Q", status: "done", moves: [{ from: "in_review", to: "qa", at: day(10) }, { from: "qa", to: "in_progress", at: day(11) }, { from: "in_progress", to: "done", at: day(12) }] }]);
    expect(JSON.parse(metrics(dir).out).M2_firstPassRate).toBe(0);
  });

  it("returns nulls, not NaN, when nothing was delivered", () => {
    const m = JSON.parse(metrics(dirOf([{ id: "Z", status: "in_progress" }])).out);
    expect(m.delivered).toBe(0);
    expect(m.M2_firstPassRate).toBeNull();
    expect(m.M3_runsPerDelivered).toEqual({ n: 0, median: null, p90: null });
  });

  it("prints a markdown table with the caveat, and shows n", () => {
    const { out } = metrics(dirOf(ISSUES), "--output", "table");
    expect(out).toContain("| M2 first-pass rate | 67% |");
    expect(out).toContain("(n=3)");
    expect(out).toContain("cannot show a cause");
  });

  it("exits 2 on a bad argument, and does not call the CLI for --help", () => {
    expect(spawnSync("node", [script], { encoding: "utf8" }).status).toBe(2);
    expect(spawnSync("node", [script, "--bogus"], { encoding: "utf8" }).status).toBe(2);
    expect(spawnSync("node", [script, "--output", "xml", "--from-dir", "."], { encoding: "utf8" }).status).toBe(2);
    const help = spawnSync("node", [script, "--help"], { encoding: "utf8" });
    expect(help.status).toBe(0);
    expect(help.stdout).toContain("usage:");
  });
});
