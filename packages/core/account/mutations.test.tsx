// @vitest-environment jsdom
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAcceptConsents, useRevokeAllSessions } from "./mutations";

const api = vi.hoisted(() => ({ revokeAllSessions: vi.fn(), acceptConsents: vi.fn() }));
const authState = vi.hoisted(() => ({
  adoptSessionToken: vi.fn(),
  user: null as unknown,
  setUser: vi.fn(),
}));

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

describe("useAcceptConsents", () => {
  let queryClient: QueryClient;
  const docs = { documents: [{ document: "terms", version: "2" }] };

  beforeEach(() => {
    vi.clearAllMocks();
    authState.user = { id: "u1", consent_required: [{ document: "terms", version: "2", url: "" }] };
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  it("clears the outstanding documents on the signed-in user", async () => {
    api.acceptConsents.mockResolvedValue({ consent_required: [] });
    const { result } = renderHook(() => useAcceptConsents(), { wrapper: wrapper(queryClient) });

    result.current.mutate(docs);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(api.acceptConsents).toHaveBeenCalledWith(docs.documents);
    expect(authState.setUser).toHaveBeenCalledWith({ id: "u1", consent_required: [] });
  });

  it("leaves the user untouched on a 409", async () => {
    api.acceptConsents.mockRejectedValue(new Error("consent_version_mismatch"));
    const { result } = renderHook(() => useAcceptConsents(), { wrapper: wrapper(queryClient) });

    result.current.mutate(docs);

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(authState.setUser).not.toHaveBeenCalled();
  });
});
