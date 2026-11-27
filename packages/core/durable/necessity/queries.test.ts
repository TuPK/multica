// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { necessityKeys, necessityTestsOptions, seatSkillsOptions } from "./queries";

const { request, listAgentSkills } = vi.hoisted(() => ({
  request: vi.fn(),
  listAgentSkills: vi.fn(),
}));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request, listAgentSkills },
}));

afterEach(() => {
  request.mockReset();
  listAgentSkills.mockReset();
});

describe("necessity query keys", () => {
  it("puts durable, wsId, necessity, and agentId in the key", () => {
    expect(necessityKeys.list("ws-1", "a-1")).toEqual(["durable", "ws-1", "necessity", "a-1", "list"]);
    expect(necessityTestsOptions("ws-1", "a-1").queryKey).toEqual([
      "durable",
      "ws-1",
      "necessity",
      "a-1",
      "list",
    ]);
    expect(seatSkillsOptions("ws-1", "a-1").queryKey).toEqual([
      "durable",
      "ws-1",
      "necessity",
      "a-1",
      "skills",
    ]);
  });

  it("separates different workspaces and seats", () => {
    expect(necessityTestsOptions("ws-1", "a-1").queryKey).not.toEqual(
      necessityTestsOptions("ws-2", "a-1").queryKey,
    );
    expect(necessityTestsOptions("ws-1", "a-1").queryKey).not.toEqual(
      necessityTestsOptions("ws-1", "a-2").queryKey,
    );
  });
});

describe("necessityTestsOptions", () => {
  it("calls GET /api/agents/{id}/necessity and parses the list", async () => {
    request.mockResolvedValueOnce({
      tests: [
        {
          id: "nt-1",
          subject_type: "skill",
          subject_ref: "sk-1",
          agent_id: "a-1",
          state: "not_needed",
          result: { pairs: 40, primary: 1, shadow: 5, tie: 34 },
          proposal_id: "p-1",
          created_at: "2026-11-26T09:00:00Z",
          completed_at: "2026-11-27T01:00:00Z",
        },
      ],
    });

    const tests = await necessityTestsOptions("ws-1", "a-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/necessity");
    expect(tests).toHaveLength(1);
    expect(tests[0]).toMatchObject({ id: "nt-1", state: "not_needed", proposal_id: "p-1" });
  });

  it("throws when the server fails so the view shows an error", async () => {
    request.mockRejectedValueOnce(new ApiError("boom", 500, "Internal Server Error"));
    await expect(necessityTestsOptions("ws-1", "a-1").queryFn!({} as never)).rejects.toBeInstanceOf(
      ApiError,
    );
  });

  it("disables the query when wsId or agentId is empty or enabled is false", () => {
    expect(necessityTestsOptions("", "a-1").enabled).toBe(false);
    expect(necessityTestsOptions("ws-1", "").enabled).toBe(false);
    expect(necessityTestsOptions("ws-1", "a-1", false).enabled).toBe(false);
  });
});

describe("seatSkillsOptions", () => {
  it("calls api.listAgentSkills and parses the seat's skills", async () => {
    listAgentSkills.mockResolvedValueOnce([{ id: "sk-1", name: "Deploy" }]);
    const skills = await seatSkillsOptions("ws-1", "a-1").queryFn!({} as never);
    expect(listAgentSkills).toHaveBeenCalledWith("a-1");
    expect(skills).toEqual([{ id: "sk-1", name: "Deploy" }]);
  });

  it("disables the query when wsId or agentId is empty or enabled is false", () => {
    expect(seatSkillsOptions("", "a-1").enabled).toBe(false);
    expect(seatSkillsOptions("ws-1", "").enabled).toBe(false);
    expect(seatSkillsOptions("ws-1", "a-1", false).enabled).toBe(false);
  });
});
