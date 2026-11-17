import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

/**
 * Plans and orders are read per workspace: the server resolves them from the
 * workspace in the URL, so two workspaces produce different data. Every key
 * carries the workspace id, which keeps one workspace's orders from showing in
 * another after a switch.
 */
export const workspaceOrderKeys = {
  all: (wsId: string) => ["workspace-orders", wsId] as const,
  list: (wsId: string) => [...workspaceOrderKeys.all(wsId), "list"] as const,
  detail: (wsId: string, orderId: string) =>
    [...workspaceOrderKeys.all(wsId), "detail", orderId] as const,
  usage: (wsId: string, orderId: string) =>
    [...workspaceOrderKeys.all(wsId), "usage", orderId] as const,
};

export const workspacePlanKeys = {
  all: (wsId: string) => ["workspace-plans", wsId] as const,
  list: (wsId: string) => [...workspacePlanKeys.all(wsId), "list"] as const,
};

/** The public plans for the plan selector. A plan list changes rarely. */
export function workspacePlansOptions(wsId: string) {
  return queryOptions({
    queryKey: workspacePlanKeys.list(wsId),
    queryFn: () => api.listWorkspacePlans(wsId),
    enabled: wsId.length > 0,
    staleTime: 5 * 60 * 1000,
  });
}

export function workspaceOrdersOptions(wsId: string) {
  return queryOptions({
    queryKey: workspaceOrderKeys.list(wsId),
    queryFn: () => api.listWorkspaceOrders(wsId),
    enabled: wsId.length > 0,
    staleTime: 30 * 1000,
  });
}

/**
 * One order, for its page. Payment is confirmed by staff after a bank transfer,
 * so the page refetches on focus to show a status that changed meanwhile.
 */
export function workspaceOrderOptions(wsId: string, orderId: string) {
  return queryOptions({
    queryKey: workspaceOrderKeys.detail(wsId, orderId),
    queryFn: () => api.getWorkspaceOrder(wsId, orderId),
    enabled: wsId.length > 0 && orderId.length > 0,
    staleTime: 30 * 1000,
    refetchOnWindowFocus: true,
  });
}

/** The day lines of a usage invoice. An invoice's lines do not change once it is issued. */
export function workspaceOrderUsageOptions(wsId: string, orderId: string) {
  return queryOptions({
    queryKey: workspaceOrderKeys.usage(wsId, orderId),
    queryFn: () => api.getWorkspaceOrderUsage(wsId, orderId),
    enabled: wsId.length > 0 && orderId.length > 0,
    staleTime: 5 * 60 * 1000,
  });
}
