import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { runCheck, runContract } from "./contract.mjs";
import { SYNC_DIR } from "./testlib.mjs";

function tree(files) {
  const root = mkdtempSync(join(tmpdir(), "contract-"));
  for (const [p, t] of Object.entries(files)) {
    mkdirSync(dirname(join(root, p)), { recursive: true });
    writeFileSync(join(root, p), t);
  }
  return root;
}

test("each check kind passes and fails as described", () => {
  const root = tree({ "a.txt": "hello fork", "d/x": "1", "p.json": '{"k":{"n":"Delegora"}}' });
  const ok = [
    { kind: "exists", path: "a.txt" },
    { kind: "absent", path: "nope" },
    { kind: "nonEmptyDir", path: "d" },
    { kind: "matches", path: "a.txt", pattern: "fo+rk" },
    { kind: "lacks", path: "a.txt", pattern: "upstream" },
    { kind: "jsonEquals", path: "p.json", key: "k.n", value: "Delegora" },
  ];
  for (const c of ok) assert.equal(runCheck(root, c), null, JSON.stringify(c));
  const bad = [
    { kind: "exists", path: "nope" },
    { kind: "absent", path: "a.txt" },
    { kind: "nonEmptyDir", path: "nope" },
    { kind: "matches", path: "a.txt", pattern: "zzz" },
    { kind: "lacks", path: "a.txt", pattern: "hello" },
    { kind: "jsonEquals", path: "p.json", key: "k.n", value: "Multica" },
    { kind: "wat", path: "a.txt" },
  ];
  writeFileSync(join(root, "broken.json"), "<<<<<<< HEAD\n{}\n");
  assert.match(runCheck(root, { kind: "jsonEquals", path: "broken.json", key: "k", value: 1 }), /not valid JSON/);
  for (const c of bad) assert.notEqual(runCheck(root, c), null, JSON.stringify(c));
  rmSync(root, { recursive: true });
});

test("the real contract is green on this checkout", () => {
  const root = join(SYNC_DIR, "..", "..");
  const contract = JSON.parse(readFileSync(join(SYNC_DIR, "fork-contract.json"), "utf8"));
  assert.deepEqual(runContract(root, contract).map((f) => `${f.feature}: ${f.error}`), []);
});

test("every feature in the real contract turns red when its files disappear", () => {
  const contract = JSON.parse(readFileSync(join(SYNC_DIR, "fork-contract.json"), "utf8"));
  const empty = mkdtempSync(join(tmpdir(), "contract-empty-"));
  const failed = new Set(runContract(empty, contract).map((f) => f.feature));
  // `excluded-ee-code` asserts absence, so an empty tree legitimately satisfies it.
  const expected = contract.features.map((f) => f.id).filter((id) => id !== "excluded-ee-code");
  assert.deepEqual([...failed].sort(), expected.sort());
  rmSync(empty, { recursive: true });
});
