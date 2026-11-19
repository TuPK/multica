import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@multica/core/api";
import { configStore } from "@multica/core/config";
import { BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG } from "@multica/core/feature-flags";
import type {
  AutopilotQuotaUsage,
  IssueLimitUsage,
  WorkspaceBillingProfile,
  WorkspaceSubscriptionEntitlements,
  WorkspaceSubscriptionSummary,
} from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  checkout: vi.fn(),
  portal: vi.fn(),
  previewSeats: vi.fn(),
  purchaseSeats: vi.fn(),
  updateInvoiceBuyer: vi.fn(),
  refetch: vi.fn(),
  refetchSummary: vi.fn(),
  refetchUsage: vi.fn(),
  refetchSubscriptionUsage: vi.fn(),
  refetchOrders: vi.fn(),
  refetchOrderDetail: vi.fn(),
  refetchOrderUsage: vi.fn(),
  refetchIssueLimitUsage: vi.fn(),
  refetchPrices: vi.fn(),
  refetchPlans: vi.fn(),
  openExternal: vi.fn(),
  workspaceId: "workspace-1",
  plans: [] as any[],
  plansLoading: false,
  orders: [] as any[],
  ordersLoading: false,
  ordersError: false,
  orderDetailById: {} as Record<string, any>,
  orderUsageById: {} as Record<string, any>,
  subscriptionUsage: null as any,
  subscriptionUsagePending: false,
  subscriptionUsageError: false,
  billingProfile: undefined as WorkspaceBillingProfile | null | undefined,
  prices: null as {
    month: {
      currency: string;
      unitAmount: number;
      interval: "month";
      intervalCount: number;
    };
    year: {
      currency: string;
      unitAmount: number;
      interval: "year";
      intervalCount: number;
    };
  } | null,
  pricesLoading: false,
  pricesFetching: false,
  pricesError: false,
  summaryPending: false,
  summaryFetching: false,
  summaryError: false,
  summaryMalformed: false,
  purchasePending: false,
  usagePending: false,
  usageFetching: false,
  usageError: false,
  summaryDataUpdatedAt: 0,
  summaryFetchedAfterMount: false,
  entitlements: {
    workspaceId: "workspace-1",
    plan: "free",
    status: "inactive",
    seats: 3,
    limits: {
      issueCount: { mode: "limited", limit: 17 },
      autopilotRuns: { mode: "limited", limit: 7 },
    },
    currentPeriodEnd: null as string | null,
    snapshotExpiresAt: null as string | null,
    version: 0,
  } as WorkspaceSubscriptionEntitlements,
  summary: {
    entitlement: null as unknown as WorkspaceSubscriptionEntitlements,
    billingInterval: null,
    humanMembers: 3,
    seatCapacity: null as {
      purchased: number;
      used: number;
      reserved: number;
      available: number;
      version: number;
      pendingQuantity: number | null;
      activePurchase: {
        requestId: string;
        targetSeats: number;
        status: "pending" | "processing" | "submitted";
        expiresAt: string | null;
      } | null;
    } | null,
    cancelAtPeriodEnd: false,
    graceUntil: null as string | null,
    hasStripeCustomer: false,
    availableActions: {
      checkout: true,
      portal: false,
      purchaseSeats: false,
    },
  } as WorkspaceSubscriptionSummary,
  issueLimitUsage: {
    used: 11,
    limit: 17,
  } as IssueLimitUsage,
  usage: {
    action: "enforce" as "off" | "observe" | "enforce",
    used: 3 as number | null,
    reserved: 2 as number | null,
    total: 5 as number | null,
    limit: 7 as number | null,
    reached: false as boolean | null,
    period_start: "2030-01-01T00:00:00Z" as string | null,
    period_end: "2030-02-01T00:00:00Z" as string | null,
    reset_at: "2030-02-01T00:00:00Z" as string | null,
    blocked_counts: {} as Record<string, number> | null,
  } as AutopilotQuotaUsage,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => mocks.useQuery(options),
}));

vi.mock("@multica/core/billing", () => ({
  workspaceBillingProfileOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "billing-profile"],
  }),
  useUpdateWorkspaceBillingProfile: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  usePreviewWorkspacePlanChange: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCreateWorkspacePlanChange: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCancelWorkspaceScheduledPlanChange: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCancelWorkspaceSubscription: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useResumeWorkspaceSubscription: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  workspacePlansOptions: (wsId: string) => ({
    queryKey: ["workspace-plans", wsId, "list"],
  }),
  workspaceOrdersOptions: (wsId: string) => ({
    queryKey: ["workspace-orders", wsId, "list"],
  }),
  workspaceOrderOptions: (wsId: string, orderId: string) => ({
    queryKey: ["workspace-orders", wsId, "detail", orderId],
    enabled: Boolean(wsId && orderId),
  }),
  workspaceOrderUsageOptions: (wsId: string, orderId: string) => ({
    queryKey: ["workspace-orders", wsId, "usage", orderId],
  }),
  useUpdateWorkspaceOrderInvoiceBuyer: () => ({
    mutateAsync: (...args: any[]) => mocks.updateInvoiceBuyer(...args),
    isPending: false,
  }),
  workspaceSubscriptionPricesOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "prices"],
  }),
  workspaceSubscriptionSummaryOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "summary"],
  }),
  workspaceSubscriptionUsageOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "usage"],
  }),
  issueLimitUsageOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "issue-limit-usage"],
  }),
  useCreateWorkspaceSubscriptionCheckout: () => ({
    mutateAsync: mocks.checkout,
    isPending: false,
  }),
  useCreateWorkspaceSubscriptionPortal: () => ({
    mutateAsync: mocks.portal,
    isPending: false,
  }),
  usePreviewWorkspaceSeatPurchase: () => ({
    mutateAsync: mocks.previewSeats,
    isPending: false,
  }),
  usePurchaseWorkspaceSeats: () => ({
    mutateAsync: mocks.purchaseSeats,
    isPending: mocks.purchasePending,
  }),
}));

vi.mock("@multica/core/autopilots", () => ({
  autopilotQuotaUsageOptions: (wsId: string) => ({
    queryKey: ["autopilots", wsId, "usage"],
  }),
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({
    id: mocks.workspaceId,
    slug: "acme",
    name: "Acme",
  }),
}));

const navigationState = {
  search: "tab=billing",
  replace: vi.fn(),
};
vi.mock("../../navigation", () => ({
  useNavigation: () => ({
    pathname: "/acme/settings",
    searchParams: new URLSearchParams(navigationState.search),
    hash: "",
    push: vi.fn(),
    replace: navigationState.replace,
    back: vi.fn(),
    getShareableUrl: vi.fn(),
  }),
}));

vi.mock("../../platform", () => ({ openExternal: mocks.openExternal }));

import { BillingTab, formatStripeMinorAmount } from "./billing-tab";

// The component formats dates in the viewer's own time zone, so a midnight-UTC
// fixture reads as the previous calendar day anywhere behind UTC. Build the
// expected text with the same formatter instead of hard-coding the calendar day.
function formatExpectedDate(iso: string): string {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(
    new Date(iso),
  );
}

const FEB_1_2030 = formatExpectedDate("2030-02-01T00:00:00Z");
const FEB_15_2030 = formatExpectedDate("2030-02-15T00:00:00Z");
const MAR_1_2030 = formatExpectedDate("2030-03-01T00:00:00Z");
const APR_1_2030 = formatExpectedDate("2030-04-01T00:00:00Z");

function setSeatCapacity({
  humanMembers = 4,
  purchased = 5,
  used = humanMembers,
  reserved = 0,
  available = Math.max(0, purchased - used - reserved),
  version = 9,
  pendingQuantity = null,
  activePurchase = null,
}: {
  humanMembers?: number;
  purchased?: number;
  used?: number;
  reserved?: number;
  available?: number;
  version?: number;
  pendingQuantity?: number | null;
  activePurchase?: NonNullable<typeof mocks.summary.seatCapacity>["activePurchase"];
} = {}) {
  mocks.summary.humanMembers = humanMembers;
  mocks.summary.seatCapacity = {
    purchased,
    used,
    reserved,
    available,
    overcommitted: used > purchased,
    version,
    pendingQuantity,
    activePurchase,
  };
  Object.assign(mocks.summary.availableActions, {
    checkout: false,
    portal: true,
    purchaseSeats: activePurchase === null,
  });
}

describe("BillingTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigationState.search = "tab=billing";
    mocks.workspaceId = "workspace-1";
    mocks.prices = {
      month: {
        currency: "usd",
        unitAmount: 1000,
        interval: "month",
        intervalCount: 1,
      },
      year: {
        currency: "usd",
        unitAmount: 9600,
        interval: "year",
        intervalCount: 1,
      },
    };
    mocks.pricesLoading = false;
    mocks.pricesFetching = false;
    mocks.pricesError = false;
    mocks.plans = [];
    mocks.plansLoading = false;
    mocks.orders = [];
    mocks.ordersLoading = false;
    mocks.ordersError = false;
    mocks.orderDetailById = {};
    mocks.orderUsageById = {};
    mocks.subscriptionUsage = null;
    mocks.subscriptionUsagePending = false;
    mocks.subscriptionUsageError = false;
    mocks.billingProfile = undefined;
    mocks.summaryPending = false;
    mocks.summaryFetching = false;
    mocks.summaryError = false;
    mocks.summaryMalformed = false;
    mocks.purchasePending = false;
    mocks.usagePending = false;
    mocks.usageFetching = false;
    mocks.usageError = false;
    mocks.summaryDataUpdatedAt = 0;
    mocks.summaryFetchedAfterMount = false;
    Object.assign(mocks.entitlements, {
      plan: "free",
      status: "inactive",
      seats: 3,
      limits: {
        issueCount: { mode: "limited", limit: 17 },
        autopilotRuns: { mode: "limited", limit: 7 },
      },
      currentPeriodEnd: null,
      snapshotExpiresAt: null,
      version: 0,
    });
    Object.assign(mocks.summary, {
      entitlement: mocks.entitlements,
      billingInterval: null,
      humanMembers: 3,
      seatCapacity: null,
      cancelAtPeriodEnd: false,
      graceUntil: null,
      hasStripeCustomer: false,
      billingModel: undefined,
      usage: undefined,
      availableActions: {
        checkout: true,
        portal: false,
        purchaseSeats: false,
      },
    });
    Object.assign(mocks.issueLimitUsage, {
      used: 11,
      limit: 17,
    });
    Object.assign(mocks.usage, {
      action: "enforce",
      used: 3,
      reserved: 2,
      total: 5,
      limit: 7,
      reached: false,
      period_start: "2030-01-01T00:00:00Z",
      period_end: "2030-02-01T00:00:00Z",
      reset_at: "2030-02-01T00:00:00Z",
      blocked_counts: {},
    });
    mocks.useQuery.mockImplementation((options: unknown) => {
      const queryKey = (options as { queryKey?: readonly unknown[] }).queryKey;
      if (
        queryKey?.[0] === "workspace-plans" ||
        queryKey?.[queryKey.length - 1] === "plans"
      ) {
        return {
          data: mocks.plans,
          isLoading: mocks.plansLoading,
          isFetching: false,
          isError: false,
          refetch: mocks.refetchPlans,
        };
      }
      if (queryKey?.[queryKey.length - 1] === "prices") {
        return {
          data: mocks.prices,
          isLoading: mocks.pricesLoading,
          isFetching: mocks.pricesFetching,
          isError: mocks.pricesError,
          refetch: mocks.refetchPrices,
        };
      }
      if (queryKey?.[queryKey.length - 1] === "summary") {
        return {
          data: mocks.summaryError
            ? undefined
            : mocks.summaryMalformed
              ? null
              : mocks.summary,
          isPending: mocks.summaryPending,
          isFetching: mocks.summaryFetching,
          isError: mocks.summaryError,
          dataUpdatedAt: mocks.summaryDataUpdatedAt,
          isFetchedAfterMount: mocks.summaryFetchedAfterMount,
          refetch: mocks.refetchSummary,
        };
      }
      if (queryKey?.[queryKey.length - 1] === "issue-limit-usage") {
        return {
          data: mocks.issueLimitUsage,
          isPending: false,
          isFetching: false,
          isError: false,
          refetch: mocks.refetchIssueLimitUsage,
        };
      }
      if (
        queryKey?.[0] === "workspace-orders" &&
        queryKey?.[2] === "list"
      ) {
        return {
          data: mocks.ordersError ? undefined : mocks.orders,
          isPending: mocks.ordersLoading,
          isFetching: false,
          isError: mocks.ordersError,
          refetch: mocks.refetchOrders,
        };
      }
      if (
        queryKey?.[0] === "workspace-orders" &&
        queryKey?.[2] === "detail"
      ) {
        const orderId = String(queryKey?.[3] ?? "");
        return {
          data: mocks.orderDetailById[orderId] ?? null,
          isPending: false,
          isFetching: false,
          isError: false,
          refetch: mocks.refetchOrderDetail,
        };
      }
      if (
        queryKey?.[0] === "workspace-orders" &&
        queryKey?.[2] === "usage"
      ) {
        const orderId = String(queryKey?.[3] ?? "");
        return {
          data: mocks.orderUsageById[orderId] ?? null,
          isPending: false,
          isFetching: false,
          isError: false,
          refetch: mocks.refetchOrderUsage,
        };
      }
      if (
        queryKey?.[0] === "workspace-subscriptions" &&
        queryKey?.[queryKey.length - 1] === "billing-profile"
      ) {
        return {
          data: mocks.billingProfile,
          isPending: false,
          isFetching: false,
          isError: false,
          refetch: mocks.refetch,
        };
      }
      if (
        queryKey?.[0] === "workspace-subscriptions" &&
        queryKey?.[queryKey.length - 1] === "usage"
      ) {
        return {
          data: mocks.subscriptionUsageError ? undefined : mocks.subscriptionUsage,
          isPending: mocks.subscriptionUsagePending,
          isFetching: false,
          isError: mocks.subscriptionUsageError,
          refetch: mocks.refetchSubscriptionUsage,
        };
      }
      if (queryKey?.[queryKey.length - 1] === "usage") {
        return {
          data: mocks.usageError ? undefined : mocks.usage,
          isPending: mocks.usagePending,
          isFetching: mocks.usageFetching,
          isError: mocks.usageError,
          refetch: mocks.refetchUsage,
        };
      }
      return {
        data: mocks.entitlements,
        isPending: false,
        isError: false,
        refetch: mocks.refetch,
      };
    });
    mocks.checkout.mockResolvedValue({
      requestId: "request-1",
      sessionId: "cs_test_1",
      url: "https://checkout.stripe.com/test-session",
    });
    mocks.portal.mockResolvedValue({
      url: "https://billing.stripe.com/test-session",
    });
    mocks.refetchSummary.mockResolvedValue({ data: mocks.summary });
    mocks.previewSeats.mockResolvedValue({
      currentSeats: 5,
      additionalSeats: 1,
      resultingSeats: 6,
      purchaseVersion: 9,
      currency: "usd",
      prorationAmount: 250,
      nextInvoiceAmount: 6000,
      quotedAt: "2030-01-01T00:00:00Z",
    });
    mocks.purchaseSeats.mockResolvedValue({
      requestId: "seat-request-1",
      currentSeats: 5,
      additionalSeats: 1,
      resultingSeats: 6,
      currency: "usd",
      prorationAmount: 250,
      nextInvoiceAmount: 6000,
      status: "submitted",
    });
    configStore.getState().setFeatureFlags({
      [BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG]: true,
    });
  });

  it("renders nothing and issues no query when the feature flag is absent", () => {
    configStore.getState().setFeatureFlags({});

    const { container } = renderWithI18n(<BillingTab />);

    expect(container).toBeEmptyDOMElement();
    expect(mocks.useQuery).not.toHaveBeenCalled();
  });

  it("shows server prices and keeps the selected interval in sync", async () => {
    const user = userEvent.setup();
    renderWithI18n(<BillingTab />);

    expect(screen.getByRole("heading", { name: "Billing" })).toBeInTheDocument();
    expect(screen.getByText("11 / 17")).toBeInTheDocument();
    expect(screen.getByText("5 / 7")).toBeInTheDocument();
    expect(screen.getByText("3 completed · 2 in progress")).toBeInTheDocument();
    expect(screen.getByText("$10.00 per human seat")).toBeInTheDocument();
    expect(mocks.useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["workspace-subscriptions", "workspace-1", "prices"],
      }),
    );

    await user.click(screen.getByRole("button", { name: "Yearly" }));

    expect(screen.getByText("$96.00 per human seat")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Retry" }),
    ).not.toBeInTheDocument();
  });

  it("formats minor units with the digits of the shared currency table", () => {
    expect(formatStripeMinorAmount(1234, "usd", "en-US")).toBe("$12.34");
    expect(formatStripeMinorAmount(149000, "vnd", "en-US")).toContain("149,000");
    expect(
      formatStripeMinorAmount(1234, "huf", "en-US")?.replace(/\s/g, " "),
    ).toBe("12.34 HUF");
    expect(formatStripeMinorAmount(1234, "", "en-US")).toBeNull();
  });

  it("shows a local price skeleton without blocking Checkout while prices load", () => {
    mocks.prices = null;
    mocks.pricesLoading = true;

    renderWithI18n(<BillingTab />);

    expect(
      screen.getByRole("status", { name: "Loading subscription prices" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Upgrade to Pro" }),
    ).toBeEnabled();
  });

  it("announces a price-only retry without blocking Checkout", () => {
    mocks.prices = null;
    mocks.pricesFetching = true;

    renderWithI18n(<BillingTab />);

    expect(screen.getByRole("button", { name: "Retry" })).toBeDisabled();
    expect(
      screen.getByRole("status", { name: "Loading subscription prices" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Upgrade to Pro" }),
    ).toBeEnabled();
  });

  it.each([
    ["query error", true],
    ["malformed response", false],
  ])(
    "falls back safely after a prices %s without blocking Checkout",
    async (_case, isError) => {
      const user = userEvent.setup();
      mocks.prices = null;
      mocks.pricesError = isError;

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByText(
          /Stripe Checkout shows the authoritative per-seat price/,
        ),
      ).toBeInTheDocument();
      expect(screen.queryByText(/\$0/)).not.toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "Retry" }));
      expect(mocks.refetchPrices).toHaveBeenCalledOnce();

      await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
      await user.click(
        screen.getByRole("button", { name: "Continue to Stripe" }),
      );

      await waitFor(() => expect(mocks.checkout).toHaveBeenCalledOnce());
    },
  );

  it("uses workspace-scoped price query keys after a workspace switch", () => {
    const { rerender } = renderWithI18n(<BillingTab />);

    mocks.workspaceId = "workspace-2";
    rerender(<BillingTab />);

    expect(mocks.useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["workspace-subscriptions", "workspace-2", "prices"],
      }),
    );
    expect(mocks.useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["workspace-subscriptions", "workspace-2", "summary"],
      }),
    );
    expect(mocks.useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["autopilots", "workspace-2", "usage"],
      }),
    );
  });

  it("shows the unit price without a zero estimated total when seats are unavailable", () => {
    mocks.entitlements.seats = 0;
    mocks.summary.humanMembers = 0;

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("$10.00 per human seat")).toBeInTheDocument();
    expect(screen.queryByText(/Estimated monthly total/)).not.toBeInTheDocument();
    expect(screen.queryByText(/\$0/)).not.toBeInTheDocument();
  });

  it("creates Checkout with a client idempotency key and opens Stripe externally", async () => {
    const user = userEvent.setup();
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    await waitFor(() => {
      expect(mocks.checkout).toHaveBeenCalledTimes(1);
    });
    const request = mocks.checkout.mock.calls[0]?.[0] as {
      interval: string;
      idempotencyKey: string;
    };
    expect(request.interval).toBe("month");
    expect(request.idempotencyKey).toMatch(/^workspace-checkout-workspace-1-/);
    expect(Object.keys(request).sort()).toEqual(["idempotencyKey", "interval"]);
    expect(mocks.openExternal).toHaveBeenCalledWith(
      "https://checkout.stripe.com/test-session",
      { webTarget: "same-tab" },
    );
  });

  it("reuses the Checkout idempotency key after an ambiguous failure", async () => {
    const user = userEvent.setup();
    mocks.checkout
      .mockRejectedValueOnce(new Error("network lost after submit"))
      .mockResolvedValueOnce({
        requestId: "request-1",
        sessionId: "cs_test_1",
        url: "https://checkout.stripe.com/test-session",
      });
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));
    await screen.findByText("Billing action failed");

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    await waitFor(() => expect(mocks.checkout).toHaveBeenCalledTimes(2));
    expect(mocks.checkout.mock.calls[1]?.[0].idempotencyKey).toBe(
      mocks.checkout.mock.calls[0]?.[0].idempotencyKey,
    );
  });

  it("starts a new Checkout intent after explicit cancellation", async () => {
    const user = userEvent.setup();
    mocks.checkout
      .mockRejectedValueOnce(new Error("network lost after submit"))
      .mockResolvedValueOnce({
        requestId: "request-2",
        sessionId: "cs_test_2",
        url: "https://checkout.stripe.com/test-session-2",
      });
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));
    await screen.findByText("Billing action failed");

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    await waitFor(() => expect(mocks.checkout).toHaveBeenCalledTimes(2));
    expect(mocks.checkout.mock.calls[1]?.[0].idempotencyKey).not.toBe(
      mocks.checkout.mock.calls[0]?.[0].idempotencyKey,
    );
  });

  it.each([
    [503, "Billing is temporarily unavailable. Retry in a moment."],
    [
      403,
      "Your workspace role no longer allows this action. Refresh the page to see your current access.",
    ],
  ])("maps a %s Checkout response to actionable copy", async (status, copy) => {
    const user = userEvent.setup();
    mocks.checkout.mockRejectedValue(
      new ApiError("request failed", status, "Request Failed"),
    );
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    expect(await screen.findByText(copy)).toBeInTheDocument();
  });

  it("explains that a disabled billing capability requires an administrator", async () => {
    const user = userEvent.setup();
    mocks.checkout.mockRejectedValue(
      new ApiError("request failed", 403, "Forbidden", {
        code: "workspace_subscriptions_disabled",
      }),
    );
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    expect(
      await screen.findByText(
        "Workspace subscriptions are not enabled for this deployment. Contact your administrator.",
      ),
    ).toBeInTheDocument();
  });

  it("consumes cancel callback params once while preserving the active tab", () => {
    navigationState.search =
      "tab=billing&result=cancel&session_id=cs_test_1&source=email";

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Checkout canceled")).toBeInTheDocument();
    expect(navigationState.replace).toHaveBeenCalledOnce();
    expect(navigationState.replace).toHaveBeenCalledWith(
      "/acme/settings?tab=billing&source=email",
    );
  });

  it("polls after a success callback and surfaces a bounded sync timeout", () => {
    vi.useFakeTimers();
    navigationState.search = "tab=billing&result=success&session_id=cs_test_1";
    try {
      renderWithI18n(<BillingTab />);

      expect(
        screen.getByText("Activating your subscription"),
      ).toBeInTheDocument();
      const summaryOptions = mocks.useQuery.mock.calls
        .map(([options]) => options)
        .find(
          (options) =>
            options.queryKey?.[options.queryKey.length - 1] === "summary",
        );
      expect(
        summaryOptions.refetchInterval({ state: { data: mocks.summary } }),
      ).toBe(2_000);
      expect(navigationState.replace).toHaveBeenCalledWith(
        "/acme/settings?tab=billing",
      );

      act(() => vi.advanceTimersByTime(30_000));

      expect(
        screen.getByText(
          "Payment was received, but the subscription is still syncing. Refresh this page in a moment.",
        ),
      ).toBeInTheDocument();
      expect(navigationState.replace).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps Checkout syncing while Cloud still exposes Checkout", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2030-01-01T00:00:00Z"));
    navigationState.search = "tab=billing&result=success&session_id=cs_test_1";
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summaryDataUpdatedAt = Date.now() - 1;
    mocks.summaryFetchedAfterMount = true;
    try {
      renderWithI18n(<BillingTab />);

      expect(
        screen.getByText("Activating your subscription"),
      ).toBeInTheDocument();
      expect(screen.queryByText("Pro is active")).not.toBeInTheDocument();
      expect(screen.queryByText("Unlimited")).not.toBeInTheDocument();
      expect(screen.getByText("11 / 17")).toBeInTheDocument();
      expect(screen.getByText("5 / 7")).toBeInTheDocument();
      const summaryOptions = mocks.useQuery.mock.calls
        .map(([options]) => options)
        .find(
          (options) =>
            options.queryKey?.[options.queryKey.length - 1] === "summary",
        );
      expect(
        summaryOptions.refetchInterval({ state: { data: mocks.summary } }),
      ).toBe(2_000);
    } finally {
      vi.useRealTimers();
    }
  });

  it("confirms Checkout from refreshed Cloud actions and limit modes", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2030-01-01T00:00:00Z"));
    navigationState.search = "tab=billing&result=success&session_id=cs_test_1";
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      limits: {
        issueCount: { mode: "unlimited", limit: null },
        autopilotRuns: { mode: "unlimited", limit: null },
      },
    });
    mocks.summary.availableActions.checkout = false;
    mocks.summaryDataUpdatedAt = Date.now() + 1;
    mocks.summaryFetchedAfterMount = true;
    try {
      renderWithI18n(<BillingTab />);

      expect(screen.getByText("Pro is active")).toBeInTheDocument();
      expect(screen.getAllByText("Unlimited")).toHaveLength(2);
      expect(screen.queryByText("5 / 7")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps plan facts visible but hides mutations denied by Cloud", () => {
    Object.assign(mocks.summary.availableActions, {
      checkout: false,
      portal: false,
      purchaseSeats: false,
    });

    renderWithI18n(<BillingTab />);

    expect(screen.getAllByText("3 members")).toHaveLength(1);
    expect(screen.queryByText("Purchased seats")).not.toBeInTheDocument();
    expect(screen.queryByText("$10.00 per human seat")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Upgrade to Pro" }),
    ).not.toBeInTheDocument();
  });

  it("renders authoritative subscription seat facts and opens their calculation help", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      currentPeriodEnd: "2030-02-01T00:00:00Z",
    });
    Object.assign(mocks.usage, {
      action: "off",
      used: null,
      reserved: null,
      limit: null,
      reset_at: null,
    });
    Object.assign(mocks.summary, {
      billingInterval: "month",
      hasStripeCustomer: true,
    });
    setSeatCapacity({
      humanMembers: 4,
      purchased: 5,
      used: 4,
      reserved: 1,
      available: 0,
      pendingQuantity: 4,
    });

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Monthly")).toBeInTheDocument();
    expect(screen.getByText("5 seats")).toBeInTheDocument();
    expect(screen.getByText("4 seats")).toBeInTheDocument();
    expect(screen.getByText("1 seat")).toBeInTheDocument();
    expect(screen.getByText("0 seats")).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(`4 seats from ${FEB_1_2030}`)),
    ).toBeInTheDocument();
    expect(screen.getAllByText("4 members")).toHaveLength(1);
    expect(screen.getByText("Available seats").closest("summary")).toBeNull();
    const formula = "Purchased seats minus members and reserved invitations.";
    expect(screen.queryByText(formula)).not.toBeInTheDocument();
    const help = screen.getByRole("button", { name: "How available seats are calculated" });
    help.focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByText(formula)).toBeVisible();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByText(formula)).not.toBeInTheDocument());
    expect(help).toHaveFocus();
  });

  it("quotes and confirms an additive seat purchase", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.previewSeats.mockImplementation(
      ({ additionalSeats }: { additionalSeats: number }) =>
        Promise.resolve({
          currentSeats: 5,
          additionalSeats,
          resultingSeats: 5 + additionalSeats,
          purchaseVersion: 9,
          currency: "usd",
          prorationAmount: 250 * additionalSeats,
          nextInvoiceAmount: 1000 * (5 + additionalSeats),
          quotedAt: "2030-01-01T00:00:00Z",
        }),
    );
    mocks.purchaseSeats.mockResolvedValue({
      requestId: "seat-request-2",
      currentSeats: 5,
      additionalSeats: 2,
      resultingSeats: 7,
      currency: "usd",
      prorationAmount: 500,
      nextInvoiceAmount: 7000,
      status: "submitted",
    });

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));

    await waitFor(() =>
      expect(mocks.previewSeats).toHaveBeenCalledWith({ additionalSeats: 1 }),
    );
    expect(screen.getByText("Estimated charge today")).toBeInTheDocument();
    expect(screen.getByText("$2.50")).toBeInTheDocument();
    expect(screen.getByText("$60.00")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Add 1 seat" }),
    ).toBeInTheDocument();

    const input = screen.getByRole("spinbutton", { name: "Additional seats" });
    await user.clear(input);
    await user.type(input, "2");
    await waitFor(() =>
      expect(mocks.previewSeats).toHaveBeenLastCalledWith({
        additionalSeats: 2,
      }),
    );
    await waitFor(() => expect(screen.getByText("$5.00")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Add 2 seats" }));
    expect(mocks.purchaseSeats).toHaveBeenCalledWith(
      expect.objectContaining({
        additionalSeats: 2,
        expectedCurrentSeats: 5,
        expectedPurchaseVersion: 9,
        acceptedProrationAmount: 500,
        currency: "usd",
        idempotencyKey: expect.stringContaining(
          "workspace-seat-purchase-workspace-1-",
        ),
      }),
    );
    await waitFor(() => expect(mocks.refetchSummary).toHaveBeenCalled());
  });

  it("refreshes and retries a preview after Stripe seat capacity changes", async () => {
    const user = userEvent.setup();
    let resolveSummaryRefresh!: (value: { data: typeof mocks.summary }) => void;
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.refetchSummary.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSummaryRefresh = resolve;
      }),
    );
    mocks.previewSeats
      .mockRejectedValueOnce(
        new ApiError("conflict", 409, "Conflict", {
          code: "seat_capacity_changed",
        }),
      )
      .mockResolvedValueOnce({
        currentSeats: 4,
        additionalSeats: 1,
        resultingSeats: 5,
        purchaseVersion: 10,
        currency: "usd",
        prorationAmount: 250,
        nextInvoiceAmount: 5000,
        quotedAt: "2030-01-01T00:00:00Z",
      });

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));

    await waitFor(() => expect(mocks.refetchSummary).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("status")).toHaveTextContent(
      "Updating estimate...",
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    await act(async () => {
      setSeatCapacity({ purchased: 4, version: 10 });
      resolveSummaryRefresh({ data: mocks.summary });
    });

    await waitFor(
      () => expect(mocks.previewSeats).toHaveBeenCalledTimes(2),
      { timeout: 3_000 },
    );
    expect(await screen.findByText("$2.50")).toBeInTheDocument();
    expect(
      screen.queryByText(
        "The estimate is temporarily unavailable. Retry in a moment.",
      ),
    ).not.toBeInTheDocument();
  });

  it("does not retry when the refreshed seat summary is unchanged", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.previewSeats.mockRejectedValueOnce(
      new ApiError("conflict", 409, "Conflict", {
        code: "seat_capacity_changed",
      }),
    );

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));

    expect(
      await screen.findByText(
        "The seat count in Stripe does not match the billing record. Contact support before retrying this purchase.",
        {},
        { timeout: 3_000 },
      ),
    ).toBeInTheDocument();
    expect(mocks.previewSeats).toHaveBeenCalledTimes(1);
    expect(mocks.refetchSummary).toHaveBeenCalledTimes(1);
  });

  it("keeps a summary refresh failure distinct from a seat mismatch", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.refetchSummary.mockResolvedValueOnce({
      data: mocks.summary,
      isError: true,
    });
    mocks.previewSeats.mockRejectedValueOnce(
      new ApiError("conflict", 409, "Conflict", {
        code: "seat_capacity_changed",
      }),
    );

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));

    expect(
      await screen.findByText(
        "The estimate is temporarily unavailable. Retry in a moment.",
        {},
        { timeout: 3_000 },
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(
        "The seat count in Stripe does not match the billing record. Contact support before retrying this purchase.",
      ),
    ).not.toBeInTheDocument();
    expect(mocks.previewSeats).toHaveBeenCalledTimes(1);
    expect(mocks.refetchSummary).toHaveBeenCalledTimes(1);
  });

  it("does not refresh again when the retried preview still conflicts", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.refetchSummary.mockImplementationOnce(async () => {
      setSeatCapacity({ purchased: 4, version: 10 });
      return { data: mocks.summary };
    });
    mocks.previewSeats.mockRejectedValue(
      new ApiError("conflict", 409, "Conflict", {
        code: "seat_capacity_changed",
      }),
    );

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));

    expect(
      await screen.findByText(
        "The seat count in Stripe does not match the billing record. Contact support before retrying this purchase.",
        {},
        { timeout: 3_000 },
      ),
    ).toBeInTheDocument();
    expect(mocks.previewSeats).toHaveBeenCalledTimes(2);
    expect(mocks.refetchSummary).toHaveBeenCalledTimes(1);
  });

  it.each([
    [
      "seat_quote_changed",
      "The seat count or estimate changed. Review the refreshed quote before confirming again.",
    ],
    [
      "seat_purchase_in_progress",
      "Another seat purchase is being processed. Wait for it to finish; contact support if it remains here.",
    ],
  ])("maps purchase conflict %s to actionable copy", async (code, copy) => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.purchaseSeats.mockRejectedValue(
      new ApiError("conflict", 409, "Conflict", { code }),
    );

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));
    await screen.findByRole("button", { name: "Add 1 seat" });
    await user.click(screen.getByRole("button", { name: "Add 1 seat" }));

    expect(await screen.findByText(copy)).toBeInTheDocument();
    expect(mocks.refetchSummary).toHaveBeenCalled();
    if (code === "seat_quote_changed") {
      await waitFor(() => expect(mocks.previewSeats).toHaveBeenCalledTimes(2));
    } else {
      expect(mocks.previewSeats).toHaveBeenCalledTimes(1);
    }
  });

  it("keeps the seat dialog open while purchase submission is pending", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();

    const { rerender } = renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));
    await screen.findByRole("button", { name: "Add 1 seat" });

    mocks.purchasePending = true;
    rerender(<BillingTab />);
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "Add seats" })).toBeInTheDocument();

    mocks.purchasePending = false;
    rerender(<BillingTab />);
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Add seats" })).not.toBeInTheDocument(),
    );
  });

  it("reuses an unreadable purchase intent after closing and reopening", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.purchaseSeats.mockResolvedValue(null);

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));
    await screen.findByRole("button", { name: "Add 1 seat" });
    await user.click(screen.getByRole("button", { name: "Add 1 seat" }));
    expect(
      await screen.findByText(
        "The purchase response could not be read. Retry this quote; the same purchase request will be reused.",
      ),
    ).toBeInTheDocument();
    const firstKey = mocks.purchaseSeats.mock.calls[0]?.[0].idempotencyKey;

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Add seats" }));
    await user.click(
      await screen.findByRole("button", { name: "Add 1 seat" }),
    );

    expect(mocks.purchaseSeats).toHaveBeenCalledTimes(2);
    expect(mocks.purchaseSeats.mock.calls[1]?.[0].idempotencyKey).toBe(firstKey);
    expect(mocks.previewSeats).toHaveBeenCalledTimes(1);
  });

  it("shows payment recovery copy and releases the local intent on 402", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity();
    mocks.purchaseSeats.mockRejectedValue(
      new ApiError("payment failed", 402, "Payment Required", {
        code: "seat_purchase_payment_failed",
      }),
    );

    renderWithI18n(<BillingTab />);
    await user.click(screen.getByRole("button", { name: "Add seats" }));
    await user.click(
      await screen.findByRole("button", { name: "Add 1 seat" }),
    );

    expect(
      await screen.findByText(
        "Payment could not be completed. Update the payment method in Stripe, then request a new quote.",
      ),
    ).toBeInTheDocument();
  });

  it("stops seat purchase polling after two minutes", () => {
    vi.useFakeTimers();
    Object.assign(mocks.entitlements, { plan: "pro", status: "active" });
    setSeatCapacity({
      activePurchase: {
        requestId: "seat-request-pending",
        targetSeats: 7,
        status: "pending",
        expiresAt: "2030-01-01T00:15:00Z",
      },
    });
    try {
      renderWithI18n(<BillingTab />);
      expect(screen.getByText("Seat purchase processing")).toBeInTheDocument();

      act(() => vi.advanceTimersByTime(2 * 60_000));

      expect(
        screen.getByText("Seat confirmation is taking longer than expected"),
      ).toBeInTheDocument();
      const formattedExpiry = new Intl.DateTimeFormat("en", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date("2030-01-01T00:15:00Z"));
      expect(
        screen.getByText(
          `Automatic checking has stopped. If the attempt is still pending after ${formattedExpiry}, reload this page before requesting another quote; contact support if it remains pending.`,
        ),
      ).toBeInTheDocument();
      const summaryOptions = mocks.useQuery.mock.calls
        .map(([options]) => options)
        .filter(
          (options) =>
            options.queryKey?.[options.queryKey.length - 1] === "summary",
        )
        .at(-1);
      expect(
        summaryOptions.refetchInterval({ state: { data: mocks.summary } }),
      ).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not promise an unlock time for a submitted seat purchase", () => {
    vi.useFakeTimers();
    Object.assign(mocks.entitlements, { plan: "pro", status: "active" });
    setSeatCapacity({
      activePurchase: {
        requestId: "seat-request-submitted",
        targetSeats: 7,
        status: "submitted",
        expiresAt: null,
      },
    });
    try {
      renderWithI18n(<BillingTab />);
      act(() => vi.advanceTimersByTime(2 * 60_000));

      expect(
        screen.getByText(
          "Automatic checking has stopped. The purchase may still complete; check again later or contact support before starting another purchase.",
        ),
      ).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("renders Cloud's completed, reserved, total, and reached facts", () => {
    Object.assign(mocks.usage, {
      used: 5,
      reserved: 2,
      total: 7,
      limit: 7,
      reached: true,
    });

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Limit reached")).toBeInTheDocument();
    expect(screen.getByText("7 / 7")).toBeInTheDocument();
    expect(screen.getByText("5 completed · 2 in progress")).toBeInTheDocument();
  });

  it("does not render missing limited usage as zero or unlimited", async () => {
    const user = userEvent.setup();
    Object.assign(mocks.usage, {
      action: "off",
      used: null,
      reserved: null,
      limit: null,
      reset_at: null,
    });

    renderWithI18n(<BillingTab />);

    expect(
      screen.getByText("Usage is temporarily unavailable"),
    ).toBeInTheDocument();
    expect(screen.getByText("7 / month")).toBeInTheDocument();
    expect(screen.queryByText("Unlimited")).not.toBeInTheDocument();
    expect(screen.queryByText(/0 \/ 7/)).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Retry automation usage" }),
    );
    expect(mocks.refetchUsage).toHaveBeenCalledOnce();
  });

  it.each([
    ["request fails", true, false],
    ["response is malformed", false, true],
  ])(
    "fails closed when the authoritative subscription summary %s",
    (_case, isError, isMalformed) => {
      mocks.summaryError = isError;
      mocks.summaryMalformed = isMalformed;

      renderWithI18n(<BillingTab />);

      expect(screen.getByText("Billing is temporarily unavailable")).toBeInTheDocument();
      expect(screen.queryByText("Free")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    },
  );

  it.each([
    ["incomplete", "Subscription setup is incomplete"],
    ["incomplete_expired", "Subscription setup expired"],
    ["paused", "Subscription is paused"],
    ["unpaid", "Subscription is unpaid"],
    ["canceled", "Subscription is canceled"],
  ])("renders a recovery notice for %s", (status, title) => {
    mocks.entitlements.status = status;

    renderWithI18n(<BillingTab />);

    expect(screen.getByText(title)).toBeInTheDocument();
  });

  it("shows grace and scheduled-cancellation dates from subscription facts", () => {
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "past_due",
      currentPeriodEnd: "2030-03-01T00:00:00Z",
      snapshotExpiresAt: "2030-02-15T00:00:00Z",
    });
    Object.assign(mocks.summary, {
      graceUntil: "2030-02-15T00:00:00Z",
      cancelAtPeriodEnd: true,
      hasStripeCustomer: true,
    });
    setSeatCapacity();

    renderWithI18n(<BillingTab />);

    expect(
      screen.getByText(
        new RegExp(`Update your payment method by ${FEB_15_2030}`),
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Cancellation is scheduled")).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(`subscription will cancel on ${MAR_1_2030}`)),
    ).toBeInTheDocument();
  });

  it("renders Cloud-resolved Free access without deriving from past_due", () => {
    Object.assign(mocks.entitlements, {
      plan: "free",
      status: "past_due",
    });
    Object.assign(mocks.summary, {
      graceUntil: null,
      availableActions: {
        checkout: false,
        portal: true,
        purchaseSeats: false,
      },
    });

    renderWithI18n(<BillingTab />);

    expect(screen.queryByText("Unlimited")).not.toBeInTheDocument();
    expect(screen.getByText("11 / 17")).toBeInTheDocument();
    expect(screen.getByText("5 / 7")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Update your payment method in billing management.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/keep Pro access/)).not.toBeInTheDocument();
  });

  it("keeps cancellation and pending-seat dates on one summary snapshot", () => {
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      currentPeriodEnd: "2030-03-01T00:00:00Z",
    });
    Object.assign(mocks.summary, {
      entitlement: {
        ...mocks.entitlements,
        currentPeriodEnd: "2030-04-01T00:00:00Z",
      },
      cancelAtPeriodEnd: true,
      hasStripeCustomer: true,
    });
    setSeatCapacity({ pendingQuantity: 4 });

    renderWithI18n(<BillingTab />);

    expect(
      screen.getByText(new RegExp(`subscription will cancel on ${APR_1_2030}`)),
    ).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(`4 seats from ${APR_1_2030}`)),
    ).toBeInTheDocument();
    expect(screen.getByText(APR_1_2030)).toBeInTheDocument();
    expect(screen.queryByText(MAR_1_2030)).not.toBeInTheDocument();
    expect(
      screen.queryByText(new RegExp(`subscription will cancel on ${MAR_1_2030}`)),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Pro remains available/)).not.toBeInTheDocument();
  });

  it("shows Cloud's explicit unlimited limits on a normal load", () => {
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      limits: {
        issueCount: { mode: "unlimited", limit: null },
        autopilotRuns: { mode: "unlimited", limit: null },
      },
      currentPeriodEnd: "2026-09-13T00:00:00Z",
      version: 3,
    });
    mocks.summary.hasStripeCustomer = true;
    setSeatCapacity({ humanMembers: 3, purchased: 3 });

    renderWithI18n(<BillingTab />);

    expect(
      screen.getByRole("button", { name: "Manage billing" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Unlimited")).toHaveLength(2);
    expect(screen.queryByText("5 / 7")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Upgrade to Pro" }),
    ).not.toBeInTheDocument();
    expect(mocks.useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["workspace-subscriptions", "workspace-1", "prices"],
        enabled: false,
      }),
    );
  });

  it("warns when actual members exceed purchased seats", () => {
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      version: 3,
    });
    setSeatCapacity({ humanMembers: 5, purchased: 4, used: 5 });

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Members exceed purchased seats")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This workspace has 5 members but only 4 purchased seats. Add enough seats or remove members before sending more invitations.",
      ),
    ).toBeInTheDocument();
  });

  it("does not expose a transient used-seat ledger delay to customers", () => {
    Object.assign(mocks.entitlements, {
      plan: "pro",
      status: "active",
      version: 3,
    });
    setSeatCapacity({ humanMembers: 5, purchased: 5, used: 5 });

    renderWithI18n(<BillingTab />);

    expect(
      screen.queryByText("Members exceed purchased seats"),
    ).not.toBeInTheDocument();
  });

  it.each(["canceled", "incomplete_expired"])(
    "keeps management and self-service repurchase available for %s Free",
    (status) => {
      Object.assign(mocks.entitlements, {
        plan: "free",
        status,
        currentPeriodEnd: "2026-09-13T00:00:00Z",
        version: 4,
      });
      Object.assign(mocks.summary, {
        seatCapacity: null,
        hasStripeCustomer: true,
        availableActions: {
          checkout: true,
          portal: true,
          purchaseSeats: false,
        },
      });

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("button", { name: "Manage billing" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Upgrade to Pro" }),
      ).toBeInTheDocument();
      expect(screen.queryByText("Purchased seats")).not.toBeInTheDocument();
      expect(mocks.useQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: ["workspace-subscriptions", "workspace-1", "prices"],
          enabled: true,
        }),
      );
    },
  );

  it("keeps payment recovery available when past-due grace has resolved to Free", () => {
    Object.assign(mocks.entitlements, {
      plan: "free",
      status: "past_due",
      snapshotExpiresAt: null,
      version: 4,
    });
    Object.assign(mocks.summary, {
      graceUntil: null,
      hasStripeCustomer: true,
      availableActions: {
        checkout: false,
        portal: true,
        purchaseSeats: false,
      },
    });

    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Payment needs attention")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Update your payment method in billing management.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/keep Pro access/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Manage billing" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Upgrade to Pro" }),
    ).not.toBeInTheDocument();
  });

  it("renders 'Create order' button label when payment_provider is manual, and 'Continue to Stripe' when absent", async () => {
    const user = userEvent.setup();

    // 1. With payment_provider = "manual"
    (mocks.summary as any).paymentProvider = "manual";
    const { unmount } = renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    expect(
      screen.getByRole("button", { name: "Create order" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Continue to Stripe" }),
    ).not.toBeInTheDocument();
    unmount();

    // 2. With payment_provider absent
    delete (mocks.summary as any).paymentProvider;
    delete (mocks.summary as any).payment_provider;
    renderWithI18n(<BillingTab />);

    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    expect(
      screen.getByRole("button", { name: "Continue to Stripe" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create order" }),
    ).not.toBeInTheDocument();
  });

  it("skips the Stripe return step when payment_provider is manual, and shows it when absent", async () => {
    navigationState.search = "tab=billing&result=success";

    // 1. When payment_provider is manual: Stripe return banner is skipped
    (mocks.summary as any).paymentProvider = "manual";
    const { unmount } = renderWithI18n(<BillingTab />);

    expect(
      screen.queryByText("Activating your subscription"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "Payment is complete. We're waiting for Stripe to confirm the plan change.",
      ),
    ).not.toBeInTheDocument();
    unmount();

    // 2. When payment_provider is absent: Stripe return banner is shown as normal
    delete (mocks.summary as any).paymentProvider;
    delete (mocks.summary as any).payment_provider;
    renderWithI18n(<BillingTab />);

    expect(
      screen.getByText("Activating your subscription"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Payment is complete. We're waiting for Stripe to confirm the plan change.",
      ),
    ).toBeInTheDocument();
  });

  it("renders the plan selector when public plans are available and passes selected plan to checkout", async () => {
    const user = userEvent.setup();
    mocks.plans = [
      {
        key: "pro",
        name: "Pro",
        currency: "vnd",
        seatPriceMonth: 149000,
        seatPriceYear: 1490000,
      },
      {
        key: "business",
        name: "Business",
        currency: "vnd",
        seatPriceMonth: 349000,
        seatPriceYear: 3490000,
      },
    ];

    renderWithI18n(<BillingTab />);

    expect(screen.getByRole("button", { name: "Pro" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Business" }),
    ).toBeInTheDocument();

    // Select Business plan
    await user.click(screen.getByRole("button", { name: "Business" }));
    expect(screen.getByRole("button", { name: "Business" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Click upgrade and checkout
    await user.click(screen.getByRole("button", { name: "Upgrade to Pro" }));
    await user.click(screen.getByRole("button", { name: "Continue to Stripe" }));

    await waitFor(() => {
      expect(mocks.checkout).toHaveBeenCalledTimes(1);
    });
    const request = mocks.checkout.mock.calls[0]?.[0] as any;
    expect(request.planKey ?? request.plan_key).toBe("business");
    expect(request.seats).toBe(3);
  });

  describe("subscription management", () => {
    beforeEach(() => {
      configStore.getState().setFeatureFlags({
        [BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG]: true,
      });
      mocks.summaryPending = false;
      mocks.summaryError = false;
      mocks.summaryMalformed = false;
      mocks.summary.pendingChange = null;
      mocks.summary.cancelAtPeriodEnd = false;
      mocks.entitlements.currentPeriodEnd = null;
      Object.assign(mocks.summary.availableActions, {
        checkout: false,
        portal: false,
        purchaseSeats: false,
        changePlan: false,
        cancel: false,
        resume: false,
        editProfile: false,
      });
    });

    it("shows the scheduled change banner from the summary", () => {
      mocks.summary.pendingChange = {
        planKey: "pro",
        interval: "year",
        seats: 2,
        requestedAt: "2030-01-15T00:00:00Z",
      };
      mocks.entitlements.currentPeriodEnd = "2030-02-01T00:00:00Z";
      Object.assign(mocks.summary.availableActions, { changePlan: true });

      renderWithI18n(<BillingTab />);

      expect(screen.getByText("Scheduled change")).toBeInTheDocument();
      expect(screen.getByText(/Your plan changes to .* on /)).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Cancel scheduled change" }),
      ).toBeInTheDocument();
    });

    it("hides plan, cancel, resume and profile controls when available_actions has none", () => {
      renderWithI18n(<BillingTab />);

      expect(
        screen.queryByRole("button", { name: "Change plan" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Cancel subscription" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Resume subscription" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Save profile" }),
      ).not.toBeInTheDocument();
    });

    it("offers resume only when available_actions allows it", () => {
      mocks.summary.cancelAtPeriodEnd = true;
      Object.assign(mocks.summary.availableActions, { resume: true });

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("button", { name: "Resume subscription" }),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Cancel subscription" }),
      ).not.toBeInTheDocument();
    });
  });

  describe("usage billing", () => {
    beforeEach(() => {
      configStore.getState().setFeatureFlags({
        [BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG]: true,
      });
    });

    it("renders the usage card and unpaid usage banner when unpaidOrderId is set", () => {
      Object.assign(mocks.entitlements, {
        plan: "payg",
        status: "active",
        limits: {
          issueCount: { mode: "unlimited", limit: null },
          autopilotRuns: { mode: "unlimited", limit: null },
        },
      });
      mocks.summary.billingModel = "usage";
      mocks.summary.usage = {
        periodStart: "2026-08-01T00:00:00Z",
        periodEnd: "2026-09-01T00:00:00Z",
        currency: "vnd",
        accruedAmount: 1_700_000,
        creditLimit: 2_000_000,
        unpaidOrderId: "ord-unpaid-1",
        meters: [
          {
            key: "tasks_run",
            quantity: 2000,
            unitPrice: 30000,
            per: 1000,
            discountPct: 0,
            amount: 60000,
          },
        ],
      };
      mocks.subscriptionUsage = {
        billingModel: "usage",
        usage: mocks.summary.usage,
      };

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("heading", { name: "Usage this period" }),
      ).toBeInTheDocument();
      expect(screen.getByText("Unpaid usage invoice")).toBeInTheDocument();
      expect(
        screen.getByText("Approaching credit limit"),
      ).toBeInTheDocument();
    });

    it("hides the unpaid usage banner when unpaidOrderId is null", () => {
      Object.assign(mocks.entitlements, {
        plan: "payg",
        status: "active",
      });
      mocks.summary.billingModel = "usage";
      mocks.summary.usage = {
        periodStart: "2026-08-01T00:00:00Z",
        periodEnd: "2026-09-01T00:00:00Z",
        currency: "vnd",
        accruedAmount: 60_000,
        creditLimit: 2_000_000,
        unpaidOrderId: null,
        meters: [
          {
            key: "tasks_run",
            quantity: 2000,
            unitPrice: 30000,
            per: 1000,
            discountPct: 0,
            amount: 60000,
          },
        ],
      };
      mocks.subscriptionUsage = {
        billingModel: "usage",
        usage: mocks.summary.usage,
      };

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("heading", { name: "Usage this period" }),
      ).toBeInTheDocument();
      expect(
        screen.queryByText("Unpaid usage invoice"),
      ).not.toBeInTheDocument();
    });

    it("shows usage plan rates and displays Business discount only when effectiveDiscountPct > 0", () => {
      mocks.plans = [
        {
          key: "pro",
          name: "Pro",
          currency: "vnd",
          seatPriceMonth: 149000,
          seatPriceYear: 1490000,
          audience: "any",
          billingModel: "seat",
          usageRates: {},
          effectiveDiscountPct: 0,
        },
        {
          key: "payg",
          name: "Pay as you go",
          currency: "vnd",
          seatPriceMonth: null,
          seatPriceYear: null,
          audience: "any",
          billingModel: "usage",
          usageRates: {
            tasks_run: { per: 1000, price: 30000 },
            storage_bytes: { per: 1073741824, price: 3000 },
            members: { per: 1, price: 49000 },
          },
          effectiveDiscountPct: 20,
        },
      ];

      const { unmount } = renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("button", { name: "Pay as you go" }),
      ).toBeInTheDocument();
      expect(screen.getByText(/Tasks run/)).toBeInTheDocument();
      expect(screen.getByText(/Business discount: 20%/)).toBeInTheDocument();
      unmount();

      mocks.plans[1].effectiveDiscountPct = 0;
      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("button", { name: "Pay as you go" }),
      ).toBeInTheDocument();
      expect(screen.getByText(/Tasks run/)).toBeInTheDocument();
      expect(
        screen.queryByText(/Business discount/i),
      ).not.toBeInTheDocument();
    });

    it("lists a usage order in invoices and expands it to per-day usage lines", async () => {
      const user = userEvent.setup();
      Object.assign(mocks.entitlements, {
        plan: "payg",
        status: "active",
      });
      mocks.summary.billingModel = "usage";
      mocks.summary.usage = {
        periodStart: "2026-08-01T00:00:00Z",
        periodEnd: "2026-09-01T00:00:00Z",
        currency: "vnd",
        accruedAmount: 0,
        creditLimit: 2_000_000,
        unpaidOrderId: "ord-usage-1",
        meters: [],
      };
      mocks.orders = [
        {
          id: "ord-usage-1",
          workspaceId: "workspace-1",
          kind: "usage",
          planKey: "payg",
          interval: "month",
          seats: 0,
          amount: 1_234_567,
          vatAmount: 123_457,
          totalAmount: 1_358_024,
          currency: "vnd",
          status: "pending",
          provider: "manual",
          transferCode: "MUL-USAGE-1",
          bankDetails: { bankName: "", accountName: "", accountNumber: "" },
          buyer: { name: "", taxCode: "", address: "", invoiceEmail: "" },
          periodStart: "2026-07-01T00:00:00Z",
          periodEnd: "2026-08-01T00:00:00Z",
          expiresAt: null,
          paidAt: null,
          createdAt: "2026-08-02T00:00:00Z",
        },
      ];
      mocks.orderUsageById["ord-usage-1"] = {
        orderId: "ord-usage-1",
        lines: [
          {
            day: "2026-07-15",
            meter: "tasks_run",
            quantity: 120,
            unitPrice: 30000,
            per: 1000,
            discountPct: 20,
            netAmount: 2880,
            note: "",
          },
        ],
      };

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("heading", { name: "Invoices" }),
      ).toBeInTheDocument();
      expect(screen.getByText("Usage invoice")).toBeInTheDocument();
      expect(screen.queryByText("120")).not.toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "View details" }));

      expect(await screen.findByText("Tasks run")).toBeInTheDocument();
      expect(screen.getByText("120")).toBeInTheDocument();
      expect(screen.getByText("-20%")).toBeInTheDocument();
    });

    it("expands the unpaid usage invoice when clicking View invoice on the banner", async () => {
      const user = userEvent.setup();
      Object.assign(mocks.entitlements, {
        plan: "payg",
        status: "past_due",
      });
      mocks.summary.billingModel = "usage";
      mocks.summary.usage = {
        periodStart: "2026-08-01T00:00:00Z",
        periodEnd: "2026-09-01T00:00:00Z",
        currency: "vnd",
        accruedAmount: 0,
        creditLimit: 2_000_000,
        unpaidOrderId: "ord-usage-unpaid",
        meters: [],
      };
      mocks.orders = [
        {
          id: "ord-usage-unpaid",
          workspaceId: "workspace-1",
          kind: "usage",
          planKey: "payg",
          interval: "month",
          seats: 0,
          amount: 100_000,
          vatAmount: 10_000,
          totalAmount: 110_000,
          currency: "vnd",
          status: "pending",
          provider: "manual",
          transferCode: "MUL-USAGE-2",
          bankDetails: { bankName: "", accountName: "", accountNumber: "" },
          buyer: { name: "", taxCode: "", address: "", invoiceEmail: "" },
          periodStart: "2026-07-01T00:00:00Z",
          periodEnd: "2026-08-01T00:00:00Z",
          expiresAt: null,
          paidAt: null,
          createdAt: "2026-08-02T00:00:00Z",
        },
      ];
      mocks.orderUsageById["ord-usage-unpaid"] = {
        orderId: "ord-usage-unpaid",
        lines: [
          {
            day: "2026-07-20",
            meter: "members",
            quantity: 2,
            unitPrice: 49000,
            per: 1,
            discountPct: 0,
            netAmount: 98000,
            note: "",
          },
        ],
      };

      renderWithI18n(<BillingTab />);

      expect(screen.queryByText("Members")).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "View invoice" }));
      expect(await screen.findByText("Members")).toBeInTheDocument();
    });
  });

  describe("optional field degradation (AC48)", () => {
    it("hides usage card, pending banner, scheduled banner, and unpaid usage banner when optional fields are absent", () => {
      delete (mocks.summary as any).pendingChange;
      delete (mocks.summary as any).paymentProvider;
      delete (mocks.summary as any).payment_provider;
      delete (mocks.summary as any).billingModel;
      delete (mocks.summary as any).usage;
      delete (mocks.summary as any).scheduledPriceChange;
      delete (mocks.summary as any).scheduled_price_change;
      mocks.plans = [
        {
          key: "pro",
          name: "Pro",
          currency: "vnd",
          seatPriceMonth: 149000,
          seatPriceYear: 1490000,
        },
      ];
      mocks.billingProfile = undefined;

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("heading", { name: "Billing" }),
      ).toBeInTheDocument();
      expect(screen.getByText("Free")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Pro" })).toBeInTheDocument();
      expect(
        screen.queryByRole("heading", { name: "Usage this period" }),
      ).not.toBeInTheDocument();
      expect(screen.queryByText("Scheduled change")).not.toBeInTheDocument();
      expect(
        screen.queryByText("Scheduled price change"),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByText("Unpaid usage invoice"),
      ).not.toBeInTheDocument();
      expect(screen.queryByText("Billing profile")).not.toBeInTheDocument();
      expect(mocks.useQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: ["workspace-subscriptions", "workspace-1", "usage"],
          enabled: false,
        }),
      );
    });

    it("renders all optional features when summary, plans, and billing profile carry them", () => {
      Object.assign(mocks.entitlements, {
        plan: "payg",
        status: "active",
        currentPeriodEnd: "2030-03-01T00:00:00Z",
      });
      Object.assign(mocks.summary, {
        paymentProvider: "manual",
        billingModel: "usage",
        pendingChange: {
          planKey: "pro",
          interval: "year",
          seats: 2,
          requestedAt: "2030-01-15T00:00:00Z",
        },
        usage: {
          periodStart: "2026-08-01T00:00:00Z",
          periodEnd: "2026-09-01T00:00:00Z",
          currency: "vnd",
          accruedAmount: 1_700_000,
          creditLimit: 2_000_000,
          unpaidOrderId: "ord-unpaid-1",
          meters: [
            {
              key: "tasks_run",
              quantity: 2000,
              unitPrice: 30000,
              per: 1000,
              discountPct: 20,
              amount: 48000,
            },
          ],
        },
        availableActions: {
          checkout: false,
          portal: false,
          purchaseSeats: false,
          changePlan: true,
          cancel: true,
          resume: false,
        },
      });
      mocks.subscriptionUsage = {
        billingModel: "usage",
        usage: mocks.summary.usage,
      };
      mocks.billingProfile = {
        accountKind: "business",
        legalName: "Acme Co",
        taxCode: "0123456789",
        address: "1 Main St",
        invoiceEmail: "billing@acme.test",
        verified: true,
      };
      mocks.plans = [
        {
          key: "payg",
          name: "Pay as you go",
          currency: "vnd",
          seatPriceMonth: null,
          seatPriceYear: null,
          audience: "any",
          billingModel: "usage",
          usageRates: { tasks_run: { per: 1000, price: 30000 } },
          effectiveDiscountPct: 20,
          scheduledPriceChange: {
            effectiveFrom: "2030-04-01T00:00:00Z",
            seatPriceMonth: 249000,
            seatPriceYear: 2490000,
          },
        },
        {
          key: "pro",
          name: "Pro",
          currency: "vnd",
          seatPriceMonth: 149000,
          seatPriceYear: 1490000,
          audience: "business",
          billingModel: "seat",
          usageRates: {},
          effectiveDiscountPct: 0,
          scheduledPriceChange: null,
        },
        {
          key: "personal",
          name: "Personal",
          currency: "vnd",
          seatPriceMonth: 99000,
          seatPriceYear: 990000,
          audience: "individual",
          billingModel: "seat",
          usageRates: {},
          effectiveDiscountPct: 0,
          scheduledPriceChange: null,
        },
      ];

      renderWithI18n(<BillingTab />);

      expect(
        screen.getByRole("heading", { name: "Usage this period" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Approaching credit limit"),
      ).toBeInTheDocument();
      expect(screen.getByText("Scheduled change")).toBeInTheDocument();
      expect(screen.getByText("Scheduled price change")).toBeInTheDocument();
      expect(screen.getByText("Unpaid usage invoice")).toBeInTheDocument();
      expect(
        screen.getByRole("heading", { name: "Billing profile" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("heading", { name: "Invoices" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Cancel subscription" }),
      ).toBeInTheDocument();
      expect(screen.getByText(/Business discount: 20%/)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Pro" })).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Personal" }),
      ).not.toBeInTheDocument();
    });

    describe("e-invoice", () => {
      const basePaidOrder = {
        id: "ord-paid-1",
        workspaceId: "workspace-1",
        kind: "subscribe",
        planKey: "pro",
        interval: "month",
        seats: 3,
        amount: 447000,
        vatAmount: 44700,
        totalAmount: 491700,
        currency: "vnd",
        status: "paid",
        provider: "manual",
        transferCode: "MUL-PAID-1",
        bankDetails: { bankName: "", accountName: "", accountNumber: "" },
        buyer: {
          name: "Acme Corp",
          taxCode: "0101234567",
          address: "1 Le Loi",
          invoiceEmail: "billing@acme.test",
        },
        periodStart: "2026-07-01T00:00:00Z",
        periodEnd: "2026-08-01T00:00:00Z",
        expiresAt: null,
        paidAt: "2026-07-01T01:00:00Z",
        createdAt: "2026-07-01T00:00:00Z",
        einvoice: null,
      };

      it("shows nothing when einvoice is absent on a paid order", async () => {
        const user = userEvent.setup();
        mocks.orders = [basePaidOrder];
        mocks.orderDetailById["ord-paid-1"] = { ...basePaidOrder, einvoice: null };

        renderWithI18n(<BillingTab />);

        expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", { name: "View details" }));
        expect(screen.queryByText("E-invoice")).not.toBeInTheDocument();
        expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
      });

      it("shows the https lookup link when expanded paid order detail is issued, and omits unsafe URLs", async () => {
        const user = userEvent.setup();
        mocks.orders = [basePaidOrder];
        mocks.orderDetailById["ord-paid-1"] = {
          ...basePaidOrder,
          einvoice: {
            state: "issued",
            number: "00009999",
            lookupUrl: "https://einvoice.sepay.vn/lookup/ord-1",
            missingFields: [],
          },
        };

        const { unmount } = renderWithI18n(<BillingTab />);
        await user.click(screen.getByRole("button", { name: "View details" }));

        const link = await screen.findByRole("link", { name: /View e-invoice/ });
        expect(link).toHaveAttribute("href", "https://einvoice.sepay.vn/lookup/ord-1");
        expect(screen.getByText("00009999")).toBeInTheDocument();
        unmount();

        mocks.orderDetailById["ord-paid-1"] = {
          ...basePaidOrder,
          einvoice: {
            state: "issued",
            number: "00009999",
            lookupUrl: "http://insecure.example.com/ord-1",
            missingFields: [],
          },
        };
        renderWithI18n(<BillingTab />);
        await user.click(screen.getByRole("button", { name: "View details" }));
        expect(await screen.findByText("Issued")).toBeInTheDocument();
        expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
      });

      it("shows the buyer edit form for needs_info and failed states and submits update", async () => {
        const user = userEvent.setup();
        mocks.updateInvoiceBuyer.mockResolvedValue({
          ...basePaidOrder,
          einvoice: {
            state: "pending",
            number: "",
            lookupUrl: "",
            missingFields: [],
          },
        });
        mocks.orders = [basePaidOrder];
        mocks.orderDetailById["ord-paid-1"] = {
          ...basePaidOrder,
          einvoice: {
            state: "needs_info",
            number: "",
            lookupUrl: "",
            missingFields: ["tax_code"],
          },
        };

        const { unmount } = renderWithI18n(<BillingTab />);
        await user.click(screen.getByRole("button", { name: "View details" }));

        expect(await screen.findByText("Needs buyer details")).toBeInTheDocument();
        const taxInput = screen.getByLabelText("Tax code");
        await user.clear(taxInput);
        await user.type(taxInput, "0301112223");
        await user.click(screen.getByRole("button", { name: "Save buyer details" }));

        await waitFor(() => {
          expect(mocks.updateInvoiceBuyer).toHaveBeenCalledWith({
            orderId: "ord-paid-1",
            buyer: {
              name: "Acme Corp",
              taxCode: "0301112223",
              address: "1 Le Loi",
              invoiceEmail: "billing@acme.test",
            },
          });
        });
        unmount();

        mocks.orderDetailById["ord-paid-1"] = {
          ...basePaidOrder,
          einvoice: {
            state: "failed",
            number: "",
            lookupUrl: "",
            missingFields: [],
          },
        };
        renderWithI18n(<BillingTab />);
        await user.click(screen.getByRole("button", { name: "View details" }));
        expect(await screen.findByText("Failed")).toBeInTheDocument();
        expect(
          screen.getByRole("button", { name: "Save buyer details" }),
        ).toBeEnabled();
      });
    });
  });
});
