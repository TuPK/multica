#!/usr/bin/env node
// Harness measures from the data the platform already keeps: runs, token usage, status history and comments.
// Plain Node, no dependencies, no LLM, no writes. Prints JSON (default) or a markdown table to paste in a report.
//
//   node harness-metrics.mjs --issues <id,id,...> | --parent <epic id> [--order <status,status,...>] [--output json|table]
//   node harness-metrics.mjs --from-dir <dir> [--order ...] [--output ...]
//
//   --issues    issues to measure (identifiers such as MUL-12, or ids)
//   --parent    measure the children of this issue (only children; issues the Lead creates in a project have no parent)
//   --order     workflow statuses from first to last; a move to an earlier one counts as a return
//               (default: backlog,todo,in_progress,in_review,qa,done, the SDLC Auto order)
//   --from-dir  read <id>.issue.json, <id>.runs.json, <id>.usage.json, <id>.timeline.json and <id>.comments.json
//               from a directory instead of calling the `multica` CLI
//
// The measures describe; with few issues they cannot show cause. The report always carries n.
// Environment: MULTICA_BIN names the CLI binary (default `multica`).
// Exit code: 0 ok, 2 usage error.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const DEFAULT_ORDER = ["backlog", "todo", "in_progress", "in_review", "qa", "done"];
const NOT_STARTED = new Set(["", "backlog", "todo"]);
const HARNESS_ID_RE = /\bH(?:1|3|4|6|7|10|14)\b|check-harness/;

// ---------------------------------------------------------------------------------------------
// Pure computation. Input: [{ id, status, runs, usage, timeline, comments }]

export function percentile(values, p) {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const rank = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.min(sorted.length - 1, Math.max(0, rank))];
}

const spread = (values) => ({ n: values.length, median: percentile(values, 50), p90: percentile(values, 90) });

const asList = (x, ...keys) => {
  if (Array.isArray(x)) return x;
  for (const k of keys) if (x && Array.isArray(x[k])) return x[k];
  return [];
};

function statusMoves(timeline) {
  return asList(timeline, "entries", "items")
    .filter((e) => e && (e.action === "status_changed" || e.action === "status_change"))
    .map((e) => {
      const d = typeof e.details === "string" ? safeJson(e.details) : (e.details ?? {});
      return { from: d.from ?? null, to: d.to ?? null, at: Date.parse(e.created_at ?? "") };
    })
    .sort((a, b) => (a.at || 0) - (b.at || 0));
}

function safeJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return {};
  }
}

function blockerClasses(comments) {
  const out = [];
  for (const c of asList(comments, "comments", "items")) {
    const text = String(c?.content ?? "");
    // Agents write the line with markdown around it: "**Blocker-class:** `delivery`".
    const m = /^\s*[*_`]*Blocker-class[*_`]*:[*_`\s]*([a-z-]+)/im.exec(text);
    if (m) out.push({ cls: m[1].toLowerCase(), text });
  }
  return out;
}

export function computeMetrics(issues, { order = DEFAULT_ORDER } = {}) {
  const approximate = new Set();
  const rank = new Map(order.map((s, i) => [s, i]));
  const delivered = issues.filter((i) => i.status === "done");

  const perIssue = delivered.map((issue) => {
    const runs = asList(issue.runs, "runs", "items");
    const moves = statusMoves(issue.timeline);
    let returned = false;
    let unknownMoves = 0;
    for (const m of moves) {
      if (m.from === null || m.to === null) {
        unknownMoves++;
        continue;
      }
      if (rank.has(m.from) && rank.has(m.to) && rank.get(m.to) < rank.get(m.from)) returned = true;
    }
    if (unknownMoves > 0) approximate.add("M2: some status changes had no from/to, so returns may be missed");
    if (moves.length === 0) approximate.add("M2/M6: an issue had no status history (timeline truncated or empty)");
    const started = moves.find((m) => m.to !== null && !NOT_STARTED.has(m.to));
    const finished = [...moves].reverse().find((m) => m.to === "done");
    const u = issue.usage ?? {};
    const input = Number(u.total_input_tokens ?? 0);
    const output = Number(u.total_output_tokens ?? 0);
    if (issue.usage === undefined || issue.usage === null) approximate.add("M4: token usage missing for some issues, counted as 0 (lower bound)");
    return {
      id: issue.id,
      runs: runs.length,
      failedRuns: runs.filter((r) => r.status === "failed").length,
      tokens: input + output,
      cacheTokens: Number(u.total_cache_read_tokens ?? 0) + Number(u.total_cache_write_tokens ?? 0),
      returned,
      cycleHours: started && finished && started.at && finished.at ? (finished.at - started.at) / 3600000 : null,
    };
  });

  // Blocks and harness-attributable blocks use every issue, delivered or not.
  const blocksByClass = {};
  let harnessBlocks = 0;
  for (const issue of issues) {
    for (const { cls, text } of blockerClasses(issue.comments)) {
      blocksByClass[cls] = (blocksByClass[cls] ?? 0) + 1;
      if ((cls === "tooling" && HARNESS_ID_RE.test(text)) || (cls === "environment" && /\bensure\b/i.test(text))) harnessBlocks++;
    }
  }

  // A block is a move into "blocked". Comments can repeat one block (BLOCKED, routed, unblock), so the events are counted here.
  const blockEvents = issues.reduce((n, issue) => n + statusMoves(issue.timeline).filter((m) => m.to === "blocked").length, 0);

  const firstPass = perIssue.filter((p) => !p.returned).length;
  return {
    issues: issues.length,
    delivered: delivered.length,
    M2_firstPassRate: delivered.length === 0 ? null : Math.round((firstPass / delivered.length) * 1000) / 1000,
    M3_runsPerDelivered: spread(perIssue.map((p) => p.runs)),
    M3_failedRuns: perIssue.reduce((n, p) => n + p.failedRuns, 0),
    M4_tokensPerDelivered: spread(perIssue.map((p) => p.tokens)),
    M4_cacheTokensPerDelivered: spread(perIssue.map((p) => p.cacheTokens)),
    M5_blocksByClass: blocksByClass,
    M5_blocksPerIssue: issues.length === 0 ? null : Math.round((Object.values(blocksByClass).reduce((a, b) => a + b, 0) / issues.length) * 100) / 100,
    M5_blockEvents: blockEvents,
    M5_blockEventsPerIssue: issues.length === 0 ? null : Math.round((blockEvents / issues.length) * 100) / 100,
    M6_cycleHours: spread(perIssue.map((p) => p.cycleHours).filter((h) => h !== null)),
    M7_harnessBlocks: harnessBlocks,
    approximate: [...approximate],
  };
}

export function renderTable(m) {
  const f = (s) => (s.n === 0 ? "n/a" : `${round(s.median)} / ${round(s.p90)} (n=${s.n})`);
  const round = (x) => (x === null ? "n/a" : Math.round(x * 10) / 10);
  const rows = [
    ["Issues measured / delivered", `${m.issues} / ${m.delivered}`],
    ["M2 first-pass rate", m.M2_firstPassRate === null ? "n/a" : `${Math.round(m.M2_firstPassRate * 100)}%`],
    ["M3 runs per delivered issue (median / p90)", f(m.M3_runsPerDelivered)],
    ["M3 failed runs", String(m.M3_failedRuns)],
    ["M4 tokens per delivered issue (median / p90)", f(m.M4_tokensPerDelivered)],
    ["M5 blocks per issue", m.M5_blocksPerIssue === null ? "n/a" : `${m.M5_blocksPerIssue} (${Object.entries(m.M5_blocksByClass).map(([k, v]) => `${k}: ${v}`).join(", ") || "none"})`],
    ["M6 cycle time, hours (median / p90)", f(m.M6_cycleHours)],
    ["M5 block events (moves to blocked)", `${m.M5_blockEvents} (${m.M5_blockEventsPerIssue} per issue)`],
    ["M7 harness-attributable blocks", String(m.M7_harnessBlocks)],
  ];
  const lines = ["| Measure | Value |", "| --- | --- |", ...rows.map(([a, b]) => `| ${a} | ${b} |`)];
  if (m.approximate.length > 0) lines.push("", ...m.approximate.map((a) => `Approximate: ${a}`));
  lines.push("", "A description, not a proof: with few issues these numbers cannot show a cause.");
  return lines.join("\n");
}

// ---------------------------------------------------------------------------------------------
// Loading

function readJson(file) {
  return existsSync(file) ? JSON.parse(readFileSync(file, "utf8")) : undefined;
}

export function loadFromDir(dir) {
  const ids = [...new Set(readdirSync(dir).filter((f) => f.endsWith(".issue.json")).map((f) => f.replace(/\.issue\.json$/, "")))].sort();
  return ids.map((id) => {
    const issue = readJson(join(dir, `${id}.issue.json`)) ?? {};
    return {
      id,
      status: issue.status,
      runs: readJson(join(dir, `${id}.runs.json`)) ?? [],
      usage: readJson(join(dir, `${id}.usage.json`)),
      timeline: readJson(join(dir, `${id}.timeline.json`)) ?? [],
      comments: readJson(join(dir, `${id}.comments.json`)) ?? [],
    };
  });
}

function multica(args) {
  const out = execFileSync(process.env.MULTICA_BIN || "multica", [...args, "--output", "json"], { encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  return out.trim() === "" ? undefined : JSON.parse(out);
}

export function loadFromCli(ids) {
  return ids.map((id) => {
    const issue = multica(["issue", "get", id]);
    return {
      id,
      status: issue?.status,
      runs: multica(["issue", "runs", id]) ?? [],
      usage: multica(["issue", "usage", id]),
      timeline: multica(["issue", "timeline", id, "--activity-only", "--action", "status_changed,task_completed,task_failed"]) ?? [],
      comments: multica(["issue", "comment", "list", id]) ?? [],
    };
  });
}

function parseArgs(argv) {
  const args = { issues: [], parent: null, order: DEFAULT_ORDER, fromDir: null, output: "json" };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      const v = argv[++i];
      if (!v) throw new Error(`${a} needs a value`);
      return v;
    };
    if (a === "--issues") args.issues = next().split(",").map((s) => s.trim()).filter(Boolean);
    else if (a === "--parent") args.parent = next();
    else if (a === "--order") args.order = next().split(",").map((s) => s.trim()).filter(Boolean);
    else if (a === "--from-dir") args.fromDir = resolve(next());
    else if (a === "--output") {
      args.output = next();
      if (!["json", "table"].includes(args.output)) throw new Error("--output is json or table");
    } else if (a === "--help" || a === "-h") args.help = true;
    else throw new Error(`unknown argument: ${a}`);
  }
  if (!args.help && !args.fromDir && args.issues.length === 0 && !args.parent) throw new Error("give --issues, --parent or --from-dir");
  return args;
}

const USAGE = "usage: harness-metrics.mjs (--issues <id,...> | --parent <epic> | --from-dir <dir>) [--order <status,...>] [--output json|table]";

function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (err) {
    process.stderr.write(`harness-metrics: ${err.message}\n${USAGE}\n`);
    process.exit(2);
  }
  if (args.help) {
    process.stdout.write(`${USAGE}\n`);
    return;
  }
  let issues;
  if (args.fromDir) issues = loadFromDir(args.fromDir);
  else {
    let ids = args.issues;
    if (args.parent) {
      const children = multica(["issue", "children", args.parent]);
      ids = asList(children, "children", "items", "issues").map((c) => c.identifier ?? c.id).filter(Boolean);
    }
    issues = loadFromCli(ids);
  }
  const metrics = computeMetrics(issues, { order: args.order });
  process.stdout.write(args.output === "table" ? `${renderTable(metrics)}\n` : `${JSON.stringify(metrics, null, 2)}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
