#!/usr/bin/env node
// Scores a code reviewer against labelled cases: recall of known bugs per tier, and findings on control commits.
//
//   node score.mjs --cases cases.json --results results.json [--json]
//   node score.mjs --selftest
//
// results.json: { "<case id>": { "raw": [finding], "verified": [finding], "final": [finding] } }
// finding:      { "file": "server/x.go", "line": 12, "summary": "...", "preExisting": false }
//
// A finding catches a bug when its file is one of the case's files and its summary contains one of the case's keywords
// (case-insensitive). Control cases hold stable merged commits: every non-pre-existing finding there is a false positive.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const TIERS = ["raw", "verified", "final"];

export function catches(finding, bug) {
  const file = String(finding.file ?? "");
  const summary = String(finding.summary ?? "").toLowerCase();
  const fileOk = bug.files.some((f) => file === f || file.endsWith(`/${f}`));
  return fileOk && bug.keywords.some((k) => summary.includes(k.toLowerCase()));
}

export function score(cases, results) {
  const out = { tiers: {}, splits: {} };
  for (const tier of TIERS) {
    out.tiers[tier] = { bugs: 0, caught: 0, controlFindings: 0, controls: 0 };
  }
  for (const c of cases) {
    const r = results[c.id] ?? {};
    for (const tier of TIERS) {
      const findings = r[tier] ?? [];
      const t = out.tiers[tier];
      const split = (out.splits[c.split] ??= {});
      const s = (split[tier] ??= { bugs: 0, caught: 0 });
      if (c.split === "control") {
        t.controls += 1;
        t.controlFindings += findings.filter((f) => !f.preExisting).length;
        continue;
      }
      t.bugs += 1;
      s.bugs += 1;
      if (findings.some((f) => catches(f, c))) {
        t.caught += 1;
        s.caught += 1;
      }
    }
  }
  for (const tier of TIERS) {
    const t = out.tiers[tier];
    t.recall = t.bugs === 0 ? null : t.caught / t.bugs;
  }
  return out;
}

function report(result) {
  const lines = [];
  for (const tier of TIERS) {
    const t = result.tiers[tier];
    const recall = t.recall === null ? "n/a" : `${t.caught}/${t.bugs} (${(t.recall * 100).toFixed(0)}%)`;
    lines.push(`${tier.padEnd(9)} recall ${recall}   findings on ${t.controls} control commits: ${t.controlFindings}`);
  }
  for (const [name, split] of Object.entries(result.splits)) {
    if (name === "control") continue;
    const parts = TIERS.map((tier) => `${tier} ${split[tier]?.caught ?? 0}/${split[tier]?.bugs ?? 0}`);
    lines.push(`split ${name}: ${parts.join(", ")}`);
  }
  return lines.join("\n");
}

function selftest() {
  const cases = [
    { id: "b1", split: "tuning", files: ["a.go"], keywords: ["audit row"] },
    { id: "b2", split: "fixed", files: ["b/c.ts"], keywords: ["permission"] },
    { id: "k1", split: "control", files: [], keywords: [] },
  ];
  const results = {
    b1: { raw: [{ file: "a.go", summary: "The audit row is no longer recorded" }], verified: [], final: [] },
    b2: {
      raw: [{ file: "b/c.ts", summary: "Wrong permission check" }],
      verified: [{ file: "b/c.ts", summary: "Wrong permission check" }],
      final: [{ file: "x.go", summary: "permission audit row" }],
    },
    k1: { raw: [{ file: "k.go", summary: "style" }, { file: "k.go", summary: "old issue", preExisting: true }], verified: [], final: [] },
  };
  const r = score(cases, results);
  const expect = (cond, msg) => {
    if (!cond) throw new Error(`selftest: ${msg}`);
  };
  expect(r.tiers.raw.caught === 2 && r.tiers.raw.bugs === 2, "raw should catch both bugs");
  expect(r.tiers.verified.caught === 1, "verified should catch only b2");
  expect(r.tiers.final.caught === 0, "file must match: final finding is in another file");
  expect(r.tiers.raw.controlFindings === 1, "pre-existing findings are not false positives");
  expect(r.splits.tuning.raw.caught === 1 && r.splits.fixed.raw.caught === 1, "per-split counts");
  console.log("selftest ok");
}

const isMain = process.argv[1] === fileURLToPath(import.meta.url);
if (isMain) {
  const args = process.argv.slice(2);
  if (args.includes("--selftest")) {
    selftest();
  } else {
    const get = (flag) => {
      const i = args.indexOf(flag);
      return i >= 0 ? args[i + 1] : null;
    };
    const casesPath = get("--cases");
    const resultsPath = get("--results");
    if (!casesPath || !resultsPath) {
      console.error("usage: score.mjs --cases <cases.json> --results <results.json> [--json] | --selftest");
      process.exit(2);
    }
    const cases = JSON.parse(readFileSync(casesPath, "utf8")).cases;
    const results = JSON.parse(readFileSync(resultsPath, "utf8"));
    const result = score(cases, results);
    console.log(args.includes("--json") ? JSON.stringify(result, null, 2) : report(result));
  }
}
