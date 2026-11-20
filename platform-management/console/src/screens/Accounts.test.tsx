// @vitest-environment jsdom
import type { ReactNode } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Accounts } from "./Accounts";
import { PlatformApiError } from "../api/client";
import { AccountApiError } from "../api/hooks";
import { findAlert } from "../test/alert";

const mockUseAccountRequests = vi.fn();
const mockUsePlans = vi.fn();
const mockCreateMutateAsync = vi.fn();
const mockCancelMutateAsync = vi.fn();
const mockResendMutateAsync = vi.fn();
let mockMeRole = "system_admin";
let useRealHooks = false;

vi.mock("../api/hooks", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/hooks")>();
  return {
    ...actual,
    useMe: () =>
      useRealHooks
        ? actual.useMe()
        : { data: { user_id: "u-admin", email: "admin@example.com", role: mockMeRole } },
    usePlans: () => (useRealHooks ? actual.usePlans() : mockUsePlans()),
    useAccountRequests: (state?: Parameters<typeof actual.useAccountRequests>[0]) =>
      useRealHooks ? actual.useAccountRequests(state) : mockUseAccountRequests(state),
    useCreateAccountRequest: () =>
      useRealHooks
        ? actual.useCreateAccountRequest()
        : { mutateAsync: mockCreateMutateAsync, isPending: false },
    useCancelAccountRequest: () =>
      useRealHooks
        ? actual.useCancelAccountRequest()
        : { mutateAsync: mockCancelMutateAsync, isPending: false },
    useResendWelcome: () =>
      useRealHooks
        ? actual.useResendWelcome()
        : { mutateAsync: mockResendMutateAsync, isPending: false },
  };
});

const mockPlans = [
  {
    key: "free",
    name: "Free",
    limits: {},
    features: {},
    currency: "vnd",
    seat_price_month: 0,
    seat_price_year: 0,
    seats_fixed: 3,
    min_seats: null,
    trial_days: 0,
    is_public: true,
    sort: 10,
    archived_at: null,
  },
  {
    key: "pro",
    name: "Pro",
    limits: {},
    features: {},
    currency: "vnd",
    seat_price_month: 149000,
    seat_price_year: 1490000,
    seats_fixed: null,
    min_seats: 1,
    trial_days: 14,
    is_public: true,
    sort: 20,
    archived_at: null,
  },
  {
    key: "business",
    name: "Business",
    limits: {},
    features: {},
    currency: "vnd",
    seat_price_month: 349000,
    seat_price_year: 3490000,
    seats_fixed: null,
    min_seats: 5,
    trial_days: 14,
    is_public: true,
    sort: 30,
    archived_at: null,
  },
];

const baseRequest = {
  id: "req-1",
  kind: "individual" as const,
  email: "pending@example.com",
  display_name: "Pending User",
  workspace_name: "Pending Space",
  workspace_slug: "",
  plan_key: "free",
  seats: 1,
  interval: "month" as const,
  trial: false,
  comp_reason: "",
  state: "pending" as const,
  error_code: "",
  attempts: 0,
  user_id: "",
  workspace_id: "",
  created_by: "u-admin",
  created_at: "2026-11-16T10:00:00Z",
  updated_at: "2026-11-16T10:00:00Z",
  completed_at: null,
  welcome_sent_at: null,
  welcome_attempts: 0,
};

const mockRequests = [
  baseRequest,
  {
    ...baseRequest,
    id: "req-2",
    kind: "business" as const,
    email: "done@example.com",
    display_name: "Done User",
    workspace_name: "Done Corp",
    plan_key: "business",
    seats: 5,
    trial: true,
    state: "completed" as const,
    attempts: 1,
    user_id: "u-2",
    workspace_id: "ws-2",
    completed_at: "2026-11-16T10:02:00Z",
    welcome_sent_at: "2026-11-16T10:02:05Z",
    welcome_attempts: 1,
  },
  {
    ...baseRequest,
    id: "req-3",
    kind: "business" as const,
    email: "partial@example.com",
    display_name: "Partial User",
    workspace_name: "Partial Corp",
    plan_key: "business",
    seats: 5,
    trial: true,
    state: "failed" as const,
    error_code: "finalize_failed",
    attempts: 3,
    user_id: "u-3",
    workspace_id: "ws-3",
  },
  {
    ...baseRequest,
    id: "req-4",
    email: "blocked@example.com",
    display_name: "Blocked User",
    workspace_name: "Blocked Space",
    state: "failed" as const,
    error_code: "slug_unavailable",
    attempts: 1,
  },
  {
    ...baseRequest,
    id: "req-5",
    email: "canceled@example.com",
    display_name: "Canceled User",
    workspace_name: "Canceled Space",
    state: "canceled" as const,
  },
];

function openCreateDialog() {
  fireEvent.click(screen.getByRole("button", { name: "Create account" }));
  return screen.getByRole("dialog");
}

function fillCommonIndividualFields(dialog: HTMLElement, email = "ana@example.com") {
  fireEvent.change(within(dialog).getByLabelText(/^Email$/i), { target: { value: email } });
  fireEvent.change(within(dialog).getByLabelText(/Confirm email/i), { target: { value: email } });
  fireEvent.change(within(dialog).getByLabelText(/Display name/i), { target: { value: "Ana" } });
  fireEvent.change(within(dialog).getByLabelText(/Workspace name/i), { target: { value: "Ana's Space" } });
}

describe("Accounts screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useRealHooks = false;
    mockMeRole = "system_admin";
    mockUsePlans.mockReturnValue({
      data: mockPlans,
      isLoading: false,
      isError: false,
    });
    mockUseAccountRequests.mockReturnValue({
      data: mockRequests,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("requires the email twice and rejects a mismatch", async () => {
    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();

    fireEvent.change(within(dialog).getByLabelText(/^Email$/i), { target: { value: "ana@example.com" } });
    fireEvent.change(within(dialog).getByLabelText(/Confirm email/i), { target: { value: "anna@example.com" } });
    fireEvent.change(within(dialog).getByLabelText(/Display name/i), { target: { value: "Ana" } });
    fireEvent.change(within(dialog).getByLabelText(/Workspace name/i), { target: { value: "Ana Space" } });

    expect(within(dialog).getByText(/Email addresses do not match/i)).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole("button", { name: "Submit request" }));

    expect(mockCreateMutateAsync).not.toHaveBeenCalled();
    expect(within(dialog).getByText(/Email addresses do not match/i)).toBeInTheDocument();
  });

  it("requires the billing profile for a business account", async () => {
    const user = userEvent.setup();
    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();

    await user.click(within(dialog).getByRole("combobox", { name: /Account kind/i }));
    await user.click(await screen.findByRole("option", { name: "Business" }));
    fillCommonIndividualFields(dialog, "boss@acme.example");
    await user.click(within(dialog).getByRole("checkbox", { name: /^Trial$/i }));

    await user.click(within(dialog).getByRole("button", { name: "Submit request" }));

    expect(mockCreateMutateAsync).not.toHaveBeenCalled();
    expect(within(dialog).getByText(/Legal name is required/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/Tax code is required/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/Address is required/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/Invoice email is required/i)).toBeInTheDocument();
  });

  it("rejects a paid plan when both trial and complimentary reason are missing", async () => {
    const user = userEvent.setup();
    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();

    fillCommonIndividualFields(dialog, "pro@example.com");
    await user.click(within(dialog).getByRole("combobox", { name: /^Plan$/i }));
    await user.click(await screen.findByRole("option", { name: /Pro \(pro\)/i }));

    await user.click(within(dialog).getByRole("button", { name: "Submit request" }));

    expect(mockCreateMutateAsync).not.toHaveBeenCalled();
    expect(
      within(dialog).getByText(/paid plan requires either a trial or a complimentary reason/i),
    ).toBeInTheDocument();
  });

  it("shows a 409 account_email_exists error from the backend", async () => {
    mockCreateMutateAsync.mockRejectedValue(
      new AccountApiError(409, "account_email_exists", "an account with this email already exists"),
    );

    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();
    fillCommonIndividualFields(dialog, "taken@example.com");

    fireEvent.click(within(dialog).getByRole("button", { name: "Submit request" }));

    await waitFor(() => {
      expect(findAlert(dialog)).toHaveTextContent(
        /an account with this email already exists/i,
      );
    });
  });

  it("shows field messages for a 400 account_request_invalid response", async () => {
    const user = userEvent.setup();
    mockCreateMutateAsync.mockRejectedValue(
      new AccountApiError(400, "account_request_invalid", "the request has invalid fields", [
        "workspace_slug",
        "billing_profile.tax_code",
      ]),
    );

    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();
    await user.click(within(dialog).getByRole("combobox", { name: /Account kind/i }));
    await user.click(await screen.findByRole("option", { name: "Business" }));
    fillCommonIndividualFields(dialog, "boss@acme.example");
    fireEvent.change(within(dialog).getByLabelText(/Workspace slug/i), { target: { value: "bad-slug" } });
    await user.click(within(dialog).getByRole("checkbox", { name: /^Trial$/i }));
    fireEvent.change(within(dialog).getByLabelText(/Legal name/i), { target: { value: "Acme Co" } });
    fireEvent.change(within(dialog).getByLabelText(/Tax code/i), { target: { value: "12345" } });
    fireEvent.change(within(dialog).getByLabelText(/^Address$/i), { target: { value: "1 Main St" } });
    fireEvent.change(within(dialog).getByLabelText(/Invoice email/i), { target: { value: "ap@acme.example" } });

    await user.click(within(dialog).getByRole("button", { name: "Submit request" }));

    await waitFor(() => {
      expect(within(dialog).getByLabelText(/Workspace slug/i)).toHaveAttribute("aria-invalid", "true");
      expect(within(dialog).getByLabelText(/Tax code/i)).toHaveAttribute("aria-invalid", "true");
    });
    expect(within(dialog).getByText(/lowercase letters, numbers, and hyphens/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/10-digit/i)).toBeInTheDocument();
  });

  it("submits with an Idempotency-Key generated per form open and closes on 202", async () => {
    mockCreateMutateAsync.mockResolvedValue({ ...baseRequest, id: "req-new", email: "ana@example.com" });

    render(<Accounts userRole="system_admin" />);
    const dialog = openCreateDialog();
    fillCommonIndividualFields(dialog, "ana@example.com");

    fireEvent.click(within(dialog).getByRole("button", { name: "Submit request" }));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledTimes(1);
    });
    const firstCall = mockCreateMutateAsync.mock.calls[0]![0];
    expect(firstCall.idempotencyKey).toBeTruthy();
    expect(firstCall.input).toMatchObject({
      kind: "individual",
      email: "ana@example.com",
      display_name: "Ana",
      workspace_name: "Ana's Space",
      plan_key: "free",
    });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    // Reopening the form generates a new Idempotency-Key
    const dialog2 = openCreateDialog();
    fillCommonIndividualFields(dialog2, "bob@example.com");
    fireEvent.click(within(dialog2).getByRole("button", { name: "Submit request" }));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledTimes(2);
    });
    const secondCall = mockCreateMutateAsync.mock.calls[1]![0];
    expect(secondCall.idempotencyKey).toBeTruthy();
    expect(secondCall.idempotencyKey).not.toBe(firstCall.idempotencyKey);
  });

  it("renders rows including a finalize_failed row with its distinct error state and label", () => {
    render(<Accounts userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Accounts" })).toBeInTheDocument();
    expect(screen.getByText("pending@example.com")).toBeInTheDocument();
    expect(screen.getByText("done@example.com")).toBeInTheDocument();
    expect(screen.getByText("partial@example.com")).toBeInTheDocument();
    expect(screen.getByText("blocked@example.com")).toBeInTheDocument();

    const finalizeRow = screen.getByText("partial@example.com").closest("tr")!;
    expect(within(finalizeRow).getByText("Finalize failed")).toBeInTheDocument();
    expect(within(finalizeRow).getByText(/Subscription finalization failed/i)).toBeInTheDocument();

    const slugFailedRow = screen.getByText("blocked@example.com").closest("tr")!;
    expect(within(slugFailedRow).getByText("Failed")).toBeInTheDocument();
    expect(within(slugFailedRow).getByText(/Workspace slug unavailable/i)).toBeInTheDocument();
  });

  it("offers Cancel only on pending rows and calls cancel endpoint after confirmation", async () => {
    mockCancelMutateAsync.mockResolvedValue({ ...baseRequest, state: "canceled" });

    render(<Accounts userRole="system_admin" />);

    const pendingRow = screen.getByText("pending@example.com").closest("tr")!;
    const completedRow = screen.getByText("done@example.com").closest("tr")!;
    const finalizeRow = screen.getByText("partial@example.com").closest("tr")!;
    const canceledRow = screen.getByText("canceled@example.com").closest("tr")!;

    expect(within(pendingRow).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    expect(within(completedRow).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
    expect(within(finalizeRow).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
    expect(within(canceledRow).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();

    fireEvent.click(within(pendingRow).getByRole("button", { name: "Cancel" }));
    expect(mockCancelMutateAsync).not.toHaveBeenCalled();

    const confirmDialog = screen.getByRole("dialog");
    expect(within(confirmDialog).getByText(/pending@example.com/)).toBeInTheDocument();
    fireEvent.click(within(confirmDialog).getByRole("button", { name: "Confirm cancel" }));

    await waitFor(() => {
      expect(mockCancelMutateAsync).toHaveBeenCalledWith({ id: "req-1" });
    });
  });

  it("offers Resend welcome on completed and finalize_failed rows and shows 429 wait message", async () => {
    mockResendMutateAsync.mockRejectedValue(
      new AccountApiError(429, "account_welcome_rate_limited", "too many welcome emails", [], "90"),
    );

    render(<Accounts userRole="system_admin" />);

    const pendingRow = screen.getByText("pending@example.com").closest("tr")!;
    const completedRow = screen.getByText("done@example.com").closest("tr")!;
    const finalizeRow = screen.getByText("partial@example.com").closest("tr")!;
    const slugFailedRow = screen.getByText("blocked@example.com").closest("tr")!;

    expect(within(pendingRow).queryByRole("button", { name: "Resend welcome" })).not.toBeInTheDocument();
    expect(within(slugFailedRow).queryByRole("button", { name: "Resend welcome" })).not.toBeInTheDocument();
    expect(within(completedRow).getByRole("button", { name: "Resend welcome" })).toBeInTheDocument();
    expect(within(finalizeRow).getByRole("button", { name: "Resend welcome" })).toBeInTheDocument();

    fireEvent.click(within(finalizeRow).getByRole("button", { name: "Resend welcome" }));

    await waitFor(() => {
      expect(mockResendMutateAsync).toHaveBeenCalledWith({ id: "req-3" });
    });
    expect(findAlert()).toHaveTextContent(/90 seconds/i);
  });

  it("shows a send failure message on 502 from resend-welcome", async () => {
    mockResendMutateAsync.mockRejectedValue(
      new AccountApiError(502, "mail_send_failed", "smtp error"),
    );

    render(<Accounts userRole="system_admin" />);
    const completedRow = screen.getByText("done@example.com").closest("tr")!;
    fireEvent.click(within(completedRow).getByRole("button", { name: "Resend welcome" }));

    await waitFor(() => {
      expect(findAlert()).toHaveTextContent(/failed to send welcome email/i);
    });
  });

  it("shows error state (not a crash) on malformed server response via zod parse failure", async () => {
    useRealHooks = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(String(input), "http://console.test");
        if (url.pathname === "/api/platform/me") {
          return new Response(
            JSON.stringify({ user_id: "u-admin", email: "admin@example.com", role: "system_admin" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.pathname === "/api/platform/plans") {
          return new Response(JSON.stringify({ plans: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.pathname === "/api/platform/accounts") {
          return new Response(JSON.stringify({ requests: "not-an-array" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response("not found", { status: 404 });
      }),
    );

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    render(<Accounts userRole="system_admin" />, { wrapper });

    await waitFor(() => {
      expect(findAlert()).toBeInTheDocument();
    });
    expect(screen.getByText(/Failed to load account requests/i)).toBeInTheDocument();
  });

  it("hides the screen behind Not found for non-admin roles", () => {
    mockMeRole = "support";
    render(<Accounts userRole="support" />);

    expect(screen.queryByRole("heading", { name: "Accounts" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Not found" })).toBeInTheDocument();
  });

  it("switches state filter tabs and falls back to text plan input when plans are unavailable", () => {
    mockUsePlans.mockReturnValue({ data: [], isLoading: false, isError: false });
    render(<Accounts userRole="system_admin" />);

    fireEvent.click(screen.getByRole("tab", { name: "Failed" }));
    expect(mockUseAccountRequests).toHaveBeenCalledWith("failed");

    const dialog = openCreateDialog();
    const planInput = within(dialog).getByLabelText(/^Plan$/i);
    expect(planInput.tagName).toBe("INPUT");
  });

  it("maps account_activation_required, plan_audience_mismatch, 429 Retry-After, and generic errors on submit", async () => {
    render(<Accounts userRole="system_admin" />);

    // 400 account_activation_required
    mockCreateMutateAsync.mockRejectedValueOnce(
      new AccountApiError(400, "account_activation_required", "a paid plan needs activation"),
    );
    const dialog1 = openCreateDialog();
    fillCommonIndividualFields(dialog1, "act@example.com");
    fireEvent.click(within(dialog1).getByRole("button", { name: "Submit request" }));
    await waitFor(() => {
      expect(findAlert(dialog1)).toHaveTextContent(/a paid plan needs activation/i);
    });

    // 409 plan_audience_mismatch
    mockCreateMutateAsync.mockRejectedValueOnce(
      new AccountApiError(409, "plan_audience_mismatch", "the plan is not sold to this kind of account"),
    );
    fireEvent.click(within(dialog1).getByRole("button", { name: "Submit request" }));
    await waitFor(() => {
      expect(within(dialog1).getByRole("combobox", { name: /^Plan$/i })).toHaveAttribute("aria-invalid", "true");
      expect(findAlert(dialog1)).toHaveTextContent(/not sold to this kind/i);
    });

    // 429 account_create_rate_limited with Retry-After
    mockCreateMutateAsync.mockRejectedValueOnce(
      new AccountApiError(429, "account_create_rate_limited", "too many requests", [], "120"),
    );
    fireEvent.click(within(dialog1).getByRole("button", { name: "Submit request" }));
    await waitFor(() => {
      expect(findAlert(dialog1)).toHaveTextContent(/2 minutes/i);
    });

    // Generic 500 error
    mockCreateMutateAsync.mockRejectedValueOnce(new PlatformApiError(500, "internal", "db crashed"));
    fireEvent.click(within(dialog1).getByRole("button", { name: "Submit request" }));
    await waitFor(() => {
      expect(findAlert(dialog1)).toHaveTextContent(/Unable to create account request/i);
    });
  });

  it("shows conflict error in cancel dialog and keeps dialog open on failure", async () => {
    mockCancelMutateAsync.mockRejectedValue(
      new AccountApiError(409, "account_request_not_cancelable", "only a pending request can be canceled"),
    );

    render(<Accounts userRole="system_admin" />);
    const pendingRow = screen.getByText("pending@example.com").closest("tr")!;
    fireEvent.click(within(pendingRow).getByRole("button", { name: "Cancel" }));

    const confirmDialog = screen.getByRole("dialog");
    fireEvent.click(within(confirmDialog).getByRole("button", { name: "Confirm cancel" }));

    await waitFor(() => {
      expect(findAlert(confirmDialog)).toHaveTextContent(
        /only a pending request can be canceled/i,
      );
    });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("shows Never logged in tag only when logged_in is false (not for true, null, or omitted)", () => {
    mockUseAccountRequests.mockReturnValue({
      data: [
        { ...baseRequest, id: "req-unseen", email: "unseen@example.com", state: "completed", logged_in: false },
        { ...baseRequest, id: "req-seen", email: "seen@example.com", state: "completed", logged_in: true },
        { ...baseRequest, id: "req-nouser", email: "nouser@example.com", state: "pending", logged_in: null },
        { ...baseRequest, id: "req-legacy", email: "legacy@example.com", state: "completed" },
      ],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Accounts userRole="system_admin" />);

    const unseenRow = screen.getByText("unseen@example.com").closest("tr")!;
    const seenRow = screen.getByText("seen@example.com").closest("tr")!;
    const noUserRow = screen.getByText("nouser@example.com").closest("tr")!;
    const legacyRow = screen.getByText("legacy@example.com").closest("tr")!;

    expect(within(unseenRow).getByText("Never logged in")).toBeInTheDocument();
    expect(within(seenRow).queryByText("Never logged in")).not.toBeInTheDocument();
    expect(within(noUserRow).queryByText("Never logged in")).not.toBeInTheDocument();
    expect(within(legacyRow).queryByText("Never logged in")).not.toBeInTheDocument();
  });
});
