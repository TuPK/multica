import { queryOptions, useQuery } from "@tanstack/react-query";
import { api, ApiError } from "../../api";
import { parsePrincipalInterview, parsePrincipalProfile } from "./schemas";

// Every key starts with "durable" and the workspace id, so a workspace never reads another's
// principal profile. Server data stays in TanStack Query; the mutations invalidate it.
export const principalKeys = {
  all: (wsId: string) => ["durable", wsId, "principal"] as const,
  profile: (wsId: string) => [...principalKeys.all(wsId), "profile"] as const,
  interview: (wsId: string) => [...principalKeys.all(wsId), "interview"] as const,
};

/**
 * The signed-in person's principal profile, or null when not enabled yet (the route answers 404
 * when no profile exists; other errors propagate to the error state).
 */
export function principalProfileOptions(wsId: string) {
  return queryOptions({
    queryKey: principalKeys.profile(wsId),
    queryFn: async () => {
      try {
        const raw = await api.request("/api/principal/profile");
        return parsePrincipalProfile(raw, "GET /api/principal/profile");
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }
    },
    enabled: !!wsId,
  });
}

/** The start-up interview questions (GET /api/principal/interview). */
export function principalInterviewOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: principalKeys.interview(wsId),
    queryFn: async () =>
      parsePrincipalInterview(await api.request("/api/principal/interview"), "GET /api/principal/interview"),
    enabled: !!wsId && enabled,
  });
}

export function usePrincipalProfile(wsId: string) {
  return useQuery(principalProfileOptions(wsId));
}

export function usePrincipalInterview(wsId: string, enabled = true) {
  return useQuery(principalInterviewOptions(wsId, enabled));
}
