// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const script = fileURLToPath(new URL("../../../catalog/skills/builtin/sdlc-harness/scripts/harness-metrics.mjs", import.meta.url));

const dirs: string[] = [];
afterEach(() => {
  for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

interface Fixture {
  id: string;
  status: string;
  title?: string;
  runs?: { id?: string; status: string; failure_reason?: string }[];
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
    w("issue", { status: i.status, title: i.title ?? "" });
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

  describe("harness fingerprint (W2)", () => {
    const FP_ISSUES: Fixture[] = [
      { id: "P", status: "done", moves: [{ from: "todo", to: "done", at: day(10) }], comments: ["Harness: fpA1\nLead plan"] },
      { id: "Q", status: "done", moves: [{ from: "todo", to: "done", at: day(11) }], comments: ["**Harness:** fpA1"] },
      { id: "R", status: "done", moves: [{ from: "todo", to: "done", at: day(12) }], comments: ["Harness: fpB2"] },
      { id: "S", status: "done", moves: [{ from: "todo", to: "done", at: day(13) }] },
    ];

    it("gives each fingerprint its own group, and reports issues with no fingerprint as unknown", () => {
      const m = JSON.parse(metrics(dirOf(FP_ISSUES)).out);
      expect(m.groups.map((g: { fingerprint: string }) => g.fingerprint)).toEqual(["fpA1", "fpB2", "unknown"]);
      expect(m.groups.map((g: { issues: number }) => g.issues)).toEqual([2, 1, 1]);
    });

    it("prints one table per fingerprint in table output", () => {
      const out = metrics(dirOf(FP_ISSUES), "--output", "table").out;
      expect(out).toContain("### Harness fingerprint fpA1 (n=2)");
      expect(out).toContain("### Harness fingerprint fpB2 (n=1)");
      expect(out).toContain("### Harness fingerprint unknown (n=1)");
    });

    it("does not merge fingerprints without --allow-mix, and says so when it merges them", () => {
      const merged = JSON.parse(metrics(dirOf(FP_ISSUES), "--allow-mix").out);
      expect(merged.groups).toBeUndefined();
      expect(merged.issues).toBe(4);
      expect(merged.approximate.join("\n")).toContain("2 harness fingerprints mixed");
      expect(merged.approximate.join("\n")).toContain("no Harness fingerprint");
    });

    it("measures a single fingerprint as one object with no mix warning", () => {
      const one = [FP_ISSUES[0]!, FP_ISSUES[1]!];
      const m = JSON.parse(metrics(dirOf(one)).out);
      expect(m.fingerprints).toEqual([{ fingerprint: "fpA1", issues: 2 }]);
      expect(m.approximate.join("\n")).not.toContain("mixed");
    });
  });

  describe("closeout (W4)", () => {
    const REQUEST: Fixture[] = [
      { id: "DEV-25", status: "stage_done" },
      { id: "DEV-26", status: "stage_done" },
      { id: "DEV-24", status: "done" },
    ];
    const reportFile = (dir: string, text: string) => {
      const file = join(dir, "closeout.md");
      writeFileSync(file, text);
      return file;
    };

    it("prints one status row per issue, with its status, from the same data the metrics read", () => {
      const out = metrics(dirOf(REQUEST), "--status-table").out;
      expect(out).toContain("| DEV-25 |");
      expect(out).toContain("| stage_done |");
      expect(out.trim().split("\n")).toHaveLength(2 + REQUEST.length);
    });

    it("refuses a closing phrase while an issue is not done, and names the phrase", () => {
      const dir = dirOf(REQUEST);
      const file = reportFile(dir, "Outcome: every sub-issue has closed.\n| DEV-25 | stage_done |\n| DEV-26 | stage_done |\n");
      const res = metrics(dir, "--lint-report", file);
      expect(res.code).toBe(1);
      expect(res.out).toContain("closing phrase while 2 issue(s) are not done");
    });

    it("refuses a report that does not list each open issue, and a plan_ready with no accepted stage", () => {
      const dir = dirOf(REQUEST);
      const file = reportFile(dir, "Outcome: review done.\nMoved to plan_ready.\n| DEV-24 | done |\n");
      const res = metrics(dir, "--lint-report", file);
      expect(res.code).toBe(1);
      expect(res.out).toContain("does not list DEV-25");
      expect(res.out).toContain("plan_ready while issues are not done");
    });

    it("passes a report with a generated table when every issue is done, even with the word every", () => {
      const dir = dirOf([{ id: "A", status: "done" }, { id: "B", status: "done" }]);
      const file = reportFile(dir, "Outcome: every row is done.\nplan_ready\n");
      expect(metrics(dir, "--lint-report", file)).toMatchObject({ code: 0 });
      expect(metrics(dir, "--lint-report", file).out).toContain("closeout ok");
    });

    it("passes a plan_ready that records the accepted stage_done rows", () => {
      const dir = dirOf(REQUEST);
      const file = reportFile(dir, "Outcome: accepted DEV-25 and DEV-26 as stage_done.\n| DEV-24 | done |\n| DEV-25 | stage_done |\n| DEV-26 | stage_done |\nplan_ready\n");
      expect(metrics(dir, "--lint-report", file).code).toBe(0);
    });
  });

  describe("measures M1, M3 ungraded and identity, M8, M9 (W3)", () => {
    it("counts a provider or timeout failure as ungraded, and only the agent's own failures as failed", () => {
      const dir = dirOf([
        {
          id: "U",
          status: "done",
          runs: [
            { id: "r1", status: "failed", failure_reason: "agent_error.provider_network" },
            { id: "r2", status: "failed", failure_reason: "agent_timeout" },
            { id: "r3", status: "failed", failure_reason: "agent_error.agent_timeout" },
            { id: "r4", status: "failed", failure_reason: "agent_error.unknown" },
            { id: "r5", status: "completed" },
          ],
        },
      ]);
      const m = JSON.parse(metrics(dir).out);
      expect(m.M3_ungradedRuns).toBe(3);
      expect(m.M3_failedRuns).toBe(1);
      expect(m.M3_runsPerDelivered.median).toBe(5);
      expect(m.metric_version).toBe(2);
    });

    it("counts a run id that appears twice once, and says so", () => {
      const dir = dirOf([
        { id: "X", status: "done", runs: [{ id: "same", status: "completed" }, { id: "x2", status: "completed" }] },
        { id: "Y", status: "done", runs: [{ id: "same", status: "completed" }] },
      ]);
      const m = JSON.parse(metrics(dir).out);
      expect(m.M3_duplicateRunIds).toBe(1);
      // X keeps its two runs; the second "same" run in Y is dropped, so Y has none.
      expect(m.M3_runsPerDelivered).toEqual({ n: 2, median: 0, p90: 2 });
      expect(m.approximate.join("\n")).toContain("1 run id(s) appeared more than once");
    });

    it("measures M1 over the issues that went through the Harness status", () => {
      const dir = dirOf([
        {
          id: "H",
          status: "done",
          runs: [{ status: "completed" }, { status: "completed" }],
          usage: { total_input_tokens: 300, total_output_tokens: 200 },
          moves: [{ from: "backlog", to: "harness", at: day(9) }, { from: "harness", to: "todo", at: day(10) }],
        },
        { id: "N", status: "done", runs: [{ status: "completed" }], moves: [{ from: "todo", to: "done", at: day(11) }] },
      ]);
      expect(JSON.parse(metrics(dir).out).M1_harnessStage).toEqual({ issues: 1, runs: 2, tokens: 500 });
    });

    it("reads M9 from smoke issues: pass when done with the negative step passed, and the discovered commands", () => {
      const dir = dirOf([
        {
          id: "S1",
          status: "done",
          title: "Harness smoke for the Expo app",
          comments: [
            "Smoke report\nNegative step: pass (broke the assertion, Done exited 1, undone)\nReport every command you had to discover:\n- pnpm db:seed\n- pnpm env:ensure\n\nDone.",
          ],
        },
        { id: "S2", status: "in_progress", title: "Harness smoke for the API", comments: ["Negative step: fail"] },
        { id: "D", status: "done", title: "Add search" },
      ]);
      const m = JSON.parse(metrics(dir).out);
      expect(m.M9_smoke).toMatchObject({ issues: 2, pass: 1, fail: 1, unknown: 0, discoveredCommands: 2, negativeStep: { pass: 1, fail: 1 } });
    });

    it("reads M8 from check-harness on the repository: footprint, certificate age, and manifests changed since the certificate", () => {
      const repo = mkdtempSync(join(tmpdir(), "harness-m8-"));
      dirs.push(repo);
      const git = (...args: string[]) => execFileSync("git", ["-C", repo, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false", ...args], { encoding: "utf8" }).trim();
      git("init", "-q");
      const agents = "# Demo\n\n## Verify\n- Done = `node -e 0` exits 0 (evidence level L1)\n\n## Rules\n- Keep it small. Enforced by: prose\n";
      mkdirSync(join(repo, "docs/sdlc"), { recursive: true });
      writeFileSync(join(repo, "AGENTS.md"), agents);
      writeFileSync(join(repo, "package.json"), JSON.stringify({ scripts: {} }));
      git("add", "-A");
      git("commit", "-q", "-m", "harness");
      const sha = git("rev-parse", "HEAD");
      writeFileSync(join(repo, "docs/sdlc/harness-cert.json"), JSON.stringify({ schema: 1, sha, date: "2026-11-01" }));
      git("add", "-A");
      git("commit", "-q", "-m", "cert");
      writeFileSync(join(repo, "package.json"), JSON.stringify({ scripts: { test: "x" } }));
      git("commit", "-q", "-am", "manifest");
      const res = spawnSync("node", [script, "--from-dir", dirOf([{ id: "Z", status: "done" }]), "--harness", repo], { encoding: "utf8" });
      expect(res.status, res.stderr).toBe(0);
      const m = JSON.parse(res.stdout);
      expect(m.M8_footprint).toMatchObject({ lines: 7, certDate: "2026-11-01", manifestsChangedSinceCert: 1, proseRules: 1 });
    });
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
