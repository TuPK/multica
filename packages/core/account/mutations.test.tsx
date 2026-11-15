// @vitest-environment jsdom
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useRevokeAllSessions } from "./mutations";

const api = vi.hoisted(() => ({ revokeAllSessions: vi.fn() }));
const authState = vi.hoisted(() => ({ adoptSessionToken: vi.fn() }));

vi.mock("../api", () => ({ api }));
vi.mock("../auth", () => ({ useAuthStore: { getState: () => authState } }));

function wrapper(queryClient: QueryClient) {
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }
  return Wrapper;
}

describe("useRevokeAllSessions", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("hands the replacement token to the auth store", async () => {
    api.revokeAllSessions.mockResolvedValue({ revoked_at: "2026-11-15T06:30:00Z", token: "new-jwt" });
    const { result } = renderHook(() => useRevokeAllSessions(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(authState.adoptSessionToken).toHaveBeenCalledTimes(1);
    expect(authState.adoptSessionToken).toHaveBeenCalledWith("new-jwt");
  });

  it("adopts nothing when the call fails", async () => {
    api.revokeAllSessions.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useRevokeAllSessions(), { wrapper: wrapper(queryClient) });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(authState.adoptSessionToken).not.toHaveBeenCalled();
  });
});
