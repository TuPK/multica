import { useMutation } from "@tanstack/react-query";
import { api } from "../api";
import { useAuthStore } from "../auth";
import type { AcceptConsentItem, AcceptConsentsResponse, RevokeAllSessionsResponse } from "./schema";

/** The body of POST /api/me/consents. */
export interface AcceptConsentsInput {
  documents: AcceptConsentItem[];
}

/**
 * Signs the user out of every device, this one excluded.
 *
 * The server revokes by login time and answers with a fresh session. Cookie
 * clients already hold it (Set-Cookie); token clients (Desktop) must store it
 * before anything else fires, which `adoptSessionToken` does. Nothing is
 * patched or invalidated here: the account and its queries are unchanged.
 */
export function useRevokeAllSessions() {
  return useMutation<RevokeAllSessionsResponse, Error, void>({
    mutationFn: () => api.revokeAllSessions(),
    onSuccess: (result) => {
      useAuthStore.getState().adoptSessionToken(result.token);
    },
  });
}

/**
 * Accepts legal documents. The server answers with what is still outstanding,
 * which replaces `consent_required` on the signed-in user so the gate closes
 * without a refetch. A 409 (the version changed under the dialog) rejects and
 * leaves the user as is; the caller refreshes the account to pick up the new
 * version.
 */
export function useAcceptConsents() {
  return useMutation<AcceptConsentsResponse, Error, AcceptConsentsInput>({
    mutationFn: ({ documents }) => api.acceptConsents(documents),
    onSuccess: (result) => {
      const { user, setUser } = useAuthStore.getState();
      if (user) setUser({ ...user, consent_required: result.consent_required });
    },
  });
}
