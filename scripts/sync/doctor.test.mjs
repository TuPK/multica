import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { copyFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { SYNC_DIR, makeRepo } from "./testlib.mjs";

const REPO_ATTRS = join(SYNC_DIR, "..", "..", ".gitattributes");
const doctor = (cwd) => spawnSync("node", [join(SYNC_DIR, "doctor.mjs")], { cwd, encoding: "utf8" });

test("doctor fails on a repo without the drivers and passes once they are defined", () => {
  const r = makeRepo("");
  copyFileSync(REPO_ATTRS, join(r.dir, ".gitattributes"));
  for (const k of ["merge.brand.driver", "merge.jsonmerge.driver", "merge.ours.driver", "rerere.enabled"]) {
    try {
      r.git("config", "--unset", k);
    } catch {}
  }
  const bad = doctor(r.dir);
  assert.equal(bad.status, 1);
  assert.match(bad.stderr, /merge\.brand\.driver is not set/);
  assert.match(bad.stderr, /rerere\.enabled/);

  r.git("config", "merge.brand.driver", `node ${join(SYNC_DIR, "merge-brand.mjs")} %O %A %B %L %P`);
  r.git("config", "merge.jsonmerge.driver", `node ${join(SYNC_DIR, "merge-json.mjs")} %O %A %B %L`);
  r.git("config", "merge.ours.driver", "true");
  r.git("config", "rerere.enabled", "true");
  const good = doctor(r.dir);
  assert.equal(good.status, 0, good.stderr);
  r.cleanup();
});

test("doctor reports a driver script that no longer exists", () => {
  const r = makeRepo("");
  copyFileSync(REPO_ATTRS, join(r.dir, ".gitattributes"));
  r.git("config", "merge.brand.driver", "node /nonexistent/merge-brand.mjs %O %A %B");
  r.git("config", "rerere.enabled", "true");
  const out = doctor(r.dir);
  assert.equal(out.status, 1);
  assert.match(out.stderr, /missing script/);
  r.cleanup();
});

test("the checked-in .gitattributes maps sample paths to the right drivers", () => {
  const root = join(SYNC_DIR, "..", "..");
  const attr = (p) => execFileSync("git", ["check-attr", "merge", "--", p], { cwd: root, encoding: "utf8" }).trim();
  assert.match(attr("pnpm-lock.yaml"), /: ours$/);
  assert.match(attr("packages/views/locales/vi/auth.json"), /: jsonmerge$/);
  assert.match(attr("apps/web/public/logo.png"), /: unspecified$/);
});
