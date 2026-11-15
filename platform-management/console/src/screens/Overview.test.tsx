// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { Overview } from "./Overview";

const mockUseOverview = vi.fn();

vi.mock("../api/hooks", () => ({
  useOverview: () => mockUseOverview(),
}));

describe("Overview screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders workspace counts by status and 7d signups", () => {
    mockUseOverview.mockReturnValue({
      data: {
        workspaces_by_status: {
          active: 42,
          read_only: 5,
          suspended: 3,
          pending_deletion: 1,
        },
        signups_7d: 19,
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Overview />);

    expect(screen.getByRole("heading", { name: "Platform Overview" })).toBeInTheDocument();
    expect(screen.getByText("Total Workspaces")).toBeInTheDocument();
    expect(screen.getByText("51")).toBeInTheDocument(); // 42+5+3+1
    expect(screen.getByText("Active Workspaces")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("Sign-ups (7 Days)")).toBeInTheDocument();
    expect(screen.getByText("19")).toBeInTheDocument();
    expect(screen.getByText("Read-Only")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("Suspended")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("Pending Deletion")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("handles loading, error, and empty states", () => {
    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<Overview />);
    expect(screen.getByText("Loading overview...")).toBeInTheDocument();

    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("Metrics failure"),
      refetch: vi.fn(),
    });

    rerender(<Overview />);
    expect(screen.getByText("Failed to load overview")).toBeInTheDocument();
    expect(screen.getByText("Metrics failure")).toBeInTheDocument();

    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    rerender(<Overview />);
    expect(screen.getByText("No overview data")).toBeInTheDocument();
  });
});
