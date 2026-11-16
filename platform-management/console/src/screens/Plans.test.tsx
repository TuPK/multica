// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Plans } from "./Plans";
import { PlatformApiError } from "../api/client";

const mockUsePlans = vi.fn();
const mockPutPlanMutateAsync = vi.fn();
const mockDeletePlanMutateAsync = vi.fn();
let mockMeRole = "admin";

vi.mock("../api/hooks", () => ({
  usePlans: () => mockUsePlans(),
  usePutPlan: () => ({
    mutateAsync: mockPutPlanMutateAsync,
    isPending: false,
  }),
  useDeletePlan: () => ({
    mutateAsync: mockDeletePlanMutateAsync,
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
    version: 1,
  },
  {
    key: "legacy",
    name: "Legacy Plan",
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
    version: 2,
  },
];

describe("Plans screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeRole = "admin";
    mockUsePlans.mockReturnValue({
      data: mockPlanList,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("lists plans with their configuration", () => {
    render(<Plans userRole="admin" />);

    expect(screen.getByRole("heading", { name: "Plans Catalogue" })).toBeInTheDocument();
    expect(screen.getByText("Pro Plan")).toBeInTheDocument();
    expect(screen.getByText("Legacy Plan")).toBeInTheDocument();
    expect(screen.getByText("pro")).toBeInTheDocument();
  });

  it("support role sees the not-found state", () => {
    mockMeRole = "support";
    render(<Plans userRole="support" />);

    expect(screen.queryByRole("heading", { name: "Plans Catalogue" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Not found" })).toBeInTheDocument();
  });

  it("edits the non-price fields without a version check", async () => {
    mockPutPlanMutateAsync.mockResolvedValue({ changed: true, plan: { ...mockPlanList[0], name: "Pro Plus" } });
    render(<Plans userRole="admin" />);

    const editBtn = screen.getByRole("button", { name: "Edit plan Pro Plan" });
    fireEvent.click(editBtn);

    expect(screen.getByRole("heading", { name: "Edit Plan: Pro Plan" })).toBeInTheDocument();

    const nameInput = screen.getByLabelText(/Plan Name/i);
    fireEvent.change(nameInput, { target: { value: "Pro Plus" } });

    const trialInput = screen.getByLabelText(/Trial Days/i);
    fireEvent.change(trialInput, { target: { value: "30" } });

    const saveBtn = screen.getByRole("button", { name: "Save changes" });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockPutPlanMutateAsync).toHaveBeenCalledWith({
        key: "pro",
        name: "Pro Plus",
        trial_days: 30,
        limits: { issue_count: 500, autopilot_runs: 50 },
        features: { sso: true },
        currency: "vnd",
        seat_price_month: 200000,
        seat_price_year: 2000000,
        seats_fixed: null,
        min_seats: 1,
        is_public: true,
        sort: 10,
        archived: false,
      });
    });
  });

  it("delete of a plan in use shows plan_in_use error", async () => {
    const error409 = new PlatformApiError(409, "plan_in_use", "subscriptions use this plan; archive it instead");
    mockDeletePlanMutateAsync.mockRejectedValue(error409);

    render(<Plans userRole="admin" />);

    const deleteBtn = screen.getByRole("button", { name: "Delete plan Pro Plan" });
    fireEvent.click(deleteBtn);

    const confirmDeleteBtn = screen.getByRole("button", { name: "Confirm delete" });
    fireEvent.click(confirmDeleteBtn);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("subscriptions use this plan; archive it instead");
    });
  });
});
