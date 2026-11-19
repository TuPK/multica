// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api-error";

const apiMock = vi.hoisted(() => ({
  setToken: vi.fn(),
  getMe: vi.fn(),
  sendCode: vi.fn(),
  verifyCode: vi.fn(),
  verifyMFA: vi.fn(),
}));

const storageMock = vi.hoisted(() => ({
  getToken: vi.fn<() => Promise<string | null>>(),
  setToken: vi.fn<(t: string) => Promise<void>>(),
  clearToken: vi.fn<() => Promise<void>>(),
}));

const epochMock = vi.hoisted(() => ({
  invalidateSessionEpoch: vi.fn(),
}));

vi.mock("@/data/api", () => ({
  api: apiMock,
  ApiError,
}));

vi.mock("@/data/secure-storage", () => storageMock);
vi.mock("@/data/session-epoch", () => epochMock);
vi.mock("@/data/workspace-store", () => ({
  useWorkspaceStore: {
    getState: () => ({ clear: vi.fn(), restoreSlug: vi.fn() }),
  },
}));

import { useAuthStore } from "./auth-store";

const SAMPLE_USER = {
  id: "user-1",
  name: "Alex",
  email: "alex@example.com",
  avatar_url: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("useAuthStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ user: null, isLoading: true });
    useAuthStore.getState().clearMFAChallenge?.();
    useAuthStore.getState().clearSignOutReason?.();
  });

  it("verifyCode returns an MFA challenge without storing a session token", async () => {
    apiMock.verifyCode.mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: "mfa-secret-token",
      expires_in: 300,
    });

    const result = await useAuthStore.getState().verifyCode("alex@example.com", "123456");

    expect(result).toEqual({ mfaRequired: true });
    expect(storageMock.setToken).not.toHaveBeenCalled();
    expect(apiMock.setToken).not.toHaveBeenCalled();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().hasMfaChallenge).toBe(true);
  });

  it("finishSSOLogin stores the session and clears the MFA challenge", async () => {
    apiMock.verifyCode.mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: "mfa-secret-token",
      expires_in: 300,
    });
    await useAuthStore.getState().verifyCode("alex@example.com", "123456");
    expect(useAuthStore.getState().hasMfaChallenge).toBe(true);

    const ssoUser = {
      ...SAMPLE_USER,
      onboarded_at: null,
      onboarding_questionnaire: {},
      language: null,
      profile_description: "",
      timezone: null,
    };
    await useAuthStore.getState().finishSSOLogin({ token: "sso-jwt", user: ssoUser });

    expect(epochMock.invalidateSessionEpoch).toHaveBeenCalledTimes(1);
    expect(storageMock.setToken).toHaveBeenCalledExactlyOnceWith("sso-jwt");
    expect(apiMock.setToken).toHaveBeenCalledExactlyOnceWith("sso-jwt");
    expect(useAuthStore.getState().user).toEqual(ssoUser);
    expect(useAuthStore.getState().hasMfaChallenge).toBe(false);
    await expect(useAuthStore.getState().verifyMFA("123456")).rejects.toMatchObject({
      code: "mfa_token_invalid",
    });
  });

  it("verifyMFA sets token and user on success and clears the in-memory MFA challenge", async () => {
    apiMock.verifyCode.mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: "mfa-secret-token",
      expires_in: 300,
    });
    apiMock.verifyMFA.mockResolvedValueOnce({
      token: "session-jwt",
      user: SAMPLE_USER,
    });

    await useAuthStore.getState().verifyCode("alex@example.com", "123456");
    const user = await useAuthStore.getState().verifyMFA("654321");

    expect(apiMock.verifyMFA).toHaveBeenCalledExactlyOnceWith("mfa-secret-token", "654321");
    expect(epochMock.invalidateSessionEpoch).toHaveBeenCalledTimes(1);
    expect(storageMock.setToken).toHaveBeenCalledExactlyOnceWith("session-jwt");
    expect(apiMock.setToken).toHaveBeenCalledExactlyOnceWith("session-jwt");
    expect(user).toEqual(SAMPLE_USER);
    expect(useAuthStore.getState().user).toEqual(SAMPLE_USER);
    expect(useAuthStore.getState().hasMfaChallenge).toBe(false);
  });

  it("verifyMFA clears the challenge when server rejects with 401 mfa_token_invalid", async () => {
    apiMock.verifyCode.mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: "mfa-secret-token",
      expires_in: 300,
    });
    apiMock.verifyMFA.mockRejectedValueOnce(
      new ApiError("expired", 401, { code: "mfa_token_invalid" }),
    );

    await useAuthStore.getState().verifyCode("alex@example.com", "123456");
    await expect(useAuthStore.getState().verifyMFA("000000")).rejects.toBeInstanceOf(ApiError);
    expect(useAuthStore.getState().hasMfaChallenge).toBe(false);
    expect(useAuthStore.getState().mfaFailureReason).toBe("mfa_token_invalid");
  });

  it("verifyMFA preserves the challenge on 400 invalid_code so the user can retry", async () => {
    apiMock.verifyCode.mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: "mfa-secret-token",
      expires_in: 300,
    });
    apiMock.verifyMFA.mockRejectedValueOnce(
      new ApiError("bad code", 400, { code: "invalid_code" }),
    );

    await useAuthStore.getState().verifyCode("alex@example.com", "123456");
    await expect(useAuthStore.getState().verifyMFA("000000")).rejects.toBeInstanceOf(ApiError);
    expect(useAuthStore.getState().hasMfaChallenge).toBe(true);
    expect(useAuthStore.getState().mfaFailureReason).toBeNull();
  });

  it("initialize clears token and records account_suspended when getMe fails with 403 account_suspended", async () => {
    storageMock.getToken.mockResolvedValueOnce("stale-jwt");
    apiMock.getMe.mockRejectedValueOnce(
      new ApiError("suspended", 403, { code: "account_suspended" }),
    );

    await useAuthStore.getState().initialize();

    expect(epochMock.invalidateSessionEpoch).toHaveBeenCalledTimes(1);
    expect(storageMock.clearToken).toHaveBeenCalledTimes(1);
    expect(apiMock.setToken).toHaveBeenCalledWith(null);
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().isLoading).toBe(false);
    expect(useAuthStore.getState().signOutReason).toBe("account_suspended");
  });

  it("initialize clears token and records session_revoked when getMe fails with 401 session_revoked", async () => {
    storageMock.getToken.mockResolvedValueOnce("revoked-jwt");
    apiMock.getMe.mockRejectedValueOnce(
      new ApiError("revoked", 401, { code: "session_revoked" }),
    );

    await useAuthStore.getState().initialize();

    expect(storageMock.clearToken).toHaveBeenCalledTimes(1);
    expect(apiMock.setToken).toHaveBeenCalledWith(null);
    expect(useAuthStore.getState().signOutReason).toBe("session_revoked");
  });

  it("initialize keeps token on transient 500 error", async () => {
    storageMock.getToken.mockResolvedValueOnce("valid-jwt");
    apiMock.getMe.mockRejectedValueOnce(new ApiError("server error", 500));

    await useAuthStore.getState().initialize();

    expect(storageMock.clearToken).not.toHaveBeenCalled();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().signOutReason).toBeNull();
  });
});
