import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  CreateWorkspaceSubscriptionCheckoutRequest,
  PreviewWorkspaceSeatPurchaseRequest,
  PurchaseWorkspaceSeatsRequest,
} from "../types";
import type {
  CreateWorkspacePlanChangeRequest,
  PreviewWorkspacePlanChangeRequest,
  UpdateWorkspaceBillingProfileRequest,
} from "../types/billing";
import { workspaceOrderKeys } from "./workspace-orders-queries";
import { workspaceSubscriptionKeys } from "./workspace-subscription-queries";

/**
 * Starts a checkout: Stripe's hosted page, or with the platform's manual
 * provider an order and the address of its page. The order list is read again
 * because an order may have been created.
 */
export function useCreateWorkspaceSubscriptionCheckout(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkspaceSubscriptionCheckoutRequest) =>
      api.createWorkspaceSubscriptionCheckout(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
      void queryClient.invalidateQueries({
        queryKey: workspaceOrderKeys.all(wsId),
      });
    },
  });
}

export function useReconcileWorkspaceSubscriptionSeats(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.reconcileWorkspaceSubscriptionSeats(),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}

export function usePreviewWorkspaceSeatPurchase() {
  return useMutation({
    mutationFn: (data: PreviewWorkspaceSeatPurchaseRequest) =>
      api.previewWorkspaceSeatPurchase(data),
  });
}

export function usePurchaseWorkspaceSeats(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: PurchaseWorkspaceSeatsRequest) =>
      api.purchaseWorkspaceSeats(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
      void queryClient.invalidateQueries({
        queryKey: workspaceOrderKeys.all(wsId),
      });
    },
  });
}

export function useCreateWorkspaceSubscriptionPortal(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (idempotencyKey: string) =>
      api.createWorkspaceSubscriptionPortal(idempotencyKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}

export function useUpdateWorkspaceBillingProfile(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: UpdateWorkspaceBillingProfileRequest) =>
      api.updateWorkspaceBillingProfile(data),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.billingProfile(wsId),
      });
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}

export function usePreviewWorkspacePlanChange() {
  return useMutation({
    mutationFn: (data: PreviewWorkspacePlanChangeRequest) =>
      api.previewWorkspacePlanChange(data),
  });
}

export function useCreateWorkspacePlanChange(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkspacePlanChangeRequest) =>
      api.createWorkspacePlanChange(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
      void queryClient.invalidateQueries({
        queryKey: workspaceOrderKeys.all(wsId),
      });
    },
  });
}

export function useCancelWorkspaceScheduledPlanChange(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (idempotencyKey: string) =>
      api.cancelWorkspaceScheduledPlanChange(idempotencyKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}

export function useCancelWorkspaceSubscription(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (idempotencyKey: string) =>
      api.cancelWorkspaceSubscription(idempotencyKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}

export function useResumeWorkspaceSubscription(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (idempotencyKey: string) =>
      api.resumeWorkspaceSubscription(idempotencyKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceSubscriptionKeys.summary(wsId),
      });
    },
  });
}
