import assert from "node:assert/strict";
import { test } from "node:test";
import { newHits, scan } from "./brand-check.mjs";
import { makeRepo } from "./testlib.mjs";

test("scan reads display surfaces only; newHits respects the recorded debt", () => {
  const r = makeRepo("");
  r.write("README.md", "Welcome to Multica\nmultica login\n");
  r.write("server/x.go", "// Multica daemon\n");
  r.write("packages/views/locales/en/a.json", '{"a": "Multica"}\n');
  r.commit("x");
  const hits = scan({ root: r.dir, files: ["README.md", "server/x.go", "packages/views/locales/en/a.json"] });
  assert.deepEqual(Object.keys(hits).sort(), ["README.md", "packages/views/locales/en/a.json"]);
  assert.deepEqual(newHits(hits, { "README.md": 1 }).map((b) => b.file), ["packages/views/locales/en/a.json"]);
  assert.deepEqual(newHits(hits, { "README.md": 1, "packages/views/locales/en/a.json": 1 }), []);
  r.cleanup();
});
