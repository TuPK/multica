// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { juryKeys, outcomeJuryOptions } from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

afterEach(() => {
  request.mockReset();
});

describe("jury query keys", () => {
  it("puts durable, wsId, jury, outcomeId in the key", () => {
    expect(juryKeys.outcome("ws-1", "o-1")).toEqual(["durable", "ws-1", "jury", "o-1"]);
    expect(outcomeJuryOptions("ws-1", "o-1").queryKey).toEqual(["durable", "ws-1", "jury", "o-1"]);
  });

  it("separates different workspaces and outcomes", () => {
    expect(outcomeJuryOptions("ws-1", "o-1").queryKey).not.toEqual(outcomeJuryOptions("ws-2", "o-1").queryKey);
    expect(outcomeJuryOptions("ws-1", "o-1").queryKey).not.toEqual(outcomeJuryOptions("ws-1", "o-2").queryKey);
  });
});

describe("outcome jury query", () => {
  it("calls GET /api/outcomes/{id}/jury and parses the panels", async () => {
    request.mockResolvedValueOnce({
      panels: [
        {
          id: "jp-1",
          criterion_id: "c-1",
          juror_agent_ids: ["a-2", "a-3"],
          diversity: "provider",
          state: "unanimous",
          verdict: "pass",
          resolved_by_type: "agent",
          created_at: "2026-11-26T09:00:00Z",
          closed_at: "2026-11-26T09:30:00Z",
          votes: [],
        },
      ],
    });
    const panels = await outcomeJuryOptions("ws-1", "o-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/outcomes/o-1/jury");
    expect(panels).toHaveLength(1);
    expect(panels[0]).toMatchObject({ id: "jp-1", state: "unanimous", verdict: "pass" });
  });

  it("throws when the server fails, so the card shows an error", async () => {
    request.mockRejectedValueOnce(new ApiError("boom", 500, "Internal Server Error"));
    await expect(outcomeJuryOptions("ws-1", "o-1").queryFn!({} as never)).rejects.toBeInstanceOf(ApiError);
  });

  it("disables the query when wsId or outcomeId is empty or enabled is false", () => {
    expect(outcomeJuryOptions("", "o-1").enabled).toBe(false);
    expect(outcomeJuryOptions("ws-1", "").enabled).toBe(false);
    expect(outcomeJuryOptions("ws-1", "o-1", false).enabled).toBe(false);
  });
});
