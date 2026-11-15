// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { renderWithI18n } from "../test/i18n";

const mockMutateAsync = vi.hoisted(() => vi.fn());
const cancelState = vi.hoisted(() => ({
  isPending: false,
}));

vi.mock("@multica/core/account", () => ({
  useCancelAccountDeletion: () => ({
    mutateAsync: mockMutateAsync,
    isPending: cancelState.isPending,
  }),
}));

const mockLogout = vi.hoisted(() => vi.fn());
vi.mock("../auth", () => ({
  useLogout: () => mockLogout,
}));

const mockSetUser = vi.hoisted(() => vi.fn());
const currentUserState = vi.hoisted(() => ({
  user: null as {
    id: string;
    name: string;
    email: string;
    deletion_scheduled_at?: string | null;
  } | null,
}));

vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (
      selector?: (state: {
        user: typeof currentUserState.user;
        setUser: typeof mockSetUser;
      }) => unknown,
    ) =>
      selector
        ? selector({ user: currentUserState.user, setUser: mockSetUser })
        : { user: currentUserState.user, setUser: mockSetUser },
    { getState: () => ({ user: currentUserState.user, setUser: mockSetUser }) },
  );
  return { useAuthStore };
});

const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
  },
}));

import { DeletionGate } from "./deletion-gate";

describe("DeletionGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    cancelState.isPending = false;
    currentUserState.user = null;
  });

  it("renders nothing when user is null", () => {
    currentUserState.user = null;
    const { container } = renderWithI18n(<DeletionGate />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when deletion_scheduled_at is absent or null", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
    };
    const { container } = renderWithI18n(<DeletionGate />);
    expect(container).toBeEmptyDOMElement();

    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: null,
    };
    const { container: container2 } = renderWithI18n(<DeletionGate />);
    expect(container2).toBeEmptyDOMElement();
  });

  it("renders blocking dialog with date, cancel button and sign out button when deletion_scheduled_at is set", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    expect(
      screen.getByRole("heading", { name: "Account scheduled for deletion" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Your account is scheduled to be deleted on/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Cancel deletion" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Sign out" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Close" })).not.toBeInTheDocument();
  });

  it("does not dismiss dialog when pressing Escape", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    fireEvent.keyDown(window, { key: "Escape" });

    expect(
      screen.getByRole("heading", { name: "Account scheduled for deletion" }),
    ).toBeInTheDocument();
  });

  it("calls logout when clicking Sign out", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    expect(mockLogout).toHaveBeenCalledTimes(1);
  });

  it("disables buttons and shows busy indicator when isPending is true", () => {
    cancelState.isPending = true;
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    const cancelBtn = screen.getByRole("button", { name: "Canceling..." });
    expect(cancelBtn).toBeDisabled();
    expect(cancelBtn).toHaveAttribute("aria-busy", "true");

    const signOutBtn = screen.getByRole("button", { name: "Sign out" });
    expect(signOutBtn).toBeDisabled();
  });

  it("calls mutateAsync on cancel and clears scheduled date on success", async () => {
    mockMutateAsync.mockResolvedValueOnce({ status: "active" });
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    const cancelBtn = screen.getByRole("button", { name: "Cancel deletion" });
    fireEvent.click(cancelBtn);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(mockSetUser).toHaveBeenCalledWith({
        ...currentUserState.user,
        deletion_scheduled_at: null,
      });
      expect(mockToastSuccess).toHaveBeenCalledWith(
        "Account deletion canceled",
      );
    });
  });

  it("shows error toast and keeps dialog open when cancel mutation rejects", async () => {
    mockMutateAsync.mockRejectedValueOnce(new Error("Network error"));
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      deletion_scheduled_at: "2026-11-29T10:00:00Z",
    };

    renderWithI18n(<DeletionGate />);

    const cancelBtn = screen.getByRole("button", { name: "Cancel deletion" });
    fireEvent.click(cancelBtn);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1);
    });

    expect(mockToastError).toHaveBeenCalledWith("Network error");
    expect(
      screen.getByRole("heading", { name: "Account scheduled for deletion" }),
    ).toBeInTheDocument();
  });
});
