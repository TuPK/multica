/**
 * Mobile auth store — Zustand. Logic mirrors packages/core/auth/store.ts:
 *   - Token written ONLY on successful verifyCode
 *   - 401 → clear token; non-401 (5xx / network blip) → preserve token so
 *     the next launch can retry
 *   - logout = clear token + clear in-memory user + setToken(null)
 *
 * NOT shared with web/desktop (per Sharing Principles in root CLAUDE.md).
 * Storage backend is expo-secure-store (mobile only); web uses HttpOnly
 * cookies, desktop uses localStorage via StorageAdapter.
 */
import { create } from "zustand";
import type { User } from "@multica/core/types";
import { api, ApiError, type VerifyCodeResponse } from "./api";
import { clearToken, getToken, setToken } from "./secure-storage";
import { invalidateSessionEpoch } from "./session-epoch";
import { useWorkspaceAccessStore } from "./stores/workspace-access-store";
import { useWorkspaceStore } from "./workspace-store";

export type SignOutReason = "session_revoked" | "account_suspended";
export type MfaFailureReason = "mfa_token_invalid" | "mfa_token_failed";

export type VerifyCodeResult =
  | { mfaRequired: false; user: User }
  | { mfaRequired: true };

// Kept strictly in module memory (never SecureStore, never logged, never in
// navigation params or persisted state) for the duration of an MFA step.
let pendingMfaToken: string | null = null;

function isMfaChallenge(
  res: VerifyCodeResponse,
): res is { mfa_required: true; mfa_token: string; expires_in: number } {
  return (
    typeof res === "object" &&
    res !== null &&
    "mfa_required" in res &&
    res.mfa_required === true &&
    typeof res.mfa_token === "string"
  );
}

interface AuthState {
  user: User | null;
  isLoading: boolean;
  hasMfaChallenge: boolean;
  signOutReason: SignOutReason | null;
  mfaFailureReason: MfaFailureReason | null;
  initialize: () => Promise<void>;
  sendCode: (email: string) => Promise<void>;
  verifyCode: (email: string, code: string) => Promise<VerifyCodeResult>;
  verifyMFA: (code: string) => Promise<User>;
  clearMFAChallenge: () => void;
  setSignOutReason: (reason: SignOutReason | null) => void;
  setMfaFailureReason: (reason: MfaFailureReason | null) => void;
  clearSignOutReason: () => void;
  logout: (reason?: SignOutReason | null) => Promise<void>;
  /** Overwrite the in-memory user — call after PATCH /api/me so name/avatar
   *  edits land without a refetch. Server response is the source of truth. */
  setUser: (user: User) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: true,
  hasMfaChallenge: false,
  signOutReason: null,
  mfaFailureReason: null,

  initialize: async () => {
    // Restore the persisted workspace slug alongside the auth token so the
    // entry redirect (app/index.tsx) can route directly to the last-used
    // workspace without flashing /select-workspace.
    await useWorkspaceStore.getState().restoreSlug();

    const token = await getToken();
    if (!token) {
      set({ isLoading: false });
      return;
    }
    api.setToken(token);
    try {
      const user = await api.getMe();
      set({ user, isLoading: false, signOutReason: null, mfaFailureReason: null });
    } catch (err) {
      // Clear token on 401 or 403 account_suspended so cold starts don't loop
      // trying a dead or suspended credential. Transient 5xx/network errors
      // keep the token so the next launch can retry.
      const isUnauthorized = err instanceof ApiError && err.status === 401;
      const isSuspended =
        err instanceof ApiError &&
        err.status === 403 &&
        err.code === "account_suspended";

      if (isUnauthorized || isSuspended) {
        invalidateSessionEpoch();
        await clearToken();
        api.setToken(null);
        const reason: SignOutReason | null = isSuspended
          ? "account_suspended"
          : err.code === "session_revoked"
            ? "session_revoked"
            : null;
        set({ user: null, isLoading: false, signOutReason: reason });
        return;
      }
      set({ user: null, isLoading: false });
    }
  },

  sendCode: async (email) => {
    pendingMfaToken = null;
    set({ hasMfaChallenge: false, signOutReason: null, mfaFailureReason: null });
    await api.sendCode(email);
  },

  verifyCode: async (email, code) => {
    const res = await api.verifyCode(email, code);
    if (isMfaChallenge(res)) {
      pendingMfaToken = res.mfa_token;
      set({ hasMfaChallenge: true, signOutReason: null, mfaFailureReason: null });
      return { mfaRequired: true };
    }
    pendingMfaToken = null;
    // Signing in replaces the credential just as decisively as signing out:
    // a renewal still in flight for the previous account must not write its
    // result over this one.
    invalidateSessionEpoch();
    await setToken(res.token);
    api.setToken(res.token);
    set({
      user: res.user,
      hasMfaChallenge: false,
      signOutReason: null,
      mfaFailureReason: null,
    });
    return { mfaRequired: false, user: res.user };
  },

  verifyMFA: async (code) => {
    const mfaToken = pendingMfaToken;
    if (!mfaToken) {
      set({ hasMfaChallenge: false });
      throw new ApiError("Missing MFA challenge", 401, {
        code: "mfa_token_invalid",
      });
    }
    try {
      const res = await api.verifyMFA(mfaToken, code);
      pendingMfaToken = null;
      invalidateSessionEpoch();
      await setToken(res.token);
      api.setToken(res.token);
      set({
        user: res.user,
        hasMfaChallenge: false,
        signOutReason: null,
        mfaFailureReason: null,
      });
      return res.user;
    } catch (err) {
      if (
        err instanceof ApiError &&
        err.status === 401 &&
        (err.code === "mfa_token_invalid" || err.code === "mfa_token_failed")
      ) {
        pendingMfaToken = null;
        set({ hasMfaChallenge: false, mfaFailureReason: err.code });
      } else if (
        err instanceof ApiError &&
        (err.status === 401 ||
          (err.status === 403 && err.code === "account_suspended"))
      ) {
        pendingMfaToken = null;
        set({ hasMfaChallenge: false });
      }
      throw err;
    }
  },

  clearMFAChallenge: () => {
    pendingMfaToken = null;
    set({ hasMfaChallenge: false });
  },

  setSignOutReason: (reason) => set({ signOutReason: reason }),
  setMfaFailureReason: (reason) => set({ mfaFailureReason: reason }),
  clearSignOutReason: () =>
    set({ signOutReason: null, mfaFailureReason: null }),

  logout: async (reason = null) => {
    // First statement in the function, before any await. The Keychain delete
    // below is async, so until it lands a concurrent read still returns the
    // token being removed — the epoch is what makes this instant.
    pendingMfaToken = null;
    invalidateSessionEpoch();
    await clearToken();
    api.setToken(null);
    useWorkspaceAccessStore.getState().clearAll();
    set({
      user: null,
      hasMfaChallenge: false,
      signOutReason: reason ?? null,
    });
  },

  setUser: (user) => set({ user }),
}));
