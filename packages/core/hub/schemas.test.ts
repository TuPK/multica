// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_HUB_IMPORT_COMMIT_RESULT,
  EMPTY_HUB_IMPORT_PREVIEW,
  HubImportCommitResultSchema,
  HubImportPreviewSchema,
  HubSourceListSchema,
  HubSourceSchema,
} from "../api/schemas";

const SOURCE = {
  id: "src-1",
  name: "Team skills",
  type: "github",
  url: "https://github.com/acme/skills",
  config: { branch: "main" },
  enabled: false,
  auto_sync_interval_sec: 3600,
  last_sync_at: "2026-02-01T00:00:00Z",
  last_sync_status: "error",
  last_sync_error: "rate limited",
  item_count: 4,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-02-01T00:00:00Z",
};

describe("HubSourceSchema", () => {
  it("parses a complete source", () => {
    const parsed = HubSourceSchema.parse(SOURCE);
    expect(parsed).toMatchObject({ id: "src-1", type: "github", enabled: false, last_sync_error: "rate limited" });
    expect(parsed.config).toEqual({ branch: "main" });
  });

  it("fills defaults for missing optional fields", () => {
    const parsed = HubSourceSchema.parse({ id: "src-2", type: "clawhub" });
    expect(parsed).toMatchObject({
      name: "",
      url: "",
      config: {},
      enabled: true,
      auto_sync_interval_sec: 0,
      last_sync_at: null,
      last_sync_status: "",
      last_sync_error: null,
      item_count: 0,
    });
  });

  it("shows an unknown source type as manual", () => {
    expect(HubSourceSchema.parse({ ...SOURCE, type: "gitlab" }).type).toBe("manual");
  });

  it("accepts the add-flow source types", () => {
    for (const type of ["url_zip", "registry", "upload"]) {
      expect(HubSourceSchema.parse({ ...SOURCE, type }).type).toBe(type);
    }
  });

  it("replaces a non-object config with an empty object", () => {
    expect(HubSourceSchema.parse({ ...SOURCE, config: "oops" }).config).toEqual({});
  });
});

describe("HubSourceListSchema", () => {
  it("rejects a non-array body, which the client turns into an empty list", () => {
    expect(HubSourceListSchema.safeParse({ sources: [] }).success).toBe(false);
  });
});

describe("HubImportPreviewSchema", () => {
  const PREVIEW = {
    preview_id: "pv-1",
    kind: "skill",
    name: "Writer",
    description: "Writes things",
    version: "1.2.0",
    files: [{ path: "SKILL.md", size_bytes: 120 }],
    scan: {
      status: "warn",
      findings: [{ rule: "net", severity: "warn", path: "SKILL.md", message: "network call" }],
    },
    conflict: { exists: true, item_id: "sk-9", kind: "skill" },
  };

  it("parses a complete preview", () => {
    const parsed = HubImportPreviewSchema.parse(PREVIEW);
    expect(parsed.scan.findings[0]).toMatchObject({ severity: "warn" });
    expect(parsed.conflict).toEqual({ exists: true, item_id: "sk-9", kind: "skill" });
    expect(parsed.registry).toBeUndefined();
  });

  it("maps an unknown finding severity to info", () => {
    const parsed = HubImportPreviewSchema.parse({
      ...PREVIEW,
      scan: { status: "clean", findings: [{ rule: "x", severity: "critical", path: "", message: "" }] },
    });
    expect(parsed.scan.findings[0]?.severity).toBe("info");
  });

  it("treats an unknown scan status as warn so a commit still needs confirmation", () => {
    const parsed = HubImportPreviewSchema.parse({ ...PREVIEW, scan: { status: "quarantined" } });
    expect(parsed.scan.status).toBe("warn");
    expect(HubImportPreviewSchema.parse({ ...PREVIEW, scan: undefined }).scan).toEqual({ status: "warn", findings: [] });
  });

  it("keeps the registry index for registry previews", () => {
    const parsed = HubImportPreviewSchema.parse({
      preview_id: "pv-2",
      kind: "registry",
      name: "",
      registry: {
        name: "Acme index",
        items: [{ kind: "plugin", name: "tool", url: "https://x/y.zip", sha256: "abc", tags: ["ops"] }, { kind: "widget", name: "w" }],
      },
    });
    expect(parsed.kind).toBe("registry");
    expect(parsed.registry?.items).toHaveLength(2);
    expect(parsed.registry?.items[0]).toMatchObject({ kind: "plugin", version: null, tags: ["ops"] });
    expect(parsed.registry?.items[1]?.kind).toBe("skill");
  });

  it("drops an unknown conflict kind without rejecting the preview", () => {
    const parsed = HubImportPreviewSchema.parse({ ...PREVIEW, conflict: { exists: true, kind: "agent" } });
    expect(parsed.conflict.exists).toBe(true);
    expect(parsed.conflict.kind).toBeUndefined();
  });

  it("falls back to the empty preview when the body is not an object", () => {
    expect(HubImportPreviewSchema.safeParse(null).success).toBe(false);
    expect(EMPTY_HUB_IMPORT_PREVIEW.preview_id).toBe("");
  });
});

describe("HubImportCommitResultSchema", () => {
  it("parses the commit result and fills defaults", () => {
    expect(HubImportCommitResultSchema.parse({ kind: "plugin", id: "p1", hub_item_id: "h1", skipped: true })).toEqual({
      kind: "plugin",
      id: "p1",
      hub_item_id: "h1",
      skipped: true,
    });
    expect(HubImportCommitResultSchema.parse({ id: "s1" })).toMatchObject({ kind: "skill", skipped: false });
    expect(EMPTY_HUB_IMPORT_COMMIT_RESULT.id).toBe("");
  });
});
