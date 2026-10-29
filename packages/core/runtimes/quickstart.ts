import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { RuntimeQuickstartRequest } from "../types/agent";
import { runtimeKeys } from "./queries";

export const runtimeQuickstartKeys = {
  request: (wsId: string, runtimeId: string, requestId: string) =>
    [...runtimeKeys.all(wsId), "quickstart", runtimeId, requestId] as const,
};

const POLL_INTERVAL_MS = 2_000;

function isLive(request: RuntimeQuickstartRequest | undefined): boolean {
  return request?.status === "pending" || request?.status === "running";
}

/**
 * Starts a quickstart on a machine and follows it until the daemon reports.
 * The request ID lives in component state, so a reload does not resume an
 * earlier run; the next click starts a fresh one, and the server refuses it
 * while one is still live.
 */
export function useRuntimeQuickstart(wsId: string, runtimeId: string) {
  const [requestId, setRequestId] = useState<string | null>(null);
  const qc = useQueryClient();

  const start = useMutation({
    mutationFn: () => api.initiateRuntimeQuickstart(runtimeId),
    onSuccess: (request) => {
      qc.setQueryData(
        runtimeQuickstartKeys.request(wsId, runtimeId, request.id),
        request,
      );
      setRequestId(request.id);
    },
  });

  const query = useQuery({
    queryKey: runtimeQuickstartKeys.request(wsId, runtimeId, requestId ?? ""),
    queryFn: () => api.getRuntimeQuickstart(runtimeId, requestId ?? ""),
    enabled: requestId !== null,
    refetchInterval: (q) =>
      isLive(q.state.data) ? POLL_INTERVAL_MS : false,
  });

  return {
    request: requestId ? query.data : undefined,
    // Ignores clicks while a request is starting or live, so a double click
    // cannot queue a second install.
    start: () => {
      if (start.isPending || isLive(query.data)) return;
      start.mutate();
    },
    isStarting: start.isPending,
    startFailed: start.isError,
    isLive: isLive(query.data),
  };
}
