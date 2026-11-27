// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { principalInterviewOptions, principalProfileOptions } from "./queries";

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
    expect(principalProfileOptions("ws-1").queryKey).not.toEqual(principalProfileOptions("ws-2").queryKey);
  });

  it("disables queries when wsId is empty", () => {
    expect(principalProfileOptions("").enabled).toBe(false);
    expect(principalInterviewOptions("").enabled).toBe(false);
    expect(principalInterviewOptions("ws-1", false).enabled).toBe(false);
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
});
