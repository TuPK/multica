// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { PlanHistory } from "./PlanHistory";

const mockUseMe = vi.fn();
const mockUsePlanHistory = vi.fn();
const mockUsePlan = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  usePlanHistory: (key: string | undefined, enabled = true) => mockUsePlanHistory(key, enabled),
  usePlan: (key: string | undefined, enabled = true) => mockUsePlan(key, enabled),
}));

describe("PlanHistory screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUsePlan.mockReturnValue({
      data: { key: "pro", name: "Pro Plan" },
      isLoading: false,
      isError: false,
    });
  });

  it("renders who when old new", () => {
    mockUsePlanHistory.mockReturnValue({
      data: [
        {
          at: "2026-11-19T09:00:00Z",
          actor: "root@example.test",
          action: "plan.changed",
          reason: "Rename and adjust limits",
          changes: [
            { field: "name", old: "Pro", new: "Pro Plus" },
            { field: "description", old: "", new: "For growing teams" },
            { field: "limits", old: { issue_count: 500 }, new: { issue_count: 1000 } },
          ],
        },
        {
          at: "2026-11-18T08:00:00Z",
          actor: "billing-ops@example.test",
          action: "plan.deactivated",
          reason: "",
          changes: [{ field: "archived_at", old: null, new: "2026-11-18T08:00:00Z" }],
        },
      ],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    const onBack = vi.fn();
    render(<PlanHistory planKey="pro" onBack={onBack} userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: /Plan History/i })).toBeInTheDocument();
    expect(screen.getByText("root@example.test")).toBeInTheDocument();
    expect(screen.getByText("billing-ops@example.test")).toBeInTheDocument();
    expect(screen.getByText(new Date("2026-11-19T09:00:00Z").toLocaleString())).toBeInTheDocument();
    expect(screen.getByText("Pro")).toBeInTheDocument();
    expect(screen.getByText("Pro Plus")).toBeInTheDocument();
    expect(screen.getByText("For growing teams")).toBeInTheDocument();
    expect(screen.getByText("Rename and adjust limits")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Back to plans/i }));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it("hides plan history from admin and support roles", () => {
    mockUsePlanHistory.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    for (const role of ["admin", "support"] as const) {
      mockUseMe.mockReturnValue({
        data: { user_id: `u-${role}`, email: `${role}@example.com`, role, tenant_id: null },
      });
      const { unmount } = render(<PlanHistory planKey="pro" userRole={role} />);
      expect(mockUsePlanHistory).toHaveBeenLastCalledWith("pro", false);
      expect(screen.getByRole("heading", { name: "Not found" })).toBeInTheDocument();
      unmount();
    }
  });
});
