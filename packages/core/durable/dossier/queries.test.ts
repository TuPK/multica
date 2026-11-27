// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { dossierKeys, seatDossierListOptions } from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

afterEach(() => {
  request.mockReset();
});

describe("seat dossier query keys", () => {
  it("include workspace id and agent id in every key", () => {
    const key = seatDossierListOptions("ws-1", "a-1", { status: "active" }).queryKey;
    expect(key.slice(0, 4)).toEqual(["durable", "ws-1", "dossier", "a-1"]);
    expect(dossierKeys.agent("ws-1", "a-1")).toEqual(["durable", "ws-1", "dossier", "a-1"]);
  });

  it("separate workspaces, agents, and status filters", () => {
    expect(seatDossierListOptions("ws-1", "a-1", { status: "active" }).queryKey).not.toEqual(
      seatDossierListOptions("ws-2", "a-1", { status: "active" }).queryKey,
    );
    expect(seatDossierListOptions("ws-1", "a-1", { status: "active" }).queryKey).not.toEqual(
      seatDossierListOptions("ws-1", "a-2", { status: "active" }).queryKey,
    );
    expect(seatDossierListOptions("ws-1", "a-1", { status: "active" }).queryKey).not.toEqual(
      seatDossierListOptions("ws-1", "a-1", { status: "retired" }).queryKey,
    );
  });
});

describe("seat dossier queries", () => {
  it("requests the agent dossier route with non-empty filters and parses entries", async () => {
    request.mockResolvedValueOnce({
      entries: [
        {
          id: "d-1",
          agent_id: "a-1",
          kind: "lesson",
          text: "Check logs first",
          work_class: "",
          evidence: [{ type: "comment", ref: "c-1" }],
          origin: "member",
          status: "active",
          created_at: "2026-11-26T09:00:00Z",
          retired_at: null,
        },
      ],
    });
    const rows = await seatDossierListOptions("ws-1", "a-1", { status: "retired", limit: 50 }).queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/dossier?status=retired&limit=50");
    expect(rows).toHaveLength(1);
    expect(rows[0]?.id).toBe("d-1");
  });

  it("stays disabled without a workspace or agent id", () => {
    expect(seatDossierListOptions("", "a-1").enabled).toBe(false);
    expect(seatDossierListOptions("ws-1", "").enabled).toBe(false);
  });
});
