import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { UpdateWorkspaceOrderBuyerInput } from "./workspace-orders-types";
import { workspaceOrderKeys } from "./workspace-orders-queries";

/**
 * Saves the invoice details of a pending order. The server answers 409
 * `order_not_pending` for an order that was paid or expired meanwhile; that
 * surfaces as an ApiError. Whatever the outcome, the order is read again, so
 * the page shows its real status.
 */
export function useUpdateWorkspaceOrderBuyer(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ orderId, buyer }: UpdateWorkspaceOrderBuyerInput) =>
      api.updateWorkspaceOrderBuyer(wsId, orderId, buyer),
    onSuccess: (order) => {
      if (order) {
        queryClient.setQueryData(
          workspaceOrderKeys.detail(wsId, order.id),
          order,
        );
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceOrderKeys.all(wsId),
      });
    },
  });
}
