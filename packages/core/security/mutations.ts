import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { SSOConnectionInput } from "../api/sso-schemas";
import type { WorkspacePolicyInput } from "../api/security-schemas";
import { securityKeys } from "./queries";

// No mutation patches the cache ahead of the server: each awaits the answer,
// then sets or invalidates.

export function useUpdateWorkspacePolicy(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: WorkspacePolicyInput) => api.updateWorkspacePolicy(wsId, input),
    onSuccess: (policy) => {
      qc.setQueryData(securityKeys.policy(wsId), policy);
    },
  });
}

export function useSaveSSOConnection(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SSOConnectionInput) => api.putSSOConnection(wsId, input),
    onSettled: () => qc.invalidateQueries({ queryKey: securityKeys.ssoConnection(wsId) }),
  });
}

export function useDeleteSSOConnection(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.deleteSSOConnection(wsId),
    onSettled: () => qc.invalidateQueries({ queryKey: securityKeys.ssoConnection(wsId) }),
  });
}

export function useAddSSODomain(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) => api.addSSODomain(wsId, domain),
    onSettled: () => qc.invalidateQueries({ queryKey: securityKeys.ssoDomains(wsId) }),
  });
}

export function useVerifySSODomain(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) => api.verifySSODomain(wsId, domain),
    onSettled: () => qc.invalidateQueries({ queryKey: securityKeys.ssoDomains(wsId) }),
  });
}

export function useRemoveSSODomain(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) => api.removeSSODomain(wsId, domain),
    onSettled: () => qc.invalidateQueries({ queryKey: securityKeys.ssoDomains(wsId) }),
  });
}

// Account-level: MFA belongs to the user, not to a workspace.

export function useEnrollMFA() {
  return useMutation({ mutationFn: () => api.enrollMFA() });
}

export function useConfirmMFA() {
  return useMutation({ mutationFn: (code: string) => api.confirmMFA(code) });
}

/** `code` is a TOTP code or a recovery code. */
export function useDisableMFA() {
  return useMutation({ mutationFn: (code: string) => api.disableMFA(code) });
}
