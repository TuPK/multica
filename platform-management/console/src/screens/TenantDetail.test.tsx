// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TenantDetail } from "./TenantDetail";
import { PlatformApiError } from "../api/client";

const mockUseMe = vi.fn();
const mockUseTenant = vi.fn();
const mockUseTenantUsage = vi.fn();
const mockUseTenantBillingProfile = vi.fn();
const mockUseTenantNotes = vi.fn();
const mockUseTenantAudit = vi.fn();
const mockChangeTenantStatusMutateAsync = vi.fn();
const mockAddTenantNoteMutateAsync = vi.fn();
const mockUpdateSubscriptionMutateAsync = vi.fn();
const mockCancelOrderMutateAsync = vi.fn();
const mockSetBillingProfileVerifiedMutateAsync = vi.fn();
const mockUpdateTenantBillingProfileMutateAsync = vi.fn();
let mockUpdateBillingProfileIsPending = false;

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useTenant: () => mockUseTenant(),
  useTenantUsage: () => mockUseTenantUsage(),
  useTenantBillingProfile: (id?: string) => mockUseTenantBillingProfile(id),
  useTenantNotes: () => mockUseTenantNotes(),
  useTenantAudit: () => mockUseTenantAudit(),
  useChangeTenantStatus: () => ({
    mutateAsync: mockChangeTenantStatusMutateAsync,
    isPending: false,
  }),
  useAddTenantNote: () => ({
    mutateAsync: mockAddTenantNoteMutateAsync,
    isPending: false,
  }),
  useUpdateSubscription: () => ({
    mutateAsync: mockUpdateSubscriptionMutateAsync,
    isPending: false,
  }),
  useCancelOrder: () => ({
    mutateAsync: mockCancelOrderMutateAsync,
    isPending: false,
  }),
  useSetBillingProfileVerified: () => ({
    mutateAsync: mockSetBillingProfileVerifiedMutateAsync,
    isPending: false,
  }),
  useUpdateTenantBillingProfile: () => ({
    mutateAsync: mockUpdateTenantBillingProfileMutateAsync,
    isPending: mockUpdateBillingProfileIsPending,
  }),
}));

const mockTenantData = {
  id: "t-123",
  name: "Acme Corp",
  slug: "acme",
  status: "active",
  member_count: 8,
  group_count: 3,
  created_at: "2026-11-01T10:00:00Z",
  last_activity_at: "2026-11-15T12:00:00Z",
  state: {
    status: "active",
    reason: "Initial active state",
    changed_by: "system",
    changed_at: "2026-11-01T10:00:00Z",
    deletion_scheduled_at: null,
  },
  subscription: {
    workspace_id: "t-123",
    plan_key: "pro",
    status: "active",
    seats: 5,
    interval: "month",
    current_period_start: "2026-11-01T00:00:00Z",
    current_period_end: "2026-12-01T00:00:00Z",
    trial_end: "2026-11-15T00:00:00Z",
    grace_until: null,
    cancel_at_period_end: false,
    limit_overrides: {},
    version: 1,
    updated_at: null,
  },
  recent_orders: [
    {
      id: "ord-101",
      workspace_id: "t-123",
      workspace_name: "Acme Corp",
      kind: "subscribe" as const,
      plan_key: "pro",
      interval: "month",
      seats: 5,
      amount: 1000000,
      vat_amount: 100000,
      currency: "vnd",
      status: "paid" as const,
      transfer_code: "MUL99999999",
      created_at: "2026-11-01T10:00:00Z",
      expires_at: "2026-11-08T10:00:00Z",
      paid_at: "2026-11-01T11:00:00Z",
      buyer: {
        name: "Acme Corp",
        tax_code: "0101234567",
        address: "123 Street",
        invoice_email: "acme@example.com",
      },
    },
  ],
  notes_count: 1,
  owner: {
    id: "u-owner",
    email: "owner@acme.com",
  },
};

describe("TenantDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUpdateBillingProfileIsPending = false;
    mockUseMe.mockReturnValue({ data: undefined });
    mockUseTenant.mockReturnValue({
      data: mockTenantData,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseTenantUsage.mockReturnValue({
      data: {
        workspace_id: "t-123",
        period_start: "2026-11-01T00:00:00Z",
        period_end: "2026-12-01T00:00:00Z",
        rows: [],
        missing_days: [],
        unpaid_orders: [],
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseTenantBillingProfile.mockReturnValue({
      data: {
        workspace_id: "t-123",
        account_kind: "individual",
        legal_name: "",
        tax_code: "",
        address: "",
        invoice_email: "",
        verified: false,
        verified_at: null,
      },
      isLoading: false,
      isError: false,
    });
    mockUseTenantNotes.mockReturnValue({
      data: [
        {
          id: "n-1",
          workspace_id: "t-123",
          author_id: "u-staff-1",
          author_email: "support@example.com",
          body: "Verified payment issue.",
          created_at: "2026-11-10T10:00:00Z",
        },
      ],
      isLoading: false,
    });
    mockUseTenantAudit.mockReturnValue({
      data: [
        {
          id: "ae-1",
          workspace_id: "t-123",
          actor_type: "staff",
          actor_id: "u-staff-1",
          actor_label: "support@example.com",
          action: "workspace.status_changed",
          target_type: "workspace",
          target_id: "t-123",
          target_label: "Acme Corp",
          detail: { status: "active" },
          ip: "127.0.0.1",
          user_agent: "agent",
          request_id: "req-1",
          created_at: "2026-11-01T10:00:00Z",
        },
      ],
      isLoading: false,
    });
  });

  it("renders tenant detail, notes, and audit events", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    expect(screen.getByRole("heading", { name: "Acme Corp" })).toBeInTheDocument();
    expect(screen.getByText("acme • ID: t-123")).toBeInTheDocument();
    expect(within(screen.getByText("Account groups").parentElement!).getByText("3")).toBeInTheDocument();
    expect(screen.getByText("owner@acme.com")).toBeInTheDocument();
    expect(screen.getByText("Verified payment issue.")).toBeInTheDocument();
    expect(screen.getByText("workspace.status_changed")).toBeInTheDocument();
  });

  it("role gating: support role does not see Pending deletion option in change status dialog", async () => {
    const user = userEvent.setup();
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    await user.click(screen.getByRole("button", { name: "Change status" }));
    expect(screen.getByRole("heading", { name: "Change workspace status" })).toBeInTheDocument();

    const trigger = screen.getByRole("combobox", { name: /Target status/i });
    await user.click(trigger);

    const options = await screen.findAllByRole("option");
    const optionLabels = options.map((opt) => opt.textContent);

    expect(optionLabels).toContain("Active");
    expect(optionLabels).toContain("Read-only");
    expect(optionLabels).toContain("Suspended");
    expect(optionLabels).not.toContain("Pending deletion");
  });

  it("role gating: admin role sees Pending deletion option in change status dialog", async () => {
    const user = userEvent.setup();
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />
    );

    await user.click(screen.getByRole("button", { name: "Change status" }));
    expect(screen.getByRole("heading", { name: "Change workspace status" })).toBeInTheDocument();

    const trigger = screen.getByRole("combobox", { name: /Target status/i });
    await user.click(trigger);

    const options = await screen.findAllByRole("option");
    const optionLabels = options.map((opt) => opt.textContent);

    expect(optionLabels).toContain("Pending deletion");
  });

  it("the reason is required before a status change can be confirmed", async () => {
    mockChangeTenantStatusMutateAsync.mockResolvedValue({ changed: true });
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />
    );

    fireEvent.click(screen.getByRole("button", { name: "Change status" }));

    const confirmBtn = screen.getByRole("button", { name: "Update status" });
    expect(confirmBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "   " } });
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(reasonInput, { target: { value: "Suspending due to TOS violation" } });
    expect(confirmBtn).toBeEnabled();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockChangeTenantStatusMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-123",
        status: "active",
        reason: "Suspending due to TOS violation",
      });
    });
  });

  it("adds an internal note", async () => {
    mockAddTenantNoteMutateAsync.mockResolvedValue({});
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    const noteInput = screen.getByPlaceholderText(/Write an internal note.../i);
    const addBtn = screen.getByRole("button", { name: /Add note/i });
    expect(addBtn).toBeDisabled();

    fireEvent.change(noteInput, { target: { value: "Investigated billing query." } });
    expect(addBtn).toBeEnabled();

    fireEvent.click(addBtn);

    await waitFor(() => {
      expect(mockAddTenantNoteMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-123",
        body: "Investigated billing query.",
      });
    });
  });

  it("subscription panel: displays plan, seats, interval, trial end, and recent orders", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />
    );

    expect(screen.getByRole("heading", { name: "Subscription" })).toBeInTheDocument();
    expect(screen.getByText("pro")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("month")).toBeInTheDocument();

    // Recent orders
    expect(screen.getByRole("heading", { name: "Recent Orders" })).toBeInTheDocument();
    expect(screen.getByText("MUL99999999")).toBeInTheDocument();
    expect(screen.getByText(/1,100,000/)).toBeInTheDocument();
  });

  it("subscription panel: edit subscription requires reason and submits PUT /tenants/{id}/subscription", async () => {
    mockUpdateSubscriptionMutateAsync.mockResolvedValue({
      changed: true,
      subscription: { ...mockTenantData.subscription, seats: 10 },
    });

    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit subscription" }));
    expect(screen.getByRole("heading", { name: "Edit Subscription" })).toBeInTheDocument();

    const seatsInput = screen.getByLabelText(/Seats/i);
    fireEvent.change(seatsInput, { target: { value: "10" } });

    const submitBtn = screen.getByRole("button", { name: "Save subscription" });
    expect(submitBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "Customer upgraded over phone" } });
    expect(submitBtn).toBeEnabled();

    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockUpdateSubscriptionMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-123",
        planKey: "pro",
        seats: 10,
        interval: "month",
        trialEnd: "2026-11-15T00:00:00Z",
        reason: "Customer upgraded over phone",
      });
    });
  });

  it("subscription panel: support role cannot edit subscription", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    expect(screen.getByRole("heading", { name: "Subscription" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit subscription" })).not.toBeInTheDocument();
  });

  it("shows the current counts and the latest daily snapshot", () => {
    mockUseTenant.mockReturnValue({
      data: {
        ...mockTenantData,
        usage_day: "2026-11-14",
        usage_members: 3,
        usage_agents: 2,
        usage_runtimes: 1,
        usage_issues_total: 40,
        usage_issues_created: 5,
        usage_tasks_run: 9,
        usage_storage_bytes: 2048,
        usage_current: { members: 4, agents: 2, runtimes: 1, issues_total: 41, storage_bytes: 4096 },
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />);

    expect(screen.getByRole("heading", { name: "Usage" })).toBeInTheDocument();
    const current = screen.getByTestId("usage-current");
    expect(current).toHaveTextContent("41");
    expect(current).toHaveTextContent("4.0 KB");
    const daily = screen.getByTestId("usage-daily");
    expect(daily).toHaveTextContent("2026-11-14");
    expect(daily).toHaveTextContent("9");
    expect(daily).toHaveTextContent("2.0 KB");
  });

  it("shows no daily snapshot, and no error, when the tenant has no usage row", () => {
    render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />);

    expect(screen.getByRole("heading", { name: "Usage" })).toBeInTheDocument();
    expect(screen.getByTestId("usage-daily").textContent).toBe("No daily snapshot yet.");
    expect(screen.getByRole("heading", { name: "Acme Corp" })).toBeInTheDocument();
  });

  describe("monthly usage and usage debt write-off", () => {
    const monthlyUsageData = {
      workspace_id: "t-123",
      period_start: "2026-11-01T00:00:00Z",
      period_end: "2026-12-01T00:00:00Z",
      rows: [
        {
          day: "2026-11-01",
          meter: "tasks_run",
          quantity: 120,
          unit_price: 30000,
          per: 1000,
          discount_pct: 20,
          net_amount: 2880,
          note: "",
          order_id: null,
        },
        {
          day: "2026-11-02",
          meter: "members",
          quantity: 4,
          unit_price: 49000,
          per: 1,
          discount_pct: 0,
          net_amount: 6533,
          note: "",
          order_id: null,
        },
      ],
      missing_days: ["2026-11-03", "2026-11-04"],
      unpaid_orders: [
        {
          id: "ord-usage-1",
          amount: 50000,
          vat_amount: 5000,
          status: "pending" as const,
          created_at: "2026-11-01T00:05:00Z",
          period_start: "2026-10-01T00:00:00Z",
          period_end: "2026-11-01T00:00:00Z",
        },
        {
          id: "ord-usage-2",
          amount: 75000,
          vat_amount: 7500,
          status: "pending" as const,
          created_at: "2026-11-10T00:05:00Z",
          period_start: null,
          period_end: null,
        },
      ],
    };

    it("renders monthly usage rows, lists missing_days as missing (never 0), and leaves usage-current intact", () => {
      mockUseMe.mockReturnValue({
        data: { user_id: "u-support", email: "support@example.com", role: "support" },
      });
      mockUseTenantUsage.mockReturnValue({
        data: monthlyUsageData,
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />);

      expect(screen.getByTestId("usage-current")).toBeInTheDocument();

      const rowsRegion = screen.getByTestId("usage-rows");
      expect(within(rowsRegion).getByText("2026-11-01")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("tasks_run")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("120")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("2,880")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("2026-11-02")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("members")).toBeInTheDocument();
      expect(within(rowsRegion).getByText("6,533")).toBeInTheDocument();

      const missingRegion = screen.getByTestId("usage-missing-days");
      for (const missingDay of ["2026-11-03", "2026-11-04"]) {
        const dayEl = within(missingRegion).getByText(missingDay);
        const container = dayEl.closest("li") ?? dayEl.parentElement!;
        expect(container).toHaveTextContent(/missing/i);
        expect(container.textContent?.replace(missingDay, "")).not.toMatch(/\b0\b/);
      }
    });

    it("shows unpaid orders without Write off button for billing and support roles", () => {
      mockUseTenantUsage.mockReturnValue({
        data: monthlyUsageData,
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      });

      for (const role of ["billing", "support"] as const) {
        mockUseMe.mockReturnValue({
          data: { user_id: `u-${role}`, email: `${role}@example.com`, role },
        });
        // Even if userRole prop were passed as admin, useMe governs Write off visibility.
        const { unmount } = render(
          <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />,
        );

        const unpaidRegion = screen.getByTestId("usage-unpaid-orders");
        expect(within(unpaidRegion).getByText("ord-usage-1")).toBeInTheDocument();
        expect(within(unpaidRegion).getByText("50,000")).toBeInTheDocument();
        expect(within(unpaidRegion).getAllByText(/pending/i).length).toBeGreaterThanOrEqual(2);
        expect(within(unpaidRegion).queryByRole("button", { name: /write off/i })).not.toBeInTheDocument();
        unmount();
      }
    });

    it("shows Write off button only for admin, requires a reason, and submits cancel with reason", async () => {
      mockCancelOrderMutateAsync.mockResolvedValue({
        order: { ...mockTenantData.recent_orders[0], id: "ord-usage-2", status: "canceled" },
        applied: true,
      });
      mockUseMe.mockReturnValue({
        data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      });
      mockUseTenantUsage.mockReturnValue({
        data: monthlyUsageData,
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);

      const unpaidRegion = screen.getByTestId("usage-unpaid-orders");
      const writeOffButtons = within(unpaidRegion).getAllByRole("button", { name: /write off/i });
      expect(writeOffButtons).toHaveLength(2);

      fireEvent.click(writeOffButtons[1]!);

      expect(screen.getByRole("heading", { name: /write off/i })).toBeInTheDocument();

      const submitBtn = screen.getByRole("button", { name: /write off order/i });
      expect(submitBtn).toBeDisabled();

      const reasonInput = screen.getByLabelText(/reason/i);
      fireEvent.change(reasonInput, { target: { value: "   " } });
      expect(submitBtn).toBeDisabled();
      expect(mockCancelOrderMutateAsync).not.toHaveBeenCalled();

      fireEvent.change(reasonInput, { target: { value: "Customer disputed usage spike" } });
      expect(submitBtn).toBeEnabled();

      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(mockCancelOrderMutateAsync).toHaveBeenCalledWith({
          orderId: "ord-usage-2",
          reason: "Customer disputed usage spike",
        });
      });
    });

    it("keeps dialog open and shows alert when write-off fails", async () => {
      mockCancelOrderMutateAsync.mockRejectedValue(new Error("only an admin can write off a usage invoice"));
      mockUseMe.mockReturnValue({
        data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      });
      mockUseTenantUsage.mockReturnValue({
        data: monthlyUsageData,
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);

      const unpaidRegion = screen.getByTestId("usage-unpaid-orders");
      fireEvent.click(within(unpaidRegion).getAllByRole("button", { name: /write off/i })[0]!);

      fireEvent.change(screen.getByLabelText(/reason/i), { target: { value: "Goodwill credit" } });
      fireEvent.click(screen.getByRole("button", { name: /write off order/i }));

      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent("only an admin can write off a usage invoice");
      });
      expect(screen.getByRole("heading", { name: /write off/i })).toBeInTheDocument();
    });
  });

  describe("billing profile verification", () => {
    function getBillingProfileCard(): HTMLElement {
      const heading = screen.getByRole("heading", { name: "Billing profile" });
      return heading.closest<HTMLElement>("[data-slot='card']") ?? heading.parentElement!.parentElement!;
    }

    it("shows verify to billing and admin", () => {
      for (const role of ["billing", "admin"] as const) {
        const { unmount } = render(
          <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole={role} />,
        );

        const card = getBillingProfileCard();
        expect(within(card).getByRole("button", { name: "Verify" })).toBeInTheDocument();
        unmount();
      }
    });

    it("hides the verify control from support", () => {
      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />);

      const card = getBillingProfileCard();
      expect(within(card).getByText("Unverified")).toBeInTheDocument();
      expect(within(card).queryByRole("button", { name: "Verify" })).not.toBeInTheDocument();
      expect(within(card).queryByRole("button", { name: "Unverify" })).not.toBeInTheDocument();
    });

    it("calls the verify endpoint and shows verified", async () => {
      const verifiedAt = "2026-11-17T10:00:00Z";
      mockSetBillingProfileVerifiedMutateAsync.mockResolvedValueOnce({
        workspace_id: "t-123",
        account_kind: "business",
        verified: true,
        verified_at: verifiedAt,
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />);

      const card = getBillingProfileCard();

      fireEvent.click(within(card).getByRole("button", { name: "Verify" }));

      await waitFor(() => {
        expect(mockSetBillingProfileVerifiedMutateAsync).toHaveBeenCalledWith({
          tenantId: "t-123",
          verified: true,
        });
      });

      await waitFor(() => {
        expect(within(card).getByText("Verified")).toBeInTheDocument();
      });
      expect(within(card).getByText("business")).toBeInTheDocument();
      expect(within(card).getByText(new Date(verifiedAt).toLocaleString())).toBeInTheDocument();
      expect(within(card).getByRole("button", { name: "Unverify" })).toBeInTheDocument();

      mockSetBillingProfileVerifiedMutateAsync.mockResolvedValueOnce({
        workspace_id: "t-123",
        account_kind: "business",
        verified: false,
        verified_at: null,
      });
      fireEvent.click(within(card).getByRole("button", { name: "Unverify" }));

      await waitFor(() => {
        expect(mockSetBillingProfileVerifiedMutateAsync).toHaveBeenLastCalledWith({
          tenantId: "t-123",
          verified: false,
        });
      });
      await waitFor(() => {
        expect(within(card).getByText("Unverified")).toBeInTheDocument();
      });
      expect(within(card).getByRole("button", { name: "Verify" })).toBeInTheDocument();
    });

    it("lists the missing fields on billing_profile_incomplete", async () => {
      mockSetBillingProfileVerifiedMutateAsync.mockRejectedValueOnce(
        new PlatformApiError(409, "billing_profile_incomplete", "billing profile is incomplete", {
          fields: ["legal_name", "tax_code", "address", "invoice_email"],
        }),
      );

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);

      const card = getBillingProfileCard();

      fireEvent.click(within(card).getByRole("button", { name: "Verify" }));

      await waitFor(() => {
        const alert = within(card).getByRole("alert");
        expect(alert).toHaveTextContent("legal_name");
        expect(alert).toHaveTextContent("tax_code");
        expect(alert).toHaveTextContent("address");
        expect(alert).toHaveTextContent("invoice_email");
      });
    });

    it("shows Verified on load when the billing profile read says verified is true, and Unverified when false, without any click", () => {
      const verifiedAt = "2026-11-17T09:30:00Z";
      mockUseTenantBillingProfile.mockReturnValue({
        data: {
          workspace_id: "t-123",
          account_kind: "business",
          legal_name: "Acme Corp",
          tax_code: "0101234567",
          address: "123 Street",
          invoice_email: "acme@example.com",
          verified: true,
          verified_at: verifiedAt,
        },
        isLoading: false,
        isError: false,
      });

      const { unmount } = render(
        <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />,
      );

      expect(mockUseTenantBillingProfile).toHaveBeenCalledWith("t-123");
      const verifiedCard = getBillingProfileCard();
      expect(within(verifiedCard).getByText("Verified")).toBeInTheDocument();
      expect(within(verifiedCard).getByText("business")).toBeInTheDocument();
      expect(within(verifiedCard).getByText(new Date(verifiedAt).toLocaleString())).toBeInTheDocument();
      expect(within(verifiedCard).getByRole("button", { name: "Unverify" })).toBeInTheDocument();
      expect(mockSetBillingProfileVerifiedMutateAsync).not.toHaveBeenCalled();
      unmount();

      mockUseTenantBillingProfile.mockReturnValue({
        data: {
          workspace_id: "t-123",
          account_kind: "individual",
          legal_name: "",
          tax_code: "",
          address: "",
          invoice_email: "",
          verified: false,
          verified_at: null,
        },
        isLoading: false,
        isError: false,
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />);
      const unverifiedCard = getBillingProfileCard();
      expect(within(unverifiedCard).getByText("Unverified")).toBeInTheDocument();
      expect(within(unverifiedCard).getByText("individual")).toBeInTheDocument();
      expect(within(unverifiedCard).getByRole("button", { name: "Verify" })).toBeInTheDocument();
      expect(mockSetBillingProfileVerifiedMutateAsync).not.toHaveBeenCalled();
    });

    it("unverifies a profile loaded as verified and updates status from the mutation response", async () => {
      const verifiedAt = "2026-11-17T09:30:00Z";
      mockUseTenantBillingProfile.mockReturnValue({
        data: {
          workspace_id: "t-123",
          account_kind: "business",
          legal_name: "Acme Corp",
          tax_code: "0101234567",
          address: "123 Street",
          invoice_email: "acme@example.com",
          verified: true,
          verified_at: verifiedAt,
        },
        isLoading: false,
        isError: false,
      });
      mockSetBillingProfileVerifiedMutateAsync.mockResolvedValueOnce({
        workspace_id: "t-123",
        account_kind: "business",
        verified: false,
        verified_at: null,
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);
      const card = getBillingProfileCard();
      expect(within(card).getByText("Verified")).toBeInTheDocument();

      fireEvent.click(within(card).getByRole("button", { name: "Unverify" }));

      await waitFor(() => {
        expect(mockSetBillingProfileVerifiedMutateAsync).toHaveBeenCalledWith({
          tenantId: "t-123",
          verified: false,
        });
      });
      await waitFor(() => {
        expect(within(card).getByText("Unverified")).toBeInTheDocument();
      });
      expect(within(card).getByRole("button", { name: "Verify" })).toBeInTheDocument();
    });
  });

  describe("billing profile editing", () => {
    function getBillingProfileCard(): HTMLElement {
      const heading = screen.getByRole("heading", { name: "Billing profile" });
      return heading.closest<HTMLElement>("[data-slot='card']") ?? heading.parentElement!.parentElement!;
    }

    it("hides the Edit button for support and shows it for billing and admin", () => {
      const { unmount } = render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />);
      expect(within(getBillingProfileCard()).queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
      unmount();

      for (const role of ["billing", "admin"] as const) {
        const view = render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole={role} />);
        expect(within(getBillingProfileCard()).getByRole("button", { name: "Edit" })).toBeInTheDocument();
        view.unmount();
      }
    });

    it("opens prefilled form, discards on Cancel, submits PUT on Save, and refreshes view", async () => {
      mockUseTenantBillingProfile.mockReturnValue({
        data: {
          workspace_id: "t-123",
          account_kind: "business",
          legal_name: "Acme Corp",
          tax_code: "0101234567",
          address: "123 Street",
          invoice_email: "acme@example.com",
          verified: false,
          verified_at: null,
        },
        isLoading: false,
        isError: false,
      });
      mockUpdateTenantBillingProfileMutateAsync.mockResolvedValueOnce({
        workspace_id: "t-123",
        account_kind: "business",
        legal_name: "Acme Holdings",
        tax_code: "0101234567",
        address: "456 Avenue",
        invoice_email: "billing@acme.test",
        verified: false,
        verified_at: null,
        changed: true,
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />);

      const card = getBillingProfileCard();
      fireEvent.click(within(card).getByRole("button", { name: "Edit" }));

      const dialog = screen.getByRole("dialog");
      expect(within(dialog).getByRole("combobox", { name: /Account kind/i })).toHaveTextContent("Business");
      expect(within(dialog).getByLabelText(/Legal name/i)).toHaveValue("Acme Corp");
      expect(within(dialog).getByLabelText(/Tax code/i)).toHaveValue("0101234567");
      expect(within(dialog).getByLabelText(/^Address$/i)).toHaveValue("123 Street");
      expect(within(dialog).getByLabelText(/Invoice email/i)).toHaveValue("acme@example.com");

      fireEvent.change(within(dialog).getByLabelText(/Legal name/i), { target: { value: "Discarded Name" } });
      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
      expect(mockUpdateTenantBillingProfileMutateAsync).not.toHaveBeenCalled();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(within(card).getByText("Acme Corp")).toBeInTheDocument();

      fireEvent.click(within(card).getByRole("button", { name: "Edit" }));
      const dialog2 = screen.getByRole("dialog");
      expect(within(dialog2).getByLabelText(/Legal name/i)).toHaveValue("Acme Corp");

      fireEvent.change(within(dialog2).getByLabelText(/Legal name/i), { target: { value: "Acme Holdings" } });
      fireEvent.change(within(dialog2).getByLabelText(/^Address$/i), { target: { value: "456 Avenue" } });
      fireEvent.change(within(dialog2).getByLabelText(/Invoice email/i), { target: { value: "billing@acme.test" } });

      fireEvent.click(within(dialog2).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(mockUpdateTenantBillingProfileMutateAsync).toHaveBeenCalledWith({
          tenantId: "t-123",
          account_kind: "business",
          legal_name: "Acme Holdings",
          tax_code: "0101234567",
          address: "456 Avenue",
          invoice_email: "billing@acme.test",
        });
      });

      await waitFor(() => {
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      });
      expect(within(card).getByText("Acme Holdings")).toBeInTheDocument();
      expect(within(card).getByText("456 Avenue")).toBeInTheDocument();
      expect(within(card).getByText("billing@acme.test")).toBeInTheDocument();
    });

    it("shows missing field errors on 409 billing_profile_incomplete and disables Save while pending", async () => {
      const user = userEvent.setup();
      mockUpdateTenantBillingProfileMutateAsync.mockRejectedValueOnce(
        new PlatformApiError(409, "billing_profile_incomplete", "billing profile is incomplete", {
          fields: ["legal_name", "tax_code", "address", "invoice_email"],
        }),
      );

      const { rerender } = render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);
      await user.click(within(getBillingProfileCard()).getByRole("button", { name: "Edit" }));

      const dialog = screen.getByRole("dialog");
      await user.click(within(dialog).getByRole("combobox", { name: /Account kind/i }));
      await user.click(await screen.findByRole("option", { name: "Business" }));
      await user.click(within(dialog).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(within(dialog).getByLabelText(/Legal name/i)).toHaveAttribute("aria-invalid", "true");
        expect(within(dialog).getByLabelText(/Tax code/i)).toHaveAttribute("aria-invalid", "true");
        expect(within(dialog).getByLabelText(/^Address$/i)).toHaveAttribute("aria-invalid", "true");
        expect(within(dialog).getByLabelText(/Invoice email/i)).toHaveAttribute("aria-invalid", "true");
      });
      expect(within(dialog).getByText(/Legal name is required/i)).toBeInTheDocument();
      expect(within(dialog).getByText(/Tax code is required/i)).toBeInTheDocument();
      expect(within(dialog).getByText(/Address is required/i)).toBeInTheDocument();
      expect(within(dialog).getByText(/Invoice email is required/i)).toBeInTheDocument();

      // Editing clears that field's error
      fireEvent.change(within(dialog).getByLabelText(/Tax code/i), { target: { value: "0101234567" } });
      expect(within(dialog).getByLabelText(/Tax code/i)).toHaveAttribute("aria-invalid", "false");
      expect(within(dialog).queryByText(/Tax code is required/i)).not.toBeInTheDocument();

      mockUpdateBillingProfileIsPending = true;
      rerender(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);
      expect(within(screen.getByRole("dialog")).getByRole("button", { name: "Save" })).toBeDisabled();
    });

    it("shows invalid field errors on 400 billing_profile_invalid and message on 409 plan_audience_mismatch", async () => {
      mockUpdateTenantBillingProfileMutateAsync
        .mockRejectedValueOnce(
          new PlatformApiError(400, "billing_profile_invalid", "invalid billing profile", {
            fields: ["tax_code"],
          }),
        )
        .mockRejectedValueOnce(
          new PlatformApiError(
            409,
            "plan_audience_mismatch",
            "current plan is only available for business accounts",
          ),
        );

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);
      fireEvent.click(within(getBillingProfileCard()).getByRole("button", { name: "Edit" }));

      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(within(dialog).getByLabelText(/Tax code/i)).toHaveAttribute("aria-invalid", "true");
      });
      expect(within(dialog).getByLabelText(/Legal name/i)).toHaveAttribute("aria-invalid", "false");
      expect(within(dialog).getByText(/10-digit/i)).toBeInTheDocument();

      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(within(dialog).getByRole("alert")).toHaveTextContent(
          /current plan is only available for business accounts/i,
        );
      });
    });

    it("saving a legal_name change clears Verified to Unverified and restores Verify control", async () => {
      mockUseTenantBillingProfile.mockReturnValue({
        data: {
          workspace_id: "t-123",
          account_kind: "business",
          legal_name: "Acme Corp",
          tax_code: "0101234567",
          address: "123 Street",
          invoice_email: "acme@example.com",
          verified: true,
          verified_at: "2026-11-17T09:30:00Z",
        },
        isLoading: false,
        isError: false,
      });
      mockUpdateTenantBillingProfileMutateAsync.mockResolvedValueOnce({
        workspace_id: "t-123",
        account_kind: "business",
        legal_name: "Renamed Corp",
        tax_code: "0101234567",
        address: "123 Street",
        invoice_email: "acme@example.com",
        verified: false,
        verified_at: null,
        changed: true,
      });

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />);
      const card = getBillingProfileCard();
      expect(within(card).getByText("Verified")).toBeInTheDocument();
      expect(within(card).getByRole("button", { name: "Unverify" })).toBeInTheDocument();

      fireEvent.click(within(card).getByRole("button", { name: "Edit" }));
      const dialog = screen.getByRole("dialog");
      fireEvent.change(within(dialog).getByLabelText(/Legal name/i), { target: { value: "Renamed Corp" } });
      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      });
      expect(within(card).getByText("Renamed Corp")).toBeInTheDocument();
      expect(within(card).getByText("Unverified")).toBeInTheDocument();
      expect(within(card).getByRole("button", { name: "Verify" })).toBeInTheDocument();
    });

    it("shows non-field errors (like malformed_response or plan_audience_mismatch) without crashing", async () => {
      mockUpdateTenantBillingProfileMutateAsync.mockRejectedValueOnce(
        new PlatformApiError(200, "malformed_response", "The server answered with data the console cannot read"),
      );

      render(<TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="billing" />);
      fireEvent.click(within(getBillingProfileCard()).getByRole("button", { name: "Edit" }));

      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(within(dialog).getByRole("alert")).toHaveTextContent(
          /The server answered with data the console cannot read/i,
        );
      });
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });
});
