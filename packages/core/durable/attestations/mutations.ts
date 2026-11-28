import { useMutation } from "@tanstack/react-query";
import { api } from "../../api";
import {
  parseAttestationDocument,
  parseVerifyAttestationResponse,
  serializeAttestationBody,
  type AttestationDocument,
  type VerifyAttestationResponse,
} from "./schemas";

export type IssueAttestationRequest = {
  agent_id: string;
  /** Period start, inclusive, YYYY-MM-DD. */
  from: string;
  /** Period end, inclusive, YYYY-MM-DD. */
  to: string;
  /** Optional Hub item reference as hub:<item>@<version>. */
  template_ref?: string;
  /** Include the workspace name in the signed document (hidden by default). */
  show_workspace?: boolean;
};

/** Signs an agent's track record for the given period (POST /api/attestations/issue). */
export function useIssueAttestation() {
  return useMutation<AttestationDocument | null, Error, IssueAttestationRequest>({
    mutationFn: async (data) =>
      parseAttestationDocument(
        await api.request("/api/attestations/issue", {
          method: "POST",
          body: JSON.stringify({
            agent_id: data.agent_id,
            from: data.from,
            to: data.to,
            template_ref: data.template_ref ?? "",
            show_workspace: data.show_workspace ?? false,
          }),
        }),
        "POST /api/attestations/issue",
      ),
  });
}

/** Verifies an attestation document imperatively (POST /api/attestations/verify). */
export function useVerifyAttestation() {
  return useMutation<VerifyAttestationResponse, Error, unknown>({
    mutationFn: async (document) =>
      parseVerifyAttestationResponse(
        await api.request("/api/attestations/verify", {
          method: "POST",
          body: serializeAttestationBody(document),
        }),
        "POST /api/attestations/verify",
      ),
  });
}
