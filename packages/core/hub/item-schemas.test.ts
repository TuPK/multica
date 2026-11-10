// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  HubActivityPageSchema,
  HubApplyUpdateResultSchema,
  HubBundleSchema,
  HubItemMetaListSchema,
  HubItemMetaSchema,
  HubQuotaSchema,
  HubSyncResultSchema,
  HubUpdateCheckSchema,
  HubUpdateDiffSchema,
} from "../api/schemas";

const ITEM = {
  id: "hi-1",
  kind: "skill",
  ref_id: "sk-1",
  source_id: "src-1",
  source_ref: "acme/writer@v1",
  tracking: "tracking",
  latest_version: "1.2.0",
  bundle_digest: "sha256:aaa",
  size_bytes: 2048,
  file_count: 3,
  scan_status: "clean",
  update_available: true,
  remote_digest: "sha256:bbb",
  checked_at: "2026-03-01T00:00:00Z",
  local_modified: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-03-01T00:00:00Z",
};

describe("HubItemMetaSchema", () => {
  it("parses a complete item", () => {
    expect(HubItemMetaSchema.parse(ITEM)).toMatchObject({
      ref_id: "sk-1",
      tracking: "tracking",
      scan_status: "clean",
      update_available: true,
      checked_at: "2026-03-01T00:00:00Z",
    });
  });

  it.each(["tracking", "forked", "manual"] as const)("keeps tracking %s", (tracking) => {
    expect(HubItemMetaSchema.parse({ ...ITEM, tracking }).tracking).toBe(tracking);
  });

  it("shows an unknown tracking mode as manual", () => {
    expect(HubItemMetaSchema.parse({ ...ITEM, tracking: "mirrored" }).tracking).toBe("manual");
    expect(HubItemMetaSchema.parse({ ...ITEM, tracking: null }).tracking).toBe("manual");
  });

  it("shows an unknown or missing scan status as warn", () => {
    expect(HubItemMetaSchema.parse({ ...ITEM, scan_status: "pending" }).scan_status).toBe("warn");
    expect(HubItemMetaSchema.parse({ ...ITEM, scan_status: undefined }).scan_status).toBe("warn");
  });

  it("falls back for a missing or wrongly typed flag instead of trusting it", () => {
    const parsed = HubItemMetaSchema.parse({ ...ITEM, update_available: "yes", local_modified: 1 });
    expect(parsed.update_available).toBe(false);
    expect(parsed.local_modified).toBe(false);
  });

  it("fills defaults for missing fields", () => {
    const parsed = HubItemMetaSchema.parse({ id: "hi-2" });
    expect(parsed).toMatchObject({
      id: "hi-2",
      kind: "skill",
      ref_id: "",
      source_id: null,
      tracking: "manual",
      size_bytes: 0,
      file_count: 0,
      scan_status: "warn",
      checked_at: null,
      update_available: false,
    });
  });

  it("shows an unknown kind as skill rather than inventing a plugin", () => {
    expect(HubItemMetaSchema.parse({ ...ITEM, kind: "agent" }).kind).toBe("skill");
  });
});

describe("HubItemMetaListSchema", () => {
  it("rejects a non-array body", () => {
    expect(HubItemMetaListSchema.safeParse({ items: [] }).success).toBe(false);
  });
});

describe("HubBundleSchema", () => {
  it("parses a presigned bundle and fills numeric defaults", () => {
    expect(HubBundleSchema.parse({ url: "https://s3/x", expires_in: 900, digest: "d", size_bytes: 10, version: "1" })).toMatchObject({
      url: "https://s3/x",
      expires_in: 900,
    });
    expect(HubBundleSchema.parse({ url: "/api/hub/items/skill/sk/bundle/download", size_bytes: "big" })).toMatchObject({
      url: "/api/hub/items/skill/sk/bundle/download",
      size_bytes: 0,
    });
  });
});

describe("HubSyncResultSchema", () => {
  it("parses a result with per-item changes", () => {
    const parsed = HubSyncResultSchema.parse({
      status: "ok",
      checked: 2,
      updates_available: 1,
      unchanged: 1,
      items: [
        { kind: "plugin", id: "p1", name: "Hello", change: "update_available" },
        { kind: "skill", id: "s1", name: "Writer", change: "error", error: "404" },
      ],
    });
    expect(parsed.items[1]).toMatchObject({ change: "error", error: "404" });
  });

  it("falls back to an error status for an unknown status or change", () => {
    const parsed = HubSyncResultSchema.parse({
      status: "partial",
      checked: 1,
      items: [{ kind: "skill", id: "s1", change: "rewritten" }],
    });
    expect(parsed.status).toBe("error");
    expect(parsed.items[0]?.change).toBe("error");
  });

  it("drops a non-array items field to an empty list", () => {
    expect(HubSyncResultSchema.parse({ status: "ok", items: "nope" }).items).toEqual([]);
  });
});

describe("HubUpdateCheckSchema", () => {
  it("shows an unknown file status as modified so it is reviewed", () => {
    const parsed = HubUpdateCheckSchema.parse({
      update_available: true,
      files: [{ path: "a.md", status: "renamed", old_size: 1, new_size: null }],
    });
    expect(parsed.files[0]).toMatchObject({ path: "a.md", status: "modified", old_size: 1, new_size: null });
    expect(parsed.local_modified).toBe(false);
  });
});

describe("HubUpdateDiffSchema", () => {
  it("parses a binary diff with null text", () => {
    expect(HubUpdateDiffSchema.parse({ path: "logo.png", old: null, new: null, binary: true })).toMatchObject({
      binary: true,
      old: null,
    });
  });
});

describe("HubApplyUpdateResultSchema", () => {
  it("maps an unknown tracking mode to manual", () => {
    expect(
      HubApplyUpdateResultSchema.parse({ kind: "plugin", id: "p1", version: "2.0.0", tracking: "mirror" }).tracking,
    ).toBe("manual");
  });
});

describe("HubActivityPageSchema", () => {
  it("parses entries and a cursor, with unknown kinds shown as null", () => {
    const parsed = HubActivityPageSchema.parse({
      entries: [
        {
          id: "e1",
          action: "update_applied",
          actor_type: "member",
          actor_id: "u1",
          actor_name: "Ada",
          kind: "widget",
          ref_id: null,
          item_name: "Writer",
          source_id: "src-1",
          detail: { version: "1.2.0" },
          created_at: "2026-03-01T00:00:00Z",
        },
      ],
      next_cursor: "c-2",
    });
    expect(parsed.entries[0]).toMatchObject({ kind: null, detail: { version: "1.2.0" } });
    expect(parsed.next_cursor).toBe("c-2");
  });

  it("treats a missing cursor as the last page", () => {
    expect(HubActivityPageSchema.parse({ entries: [] }).next_cursor).toBeNull();
  });

  it("replaces a non-object detail with an empty object", () => {
    const parsed = HubActivityPageSchema.parse({ entries: [{ id: "e2", detail: "oops" }] });
    expect(parsed.entries[0]?.detail).toEqual({});
  });
});

describe("HubQuotaSchema", () => {
  it("parses usage and limits", () => {
    expect(
      HubQuotaSchema.parse({ items: { used: 3, limit: 50 }, storage_bytes: { used: 10, limit: 0 } }),
    ).toEqual({ items: { used: 3, limit: 50 }, storage_bytes: { used: 10, limit: 0 } });
  });

  it("falls back to zero usage for a malformed section", () => {
    expect(HubQuotaSchema.parse({ items: "many", storage_bytes: { used: 1, limit: 2 } })).toEqual({
      items: { used: 0, limit: 0 },
      storage_bytes: { used: 1, limit: 2 },
    });
  });
});
