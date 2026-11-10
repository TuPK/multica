// @vitest-environment node

import { describe, expect, it } from "vitest";
import type { HubActivityEntry } from "@multica/core/hub";
import { hubActivityActionKey, hubActivityFacts } from "./hub-activity-format";

function entry(detail: Record<string, unknown>, action = "source_synced"): HubActivityEntry {
  return {
    id: "a1",
    action,
    actor_type: "member",
    actor_id: "u1",
    actor_name: "Ada",
    kind: null,
    ref_id: null,
    item_name: "",
    source_id: null,
    detail,
    created_at: "2026-11-01T00:00:00Z",
  };
}

describe("hubActivityActionKey", () => {
  it("keeps the actions the Hub writes", () => {
    expect(hubActivityActionKey("update_applied")).toBe("update_applied");
    expect(hubActivityActionKey("plugin_published")).toBe("plugin_published");
  });

  it("maps an action the client does not know to the generic label", () => {
    expect(hubActivityActionKey("future_thing")).toBe("other");
    expect(hubActivityActionKey("")).toBe("other");
  });
});

describe("hubActivityFacts", () => {
  it("reads the fields the Hub writes, in display order", () => {
    expect(
      hubActivityFacts(
        entry({ version: "2.0.0", checked: 4, updates_available: 1, path: "SKILL.md", file_count: 3 }),
      ),
    ).toEqual([
      { kind: "version", version: "2.0.0" },
      { kind: "files", count: 3 },
      { kind: "checked", count: 4 },
      { kind: "updates", count: 1 },
      { kind: "path", path: "SKILL.md" },
    ]);
  });

  it("ignores fields with the wrong type rather than showing them", () => {
    expect(hubActivityFacts(entry({ version: 2, count: "3", checked: -1, path: "  " }))).toEqual([]);
  });

  it("formats a byte size", () => {
    const [fact] = hubActivityFacts(entry({ size_bytes: 2048 }));
    expect(fact).toMatchObject({ kind: "size" });
    expect(fact && fact.kind === "size" ? fact.size : "").not.toBe("");
  });

  it("returns nothing for an entry without detail", () => {
    expect(hubActivityFacts(entry({}))).toEqual([]);
  });
});
