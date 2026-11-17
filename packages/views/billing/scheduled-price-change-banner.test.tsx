import { screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ScheduledPriceChange } from "@multica/core/billing";
import { configStore } from "@multica/core/config";
import { BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG } from "@multica/core/feature-flags";
import type { WorkspaceSubscriptionSummary } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { ScheduledPriceChangeBanner } from "./scheduled-price-change-banner";

const EFFECTIVE_DATE = "2030-02-01T00:00:00Z";
const DATE_TEXT_EN = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
  timeZone: "UTC",
}).format(new Date(EFFECTIVE_DATE));
const DATE_TEXT_ZH = new Intl.DateTimeFormat("zh-Hans", {
  dateStyle: "medium",
  timeZone: "UTC",
}).format(new Date(EFFECTIVE_DATE));

const scheduled: ScheduledPriceChange = {
  effectiveFrom: EFFECTIVE_DATE,
  seatPriceMonth: 249000,
  seatPriceYear: 2490000,
};

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  plans: [] as any[],
  summary: {
    entitlement: {
      workspaceId: "ws-1",
      plan: "pro",
      status: "active",
      seats: 3,
      limits: {
        issueCount: { mode: "unlimited", limit: null },
        autopilotRuns: { mode: "limited", limit: 2000 },
      },
      currentPeriodEnd: "2030-03-01T00:00:00Z",
      snapshotExpiresAt: null,
      version: 1,
    },
    billingInterval: "month",
    humanMembers: 3,
    seatCapacity: null,
    cancelAtPeriodEnd: false,
    graceUntil: null,
    hasStripeCustomer: false,
    availableActions: {
      checkout: false,
      portal: false,
      purchaseSeats: false,
      changePlan: true,
    },
    pendingChange: null,
    billingModel: "seat",
    usage: null,
  } as WorkspaceSubscriptionSummary,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => mocks.useQuery(options),
}));

vi.mock("@multica/core/billing", () => ({
  workspaceBillingProfileOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "billing-profile"],
  }),
  useUpdateWorkspaceBillingProfile: () => ({ mutateAsync: vi.fn(), isPending: false }),
  usePreviewWorkspacePlanChange: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateWorkspacePlanChange: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCancelWorkspaceScheduledPlanChange: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCancelWorkspaceSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResumeWorkspaceSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  workspacePlansOptions: (wsId: string) => ({
    queryKey: ["workspace-plans", wsId, "list"],
  }),
  workspaceOrdersOptions: (wsId: string) => ({
    queryKey: ["workspace-orders", wsId, "list"],
  }),
  workspaceOrderUsageOptions: (wsId: string, orderId: string) => ({
    queryKey: ["workspace-orders", wsId, "usage", orderId],
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
  useCreateWorkspaceSubscriptionCheckout: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateWorkspaceSubscriptionPortal: () => ({ mutateAsync: vi.fn(), isPending: false }),
  usePreviewWorkspaceSeatPurchase: () => ({ mutateAsync: vi.fn(), isPending: false }),
  usePurchaseWorkspaceSeats: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("@multica/core/autopilots", () => ({
  autopilotQuotaUsageOptions: (wsId: string) => ({
    queryKey: ["autopilots", wsId, "usage"],
  }),
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme", name: "Acme" }),
}));

vi.mock("../navigation", () => ({
  useNavigation: () => ({
    pathname: "/acme/settings",
    searchParams: new URLSearchParams("tab=billing"),
    hash: "",
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    getShareableUrl: vi.fn(),
  }),
}));

vi.mock("../platform", () => ({ openExternal: vi.fn() }));

import { BillingTab } from "../settings/components/billing-tab";

describe("ScheduledPriceChangeBanner", () => {
  it("renders nothing when scheduledPriceChange is null", () => {
    const { container } = renderWithI18n(
      <ScheduledPriceChangeBanner
        scheduledPriceChange={null}
        planName="Pro"
        currency="vnd"
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders plan name, effective date, and VND seat prices with no decimals and no cancel button", () => {
    renderWithI18n(
      <ScheduledPriceChangeBanner
        scheduledPriceChange={scheduled}
        planName="Pro"
        currency="vnd"
      />,
    );

    expect(screen.getByText("Scheduled price change")).toBeInTheDocument();
    expect(
      screen.getByText(`Pro pricing updates on ${DATE_TEXT_EN}.`),
    ).toBeInTheDocument();
    expect(screen.getByText(/Monthly:.*249,000/)).toBeInTheDocument();
    expect(screen.getByText(/Yearly:.*2,490,000/)).toBeInTheDocument();
    expect(screen.queryByText(/249,000\.00/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("omits null monthly or yearly price line", () => {
    renderWithI18n(
      <ScheduledPriceChangeBanner
        scheduledPriceChange={{
          effectiveFrom: EFFECTIVE_DATE,
          seatPriceMonth: 249000,
          seatPriceYear: null,
        }}
        planName="Pro"
        currency="vnd"
      />,
    );

    expect(screen.getByText(/Monthly:.*249,000/)).toBeInTheDocument();
    expect(screen.queryByText(/Yearly:/i)).not.toBeInTheDocument();
  });

  it("renders translated strings in non-English locales", () => {
    renderWithI18n(
      <ScheduledPriceChangeBanner
        scheduledPriceChange={scheduled}
        planName="Pro"
        currency="vnd"
      />,
      { locale: "zh-Hans" },
    );

    expect(screen.getByText("价格调整通知")).toBeInTheDocument();
    expect(
      screen.getByText(`Pro 将于 ${DATE_TEXT_ZH} 起调整价格。`),
    ).toBeInTheDocument();
    expect(screen.getByText(/月付：.*249,000/)).toBeInTheDocument();
    expect(screen.getByText(/年付：.*2,490,000/)).toBeInTheDocument();
  });
});

describe("BillingTab scheduled price change banner", () => {
  beforeEach(() => {
    configStore.getState().setFeatureFlags({
      [BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG]: true,
    });
    mocks.summary.availableActions = {
      checkout: false,
      portal: false,
      purchaseSeats: false,
      changePlan: true,
    };
    mocks.plans = [
      {
        key: "pro",
        name: "Pro",
        currency: "vnd",
        seatPriceMonth: 149000,
        seatPriceYear: 1490000,
        seatsFixed: null,
        minSeats: 1,
        trialDays: 14,
        audience: "any",
        billingModel: "seat",
        usageRates: {},
        effectiveDiscountPct: 0,
        scheduledPriceChange: scheduled,
      },
    ];
    mocks.useQuery.mockImplementation((options: unknown) => {
      const key = (options as { queryKey?: readonly unknown[] }).queryKey;
      const tail = key?.[key.length - 1];
      if (key?.[0] === "workspace-plans") {
        return { data: mocks.plans, isLoading: false, isError: false, refetch: vi.fn() };
      }
      if (tail === "summary") {
        return {
          data: mocks.summary,
          isPending: false,
          isFetching: false,
          isError: false,
          dataUpdatedAt: 1,
          isFetchedAfterMount: true,
          refetch: vi.fn(),
        };
      }
      if (tail === "usage" && key?.[0] === "autopilots") {
        return {
          data: { action: "off", used: null, reserved: null, total: null, limit: null, reached: null, period_start: null, period_end: null, reset_at: null, blocked_counts: null },
          isPending: false,
          isFetching: false,
          isError: false,
          refetch: vi.fn(),
        };
      }
      return { data: undefined, isPending: false, isFetching: false, isError: false, refetch: vi.fn() };
    });
  });

  it("renders the scheduled price change banner when owner/billing actions are enabled and current plan has scheduledPriceChange", () => {
    renderWithI18n(<BillingTab />);

    expect(screen.getByText("Scheduled price change")).toBeInTheDocument();
    expect(
      screen.getByText(`Pro pricing updates on ${DATE_TEXT_EN}.`),
    ).toBeInTheDocument();
  });

  it("hides the banner when scheduledPriceChange is null or when neither checkout nor changePlan is available", () => {
    mocks.plans[0].scheduledPriceChange = null;
    const { unmount } = renderWithI18n(<BillingTab />);
    expect(screen.queryByText("Scheduled price change")).not.toBeInTheDocument();
    unmount();

    mocks.plans[0].scheduledPriceChange = scheduled;
    mocks.summary.availableActions.checkout = false;
    mocks.summary.availableActions.changePlan = false;
    renderWithI18n(<BillingTab />);
    expect(screen.queryByText("Scheduled price change")).not.toBeInTheDocument();
  });

  it("hides the banner without throwing when plans are empty or omit scheduledPriceChange", () => {
    mocks.plans = [];
    const { unmount } = renderWithI18n(<BillingTab />);
    expect(screen.queryByText("Scheduled price change")).not.toBeInTheDocument();
    unmount();

    mocks.plans = [
      {
        key: "pro",
        name: "Pro",
        currency: "vnd",
        seatPriceMonth: 149000,
        seatPriceYear: 1490000,
      },
    ];
    renderWithI18n(<BillingTab />);
    expect(screen.queryByText("Scheduled price change")).not.toBeInTheDocument();
  });
});
