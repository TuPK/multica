// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import {
  principalDelegationsOptions,
  principalEvidenceCandidatesOptions,
  principalInterviewOptions,
  principalPredictionOptions,
  principalProfileOptions,
} from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const profileRow = {
  member_id: "u-1",
  status: "active",
  consent_at: "2026-11-26T08:00:00Z",
  share_with_owners: false,
  version: 1,
  traits: [],
};

afterEach(() => {
  request.mockReset();
});

describe("principal query keys", () => {
  it("scopes every query key by workspace id", () => {
    expect(principalProfileOptions("ws-1").queryKey.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(principalInterviewOptions("ws-1").queryKey.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(principalDelegationsOptions("ws-1").queryKey.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(principalEvidenceCandidatesOptions("ws-1").queryKey.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(principalPredictionOptions("ws-1", "proposal", "p-1").queryKey.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(principalProfileOptions("ws-1").queryKey).not.toEqual(principalProfileOptions("ws-2").queryKey);
    expect(principalPredictionOptions("ws-1", "proposal", "p-1").queryKey).not.toEqual(
      principalPredictionOptions("ws-2", "proposal", "p-1").queryKey,
    );
  });

  it("disables queries when wsId or subjectId is empty or flag is off", () => {
    expect(principalProfileOptions("").enabled).toBe(false);
    expect(principalInterviewOptions("").enabled).toBe(false);
    expect(principalInterviewOptions("ws-1", false).enabled).toBe(false);
    expect(principalDelegationsOptions("").enabled).toBe(false);
    expect(principalDelegationsOptions("ws-1", false).enabled).toBe(false);
    expect(principalEvidenceCandidatesOptions("").enabled).toBe(false);
    expect(principalEvidenceCandidatesOptions("ws-1", false).enabled).toBe(false);
    expect(principalPredictionOptions("", "proposal", "p-1").enabled).toBe(false);
    expect(principalPredictionOptions("ws-1", "proposal", "").enabled).toBe(false);
    expect(principalPredictionOptions("ws-1", "proposal", "p-1", false).enabled).toBe(false);
  });
});

describe("principal queries", () => {
  it("reads the profile from GET /api/principal/profile", async () => {
    request.mockResolvedValueOnce(profileRow);
    const out = await principalProfileOptions("ws-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/principal/profile");
    expect(out).toEqual(profileRow);
  });

  it("maps 404 to null (not enabled)", async () => {
    request.mockRejectedValueOnce(new ApiError("the person has no principal profile", 404, "Not Found"));
    await expect(principalProfileOptions("ws-1").queryFn!({} as never)).resolves.toBeNull();
  });

  it("rethrows non-404 errors", async () => {
    request.mockRejectedValueOnce(new ApiError("server error", 500, "Internal Server Error"));
    await expect(principalProfileOptions("ws-1").queryFn!({} as never)).rejects.toBeInstanceOf(ApiError);
  });

  it("reads the interview questions from GET /api/principal/interview", async () => {
    request.mockResolvedValueOnce({ questions: ["Question 1", "Question 2"] });
    const out = await principalInterviewOptions("ws-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/principal/interview");
    expect(out).toEqual({ questions: ["Question 1", "Question 2"] });
  });

  it("reads delegations and kind accuracy from GET /api/principal/delegations", async () => {
    const payload = {
      delegations: [
        {
          decision_kind: "proposal:general",
          enabled: true,
          min_confidence: 0.9,
          enabled_at: "2026-11-26T08:00:00Z",
          disabled_reason: "",
        },
      ],
      kinds: [{ decision_kind: "proposal:general", matched: 20, n: 22, lower: 0.72, eligible: true }],
    };
    request.mockResolvedValueOnce(payload);
    const out = await principalDelegationsOptions("ws-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/principal/delegations");
    expect(out).toEqual(payload);
  });

  it("reads evidence candidates from GET /api/principal/evidence-candidates", async () => {
    const payload = {
      decisions: [
        {
          id: "00000000-0000-0000-0000-000000000001",
          title: "Approve rollout",
          decision: "approved",
          decided_at: "2026-11-25T10:00:00Z",
        },
      ],
    };
    request.mockResolvedValueOnce(payload);
    const out = await principalEvidenceCandidatesOptions("ws-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/principal/evidence-candidates");
    expect(out).toEqual(payload);
  });

  it("reads a prediction from GET /api/principal/predictions and maps 404 to null", async () => {
    const pred = {
      predicted: "approve",
      confidence: 0.92,
      why: "Consistent with reversible rollout traits",
      actual: null,
      matched: null,
    };
    request.mockResolvedValueOnce(pred);
    const out = await principalPredictionOptions("ws-1", "proposal", "p-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/principal/predictions?subject_type=proposal&subject_id=p-1");
    expect(out).toEqual(pred);

    request.mockRejectedValueOnce(new ApiError("no prediction", 404, "Not Found"));
    await expect(
      principalPredictionOptions("ws-1", "effect_intent", "i-1").queryFn!({} as never),
    ).resolves.toBeNull();
  });
});
