import { useQuery } from "@tanstack/react-query";
import { ssoConnectionOptions, ssoDomainsOptions, workspacePolicyOptions } from "./queries";

export function useWorkspacePolicy(wsId: string) {
  return useQuery(workspacePolicyOptions(wsId));
}

export function useSSOConnection(wsId: string) {
  return useQuery(ssoConnectionOptions(wsId));
}

export function useSSODomains(wsId: string) {
  return useQuery(ssoDomainsOptions(wsId));
}
