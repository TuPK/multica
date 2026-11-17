/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { createQueryClient } from "../query-client";
import {
  ssoConnectionOptions,
  ssoDomainsOptions,
  useAddSSODomain,
  useConfirmMFA,
  useDeleteSSOConnection,
  useDisableMFA,
  useEnrollMFA,
  useRemoveSSODomain,
  useSaveSSOConnection,
  useSSOConnection,
  useSSODomains,
  useUpdateWorkspacePolicy,
  useVerifySSODomain,
  useWorkspacePolicy,
  workspacePolicyOptions,
} from "./index";

const policy = {
  allowed_invite_domains: [],
  share_links_enabled: true,
  require_sso: false,
  require_mfa: true,
  session_max_age_hours: 0,
  updated_by: null,
  updated_at: null,
};

describe("security hooks", () => {
  let qc: QueryClient;
  let api: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    qc = createQueryClient();
    api = {
      getWorkspacePolicy: vi.fn().mockResolvedValue(policy),
      updateWorkspacePolicy: vi.fn().mockResolvedValue({ ...policy, require_sso: true }),
      getSSOConnection: vi.fn().mockResolvedValue(null),
      putSSOConnection: vi.fn().mockResolvedValue({ id: "c1" }),
      deleteSSOConnection: vi.fn().mockResolvedValue(undefined),
      listSSODomains: vi.fn().mockResolvedValue({ domains: [] }),
      addSSODomain: vi.fn().mockResolvedValue({ domain: "a.test" }),
      verifySSODomain: vi.fn().mockResolvedValue({ domain: "a.test" }),
      removeSSODomain: vi.fn().mockResolvedValue(undefined),
      enrollMFA: vi.fn().mockResolvedValue({ secret: "S", otpauth_uri: "u" }),
      confirmMFA: vi.fn().mockResolvedValue({ recovery_codes: ["r"] }),
      disableMFA: vi.fn().mockResolvedValue(undefined),
    };
    setApiInstance(api as unknown as ApiClient);
    vi.spyOn(qc, "invalidateQueries");
  });
  afterEach(() => vi.restoreAllMocks());

  function render<T>(hook: () => T) {
    return renderHook(hook, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      ),
    });
  }
  const invalidated = () =>
    vi.mocked(qc.invalidateQueries).mock.calls.map(([f]) => (f as { queryKey: unknown }).queryKey);

  it("keys every workspace-scoped query with the workspace id", () => {
    for (const o of [workspacePolicyOptions("ws-1"), ssoConnectionOptions("ws-1"), ssoDomainsOptions("ws-1")]) {
      expect(o.queryKey).toContain("ws-1");
    }
    expect(workspacePolicyOptions("ws-2").queryKey).not.toEqual(workspacePolicyOptions("ws-1").queryKey);
  });

  it("reads the policy, the connection and the domains for the workspace", async () => {
    const p = render(() => useWorkspacePolicy("ws-1"));
    const c = render(() => useSSOConnection("ws-1"));
    const d = render(() => useSSODomains("ws-1"));
    await waitFor(() => expect(p.result.current.data).toEqual(policy));
    await waitFor(() => expect(c.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(d.result.current.data).toEqual({ domains: [] }));
    expect(api.getWorkspacePolicy).toHaveBeenCalledWith("ws-1");
    expect(api.getSSOConnection).toHaveBeenCalledWith("ws-1");
    expect(api.listSSODomains).toHaveBeenCalledWith("ws-1");
  });

  it("sets the policy cache with the server's answer after an update", async () => {
    const { result } = render(() => useUpdateWorkspacePolicy("ws-1"));
    await result.current.mutateAsync({ require_sso: true });
    expect(api.updateWorkspacePolicy).toHaveBeenCalledWith("ws-1", { require_sso: true });
    expect(qc.getQueryData(workspacePolicyOptions("ws-1").queryKey)).toMatchObject({ require_sso: true });
    expect(qc.getQueryData(workspacePolicyOptions("ws-2").queryKey)).toBeUndefined();
  });

  it("does not touch the cache when the update fails", async () => {
    qc.setQueryData(workspacePolicyOptions("ws-1").queryKey, policy);
    api.updateWorkspacePolicy!.mockRejectedValue(new Error("no"));
    const { result } = render(() => useUpdateWorkspacePolicy("ws-1"));
    await expect(result.current.mutateAsync({ require_sso: true })).rejects.toThrow("no");
    expect(qc.getQueryData(workspacePolicyOptions("ws-1").queryKey)).toEqual(policy);
  });

  it("saves and deletes the connection, then refreshes it", async () => {
    const s = render(() => useSaveSSOConnection("ws-1"));
    await s.result.current.mutateAsync({ issuer: "i", client_id: "c" });
    expect(api.putSSOConnection).toHaveBeenCalledWith("ws-1", { issuer: "i", client_id: "c" });
    const d = render(() => useDeleteSSOConnection("ws-1"));
    await d.result.current.mutateAsync();
    expect(api.deleteSSOConnection).toHaveBeenCalledWith("ws-1");
    expect(invalidated()).toContainEqual(ssoConnectionOptions("ws-1").queryKey);
  });

  it("invalidates the domains query after add, verify and remove", async () => {
    const a = render(() => useAddSSODomain("ws-1"));
    await a.result.current.mutateAsync("a.test");
    expect(api.addSSODomain).toHaveBeenCalledWith("ws-1", "a.test");
    const v = render(() => useVerifySSODomain("ws-1"));
    await v.result.current.mutateAsync("a.test");
    expect(api.verifySSODomain).toHaveBeenCalledWith("ws-1", "a.test");
    const r = render(() => useRemoveSSODomain("ws-1"));
    await r.result.current.mutateAsync("a.test");
    expect(api.removeSSODomain).toHaveBeenCalledWith("ws-1", "a.test");
    const keys = invalidated().filter((k) => JSON.stringify(k) === JSON.stringify(ssoDomainsOptions("ws-1").queryKey));
    expect(keys).toHaveLength(3);
  });

  it("invalidates the domains query even when verify fails", async () => {
    api.verifySSODomain!.mockRejectedValue(new Error("nope"));
    const v = render(() => useVerifySSODomain("ws-1"));
    await expect(v.result.current.mutateAsync("a.test")).rejects.toThrow("nope");
    expect(invalidated()).toContainEqual(ssoDomainsOptions("ws-1").queryKey);
  });

  it("runs the account-level MFA mutations", async () => {
    const e = render(() => useEnrollMFA());
    await expect(e.result.current.mutateAsync()).resolves.toEqual({ secret: "S", otpauth_uri: "u" });
    const c = render(() => useConfirmMFA());
    await expect(c.result.current.mutateAsync("123456")).resolves.toEqual({ recovery_codes: ["r"] });
    expect(api.confirmMFA).toHaveBeenCalledWith("123456");
    const d = render(() => useDisableMFA());
    await d.result.current.mutateAsync("123456");
    expect(api.disableMFA).toHaveBeenCalledWith("123456");
  });
});
