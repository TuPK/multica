// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { COLLECTOR_METHODS, SALES_FAMILIES, findSecretKey } from "./sales-recipe";
import { SALES_STARTER_SOURCES, starterSourceDocument } from "./sales-starter-sources";

const sources = SALES_STARTER_SOURCES;

describe("starter sales sources", () => {
  it("has a unique stable id for every source", () => {
    const ids = sources.map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
    for (const id of ids) expect(id).toMatch(/^[a-z0-9-]+$/);
  });

  it("holds no secret in any recipe", () => {
    for (const s of sources) {
      expect(findSecretKey(s.recipe), s.id).toBeNull();
    }
  });

  it("writes recipes at version 1 with a rung and method that agree", () => {
    for (const s of sources) {
      expect(s.recipe.version, s.id).toBe(1);
      expect(s.recipe.rung, s.id).toBe(s.rung);
      expect(s.recipe.method, s.id).toBe(s.method);
      expect(s.rung, s.id).toBeGreaterThanOrEqual(1);
      expect(s.rung, s.id).toBeLessThanOrEqual(7);
    }
  });

  it("uses only known families, and leaves a social source's family to its posts", () => {
    for (const s of sources) {
      if (s.family === null) continue;
      expect(SALES_FAMILIES, s.id).toContain(s.family);
    }
    for (const s of sources.filter((x) => x.id.startsWith("social-"))) {
      // A social row takes its family from the posts, except a row that reads one kind of feed (the Ad Library).
      if (s.family !== null) expect(s.family, s.id).toBe(s.recipe.map.family);
    }
  });

  it("marks a source active only when the recipe was checked on a saved response, or it reads the knowledge base", () => {
    for (const s of sources.filter((x) => x.status === "active")) {
      if (s.method === "kb_internal") continue;
      expect(s.recipe.sample_count, s.id).toBeGreaterThanOrEqual(5);
      expect(s.recipe.field_coverage, s.id).toBeGreaterThanOrEqual(0.8);
      expect(s.recipe.verified_at, s.id).not.toBeNull();
      expect(COLLECTOR_METHODS.includes(s.method), s.id).toBe(true);
    }
  });

  it("gives every source that needs a person a sentence that says what unlocks it", () => {
    for (const s of sources.filter((x) => x.status === "needs_connector" || x.status === "manual_only")) {
      expect(s.unlock, s.id).toBeTruthy();
      expect(s.unlock!.length, s.id).toBeGreaterThan(40);
    }
  });

  it("keeps every paid provider at a cost cap of 0 until an owner sets one", () => {
    for (const s of sources.filter((x) => x.method === "provider")) {
      expect(s.recipe.provider, s.id).toBeDefined();
      expect(s.recipe.provider!.cost_cap_usd, s.id).toBe(0);
    }
  });

  it("never activates a source whose terms are restricted or unknown", () => {
    for (const s of sources.filter((x) => x.status === "active")) {
      expect(["ok", "attribution"], s.id).toContain(s.terms);
    }
  });

  it("lists the social networks of the spec as their own rows, with a connection or a share", () => {
    const social = sources.filter((s) => s.id.startsWith("social-"));
    expect(social.length).toBe(15);
    for (const s of social) {
      expect(["needs_connector", "manual_only"], s.id).toContain(s.status);
    }
  });

  it("writes a source document with its key, its kind and the sections the kind requires, in order", () => {
    const doc = starterSourceDocument(sources[0]!);
    expect(doc.kind).toBe("sales_source");
    expect(doc.kb).toBe("sales");
    expect(doc.idempotency_key).toBe(`sales:source:${sources[0]!.id}`);
    const headings = [...doc.content.matchAll(/^## (.+)$/gm)].map((m) => m[1]);
    expect(headings).toEqual([
      "Record",
      "What it gives",
      "How to read it",
      "Recipe",
      "Terms and limits",
      "Probe log",
    ]);
  });

  it("puts the unlock sentence into the document of a source that needs a person", () => {
    const row = sources.find((s) => s.status === "needs_connector")!;
    expect(starterSourceDocument(row).content).toContain(row.unlock!);
  });

  // The Go collector tests read the same catalogue from a JSON export; a change here must come
  // with a regenerated file (see server/internal/salescollect/starter_test.go).
  it("matches the catalogue the Go collector tests read", () => {
    const path = fileURLToPath(
      new URL("../../../server/internal/salescollect/testdata/starter_recipes.json", import.meta.url),
    );
    const saved = JSON.parse(readFileSync(path, "utf8"));
    expect(saved).toEqual(
      sources.map((s) => ({
        id: s.id,
        status: s.status,
        method: s.method,
        rung: s.rung,
        family: s.family,
        recipe: s.recipe,
      })),
    );
  });
});
