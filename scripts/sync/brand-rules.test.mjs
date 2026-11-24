import assert from "node:assert/strict";
import { test } from "node:test";
import { bareUpstreamName, brand, canon, isDisplaySurface } from "./brand-rules.mjs";

test("brand() renames prose and keeps every identifier spelling", () => {
  const renamed = [
    ["Welcome to Multica.", "Welcome to Delegora."],
    ["Multica는 좋다", "Delegora는 좋다"],
    ['"title": "Multica"', '"title": "Delegora"'],
    ["Multica's agents", "Delegora's agents"],
    ["@Multica Bot hello", "@Delegora Bot hello"],
    ["/Applications/Multica.app/Contents", "/Applications/Delegora.app/Contents"],
  ];
  for (const [from, to] of renamed) assert.equal(brand(from), to);
  const kept = [
    "multica login",
    "@multica/core",
    "X-Multica-Workspace",
    "Multica-Agent: a",
    "MULTICA_TOKEN",
    "MulticaFoo",
    "Multica_x",
    "https://multica.ai/docs",
    "github.com/multica-ai/multica",
    "api/Multica",
    "a.Multica",
    "Multica Canary",
    "the Multica License",
    "© Multica",
    "Copyright 2026 Multica",
    "(c) Multica",
  ];
  for (const s of kept) assert.equal(brand(s), s, s);
});

test("canon() and brand() round-trip plain prose", () => {
  const s = "Delegora runs your agents.";
  assert.equal(canon(s), "Multica runs your agents.");
  assert.equal(brand(canon(s)), s);
});

test("bareUpstreamName() reports lines the rule would rename", () => {
  const hits = bareUpstreamName("ok\nWelcome to Multica\nmultica login\n");
  assert.deepEqual(hits, [{ line: 2, text: "Welcome to Multica" }]);
});

test("isDisplaySurface() matches reader-facing files only", () => {
  assert.ok(isDisplaySurface("packages/views/locales/en/auth.json"));
  assert.ok(isDisplaySurface("README.zh.md"));
  assert.ok(isDisplaySurface("apps/docs/content/docs/agents.mdx"));
  assert.ok(!isDisplaySurface("server/internal/daemon/daemon.go"));
  assert.ok(!isDisplaySurface("LICENSE"));
});
