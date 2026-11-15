import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError, errorCode } from "../api/client";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respond(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("ApiClient.previewAccountDeletion", () => {
  it("POSTs with preview=1 and parses the list", async () => {
    const fetchMock = respond({ sole_workspaces: [{ id: "w1", name: "Solo" }], scheduled_at: null });

    const result = await new ApiClient("https://api.test").previewAccountDeletion();

    expect(result).toEqual({ sole_workspaces: [{ id: "w1", name: "Solo" }], scheduled_at: null });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/deletion?preview=1");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
  });

  it("falls back to an empty preview for a malformed body", async () => {
    respond({ sole_workspaces: 5 });

    expect(await new ApiClient("https://api.test").previewAccountDeletion()).toEqual({
      sole_workspaces: [],
      scheduled_at: null,
    });
  });

  it("surfaces the sole_owner body on the error", async () => {
    respond({ error: "blocked", code: "sole_owner", workspaces: [{ id: "w1", name: "T", member_count: 2 }] }, 409);

    const err = await new ApiClient("https://api.test").previewAccountDeletion().catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(errorCode(err)).toBe("sole_owner");
    expect((err as ApiError).body).toMatchObject({ workspaces: [{ id: "w1", member_count: 2 }] });
  });
});

describe("ApiClient.requestAccountDeletion", () => {
  it("POSTs without preview and parses the date", async () => {
    const fetchMock = respond({ scheduled_at: "2030-01-15T10:00:00Z" }, 202);

    const result = await new ApiClient("https://api.test").requestAccountDeletion();

    expect(result).toEqual({ scheduled_at: "2030-01-15T10:00:00Z" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/deletion");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
  });

  it("keeps an unreadable body from failing a request that succeeded", async () => {
    respond({ scheduled_at: 4 }, 202);

    expect(await new ApiClient("https://api.test").requestAccountDeletion()).toEqual({ scheduled_at: "" });
  });

  it("surfaces recent_login_required", async () => {
    respond({ error: "sign in again", code: "recent_login_required" }, 403);

    const err = await new ApiClient("https://api.test").requestAccountDeletion().catch((e: unknown) => e);

    expect(errorCode(err)).toBe("recent_login_required");
  });
});

describe("ApiClient.cancelAccountDeletion", () => {
  it("DELETEs the endpoint and parses the status", async () => {
    const fetchMock = respond({ status: "active" });

    const result = await new ApiClient("https://api.test").cancelAccountDeletion();

    expect(result).toEqual({ status: "active" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.test/api/me/deletion");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("DELETE");
  });

  it("surfaces not_pending_deletion", async () => {
    respond({ error: "not pending", code: "not_pending_deletion" }, 409);

    const err = await new ApiClient("https://api.test").cancelAccountDeletion().catch((e: unknown) => e);

    expect(errorCode(err)).toBe("not_pending_deletion");
  });
});
