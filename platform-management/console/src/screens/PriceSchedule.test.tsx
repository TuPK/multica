// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { PriceSchedule } from "./PriceSchedule";
import { PlatformApiError } from "../api/client";
import type { Plan, PriceChange } from "../api/schemas";

const mockUsePriceChanges = vi.fn();
const mockScheduleMutateAsync = vi.fn();
const mockCancelMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  usePriceChanges: (key: string | undefined, enabled = true) => mockUsePriceChanges(key, enabled),
  useSchedulePriceChange: () => ({
    mutateAsync: mockScheduleMutateAsync,
    isPending: false,
  }),
  useCancelScheduledPriceChange: () => ({
    mutateAsync: mockCancelMutateAsync,
    isPending: false,
  }),
}));

const proPlan: Plan = {
  key: "pro",
  name: "Pro Plan",
  limits: { issue_count: null },
  features: { audit_export: true },
  currency: "vnd",
  seat_price_month: 200000,
  seat_price_year: 2000000,
  seats_fixed: null,
  min_seats: 1,
  trial_days: 14,
  is_public: true,
  sort: 20,
  archived_at: null,
  audience: "any",
  billing_model: "seat",
  version: 1,
};

const sampleHistory: PriceChange[] = [
  {
    id: "pc-1",
    plan_key: "pro",
    state: "scheduled",
    effective_from: "2026-12-20T00:00:00Z",
    old: { seat_price_month: 200000, seat_price_year: 2000000 },
    new: { seat_price_month: 240000, seat_price_year: 2400000 },
    reason: "Q4 adjustment",
    notice_sent_at: null,
    created_by: "u-admin",
    created_at: "2026-11-17T00:00:00Z",
    applied_at: null,
    canceled_by: "",
    canceled_at: null,
  },
  {
    id: "pc-0",
    plan_key: "pro",
    state: "applied",
    effective_from: "2026-06-01T00:00:00Z",
    old: { seat_price_month: 149000, seat_price_year: 1490000 },
    new: { seat_price_month: 200000, seat_price_year: 2000000 },
    reason: "Initial launch pricing",
    notice_sent_at: "2026-05-01T00:00:00Z",
    created_by: "u-admin",
    created_at: "2026-05-01T00:00:00Z",
    applied_at: "2026-06-01T00:00:00Z",
    canceled_by: "",
    canceled_at: null,
  },
];

describe("PriceSchedule screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePriceChanges.mockReturnValue({
      data: sampleHistory,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("dry-run preview calls POST with dryRun=true and displays impact without saving", async () => {
    mockScheduleMutateAsync.mockResolvedValue({
      dry_run: true,
      affected_subscribers: 14,
      notice_date: "2026-11-17",
    });

    render(<PriceSchedule plan={proPlan} />);

    fireEvent.change(screen.getByLabelText(/Effective from/i), {
      target: { value: "2026-12-20T00:00" },
    });
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "220000" },
    });
    fireEvent.change(screen.getByLabelText(/Yearly seat price/i), {
      target: { value: "2200000" },
    });
    fireEvent.change(screen.getByLabelText(/Reason/i), {
      target: { value: "Inflation index" },
    });

    fireEvent.click(screen.getByRole("button", { name: /Preview impact/i }));

    await waitFor(() => {
      expect(mockScheduleMutateAsync).toHaveBeenCalledTimes(1);
    });
    expect(mockScheduleMutateAsync).toHaveBeenCalledWith({
      key: "pro",
      effective_from: "2026-12-20T00:00:00Z",
      new: { seat_price_month: 220000, seat_price_year: 2200000 },
      reason: "Inflation index",
      confirm_large_change: false,
      dryRun: true,
    });

    const previewBox = screen.getByTestId("price-dry-run-preview");
    expect(within(previewBox).getByText("14")).toBeInTheDocument();
    expect(within(previewBox).getByText("2026-11-17")).toBeInTheDocument();
    // Only the dry-run call happened; no real Schedule call until Schedule is clicked.
    expect(mockScheduleMutateAsync).toHaveBeenCalledTimes(1);

    mockScheduleMutateAsync.mockResolvedValue({
      price_change: sampleHistory[0],
      applied: false,
      affected_subscribers: 14,
      notice_date: "2026-11-17",
    });
    fireEvent.click(screen.getByRole("button", { name: /^Schedule$/i }));

    await waitFor(() => {
      expect(mockScheduleMutateAsync).toHaveBeenCalledTimes(2);
    });
    expect(mockScheduleMutateAsync).toHaveBeenLastCalledWith({
      key: "pro",
      effective_from: "2026-12-20T00:00:00Z",
      new: { seat_price_month: 220000, seat_price_year: 2200000 },
      reason: "Inflation index",
      confirm_large_change: false,
      dryRun: false,
    });
  });

  it("shows large-change warning above 50% and requires confirm checkbox to submit", async () => {
    mockScheduleMutateAsync.mockResolvedValue({
      price_change: sampleHistory[0],
      applied: false,
      affected_subscribers: 2,
      notice_date: "2026-11-17",
    });

    render(<PriceSchedule plan={proPlan} />);

    fireEvent.change(screen.getByLabelText(/Effective from/i), {
      target: { value: "2026-12-20T00:00" },
    });
    // 200,000 -> 400,000 is +100% (> 50%)
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "400000" },
    });
    fireEvent.change(screen.getByLabelText(/Reason/i), {
      target: { value: "Tier reposition" },
    });

    expect(screen.getByText(/100%/)).toBeInTheDocument();
    const confirmCheckbox = screen.getByRole("checkbox", { name: /Confirm large price change/i });
    expect(confirmCheckbox).not.toBeChecked();

    const scheduleBtn = screen.getByRole("button", { name: /^Schedule$/i });
    expect(scheduleBtn).toBeDisabled();

    fireEvent.click(confirmCheckbox);
    expect(confirmCheckbox).toBeChecked();
    expect(scheduleBtn).toBeEnabled();

    fireEvent.click(scheduleBtn);
    await waitFor(() => {
      expect(mockScheduleMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ confirm_large_change: true, dryRun: false }),
      );
    });
  });

  it("shows large-change warning for a >50% price decrease and requires confirmation", () => {
    render(<PriceSchedule plan={proPlan} />);

    // 200,000 -> 80,000 is -60% (> 50% threshold)
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "80000" },
    });

    expect(screen.getByText(/60%/)).toBeInTheDocument();
    const scheduleBtn = screen.getByRole("button", { name: /^Schedule$/i });
    expect(scheduleBtn).toBeDisabled();

    fireEvent.click(screen.getByRole("checkbox", { name: /Confirm large price change/i }));
    expect(scheduleBtn).toBeEnabled();
  });

  it("handles 409 price_change_confirm_required from server by prompting confirmation", async () => {
    mockScheduleMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(409, "price_change_confirm_required", "the change is large", {
        change_pct: 40,
      }),
    );

    render(<PriceSchedule plan={proPlan} />);

    fireEvent.change(screen.getByLabelText(/Effective from/i), {
      target: { value: "2026-12-20T00:00" },
    });
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "280000" },
    });
    fireEvent.change(screen.getByLabelText(/Reason/i), {
      target: { value: "Adjust" },
    });

    fireEvent.click(screen.getByRole("button", { name: /^Schedule$/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/40%/);
    });
    expect(screen.getByRole("checkbox", { name: /Confirm large price change/i })).toBeInTheDocument();
  });

  it("handles 409 price_notice_too_short by setting input min and naming min_effective_from", async () => {
    mockScheduleMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(
        409,
        "price_notice_too_short",
        "a price increase needs notice before it takes effect",
        { min_effective_from: "2026-12-20T10:30:00Z" },
      ),
    );

    render(<PriceSchedule plan={proPlan} />);

    const effectiveInput = screen.getByLabelText(/Effective from/i);
    fireEvent.change(effectiveInput, { target: { value: "2026-11-20T00:00" } });
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "220000" },
    });
    fireEvent.change(screen.getByLabelText(/Reason/i), {
      target: { value: "Raise" },
    });

    fireEvent.click(screen.getByRole("button", { name: /^Schedule$/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("2026-12-20T10:30:00Z");
    });
    expect(effectiveInput).toHaveAttribute("min", "2026-12-20T10:30:00");
  });

  it("handles 409 price_change_pending by showing an error without clearing or closing the form", async () => {
    mockScheduleMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(409, "price_change_pending", "the plan already has a scheduled price change"),
    );

    render(<PriceSchedule plan={proPlan} />);

    const effectiveInput = screen.getByLabelText(/Effective from/i);
    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(effectiveInput, { target: { value: "2026-12-20T00:00" } });
    fireEvent.change(screen.getByLabelText(/Monthly seat price/i), {
      target: { value: "220000" },
    });
    fireEvent.change(reasonInput, { target: { value: "Raise again" } });

    fireEvent.click(screen.getByRole("button", { name: /^Schedule$/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("the plan already has a scheduled price change");
    });
    expect(screen.getByRole("button", { name: /^Schedule$/i })).toBeInTheDocument();
    expect(reasonInput).toHaveValue("Raise again");
  });

  it("renders history list with state, effective_from, old and new prices, and reason, and supports canceling scheduled change", async () => {
    mockCancelMutateAsync.mockResolvedValue({
      price_change: { ...sampleHistory[0], state: "canceled" },
    });

    render(<PriceSchedule plan={proPlan} />);

    expect(screen.getByText("Scheduled")).toBeInTheDocument();
    expect(screen.getByText("Applied")).toBeInTheDocument();
    expect(screen.getByText("2026-12-20T00:00:00Z")).toBeInTheDocument();
    expect(screen.getByText("2026-06-01T00:00:00Z")).toBeInTheDocument();
    expect(screen.getByText(/240,000/)).toBeInTheDocument();
    expect(screen.getByText(/149,000/)).toBeInTheDocument();
    expect(screen.getByText("Q4 adjustment")).toBeInTheDocument();
    expect(screen.getByText("Initial launch pricing")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Cancel scheduled change/i }));
    await waitFor(() => {
      expect(mockCancelMutateAsync).toHaveBeenCalledWith({ key: "pro" });
    });
  });

  it("hides schedule form and cancel action when readOnly is true", () => {
    render(<PriceSchedule planKey="pro" readOnly />);

    expect(screen.queryByRole("button", { name: /Preview impact/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Schedule$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Cancel scheduled change/i })).not.toBeInTheDocument();
    expect(screen.getByText("Q4 adjustment")).toBeInTheDocument();
  });
});
