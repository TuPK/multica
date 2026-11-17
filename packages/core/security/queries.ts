import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const securityKeys = {
  all: (wsId: string) => ["security", wsId] as const,
  policy: (wsId: string) => [...securityKeys.all(wsId), "policy"] as const,
  ssoConnection: (wsId: string) => [...securityKeys.all(wsId), "sso-connection"] as const,
  ssoDomains: (wsId: string) => [...securityKeys.all(wsId), "sso-domains"] as const,
};

export function workspacePolicyOptions(wsId: string) {
  return queryOptions({
    queryKey: securityKeys.policy(wsId),
    queryFn: () => api.getWorkspacePolicy(wsId),
    enabled: !!wsId,
  });
}

export function ssoConnectionOptions(wsId: string) {
  return queryOptions({
    queryKey: securityKeys.ssoConnection(wsId),
    queryFn: () => api.getSSOConnection(wsId),
    enabled: !!wsId,
  });
}

export function ssoDomainsOptions(wsId: string) {
  return queryOptions({
    queryKey: securityKeys.ssoDomains(wsId),
    queryFn: () => api.listSSODomains(wsId),
    enabled: !!wsId,
  });
}
