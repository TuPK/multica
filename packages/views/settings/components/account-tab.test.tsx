// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";

const mockMutateAsync = vi.hoisted(() => vi.fn());
const revokeState = vi.hoisted(() => ({
  isPending: false,
}));

const mockSecurityEventsState = vi.hoisted(() => ({
  events: [
    {
      id: "sec-1",
      workspace_id: null,
      actor_type: "member",
      actor_id: "user-1",
      actor_label: "Jane Doe",
      action: "auth.login",
      target_type: "",
      target_id: "",
      target_label: "",
      detail: {},
      ip: "127.0.0.1",
      user_agent: "Mozilla",
      request_id: "req-1",
      created_at: "2026-11-15T10:00:00Z",
    },
    {
      id: "sec-2",
      workspace_id: null,
      actor_type: "member",
      actor_id: "user-1",
      actor_label: "Jane Doe",
      action: "pat.created",
      target_type: "pat",
      target_id: "pat-1",
      target_label: "CLI token",
      detail: {},
      ip: "127.0.0.1",
      user_agent: "Mozilla",
      request_id: "req-2",
      created_at: "2026-11-14T10:00:00Z",
    },
  ],
  isError: false,
}));

vi.mock("@multica/core/audit", () => ({
  securityEventsOptions: (userId: string, limit?: number) => ({
    queryKey: ["security-events", userId, limit],
    queryFn: () => {
      if (mockSecurityEventsState.isError) {
        return Promise.reject(new Error("Failed to load security events"));
      }
      return Promise.resolve({
        events: mockSecurityEventsState.events,
        next_cursor: null,
      });
    },
  }),
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
    mockSecurityEventsState.isError = false;
    mockSecurityEventsState.events = [
      {
        id: "sec-1",
        workspace_id: null,
        actor_type: "member",
        actor_id: "user-1",
        actor_label: "Jane Doe",
        action: "auth.login",
        target_type: "",
        target_id: "",
        target_label: "",
        detail: {},
        ip: "127.0.0.1",
        user_agent: "Mozilla",
        request_id: "req-1",
        created_at: "2026-11-15T10:00:00Z",
      },
      {
        id: "sec-2",
        workspace_id: null,
        actor_type: "member",
        actor_id: "user-1",
        actor_label: "Jane Doe",
        action: "pat.created",
        target_type: "pat",
        target_id: "pat-1",
        target_label: "CLI token",
        detail: {},
        ip: "127.0.0.1",
        user_agent: "Mozilla",
        request_id: "req-2",
        created_at: "2026-11-14T10:00:00Z",
      },
    ];
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

  it("renders the security events section with title and events", async () => {
    renderAccountTab();

    expect(screen.getByText("Security events")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("auth.login")).toBeInTheDocument();
      expect(screen.getByText("pat.created")).toBeInTheDocument();
    });
  });

  it("renders empty state when there are no security events", async () => {
    mockSecurityEventsState.events = [];
    renderAccountTab();

    expect(screen.getByText("Security events")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("No security events")).toBeInTheDocument();
    });
  });

  it("renders error state when security events query fails", async () => {
    mockSecurityEventsState.isError = true;
    renderAccountTab();

    expect(screen.getByText("Security events")).toBeInTheDocument();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load security events"),
      ).toBeInTheDocument();
    });
  });
});
