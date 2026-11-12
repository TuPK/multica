// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseResearchSettings } from "./schema";

const SETTINGS = {
  openalex_configured: true,
  semantic_scholar_configured: false,
  contact_email: "lab@example.org",
  keys_can_be_stored: true,
  updated_at: "2026-11-12T08:00:00Z",
};

describe("parseResearchSettings", () => {
  it("reads the settings the server returns", () => {
    expect(parseResearchSettings(SETTINGS)).toEqual(SETTINGS);
  });

  it("fills a missing field and keeps one it does not know", () => {
    const parsed = parseResearchSettings({ openalex_configured: true, future_field: 1 });
    expect(parsed.openalex_configured).toBe(true);
    expect(parsed.semantic_scholar_configured).toBe(false);
    expect(parsed.contact_email).toBe("");
    expect(parsed.updated_at).toBeNull();
    expect((parsed as unknown as Record<string, unknown>).future_field).toBe(1);
  });

  it("reads a malformed answer as nothing configured, with keys that cannot be stored", () => {
    for (const bad of ["nope", null, 42, { openalex_configured: "yes" }]) {
      const parsed = parseResearchSettings(bad);
      expect(parsed.openalex_configured).toBe(false);
      expect(parsed.keys_can_be_stored).toBe(false);
    }
  });

  it("never carries a key field", () => {
    const parsed = parseResearchSettings({ ...SETTINGS });
    expect(Object.keys(parsed).some((k) => k.includes("api_key"))).toBe(false);
  });
});
