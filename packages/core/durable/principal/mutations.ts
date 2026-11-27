import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { principalKeys } from "./queries";
import {
  DEFAULT_DELEGATION_MIN_CONFIDENCE,
  parseDelegationEnableResult,
  parsePrincipalProfile,
  parsePrincipalTrait,
  type DelegationEnableResult,
  type PrincipalEvidence,
  type PrincipalOrigin,
  type PrincipalProfile,
  type PrincipalTrait,
} from "./schemas";

export type EnablePrincipalProfileRequest = {
  consent_text: string;
  share_with_owners: boolean;
};

export type AddPrincipalTraitRequest = {
  decision_kind: string;
  statement: string;
  origin: PrincipalOrigin;
  evidence: PrincipalEvidence[];
};

export type EditPrincipalTraitRequest = {
  id: string;
  statement: string;
};

export type AnswerPrincipalInterviewRequest = {
  question: number;
  answer: string;
  message_id: string;
};

export type EnablePrincipalDelegationRequest = {
  decision_kind: string;
  min_confidence?: number;
};

/**
 * Turns the caller's principal profile on with explicit consent, or updates `share_with_owners`
 * by re-sending consent (PUT /api/principal/profile). Refreshes the profile query on settle so
 * active traits stay in sync (PUT returns an empty traits slice).
 */
export function useEnablePrincipalProfile(wsId: string) {
  const qc = useQueryClient();
  return useMutation<PrincipalProfile | null, Error, EnablePrincipalProfileRequest>({
    mutationFn: async (body) =>
      parsePrincipalProfile(
        await api.request("/api/principal/profile", {
          method: "PUT",
          body: JSON.stringify(body),
        }),
        "PUT /api/principal/profile",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/** Deletes the caller's principal profile, its traits and its delegations (DELETE /api/principal/profile). */
export function useDeletePrincipalProfile(wsId: string) {
  const qc = useQueryClient();
  return useMutation<void, Error, void>({
    mutationFn: async () => {
      await api.request("/api/principal/profile", { method: "DELETE" });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/**
 * Adds a trait with evidence owned by the caller (POST /api/principal/profile/traits). The server
 * rejects traits without valid evidence (422 principal_evidence_invalid).
 */
export function useAddPrincipalTrait(wsId: string) {
  const qc = useQueryClient();
  return useMutation<PrincipalTrait | null, Error, AddPrincipalTraitRequest>({
    mutationFn: async (body) =>
      parsePrincipalTrait(
        await api.request("/api/principal/profile/traits", {
          method: "POST",
          body: JSON.stringify(body),
        }),
        "POST /api/principal/profile/traits",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/** Edits a trait's sentence (PATCH /api/principal/profile/traits/:id); the trait becomes `edited`. */
export function useEditPrincipalTrait(wsId: string) {
  const qc = useQueryClient();
  return useMutation<PrincipalTrait | null, Error, EditPrincipalTraitRequest>({
    mutationFn: async ({ id, statement }) =>
      parsePrincipalTrait(
        await api.request(`/api/principal/profile/traits/${encodeURIComponent(id)}`, {
          method: "PATCH",
          body: JSON.stringify({ statement }),
        }),
        "PATCH /api/principal/profile/traits/:id",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/** Retires a trait from the caller's profile (DELETE /api/principal/profile/traits/:id). */
export function useRetirePrincipalTrait(wsId: string) {
  const qc = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: async (id) => {
      await api.request(`/api/principal/profile/traits/${encodeURIComponent(id)}`, {
        method: "DELETE",
      });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/** Records one start-up interview answer as an `interview` trait (POST /api/principal/interview/answers). */
export function useAnswerPrincipalInterview(wsId: string) {
  const qc = useQueryClient();
  return useMutation<PrincipalTrait | null, Error, AnswerPrincipalInterviewRequest>({
    mutationFn: async (body) =>
      parsePrincipalTrait(
        await api.request("/api/principal/interview/answers", {
          method: "POST",
          body: JSON.stringify(body),
        }),
        "POST /api/principal/interview/answers",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: principalKeys.profile(wsId) });
    },
  });
}

/** Enables delegation by prediction for one decision kind (PUT /api/principal/delegations). */
export function useEnablePrincipalDelegation() {
  return useMutation<DelegationEnableResult | null, Error, EnablePrincipalDelegationRequest>({
    mutationFn: async ({ decision_kind, min_confidence = DEFAULT_DELEGATION_MIN_CONFIDENCE }) =>
      parseDelegationEnableResult(
        await api.request("/api/principal/delegations", {
          method: "PUT",
          body: JSON.stringify({ decision_kind, min_confidence }),
        }),
        "PUT /api/principal/delegations",
      ),
  });
}

/** Disables delegation by prediction for one decision kind (DELETE /api/principal/delegations?decision_kind=). */
export function useDisablePrincipalDelegation() {
  return useMutation<void, Error, string>({
    mutationFn: async (decisionKind) => {
      await api.request(`/api/principal/delegations?decision_kind=${encodeURIComponent(decisionKind)}`, {
        method: "DELETE",
      });
    },
  });
}
