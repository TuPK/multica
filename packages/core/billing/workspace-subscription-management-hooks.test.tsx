/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { createQueryClient } from "../query-client";
import { workspaceOrderKeys } from "./workspace-orders-queries";
import { workspaceSubscriptionKeys } from "./workspace-subscription-queries";
import {
  useCancelWorkspaceScheduledPlanChange,
  useCancelWorkspaceSubscription,
  useCreateWorkspacePlanChange,
  usePreviewWorkspacePlanChange,
  useResumeWorkspaceSubscription,
  useUpdateWorkspaceBillingProfile,
} from "./workspace-subscription-mutations";

const cancelState = {
  cancelAtPeriodEnd: true,
  status: "active",
  currentPeriodEnd: "2030-05-31T00:00:00Z",
  version: 5,
  changed: true,
};

describe("subscription management mutations", () => {
  let qc: QueryClient;
  let api: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    qc = createQueryClient();
    api = {
      updateWorkspaceBillingProfile: vi.fn().mockResolvedValue({
        accountKind: "business",
        legalName: "ACME",
        taxCode: "0123456789",
        address: "1 Le Loi",
        invoiceEmail: "ke.toan@example.com",
        verified: false,
      }),
      previewWorkspacePlanChange: vi.fn().mockResolvedValue({
        mode: "schedule",
        planKey: "pro",
        interval: "year",
        currentSeats: 5,
        resultingSeats: 5,
        currency: "vnd",
        amount: 0,
        vatAmount: 0,
        effectiveAt: "2030-05-31T00:00:00Z",
        expectedVersion: 4,
        quotedAt: "2030-05-11T00:00:00Z",
      }),
      createWorkspacePlanChange: vi.fn().mockResolvedValue({
        outcome: "scheduled",
        planKey: "pro",
        interval: "year",
        seats: 5,
        effectiveAt: "2030-05-31T00:00:00Z",
        version: 5,
        changed: true,
      }),
      cancelWorkspaceScheduledPlanChange: vi.fn().mockResolvedValue({
        pendingChange: null,
        version: 5,
        changed: true,
      }),
      cancelWorkspaceSubscription: vi.fn().mockResolvedValue(cancelState),
      resumeWorkspaceSubscription: vi
        .fn()
        .mockResolvedValue({ ...cancelState, cancelAtPeriodEnd: false, version: 6 }),
    };
    setApiInstance(api as unknown as ApiClient);
    vi.spyOn(qc, "invalidateQueries");
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  function render<T>(hook: () => T) {
    return renderHook(hook, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      ),
    });
  }

  function invalidatedKeys(): unknown[] {
    return vi
      .mocked(qc.invalidateQueries)
      .mock.calls.map(([filter]) => (filter as { queryKey: unknown }).queryKey);
  }

  it("saves the billing profile and refreshes the profile and the summary of this workspace", async () => {
    const { result } = render(() => useUpdateWorkspaceBillingProfile("ws-1"));
    await result.current.mutateAsync({
      accountKind: "business",
      legalName: "ACME",
      taxCode: "0123456789",
      address: "1 Le Loi",
      invoiceEmail: "ke.toan@example.com",
    });
    expect(api.updateWorkspaceBillingProfile).toHaveBeenCalledWith(
      expect.objectContaining({ accountKind: "business" }),
    );
    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining([
        workspaceSubscriptionKeys.billingProfile("ws-1"),
        workspaceSubscriptionKeys.summary("ws-1"),
      ]),
    );
  });

  it("previews a plan change without invalidating anything", async () => {
    const { result } = render(() => usePreviewWorkspacePlanChange());
    const preview = await result.current.mutateAsync({
      planKey: "pro",
      interval: "year",
      seats: 5,
    });
    expect(preview).toMatchObject({ mode: "schedule", effectiveAt: "2030-05-31T00:00:00Z" });
    expect(api.previewWorkspacePlanChange).toHaveBeenCalledWith({
      planKey: "pro",
      interval: "year",
      seats: 5,
    });
    expect(qc.invalidateQueries).not.toHaveBeenCalled();
  });

  it("applies a plan change, then refreshes the summary and the orders of this workspace", async () => {
    const { result } = render(() => useCreateWorkspacePlanChange("ws-1"));
    await result.current.mutateAsync({
      planKey: "pro",
      interval: "year",
      seats: 5,
      expectedVersion: 4,
      acceptedAmount: 0,
      idempotencyKey: "k1",
    });
    expect(api.createWorkspacePlanChange).toHaveBeenCalledWith(
      expect.objectContaining({ idempotencyKey: "k1" }),
    );
    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining([
        workspaceSubscriptionKeys.summary("ws-1"),
        workspaceOrderKeys.all("ws-1"),
      ]),
    );
  });

  it("clears a scheduled change and refreshes the summary", async () => {
    const { result } = render(() => useCancelWorkspaceScheduledPlanChange("ws-1"));
    await result.current.mutateAsync("k2");
    expect(api.cancelWorkspaceScheduledPlanChange).toHaveBeenCalledWith("k2");
    expect(invalidatedKeys()).toContainEqual(workspaceSubscriptionKeys.summary("ws-1"));
  });

  it("cancels at the period end and refreshes the summary", async () => {
    const { result } = render(() => useCancelWorkspaceSubscription("ws-1"));
    await result.current.mutateAsync("k3");
    expect(api.cancelWorkspaceSubscription).toHaveBeenCalledWith("k3");
    expect(invalidatedKeys()).toContainEqual(workspaceSubscriptionKeys.summary("ws-1"));
  });

  it("resumes and refreshes the summary", async () => {
    const { result } = render(() => useResumeWorkspaceSubscription("ws-1"));
    await result.current.mutateAsync("k4");
    expect(api.resumeWorkspaceSubscription).toHaveBeenCalledWith("k4");
    expect(invalidatedKeys()).toContainEqual(workspaceSubscriptionKeys.summary("ws-1"));
  });

  it("never invalidates another workspace's keys", async () => {
    const { result } = render(() => useCancelWorkspaceSubscription("ws-1"));
    await result.current.mutateAsync("k5");
    for (const key of invalidatedKeys()) {
      expect(JSON.stringify(key)).not.toContain("ws-2");
    }
  });
});
