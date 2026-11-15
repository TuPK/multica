// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";

const mockMutateAsync = vi.hoisted(() => vi.fn());
const revokeState = vi.hoisted(() => ({
  isPending: false,
}));

vi.mock("@multica/core/account", () => ({
  useRevokeAllSessions: () => ({
    mutateAsync: mockMutateAsync,
    isPending: revokeState.isPending,
  }),
}));

const mockSetUser = vi.hoisted(() => vi.fn());
const currentUser = vi.hoisted(() => ({
  id: "user-1",
  name: "Jane Doe",
  profile_description: "Software Engineer",
  avatar_url: null,
}));

vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (
      selector?: (state: {
        user: typeof currentUser | null;
        setUser: typeof mockSetUser;
      }) => unknown,
    ) =>
      selector
        ? selector({ user: currentUser, setUser: mockSetUser })
        : { user: currentUser, setUser: mockSetUser },
    { getState: () => ({ user: currentUser, setUser: mockSetUser }) },
  );
  return { useAuthStore };
});

vi.mock("@multica/core/api", () => ({
  api: {
    updateMe: vi.fn(),
  },
}));

const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
  },
}));

import { AccountTab } from "./account-tab";

function renderAccountTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <AccountTab />
    </QueryClientProvider>,
  );
}

describe("AccountTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    revokeState.isPending = false;
    mockMutateAsync.mockResolvedValue({
      revoked_at: "2026-11-15T00:00:00Z",
      token: "new-jwt-token",
    });
  });

  it("renders the account profile and sessions section with sign out action", () => {
    renderAccountTab();

    expect(screen.getByText("Personal information")).toBeInTheDocument();
    expect(screen.getByText("Sessions")).toBeInTheDocument();
    expect(
      screen.getByText("Personal access tokens are not revoked."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Sign out of all devices" }),
    ).toBeInTheDocument();
  });

  it("opens confirmation dialog when clicking Sign out of all devices", () => {
    renderAccountTab();

    const openButton = screen.getByRole("button", {
      name: "Sign out of all devices",
    });
    fireEvent.click(openButton);

    expect(
      screen.getByRole("heading", { name: "Sign out of all devices" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("You will be signed out on all other devices."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Sign out" }),
    ).toBeInTheDocument();
  });

  it("closes dialog on cancel without revoking", async () => {
    renderAccountTab();

    fireEvent.click(
      screen.getByRole("button", { name: "Sign out of all devices" }),
    );
    expect(
      screen.getByRole("heading", { name: "Sign out of all devices" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(
        screen.queryByText("You will be signed out on all other devices."),
      ).not.toBeInTheDocument();
    });
    expect(mockMutateAsync).not.toHaveBeenCalled();
    expect(mockToastSuccess).not.toHaveBeenCalled();
  });

  it("calls revoke mutation and closes dialog on success with toast", async () => {
    renderAccountTab();

    fireEvent.click(
      screen.getByRole("button", { name: "Sign out of all devices" }),
    );

    const confirmButton = screen.getByRole("button", { name: "Sign out" });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith("Signed out of all devices");
      expect(
        screen.queryByText("You will be signed out on all other devices."),
      ).not.toBeInTheDocument();
    });
  });

  it("shows error toast and keeps dialog open on failure", async () => {
    mockMutateAsync.mockRejectedValueOnce(new Error("Revocation failed"));
    renderAccountTab();

    fireEvent.click(
      screen.getByRole("button", { name: "Sign out of all devices" }),
    );

    const confirmButton = screen.getByRole("button", { name: "Sign out" });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1);
      expect(mockToastError).toHaveBeenCalledWith("Revocation failed");
    });

    // Dialog stays open
    expect(
      screen.getByText("You will be signed out on all other devices."),
    ).toBeInTheDocument();
  });

  it("disables buttons and shows busy state when isPending is true", () => {
    revokeState.isPending = true;
    renderAccountTab();

    fireEvent.click(
      screen.getByRole("button", { name: "Sign out of all devices" }),
    );

    const confirmButton = screen.getByRole("button", { name: "Signing out..." });
    expect(confirmButton).toBeDisabled();
    expect(confirmButton).toHaveAttribute("aria-busy", "true");

    const cancelButton = screen.getByRole("button", { name: "Cancel" });
    expect(cancelButton).toBeDisabled();
  });
});
