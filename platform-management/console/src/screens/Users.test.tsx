// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Users } from "./Users";

const mockUseUsers = vi.fn();

vi.mock("../api/hooks", () => ({
  useUsers: (filters: { q?: string }) => mockUseUsers(filters),
}));

const mockUsersList = [
  {
    id: "u-1",
    email: "alice@example.com",
    name: "Alice",
    status: "active",
    memberships_count: 3,
    created_at: "2026-10-01T00:00:00Z",
  },
  {
    id: "u-2",
    email: "bob@example.com",
    name: "Bob",
    status: "suspended",
    memberships_count: 1,
    created_at: "2026-10-05T00:00:00Z",
  },
];

describe("Users screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders users list", () => {
    mockUseUsers.mockReturnValue({
      data: { users: mockUsersList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Users onSelectUser={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Users" })).toBeInTheDocument();
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("calls onSelectUser when View button is clicked", () => {
    mockUseUsers.mockReturnValue({
      data: { users: mockUsersList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    const onSelect = vi.fn();

    render(<Users onSelectUser={onSelect} />);

    const viewButtons = screen.getAllByRole("button", { name: "View" });
    expect(viewButtons[0]).toBeDefined();
    fireEvent.click(viewButtons[0]!);

    expect(onSelect).toHaveBeenCalledWith("u-1");
  });

  it("handles loading, error, and empty states", () => {
    mockUseUsers.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<Users onSelectUser={vi.fn()} />);
    expect(screen.getByText("Loading users...")).toBeInTheDocument();

    mockUseUsers.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("Database error"),
      refetch: vi.fn(),
    });

    rerender(<Users onSelectUser={vi.fn()} />);
    expect(screen.getByText("Failed to load users")).toBeInTheDocument();
    expect(screen.getByText("Database error")).toBeInTheDocument();

    mockUseUsers.mockReturnValue({
      data: { users: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    rerender(<Users onSelectUser={vi.fn()} />);
    expect(screen.getByText("No users found")).toBeInTheDocument();
  });

  it("offers Clear search when a search query returns no users", () => {
    mockUseUsers.mockReturnValue({
      data: { users: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Users onSelectUser={vi.fn()} />);
    const input = screen.getByPlaceholderText("Search by email or name...");
    fireEvent.change(input, { target: { value: "missing@example.com" } });
    expect(mockUseUsers).toHaveBeenLastCalledWith({ q: "missing@example.com" });

    const clearButtons = screen.getAllByRole("button", { name: /Clear search/i });
    fireEvent.click(clearButtons[clearButtons.length - 1]!);
    expect(input).toHaveValue("");
    expect(mockUseUsers).toHaveBeenLastCalledWith({ q: undefined });
  });

  it("scrolls horizontally inside the table card without hiding columns", () => {
    mockUseUsers.mockReturnValue({
      data: { users: mockUsersList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Users onSelectUser={vi.fn()} />);

    for (const col of ["Email", "Name", "Status", "Workspaces", "Created", "Action"]) {
      expect(screen.getByRole("columnheader", { name: col })).toBeInTheDocument();
    }

    const tableCard = screen.getByRole("table").closest(".astryx-card");
    expect(tableCard).not.toBeNull();
    expect(tableCard).toHaveClass("overflow-x-auto");

    const emailEl = screen.getByText("alice@example.com");
    expect(emailEl).toHaveClass("truncate");
    expect(emailEl).toHaveAttribute("title", "alice@example.com");
  });
});
