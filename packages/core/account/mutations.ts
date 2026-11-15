import { useMutation } from "@tanstack/react-query";
import { api } from "../api";
import { useAuthStore } from "../auth";
import type { RevokeAllSessionsResponse } from "./schema";

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
