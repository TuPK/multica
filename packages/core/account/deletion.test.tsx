// @vitest-environment jsdom
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError, errorCode } from "../api/client";
import { EMPTY_USER, UserSchema } from "../api/schemas";
import { parseWithFallback } from "../api/schema";
import type { User } from "../types/workspace";
import {
  RecentLoginRequiredError,
  SoleOwnerError,
  isRecentLoginRequiredError,
  isSoleOwnerError,
  useCancelAccountDeletion,
  usePreviewAccountDeletion,
  useRequestAccountDeletion,
} from "./deletion";

const api = vi.hoisted(() => ({
  previewAccountDeletion: vi.fn(),
  requestAccountDeletion: vi.fn(),
  cancelAccountDeletion: vi.fn(),
}));
const authState = vi.hoisted(() => ({ user: null as unknown, setUser: vi.fn() }));

vi.mock("../api", () => ({ api }));
vi.mock("../auth", () => ({ useAuthStore: { getState: () => authState } }));

function wrapper(queryClient: QueryClient) {
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }
  return Wrapper;
}

function parseUser(raw: unknown): User {
  return parseWithFallback<User>(raw, UserSchema, EMPTY_USER, { endpoint: "GET /api/me" });
}

describe("UserSchema deletion_scheduled_at", () => {
  const base = { id: "u1", name: "Ann", email: "ann@x.test" };

  it("keeps the date while the account is pending", () => {
    expect(parseUser({ ...base, deletion_scheduled_at: "2030-01-15T10:00:00Z" }).deletion_scheduled_at).toBe(
      "2030-01-15T10:00:00Z",
    );
  });

  it("leaves the field absent when the server omits it", () => {
    const user = parseUser(base);
    expect(user.id).toBe("u1");
    expect(user.deletion_scheduled_at).toBeUndefined();
  });

  it("accepts null", () => {
    expect(parseUser({ ...base, deletion_scheduled_at: null }).deletion_scheduled_at).toBeNull();
  });

  it("drops a malformed value without discarding the user", () => {
    const user = parseUser({ ...base, deletion_scheduled_at: 12 });
    expect(user.id).toBe("u1");
    expect(user.deletion_scheduled_at).toBeUndefined();
  });
});

describe("usePreviewAccountDeletion", () => {
  let queryClient: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("returns the workspaces that would be deleted", async () => {
    const preview = { sole_workspaces: [{ id: "w1", name: "Solo" }], scheduled_at: null };
    api.previewAccountDeletion.mockResolvedValue(preview);
    const { result } = renderHook(() => usePreviewAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(preview);
  });

  it("maps the sole_owner 409 to an error that carries the workspaces", async () => {
    const workspaces = [{ id: "w1", name: "Team", member_count: 3 }];
    api.previewAccountDeletion.mockRejectedValue(
      new ApiError("blocked", 409, "Conflict", { error: "blocked", code: "sole_owner", workspaces }),
    );
    const { result } = renderHook(() => usePreviewAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    const err = result.current.error;
    expect(err).toBeInstanceOf(SoleOwnerError);
    expect(isSoleOwnerError(err)).toBe(true);
    expect((err as SoleOwnerError).code).toBe("sole_owner");
    expect((err as SoleOwnerError).workspaces).toEqual(workspaces);
  });

  it("reads the sole_owner list defensively", async () => {
    api.previewAccountDeletion.mockRejectedValue(
      new ApiError("blocked", 409, "Conflict", { code: "sole_owner", workspaces: "nope" }),
    );
    const { result } = renderHook(() => usePreviewAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as SoleOwnerError).workspaces).toEqual([]);
  });

  it("maps recent_login_required to an error readable by code", async () => {
    api.previewAccountDeletion.mockRejectedValue(
      new ApiError("sign in again", 403, "Forbidden", { code: "recent_login_required" }),
    );
    const { result } = renderHook(() => usePreviewAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    const err = result.current.error;
    expect(err).toBeInstanceOf(RecentLoginRequiredError);
    expect(isRecentLoginRequiredError(err)).toBe(true);
    expect((err as RecentLoginRequiredError).code).toBe("recent_login_required");
  });

  it("passes any other failure through unchanged", async () => {
    const failure = new ApiError("nope", 403, "Forbidden", { code: "account_suspended" });
    api.previewAccountDeletion.mockRejectedValue(failure);
    const { result } = renderHook(() => usePreviewAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBe(failure);
    expect(errorCode(result.current.error)).toBe("account_suspended");
  });
});

describe("useRequestAccountDeletion", () => {
  let queryClient: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    authState.user = { id: "u1" };
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("returns the scheduled date and leaves the user untouched", async () => {
    api.requestAccountDeletion.mockResolvedValue({ scheduled_at: "2030-01-15T10:00:00Z" });
    const { result } = renderHook(() => useRequestAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ scheduled_at: "2030-01-15T10:00:00Z" });
    expect(authState.setUser).not.toHaveBeenCalled();
  });

  it("maps sole_owner and recent_login_required", async () => {
    api.requestAccountDeletion.mockRejectedValueOnce(
      new ApiError("blocked", 409, "Conflict", {
        code: "sole_owner",
        workspaces: [{ id: "w1", name: "Team", member_count: 2 }],
      }),
    );
    const first = renderHook(() => useRequestAccountDeletion(), { wrapper: wrapper(queryClient) });
    first.result.current.mutate();
    await waitFor(() => expect(first.result.current.isError).toBe(true));
    expect(first.result.current.error).toBeInstanceOf(SoleOwnerError);

    api.requestAccountDeletion.mockRejectedValueOnce(
      new ApiError("sign in", 403, "Forbidden", { code: "recent_login_required" }),
    );
    const second = renderHook(() => useRequestAccountDeletion(), { wrapper: wrapper(queryClient) });
    second.result.current.mutate();
    await waitFor(() => expect(second.result.current.isError).toBe(true));
    expect(second.result.current.error).toBeInstanceOf(RecentLoginRequiredError);
  });

  it("does not turn an unrelated 409 into a sole_owner error", async () => {
    const failure = new ApiError("account is not active", 409, "Conflict");
    api.requestAccountDeletion.mockRejectedValue(failure);
    const { result } = renderHook(() => useRequestAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBe(failure);
  });
});

describe("useCancelAccountDeletion", () => {
  let queryClient: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    authState.user = { id: "u1", deletion_scheduled_at: "2030-01-15T10:00:00Z" };
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("clears the date on the signed-in user", async () => {
    api.cancelAccountDeletion.mockResolvedValue({ status: "active" });
    const { result } = renderHook(() => useCancelAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(authState.setUser).toHaveBeenCalledWith({ id: "u1", deletion_scheduled_at: null });
  });

  it("keeps the date and surfaces the code when nothing is pending", async () => {
    api.cancelAccountDeletion.mockRejectedValue(
      new ApiError("not pending", 409, "Conflict", { code: "not_pending_deletion" }),
    );
    const { result } = renderHook(() => useCancelAccountDeletion(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(errorCode(result.current.error)).toBe("not_pending_deletion");
    expect(authState.setUser).not.toHaveBeenCalled();
  });
});
