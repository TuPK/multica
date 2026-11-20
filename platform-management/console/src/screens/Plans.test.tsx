// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { Plans } from "./Plans";
import { PlatformApiError } from "../api/client";
import { findAlert } from "../test/alert";

const mockUsePlans = vi.fn();
const mockUsePriceChanges = vi.fn();
const mockUsePlanHistory = vi.fn();
const mockUsePlan = vi.fn();
const mockCreatePlanMutateAsync = vi.fn();
const mockPutPlanMutateAsync = vi.fn();
const mockDeletePlanMutateAsync = vi.fn();
const mockActivatePlanMutateAsync = vi.fn();
const mockDeactivatePlanMutateAsync = vi.fn();
const mockSchedulePriceMutateAsync = vi.fn();
const mockCancelPriceMutateAsync = vi.fn();
let mockMeRole = "system_admin";

vi.mock("../api/hooks", () => ({
  usePlans: (enabled = true) => mockUsePlans(enabled),
  usePlan: (key: string | undefined, enabled = true) => mockUsePlan(key, enabled),
  usePlanHistory: (key: string | undefined, enabled = true) => mockUsePlanHistory(key, enabled),
  usePriceChanges: (key: string | undefined, enabled = true) => mockUsePriceChanges(key, enabled),
  useCreatePlan: () => ({
    mutateAsync: mockCreatePlanMutateAsync,
    isPending: false,
  }),
  usePutPlan: () => ({
    mutateAsync: mockPutPlanMutateAsync,
    isPending: false,
  }),
  useDeletePlan: () => ({
    mutateAsync: mockDeletePlanMutateAsync,
    isPending: false,
  }),
  useActivatePlan: () => ({
    mutateAsync: mockActivatePlanMutateAsync,
    isPending: false,
  }),
  useDeactivatePlan: () => ({
    mutateAsync: mockDeactivatePlanMutateAsync,
    isPending: false,
  }),
  useSchedulePriceChange: () => ({
    mutateAsync: mockSchedulePriceMutateAsync,
    isPending: false,
  }),
  useCancelScheduledPriceChange: () => ({
    mutateAsync: mockCancelPriceMutateAsync,
    isPending: false,
  }),
  useMe: () => ({
    data: { user_id: "u-staff", email: "staff@example.com", role: mockMeRole },
  }),
}));

const mockPlanList = [
  {
    key: "pro",
    name: "Pro Plan",
    description: "For growing teams",
    limits: { issue_count: 500, autopilot_runs: 50 },
    features: { sso: true },
    currency: "vnd",
    seat_price_month: 200000,
    seat_price_year: 2000000,
    seats_fixed: null,
    min_seats: 1,
    trial_days: 14,
    is_public: true,
    sort: 10,
    archived_at: null,
    audience: "business" as const,
    billing_model: "seat" as const,
    version: 1,
  },
  {
    key: "legacy",
    name: "Legacy Plan",
    description: "",
    limits: {},
    features: {},
    currency: "vnd",
    seat_price_month: 100000,
    seat_price_year: null,
    seats_fixed: 5,
    min_seats: null,
    trial_days: 0,
    is_public: false,
    sort: 99,
    archived_at: "2026-01-01T00:00:00Z",
    audience: "any" as const,
    billing_model: "usage" as const,
    version: 2,
  },
];

describe("Plans screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeRole = "system_admin";
    mockUsePlans.mockReturnValue({
      data: mockPlanList,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUsePlan.mockReturnValue({
      data: mockPlanList[0],
      isLoading: false,
      isError: false,
    });
    mockUsePlanHistory.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUsePriceChanges.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("lists plans with their configuration and edit/schedule/delete actions for admin", () => {
    render(<Plans userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Plans Catalogue" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Audience" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Billing Model" })).toBeInTheDocument();
    expect(screen.getByText("Pro Plan")).toBeInTheDocument();
    expect(screen.getAllByText("Business").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Seat")[0]).toBeInTheDocument();
    expect(screen.getByText("Legacy Plan")).toBeInTheDocument();
    expect(screen.getAllByText("Any")[0]).toBeInTheDocument();
    expect(screen.getAllByText("Usage")[0]).toBeInTheDocument();
    expect(screen.getByText("pro")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit plan Pro Plan" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Price schedule plan Pro Plan" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete plan Pro Plan" })).toBeInTheDocument();
  });

  it("billing role sees read-only price change history without plans table or edit/schedule actions", () => {
    mockMeRole = "billing";
    mockUsePriceChanges.mockReturnValue({
      data: [
        {
          id: "pc-1",
          plan_key: "pro",
          state: "scheduled",
          effective_from: "2026-12-20T00:00:00Z",
          old: { seat_price_month: 149000, seat_price_year: 1490000 },
          new: { seat_price_month: 199000, seat_price_year: 1990000 },
          reason: "Annual price revision",
          notice_sent_at: null,
          created_by: "u-admin",
          created_at: "2026-11-17T00:00:00Z",
          applied_at: null,
          canceled_by: "",
          canceled_at: null,
        },
      ],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Plans userRole="billing" />);

    expect(mockUsePlans).toHaveBeenCalledWith(false);
    expect(mockUsePriceChanges).toHaveBeenCalledWith("pro", true);
    expect(screen.queryByRole("heading", { name: "Plans Catalogue" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Edit plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Schedule price/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Preview impact/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Cancel scheduled change/i })).not.toBeInTheDocument();

    expect(screen.getByRole("heading", { name: "Price Change History" })).toBeInTheDocument();
    expect(screen.getByText(/scheduled/i)).toBeInTheDocument();
    expect(screen.getByText(/2026-12-20/)).toBeInTheDocument();
    expect(screen.getByText(/149,000/)).toBeInTheDocument();
    expect(screen.getByText(/199,000/)).toBeInTheDocument();
    expect(screen.getByText("Annual price revision")).toBeInTheDocument();
  });

  it("support role sees the not-found state", () => {
    mockMeRole = "support";
    render(<Plans userRole="support" />);

    expect(screen.queryByRole("heading", { name: "Plans Catalogue" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Not found" })).toBeInTheDocument();
  });

  it("edits non-price fields without sending seat_price_month or seat_price_year", async () => {
    mockPutPlanMutateAsync.mockResolvedValue({ changed: true, plan: { ...mockPlanList[0], name: "Pro Plus" } });
    render(<Plans userRole="system_admin" />);

    const editBtn = screen.getByRole("button", { name: "Edit plan Pro Plan" });
    fireEvent.click(editBtn);

    expect(screen.getByRole("heading", { name: "Edit Plan: Pro Plan" })).toBeInTheDocument();

    const nameInput = within(screen.getByRole("dialog")).getByLabelText(/Plan Name/i);
    fireEvent.change(nameInput, { target: { value: "Pro Plus" } });

    const trialInput = within(screen.getByRole("dialog")).getByLabelText(/Trial Days/i);
    fireEvent.change(trialInput, { target: { value: "30" } });
    fireEvent.blur(trialInput);

    const saveBtn = screen.getByRole("button", { name: "Save changes" });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockPutPlanMutateAsync).toHaveBeenCalledWith({
        key: "pro",
        name: "Pro Plus",
        description: "For growing teams",
        expected_version: 1,
        trial_days: 30,
        limits: { issue_count: 500, autopilot_runs: 50 },
        features: { sso: true },
        currency: "vnd",
        seats_fixed: null,
        min_seats: 1,
        is_public: true,
        sort: 10,
        archived: false,
      });
    });

    const sent = mockPutPlanMutateAsync.mock.calls[0]?.[0] as Record<string, unknown>;
    expect(sent).not.toHaveProperty("seat_price_month");
    expect(sent).not.toHaveProperty("seat_price_year");
  });

  it("opens PriceSchedule when admin clicks Schedule price for a plan", () => {
    render(<Plans userRole="system_admin" />);
    fireEvent.click(screen.getByRole("button", { name: "Price schedule plan Pro Plan" }));
    expect(screen.getByRole("heading", { name: /Price Schedule: Pro Plan/i })).toBeInTheDocument();
  });

  it("delete of a plan in use shows plan_in_use error", async () => {
    const error409 = new PlatformApiError(409, "plan_in_use", "subscriptions use this plan; archive it instead");
    mockDeletePlanMutateAsync.mockRejectedValue(error409);

    render(<Plans userRole="system_admin" />);

    const deleteBtn = screen.getByRole("button", { name: "Delete plan Pro Plan" });
    fireEvent.click(deleteBtn);

    const confirmDeleteBtn = screen.getByRole("button", { name: "Confirm delete" });
    fireEvent.click(confirmDeleteBtn);

    const dialog = screen.getByRole("dialog");
    await waitFor(() => {
      expect(findAlert(dialog)).toHaveTextContent("subscriptions use this plan; archive it instead");
    });
  });

  it("shows the currency lock message on plan_currency_locked", async () => {
    mockPutPlanMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(
        409,
        "plan_currency_locked",
        "the currency of a plan that has a subscription or an order cannot change",
      ),
    );

    render(<Plans userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: "Edit plan Pro Plan" }));

    const currencyInput = within(screen.getByRole("dialog")).getByLabelText(/Currency/i);
    fireEvent.change(currencyInput, { target: { value: "usd" } });

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mockPutPlanMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ key: "pro", currency: "usd" }),
      );
    });

    await screen.findByText(
      "The currency can't change because this plan has subscriptions or orders.",
    );
  });

  it("keeps other save errors unchanged", async () => {
    mockPutPlanMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(400, "invalid_plan", "currency must be a three-letter lower-case code"),
    );

    render(<Plans userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: "Edit plan Pro Plan" }));

    const currencyInput = within(screen.getByRole("dialog")).getByLabelText(/Currency/i);
    fireEvent.change(currencyInput, { target: { value: "US" } });

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    const dialog = screen.getByRole("dialog");
    await waitFor(() => {
      expect(findAlert(dialog)).toHaveTextContent(
        "currency must be a three-letter lower-case code",
      );
    });
    expect(currencyInput.closest("div")).not.toHaveTextContent(
      "The currency can't change because this plan has subscriptions or orders.",
    );
  });

  it("admin sees plans read only", () => {
    mockMeRole = "admin";
    render(<Plans userRole="admin" />);

    expect(mockUsePlans).toHaveBeenCalledWith(true);
    expect(screen.getByRole("heading", { name: "Plans Catalogue" })).toBeInTheDocument();
    expect(screen.getByText("Pro Plan")).toBeInTheDocument();
    expect(screen.getByText("For growing teams")).toBeInTheDocument();

    expect(screen.queryByRole("button", { name: /Create plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Edit plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Delete plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Schedule price/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Deactivate plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Activate plan/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /View history/i })).not.toBeInTheDocument();
  });

  it("deactivate and activate call the routes and show the inactive badge", async () => {
    mockDeactivatePlanMutateAsync.mockResolvedValueOnce({
      changed: true,
      plan: { ...mockPlanList[0], archived_at: "2026-11-20T00:00:00Z" },
    });
    mockActivatePlanMutateAsync.mockResolvedValueOnce({
      changed: true,
      plan: { ...mockPlanList[1], archived_at: null },
    });

    render(<Plans userRole="system_admin" />);

    // Inactive plan shows the Inactive badge
    const legacyRow = screen.getByText("Legacy Plan").closest("tr")!;
    expect(within(legacyRow).getByText("Inactive")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Deactivate plan Pro Plan" }));
    await waitFor(() => {
      expect(mockDeactivatePlanMutateAsync).toHaveBeenCalledWith({ key: "pro" });
    });

    fireEvent.click(screen.getByRole("button", { name: "Activate plan Legacy Plan" }));
    await waitFor(() => {
      expect(mockActivatePlanMutateAsync).toHaveBeenCalledWith({ key: "legacy" });
    });
  });

  it("delete shows plan_in_use message", async () => {
    mockDeletePlanMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(409, "plan_in_use", "subscriptions use this plan; archive it instead"),
    );

    render(<Plans userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: "Delete plan Pro Plan" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm delete" }));

    const dialog = screen.getByRole("dialog");
    await waitFor(() => {
      expect(findAlert(dialog)).toHaveTextContent("subscriptions use this plan; archive it instead");
    });
  });

  it("lowering a limit asks for confirmation", async () => {
    mockPutPlanMutateAsync.mockResolvedValueOnce({
      changed: true,
      plan: { ...mockPlanList[0], limits: { issue_count: 100, autopilot_runs: 50 } },
    });

    render(<Plans userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: "Edit plan Pro Plan" }));

    const issueLimitInput = screen.getByLabelText(/Issue limit/i);
    fireEvent.change(issueLimitInput, { target: { value: "100" } });
    fireEvent.blur(issueLimitInput);

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    // Does not submit before explicit confirmation
    expect(mockPutPlanMutateAsync).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/Lowered limits/i)).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole("button", { name: /Confirm limit reduction/i }));

    await waitFor(() => {
      expect(mockPutPlanMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          key: "pro",
          limits: expect.objectContaining({ issue_count: 100 }),
        }),
      );
    });
  });

  it("opens PlanHistory when admin clicks View history for a plan", () => {
    render(<Plans userRole="system_admin" />);
    fireEvent.click(screen.getByRole("button", { name: "History plan Pro Plan" }));
    expect(screen.getByRole("heading", { name: /Plan History: Pro Plan/i })).toBeInTheDocument();
  });
});
