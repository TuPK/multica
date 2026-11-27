import { queryOptions, useQuery } from "@tanstack/react-query";
import { api, ApiError } from "../../api";
import { toQueryString } from "../query-string";
import {
  parsePrincipalDelegations,
  parsePrincipalEvidenceCandidates,
  parsePrincipalInterview,
  parsePrincipalPrediction,
  parsePrincipalProfile,
  type PrincipalPredictionSubjectType,
} from "./schemas";

// Every key starts with "durable" and the workspace id, so a workspace never reads another's
// principal profile. Server data stays in TanStack Query; the mutations invalidate it.
export const principalKeys = {
  all: (wsId: string) => ["durable", wsId, "principal"] as const,
  profile: (wsId: string) => [...principalKeys.all(wsId), "profile"] as const,
  interview: (wsId: string) => [...principalKeys.all(wsId), "interview"] as const,
  delegations: (wsId: string) => [...principalKeys.all(wsId), "delegations"] as const,
  evidenceCandidates: (wsId: string) => [...principalKeys.all(wsId), "evidence-candidates"] as const,
  prediction: (wsId: string, subjectType: PrincipalPredictionSubjectType, subjectId: string) =>
    [...principalKeys.all(wsId), "prediction", subjectType, subjectId] as const,
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

/** Active/latest-disabled delegations and per-kind prediction accuracy (GET /api/principal/delegations). */
export function principalDelegationsOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: principalKeys.delegations(wsId),
    queryFn: async () =>
      parsePrincipalDelegations(
        await api.request("/api/principal/delegations"),
        "GET /api/principal/delegations",
      ),
    enabled: !!wsId && enabled,
  });
}

/** Recent proposal decisions the person can cite as manual trait evidence (GET /api/principal/evidence-candidates). */
export function principalEvidenceCandidatesOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: principalKeys.evidenceCandidates(wsId),
    queryFn: async () =>
      parsePrincipalEvidenceCandidates(
        await api.request("/api/principal/evidence-candidates"),
        "GET /api/principal/evidence-candidates",
      ),
    enabled: !!wsId && enabled,
  });
}

/**
 * The signed-in person's own prediction for one subject (GET /api/principal/predictions).
 * 404 maps to null (no prediction exists for this subject); retry is disabled so missing or
 * unavailable predictions never hammer the server from card lists.
 */
export function principalPredictionOptions(
  wsId: string,
  subjectType: PrincipalPredictionSubjectType,
  subjectId: string,
  enabled = true,
) {
  return queryOptions({
    queryKey: principalKeys.prediction(wsId, subjectType, subjectId),
    queryFn: async () => {
      try {
        const raw = await api.request(
          `/api/principal/predictions${toQueryString({ subject_type: subjectType, subject_id: subjectId })}`,
        );
        return parsePrincipalPrediction(raw, "GET /api/principal/predictions");
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }
    },
    enabled: !!wsId && !!subjectId && enabled,
    retry: false,
  });
}

export function usePrincipalProfile(wsId: string) {
  return useQuery(principalProfileOptions(wsId));
}

export function usePrincipalInterview(wsId: string, enabled = true) {
  return useQuery(principalInterviewOptions(wsId, enabled));
}

export function usePrincipalDelegations(wsId: string, enabled = true) {
  return useQuery(principalDelegationsOptions(wsId, enabled));
}

export function usePrincipalEvidenceCandidates(wsId: string, enabled = true) {
  return useQuery(principalEvidenceCandidatesOptions(wsId, enabled));
}

export function usePrincipalPrediction(
  wsId: string,
  subjectType: PrincipalPredictionSubjectType,
  subjectId: string,
  enabled = true,
) {
  return useQuery(principalPredictionOptions(wsId, subjectType, subjectId, enabled));
}
