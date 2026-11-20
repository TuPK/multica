// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { LegalHolds } from "./LegalHolds";
import { Layout } from "../components/Layout";

const mockUseMe = vi.fn();
const mockUseLegalHolds = vi.fn();
const mockPlaceMutateAsync = vi.fn();
const mockReleaseMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useLegalHolds: (enabled = true) => mockUseLegalHolds(enabled),
  usePlaceLegalHold: () => ({
    mutateAsync: mockPlaceMutateAsync,
    isPending: false,
  }),
  useReleaseLegalHold: () => ({
    mutateAsync: mockReleaseMutateAsync,
    isPending: false,
  }),
}));

const activeHold = {
  id: "lh-1",
  scope_kind: "tenant" as const,
  scope_id: "t-1",
  reason: "Subpoena #42",
  case_ref: "CASE-2026-42",
  placed_by: "u-sys",
  released: false,
  released_by: "",
  placed_at: null,
  released_at: null,
};

describe("LegalHolds screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseLegalHolds.mockReturnValue({
      data: [activeHold],
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("place requires reason and case ref", async () => {
    mockPlaceMutateAsync.mockResolvedValueOnce({
      ...activeHold,
      id: "lh-2",
      scope_id: "t-2",
      case_ref: "CASE-99",
      reason: "Litigation preservation",
    });

    render(<LegalHolds userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: /Place legal hold/i }));
    const dialog = screen.getByRole("dialog");
    const submitBtn = within(dialog).getByRole("button", { name: /^Place hold$/i });
    expect(submitBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Scope ID/i), {
      target: { value: "t-2" },
    });
    expect(submitBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Case reference/i), {
      target: { value: "CASE-99" },
    });
    expect(submitBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Litigation preservation" },
    });
    expect(submitBtn).not.toBeDisabledControl();

    fireEvent.click(submitBtn);
    await waitFor(() => {
      expect(mockPlaceMutateAsync).toHaveBeenCalledWith({
        scope_kind: "tenant",
        scope_id: "t-2",
        case_ref: "CASE-99",
        reason: "Litigation preservation",
      });
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("admin has no menu entry", () => {
    render(
      <Layout
        currentScreen="tenants"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(within(nav).queryByRole("button", { name: /Legal holds/i })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" },
    });
    render(<LegalHolds userRole="admin" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });

  it("system_admin releases an active hold", async () => {
    mockReleaseMutateAsync.mockResolvedValueOnce({ ...activeHold, released: true });

    render(<LegalHolds userRole="system_admin" />);
    const row = screen.getByText("CASE-2026-42").closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Release" }));

    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Release hold" }));

    await waitFor(() => {
      expect(mockReleaseMutateAsync).toHaveBeenCalledWith({ id: "lh-1" });
    });
  });
});
