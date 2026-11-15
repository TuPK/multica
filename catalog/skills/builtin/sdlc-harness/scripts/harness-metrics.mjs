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
//   --allow-mix measure issues with different harness fingerprints as one group. Off by default: each fingerprint gets its own table.
//   --status-table  print the closeout table of the issues (id, title, status, assignee, parent) and nothing else
//   --lint-report <file>  check a closeout report against the same issues: closing words are refused while an issue is not done
//   --harness   repository whose harness is measured (M8: footprint, certificate age, manifests changed since the certificate)
//
// The fingerprint is the value of the `Harness:` line in the Lead's first comment on an issue (sdlc-squad-lead). An issue with no
// such line is grouped as `unknown` and always reported.
//
// The measures describe; with few issues they cannot show cause. The report always carries n.
// Environment: MULTICA_BIN names the CLI binary (default `multica`).
// Exit code: 0 ok, 2 usage error.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { checkHarness } from "./check-harness.mjs";

// metric_version 2: M3_failedRuns no longer counts runs that failed on the platform (ungraded), and the output has
// M1, M3_ungradedRuns, M3_duplicateRunIds, M8 and M9. Version 1 (no field) counted every failed run as failed.
export const METRIC_VERSION = 2;

const DEFAULT_ORDER = ["backlog", "todo", "in_progress", "in_review", "qa", "done"];
const UNKNOWN_FINGERPRINT = "unknown";
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

/**
 * A failed run whose reason is the platform's or the provider's, not the agent's work: ungraded, not failed.
 * The reasons come from server/pkg/taskfailure: provider errors, timeouts, and runtime or queue loss.
 */
export function isInfrastructureFailure(run) {
  const reason = String(run?.failure_reason ?? "");
  return /^agent_error\.provider_/.test(reason) || /(^|[._])timeout$/.test(reason) || /^(runtime_(offline|access_revoked|recovery)|queued_expired)$/.test(reason);
}

const isSmoke = (issue) => /harness smoke/i.test(issue.meta?.title ?? "");

/** The number of bullets under "Report every command you had to discover" in a Developer comment (the last one that has it). */
function discoveredCommands(comments) {
  let count = null;
  for (const c of asList(comments, "comments", "items")) {
    const lines = String(c?.content ?? "").split("\n");
    const at = lines.findIndex((l) => /report every command you had to discover/i.test(l));
    if (at < 0) continue;
    let n = 0;
    for (const line of lines.slice(at + 1)) {
      if (/^\s*(?:[-*]|\d+\.)\s+/.test(line)) n += 1;
      else if (line.trim() !== "" && n > 0) break;
    }
    count = n;
  }
  return count ?? 0;
}

/** "pass" or "fail" for the negative step (the deliberate break) of a smoke issue, or null when no comment states it. */
function negativeStep(comments) {
  let result = null;
  for (const c of asList(comments, "comments", "items")) {
    for (const m of String(c?.content ?? "").matchAll(/negative step[^\n]*?\b(pass(?:ed)?|fail(?:ed)?)\b/gi)) {
      result = /^pass/i.test(m[1]) ? "pass" : "fail";
    }
  }
  return result;
}

export function computeMetrics(issues, { order = DEFAULT_ORDER } = {}) {
  const approximate = new Set();
  const rank = new Map(order.map((s, i) => [s, i]));
  const delivered = issues.filter((i) => i.status === "done");
  const seenRunIds = new Set();
  let duplicateRunIds = 0;

  const perIssue = delivered.map((issue) => {
    // A run has one id. The same id seen twice is a data error, so it is counted once.
    const runs = asList(issue.runs, "runs", "items").filter((r) => {
      if (!r?.id) return true;
      if (seenRunIds.has(r.id)) {
        duplicateRunIds += 1;
        return false;
      }
      seenRunIds.add(r.id);
      return true;
    });
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
      failedRuns: runs.filter((r) => r.status === "failed" && !isInfrastructureFailure(r)).length,
      ungradedRuns: runs.filter((r) => r.status === "failed" && isInfrastructureFailure(r)).length,
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

  // M1: the cost of building the harness, over every issue that went through the Harness status.
  const harnessIssues = issues.filter((i) => statusMoves(i.timeline).some((m) => m.to === "harness"));
  const harnessRuns = harnessIssues.reduce((n, i) => n + asList(i.runs, "runs", "items").length, 0);
  const harnessTokens = harnessIssues.reduce((n, i) => n + Number(i.usage?.total_input_tokens ?? 0) + Number(i.usage?.total_output_tokens ?? 0), 0);

  // M9: smoke issues, identified by title. A smoke passes when it is done and its negative step passed.
  const smoke = issues.filter(isSmoke).map((i) => {
    const neg = negativeStep(i.comments);
    const result = i.status === "done" && neg === "pass" ? "pass" : i.status === "done" && neg === null ? "unknown" : "fail";
    return { result, runs: asList(i.runs, "runs", "items").length, discovered: discoveredCommands(i.comments), negative: neg };
  });

  // A block is a move into "blocked". Comments can repeat one block (BLOCKED, routed, unblock), so the events are counted here.
  const blockEvents = issues.reduce((n, issue) => n + statusMoves(issue.timeline).filter((m) => m.to === "blocked").length, 0);

  const firstPass = perIssue.filter((p) => !p.returned).length;
  if (duplicateRunIds > 0) approximate.add(`M3: ${duplicateRunIds} run id(s) appeared more than once and were counted once`);
  return {
    metric_version: METRIC_VERSION,
    issues: issues.length,
    delivered: delivered.length,
    M2_firstPassRate: delivered.length === 0 ? null : Math.round((firstPass / delivered.length) * 1000) / 1000,
    M3_runsPerDelivered: spread(perIssue.map((p) => p.runs)),
    M3_failedRuns: perIssue.reduce((n, p) => n + p.failedRuns, 0),
    M3_ungradedRuns: perIssue.reduce((n, p) => n + p.ungradedRuns, 0),
    M3_duplicateRunIds: duplicateRunIds,
    M1_harnessStage: { issues: harnessIssues.length, runs: harnessRuns, tokens: harnessTokens },
    M9_smoke: {
      issues: smoke.length,
      pass: smoke.filter((x) => x.result === "pass").length,
      fail: smoke.filter((x) => x.result === "fail").length,
      unknown: smoke.filter((x) => x.result === "unknown").length,
      runs: smoke.reduce((n, x) => n + x.runs, 0),
      discoveredCommands: smoke.reduce((n, x) => n + x.discovered, 0),
      negativeStep: { pass: smoke.filter((x) => x.negative === "pass").length, fail: smoke.filter((x) => x.negative === "fail").length },
    },
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

/** The harness fingerprint an issue was measured under: the value of its `Harness:` line, or `unknown`. */
export function fingerprintOf(issue) {
  for (const c of asList(issue.comments, "comments", "items")) {
    const m = /^\s*[*_`]*Harness[*_`]*:[*_`\s]*(\S+)/im.exec(String(c?.content ?? ""));
    if (m) return m[1];
  }
  return UNKNOWN_FINGERPRINT;
}

/**
 * Metrics per harness fingerprint. One fingerprint (or --allow-mix) gives one metrics object; several give one per group.
 * Unknown issues are always their own group and always reported in `approximate`, so they are never dropped.
 */
export function measureByHarness(issues, { order = DEFAULT_ORDER, allowMix = false } = {}) {
  const groups = new Map();
  for (const issue of issues) {
    const fp = fingerprintOf(issue);
    if (!groups.has(fp)) groups.set(fp, []);
    groups.get(fp).push(issue);
  }
  const known = [...groups.keys()].filter((fp) => fp !== UNKNOWN_FINGERPRINT);
  const unknown = groups.get(UNKNOWN_FINGERPRINT)?.length ?? 0;
  const note = [];
  if (unknown > 0) note.push(`W2: ${unknown} issue(s) have no Harness fingerprint and are reported as unknown`);
  const summarize = (fp, group) => ({ fingerprint: fp, issues: group.length, ...computeMetrics(group, { order }) });

  if (allowMix || groups.size <= 1) {
    const metrics = computeMetrics(issues, { order });
    if (allowMix && known.length > 1) {
      metrics.approximate.push(`W2: ${known.length} harness fingerprints mixed (${known.join(", ")}): the numbers describe a blend, not one harness`);
    }
    metrics.approximate.push(...note);
    const fingerprints = [...groups.entries()].map(([fingerprint, group]) => ({ fingerprint, issues: group.length }));
    return { ...metrics, fingerprints };
  }
  const ordered = [...known.sort(), ...(unknown > 0 ? [UNKNOWN_FINGERPRINT] : [])];
  return { groups: ordered.map((fp) => summarize(fp, groups.get(fp))), approximate: note };
}

/** M8: the footprint and age of the harness, read from check-harness on the repository. */
export function footprintOf(repo) {
  const report = checkHarness(repo, { refresh: true });
  const refresh = report.refresh ?? {};
  return {
    lines: report.footprint.lines,
    bytes: report.footprint.bytes,
    instructions: report.footprint.instructions,
    proseRules: report.footprint.proseRules,
    certDate: refresh.certDate ?? null,
    staleDays: refresh.staleDays ?? null,
    manifestsChangedSinceCert: (refresh.manifestsChanged ?? []).length,
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
    ["M3 ungraded runs (platform or provider failures)", String(m.M3_ungradedRuns)],
    ["M1 harness stage: issues / runs / tokens", `${m.M1_harnessStage.issues} / ${m.M1_harnessStage.runs} / ${m.M1_harnessStage.tokens}`],
    ["M9 smoke: pass / fail / unknown", `${m.M9_smoke.pass} / ${m.M9_smoke.fail} / ${m.M9_smoke.unknown} (commands discovered: ${m.M9_smoke.discoveredCommands})`],
  ];
  if (m.M8_footprint) {
    const f = m.M8_footprint;
    rows.push(["M8 footprint: lines / bytes / prose rules", `${f.lines} / ${f.bytes} / ${f.proseRules}`]);
    rows.push(["M8 cert age in days / manifests changed since cert", `${f.staleDays ?? "n/a"} / ${f.manifestsChangedSinceCert}`]);
  }
  const lines = ["| Measure | Value |", "| --- | --- |", ...rows.map(([a, b]) => `| ${a} | ${b} |`)];
  if (m.approximate.length > 0) lines.push("", ...m.approximate.map((a) => `Approximate: ${a}`));
  lines.push("", "A description, not a proof: with few issues these numbers cannot show a cause.");
  return lines.join("\n");
}

// ---------------------------------------------------------------------------------------------
// Closeout (W4): the status table is the only source for a report's list of issues

/** Closing words a report must not use while an issue of the request is not done. Lowercase, English and Vietnamese. */
const CLOSEOUT_BANNED = [
  /\bevery\b/i,
  /\ball\s+(?:the\s+)?(?:sub-?issues|issues|children|tasks|stages)\b/i,
  /\bhas\s+(?:all\s+)?(?:been\s+)?closed\b/i,
  /\ball\s+(?:closed|done)\b/i,
  /đã đóng hết/i,
  /tất cả/i,
];

export function statusRows(issues) {
  return issues.map((i) => ({ id: i.meta?.identifier || i.id, title: i.meta?.title ?? "", status: i.status ?? "", assignee: i.meta?.assignee ?? "-", parent: i.meta?.parent ?? "-" }));
}

export function renderStatusTable(rows) {
  const cell = (s) => String(s).replace(/\|/g, "\\|").replace(/\s+/g, " ").trim();
  return ["| id | title | status | assignee | parent |", "| --- | --- | --- | --- | --- |", ...rows.map((r) => `| ${cell(r.id)} | ${cell(r.title)} | ${cell(r.status)} | ${cell(r.assignee)} | ${cell(r.parent)} |`)].join("\n");
}

/** Problems with a closeout report, given the rows of its issues. An empty list means the report may go out. */
export function lintCloseout(reportText, rows) {
  const open = rows.filter((r) => r.status !== "done");
  if (open.length === 0) return [];
  const problems = [];
  const phrases = CLOSEOUT_BANNED.filter((re) => re.test(reportText)).map((re) => re.source);
  if (phrases.length > 0) problems.push(`the report uses a closing phrase while ${open.length} issue(s) are not done: ${phrases.join(", ")}`);
  for (const r of open) {
    if (!reportText.includes(r.id)) problems.push(`the report does not list ${r.id} (status ${r.status}): paste the status table`);
  }
  if (/plan[_ ]ready/i.test(reportText) && !/accepted/i.test(reportText)) {
    problems.push("the report moves the request to plan_ready while issues are not done, and records no accepted stage_done");
  }
  return problems;
}

// ---------------------------------------------------------------------------------------------
// Loading

/** The fields the closeout table shows. The platform's field names vary by version, so each one has fallbacks. */
function issueMeta(issue) {
  return {
    identifier: issue.identifier ?? issue.id ?? "",
    title: issue.title ?? "",
    assignee: issue.assignee_name ?? issue.assignee ?? issue.assignee_id ?? "-",
    parent: issue.parent_issue_id ?? issue.parent_id ?? "-",
  };
}

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
      meta: issueMeta(issue),
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
      meta: issueMeta(issue ?? {}),
      runs: multica(["issue", "runs", id]) ?? [],
      usage: multica(["issue", "usage", id]),
      timeline: multica(["issue", "timeline", id, "--activity-only", "--action", "status_changed,task_completed,task_failed"]) ?? [],
      comments: multica(["issue", "comment", "list", id]) ?? [],
    };
  });
}

function parseArgs(argv) {
  const args = { issues: [], parent: null, order: DEFAULT_ORDER, fromDir: null, output: "json", allowMix: false, statusTable: false, lintReport: null, harness: null };
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
    else if (a === "--allow-mix") args.allowMix = true;
    else if (a === "--status-table") args.statusTable = true;
    else if (a === "--lint-report") args.lintReport = resolve(next());
    else if (a === "--harness") args.harness = resolve(next());
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
  if (args.statusTable) {
    process.stdout.write(`${renderStatusTable(statusRows(issues))}\n`);
    return;
  }
  if (args.lintReport) {
    const problems = lintCloseout(readFileSync(args.lintReport, "utf8"), statusRows(issues));
    if (problems.length === 0) process.stdout.write("closeout ok\n");
    else {
      process.stdout.write(`${problems.map((p) => `closeout: ${p}`).join("\n")}\n`);
      process.exitCode = 1;
    }
    return;
  }
  const measured = measureByHarness(issues, { order: args.order, allowMix: args.allowMix });
  if (args.harness) measured.M8_footprint = footprintOf(args.harness);
  if (args.output === "table") {
    const tables = measured.groups
      ? measured.groups.map((g) => `### Harness fingerprint ${g.fingerprint} (n=${g.issues})\n\n${renderTable(g)}`)
      : [renderTable(measured)];
    process.stdout.write(`${tables.join("\n\n")}\n`);
  } else {
    process.stdout.write(`${JSON.stringify(measured, null, 2)}\n`);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
