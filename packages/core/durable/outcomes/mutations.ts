import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, attentionOpenedHeaders } from "../../api";
import { outcomeKeys } from "./queries";
import {
  parseCriterion,
  parseVerification,
  type CheckKind,
  type Criterion,
  type Severity,
  type Verification,
  type VerificationResult,
} from "./schemas";

export type CreateCriterionRequest = {
  text: string;
  check_kind: CheckKind;
  check_spec?: Record<string, unknown>;
};

export type VerifyOutcomeRequest = {
  outcomeId: string;
  criterion_id?: string | null;
  result: VerificationResult;
  severity?: Severity;
  note?: string;
  evidence?: unknown;
  /** Epoch milliseconds when the person opened the outcome check (durable spec 7.2). */
  openedAt?: number;
};

// The follow-up work lives on useMutation's own onSettled, not on per-call callbacks: it runs
// even when the caller has unmounted before the request settles. A toast belongs to the
// caller and attaches to mutateAsync().then.
function refreshCriteriaAndOutcomes(qc: QueryClient, wsId: string, issueId: string) {
  void qc.invalidateQueries({ queryKey: outcomeKeys.criteria(wsId, issueId) });
  void qc.invalidateQueries({ queryKey: outcomeKeys.all(wsId) });
}

/** Adds a criterion to an issue. An agent adds one as itself; the server records the source. */
export function useAddCriterion(wsId: string, issueId: string) {
  const qc = useQueryClient();
  return useMutation<Criterion | null, Error, CreateCriterionRequest>({
    mutationFn: async (data) =>
      parseCriterion(
        await api.request(`/api/issues/${encodeURIComponent(issueId)}/criteria`, {
          method: "POST",
          body: JSON.stringify(data),
        }),
        "POST /api/issues/:id/criteria",
      ),
    onSettled: () => refreshCriteriaAndOutcomes(qc, wsId, issueId),
  });
}

/** Archives a criterion. The server refuses it once the outcome has settled or been checked. */
export function useArchiveCriterion(wsId: string, issueId: string) {
  const qc = useQueryClient();
  return useMutation<Criterion | null, Error, string>({
    mutationFn: async (criterionId) =>
      parseCriterion(
        await api.request(`/api/issues/${encodeURIComponent(issueId)}/criteria/${encodeURIComponent(criterionId)}`, {
          method: "PATCH",
          body: JSON.stringify({ archived: true }),
        }),
        "PATCH /api/issues/:id/criteria/:cid",
      ),
    onSettled: () => refreshCriteriaAndOutcomes(qc, wsId, issueId),
  });
}

/** Records one check on an outcome. The outcome id names the route and is not sent in the body. */
export function useVerifyOutcome(wsId: string) {
  const qc = useQueryClient();
  return useMutation<Verification | null, Error, VerifyOutcomeRequest>({
    mutationFn: async ({ outcomeId, openedAt, ...body }) =>
      parseVerification(
        await api.request(`/api/outcomes/${encodeURIComponent(outcomeId)}/verifications`, {
          method: "POST",
          headers: attentionOpenedHeaders(openedAt),
          body: JSON.stringify(body),
        }),
        "POST /api/outcomes/:id/verifications",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: outcomeKeys.all(wsId) });
    },
  });
}
