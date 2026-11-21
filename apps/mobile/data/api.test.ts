// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, ApiError } from "./api";

// api.ts refuses to load without a base URL; set it before the import runs.
vi.hoisted(() => {
  process.env.EXPO_PUBLIC_API_URL = "https://api.example.test";
});

// The real store pulls in expo-secure-store; the client only needs the slug.
vi.mock("@/data/workspace-store", () => ({ getCurrentSlug: () => null }));

describe("api.listAgents", () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    [undefined, "/api/agents"],
    [{ includeArchived: false }, "/api/agents"],
    [{ includeArchived: true }, "/api/agents?include_archived=true"],
  ])("with %j requests %s", async (opts, path) => {
    const fetchMock = vi.fn(async () => Response.json([]));
    vi.stubGlobal("fetch", fetchMock);
    await expect(api.listAgents(opts)).resolves.toEqual([]);
    expect(fetchMock).toHaveBeenCalledExactlyOnceWith(`https://api.example.test${path}`, expect.any(Object));
  });
});

// The server leaves guests out of the member list unless the caller declares
// it can render them; useIsGuest reads the caller's own row from that list.
describe("api.listMembers", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("advertises the guest capability", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => Response.json([]));
    vi.stubGlobal("fetch", fetchMock);
    await api.listMembers("ws-1");
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["X-Client-Capabilities"]).toBe("workspace-guests-v1");
  });
});

describe("api.deleteComment", () => {
  const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));

  beforeEach(() => {
    fetchMock.mockClear();
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(console, "log").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  // #8296: servers that keep a deleted comment's replies also route
  // /keep-replies; older servers do not, so a keep-replies delete that reaches
  // one fails instead of deleting the replies too.
  it.each([
    [{ keepReplies: true }, "https://api.example.test/api/comments/comment-1/keep-replies"],
    [{ keepReplies: false }, "https://api.example.test/api/comments/comment-1"],
    [undefined, "https://api.example.test/api/comments/comment-1"],
  ])("with %j sends DELETE %s", async (opts, url) => {
    await api.deleteComment("comment-1", opts);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(url, expect.objectContaining({ method: "DELETE" }));
  });
});

describe("ApiClient error codes and refusal hooks", () => {
  beforeEach(() => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.spyOn(console, "warn").mockImplementation(() => {});
  });

  afterEach(() => {
    api.setOptions({
      onUnauthorized: undefined,
      onAccountSuspended: undefined,
      onWorkspaceRefused: undefined,
    });
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("extracts ApiError.code from JSON error body while keeping message unchanged", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          { error: "raw server text", code: "plan_limit_reached", gate: "agents", limit: 3, used: 3 },
          { status: 402, statusText: "Payment Required" },
        ),
      ),
    );

    let thrown: unknown;
    try {
      await api.getMe();
    } catch (err) {
      thrown = err;
    }

    expect(thrown).toBeInstanceOf(ApiError);
    expect((thrown as ApiError).status).toBe(402);
    expect((thrown as ApiError).code).toBe("plan_limit_reached");
    expect((thrown as ApiError).message).not.toBe("raw server text");
  });

  it("calls onUnauthorized with session_revoked on 401 for API routes", async () => {
    const onUnauthorized = vi.fn();
    api.setOptions({ onUnauthorized });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          { error: "session revoked", code: "session_revoked" },
          { status: 401 },
        ),
      ),
    );

    await expect(api.getMe()).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).toHaveBeenCalledExactlyOnceWith("session_revoked");
  });

  it("does NOT call onUnauthorized for 401 from /auth/* routes such as /auth/mfa/verify", async () => {
    const onUnauthorized = vi.fn();
    api.setOptions({ onUnauthorized });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          { error: "token expired", code: "mfa_token_invalid" },
          { status: 401 },
        ),
      ),
    );

    await expect(api.verifyMFA("mfa-tok", "123456")).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).not.toHaveBeenCalled();
  });

  it("calls onAccountSuspended on 403 account_suspended", async () => {
    const onAccountSuspended = vi.fn();
    const onWorkspaceRefused = vi.fn();
    api.setOptions({ onAccountSuspended, onWorkspaceRefused });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          { error: "account suspended", code: "account_suspended" },
          { status: 403 },
        ),
      ),
    );

    await expect(api.getMe()).rejects.toBeInstanceOf(ApiError);
    expect(onAccountSuspended).toHaveBeenCalledTimes(1);
    expect(onWorkspaceRefused).not.toHaveBeenCalled();
  });

  it.each([
    "workspace_read_only",
    "workspace_suspended",
    "workspace_pending_deletion",
  ] as const)("calls onWorkspaceRefused(%s) on 403", async (code) => {
    const onWorkspaceRefused = vi.fn();
    api.setOptions({ onWorkspaceRefused });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ error: "blocked", code }, { status: 403 })),
    );

    await expect(api.listWorkspaces()).rejects.toBeInstanceOf(ApiError);
    expect(onWorkspaceRefused).toHaveBeenCalledExactlyOnceWith(code);
  });

  it("supports verifyCode returning an MFA challenge and verifyMFA completing login", async () => {
    const user = {
      id: "u1",
      name: "Alex",
      email: "alex@example.com",
      avatar_url: null,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          mfa_required: true,
          mfa_token: "mfa-tok-123",
          expires_in: 300,
        }),
      )
      .mockResolvedValueOnce(Response.json({ token: "session-jwt", user }));
    vi.stubGlobal("fetch", fetchMock);

    const challenge = await api.verifyCode("alex@example.com", "123456");
    expect(challenge).toEqual({
      mfa_required: true,
      mfa_token: "mfa-tok-123",
      expires_in: 300,
    });

    const login = await api.verifyMFA("mfa-tok-123", "654321");
    expect(login).toEqual({ token: "session-jwt", user });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "https://api.example.test/auth/mfa/verify",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ mfa_token: "mfa-tok-123", code: "654321" }),
      }),
    );
  });

  it("wires 401 session_revoked and 403 refusals on uploadFile", async () => {
    const onUnauthorized = vi.fn();
    const onAccountSuspended = vi.fn();
    const onWorkspaceRefused = vi.fn();
    api.setOptions({ onUnauthorized, onAccountSuspended, onWorkspaceRefused });

    const asset = { uri: "file:///tmp/a.png", name: "a.png", type: "image/png" };
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ code: "session_revoked" }, { status: 401 }),
      ),
    );
    await expect(api.uploadFile(asset)).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).toHaveBeenCalledExactlyOnceWith("session_revoked");

    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ code: "workspace_read_only" }, { status: 403 }),
      ),
    );
    await expect(api.uploadFile(asset)).rejects.toBeInstanceOf(ApiError);
    expect(onWorkspaceRefused).toHaveBeenCalledExactlyOnceWith("workspace_read_only");
  });
});


describe("api.listIssues requester compatibility", () => {
  afterEach(() => vi.unstubAllGlobals());
  it.each([false, true])("checks server support before returning Created rows (supported=%s)", async (supported) => {
    const fetchMock = vi.fn(async (_url: string) => Response.json({ issues: [], total: 0, ...(supported ? { created_by_user_id: "me" } : {}) }));
    vi.stubGlobal("fetch", fetchMock);
    await api.listIssues({ created_by_user_id: "me" });
    expect(fetchMock).toHaveBeenCalledTimes(supported ? 1 : 2);
    if (!supported) expect(String(fetchMock.mock.calls[1]?.[0])).toContain("creator_id=me");
  });
});

describe("api.sendChatMessage steering", () => {
  const accepted = {
    message_id: "message-1",
    task_id: "task-1",
    created_at: "2026-11-16T00:00:00Z",
  };

  afterEach(() => vi.unstubAllGlobals());

  // The server decides whether a send can join the running reply; the client
  // only states intent. Default sends ask to steer, the explicit queue does not.
  it.each([
    [undefined, true],
    [{ attachmentIds: ["attachment-1"] }, true],
    [{ queue: false }, true],
    [{ queue: true }, false],
  ])("with %j sends steer=%s", async (opts, steer) => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      Response.json(accepted, { status: 201 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.sendChatMessage("session-1", "use the new path", opts);

    const body = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(body.steer).toBe(steer);
    expect(body.content).toBe("use the new path");
  });
});
