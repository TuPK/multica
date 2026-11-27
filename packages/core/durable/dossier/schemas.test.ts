// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import { parseSeatDossierEntry, parseSeatDossierList } from "./schemas";

const entryRow = {
  id: "d-1",
  agent_id: "a-1",
  kind: "pitfall",
  text: "Always check migration numbers against origin/main",
  work_class: "flow:pr:review",
  evidence: [{ type: "outcome", ref: "o-1" }],
  origin: "outcome",
  status: "active",
  created_at: "2026-11-26T09:00:00Z",
  retired_at: null,
};

describe("seat dossier schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("reads a dossier entry as the server sends it", () => {
    const [entry] = parseSeatDossierList({ entries: [entryRow] }, "GET /api/agents/:id/dossier");
    expect(entry).toMatchObject({
      id: "d-1",
      agent_id: "a-1",
      kind: "pitfall",
      work_class: "flow:pr:review",
      evidence: [{ type: "outcome", ref: "o-1" }],
      origin: "outcome",
      status: "active",
      retired_at: null,
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it("degrades unknown kind, origin, status and evidence type to safe fallbacks", () => {
    const [entry] = parseSeatDossierList(
      {
        entries: [
          {
            ...entryRow,
            kind: "mystery_kind",
            origin: "cron",
            status: "archived",
            evidence: [
              { type: "slack", ref: "msg-1" },
              { type: "outcome", ref: "" },
              null,
            ],
          },
        ],
      },
      "GET /api/agents/:id/dossier",
    );
    expect(entry).toMatchObject({
      kind: "lesson",
      origin: "agent",
      status: "active",
      evidence: [{ type: "document", ref: "msg-1" }],
    });
  });

  it("applies defaults for omitted optional fields", () => {
    const entry = parseSeatDossierEntry({ id: "d-2" }, "POST /api/agents/:id/dossier/:entryId/retire");
    expect(entry).toMatchObject({
      id: "d-2",
      agent_id: "",
      kind: "lesson",
      text: "",
      work_class: "",
      evidence: [],
      origin: "agent",
      status: "active",
      created_at: "",
      retired_at: null,
    });
  });

  it("drops malformed entries and keeps valid ones", () => {
    const entries = parseSeatDossierList(
      { entries: [null, 42, { text: "missing id" }, entryRow] },
      "GET /api/agents/:id/dossier",
    );
    expect(entries.map((e) => e.id)).toEqual(["d-1"]);
  });

  it("returns an empty list for a non-object or non-array body", () => {
    expect(parseSeatDossierList({ entries: "oops" }, "GET /api/agents/:id/dossier")).toEqual([]);
    expect(parseSeatDossierList(null, "GET /api/agents/:id/dossier")).toEqual([]);
  });

  it("returns null when a single entry response has no valid id", () => {
    expect(parseSeatDossierEntry({ text: "bad" }, "POST /api/agents/:id/dossier/:entryId/retire")).toBeNull();
    expect(parseSeatDossierEntry("oops", "POST /api/agents/:id/dossier/:entryId/retire")).toBeNull();
  });
});
