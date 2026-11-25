// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import {
  issueCriteriaOptions,
  issueOutcomeOptions,
  outcomeListOptions,
  outcomeOptions,
  outcomeSummaryOptions,
} from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const outcomeRow = {
  id: "o-1",
  issue_id: "i-1",
  verdict: "pass",
  severity: "none",
  fail_cause: "",
  verification_mode: "full",
  sampled: false,
  settle_at: "2026-11-20T10:00:00Z",
  created_at: "2026-11-18T10:00:00Z",
};

describe("outcome query keys", () => {
  it("put the workspace second in every key, so one workspace never reads another's cache", () => {
    const keys = [
      issueOutcomeOptions("ws-1", "i-1").queryKey,
      issueCriteriaOptions("ws-1", "i-1").queryKey,
      outcomeListOptions("ws-1", { verdict: "fail" }).queryKey,
      outcomeOptions("ws-1", "o-1").queryKey,
      outcomeSummaryOptions("ws-1", { group_by: "agent", period: "week" }).queryKey,
    ];
    for (const key of keys) {
      expect(key.slice(0, 2)).toEqual(["durable", "ws-1"]);
    }
  });

  it("separates two workspaces that look at the same issue", () => {
    expect(issueOutcomeOptions("ws-1", "i-1").queryKey).not.toEqual(issueOutcomeOptions("ws-2", "i-1").queryKey);
    expect(outcomeListOptions("ws-1", {}).queryKey).not.toEqual(outcomeListOptions("ws-2", {}).queryKey);
  });

  it("keys a filtered list by its filters", () => {
    expect(outcomeListOptions("ws-1", { verdict: "fail" }).queryKey).not.toEqual(
      outcomeListOptions("ws-1", { verdict: "pass" }).queryKey,
    );
  });
});

describe("outcome queries", () => {
  it("reads an issue's outcome from its route and parses it", async () => {
    request.mockResolvedValueOnce(outcomeRow);
    const out = await issueOutcomeOptions("ws-1", "i-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/issues/i-1/outcome");
    expect(out).toMatchObject({ id: "o-1", verdict: "pass" });
  });

  it("reads 404 as no outcome yet rather than an error", async () => {
    request.mockRejectedValueOnce(new ApiError("outcome_not_found", 404, "Not Found", { code: "outcome_not_found" }));
    await expect(issueOutcomeOptions("ws-1", "i-1").queryFn!({} as never)).resolves.toBeNull();
  });

  it("still throws on a server failure", async () => {
    request.mockRejectedValueOnce(new ApiError("boom", 500, "Internal Server Error"));
    await expect(issueOutcomeOptions("ws-1", "i-1").queryFn!({} as never)).rejects.toBeInstanceOf(ApiError);
  });

  it("builds the list query string without empty filters", async () => {
    request.mockResolvedValueOnce({ outcomes: [outcomeRow], next_cursor: "" });
    const page = await outcomeListOptions("ws-1", { verdict: "fail", agent_id: "a-1", since: undefined, work_class: "" }).queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/outcomes?verdict=fail&agent_id=a-1");
    expect(page.outcomes[0]?.id).toBe("o-1");
  });

  it("reads a criteria list as its items", async () => {
    request.mockResolvedValueOnce({ criteria: [] });
    await expect(issueCriteriaOptions("ws-1", "i-1").queryFn!({} as never)).resolves.toEqual([]);
    expect(request).toHaveBeenCalledWith("/api/issues/i-1/criteria");
  });

  it("does not run without a workspace or an issue", () => {
    expect(issueOutcomeOptions("", "i-1").enabled).toBe(false);
    expect(issueCriteriaOptions("ws-1", "").enabled).toBe(false);
  });
});
