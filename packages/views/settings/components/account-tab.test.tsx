// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";

const mockMutateAsync = vi.hoisted(() => vi.fn());
const mockDownloadAccountExport = vi.hoisted(() => vi.fn());
const mockSaveBlob = vi.hoisted(() => vi.fn());
const revokeState = vi.hoisted(() => ({
  isPending: false,
}));
const mockPreviewMutateAsync = vi.hoisted(() => vi.fn());
const mockRequestMutateAsync = vi.hoisted(() => vi.fn());
const deleteState = vi.hoisted(() => ({
  isDeleting: false,
}));
const mockLogout = vi.hoisted(() => vi.fn());

vi.mock("../../auth", () => ({
  useLogout: () => mockLogout,
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
  downloadAccountExport: mockDownloadAccountExport,
  usePreviewAccountDeletion: () => ({
    mutateAsync: mockPreviewMutateAsync,
  }),
  useRequestAccountDeletion: () => ({
    mutateAsync: mockRequestMutateAsync,
    isPending: deleteState.isDeleting,
  }),
}));

vi.mock("../../governance/save-blob", () => ({
  saveBlob: mockSaveBlob,
}));

const mockSetUser = vi.hoisted(() => vi.fn());
const currentUser = vi.hoisted(() => ({
  id: "user-1",
  name: "Jane Doe",
  email: "jane@example.com",
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
    deleteState.isDeleting = false;
    mockPreviewMutateAsync.mockResolvedValue({
      sole_workspaces: [],
      scheduled_at: null,
    });
    mockRequestMutateAsync.mockResolvedValue({
      scheduled_at: "2026-11-29T10:00:00Z",
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

  it("renders the download my data row with title, description, and button", () => {
    renderAccountTab();

    expect(screen.getByText("Download my data")).toBeInTheDocument();
    expect(
      screen.getByText("Available once per hour in JSON format."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Download" }),
    ).toBeInTheDocument();
  });

  it("calls downloadAccountExport, saves the file, and shows success toast on button click", async () => {
    mockDownloadAccountExport.mockResolvedValueOnce({
      profile: { id: "user-1", name: "Jane Doe" },
    });
    renderAccountTab();

    const downloadButton = screen.getByRole("button", { name: "Download" });
    fireEvent.click(downloadButton);

    await waitFor(() => {
      expect(mockDownloadAccountExport).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(mockSaveBlob).toHaveBeenCalledTimes(1);
      const [blob, filename] = mockSaveBlob.mock.calls[0] as [Blob, string];
      expect(filename).toBe("multica-account-export.json");
      expect(blob).toBeInstanceOf(Blob);
      expect(mockToastSuccess).toHaveBeenCalledWith("Account data downloaded");
    });
  });

  it("shows rate-limited error toast when export returns 429", async () => {
    mockDownloadAccountExport.mockRejectedValueOnce({
      code: "export_rate_limited",
      status: 429,
    });
    renderAccountTab();

    const downloadButton = screen.getByRole("button", { name: "Download" });
    fireEvent.click(downloadButton);

    await waitFor(() => {
      expect(mockDownloadAccountExport).toHaveBeenCalledTimes(1);
      expect(mockToastError).toHaveBeenCalledWith(
        "Account data can be downloaded once an hour",
      );
    });
    expect(mockSaveBlob).not.toHaveBeenCalled();
  });

  it("shows general error toast when export fails", async () => {
    mockDownloadAccountExport.mockRejectedValueOnce(
      new Error("Failed to export"),
    );
    renderAccountTab();

    const downloadButton = screen.getByRole("button", { name: "Download" });
    fireEvent.click(downloadButton);

    await waitFor(() => {
      expect(mockDownloadAccountExport).toHaveBeenCalledTimes(1);
      expect(mockToastError).toHaveBeenCalledWith("Failed to export");
    });
    expect(mockSaveBlob).not.toHaveBeenCalled();
  });

  it("renders the delete account section with title, description, and button", () => {
    renderAccountTab();

    expect(
      screen.getByRole("heading", { name: "Delete account" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Permanently delete your account and personal data after a 14-day grace period.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Delete account" }),
    ).toBeInTheDocument();
  });

  it("opens delete dialog and calls preview mutation on button click", async () => {
    renderAccountTab();

    const openButton = screen.getByRole("button", { name: "Delete account" });
    fireEvent.click(openButton);

    await waitFor(() => {
      expect(mockPreviewMutateAsync).toHaveBeenCalledTimes(1);
    });

    expect(
      screen.getByText(
        "Deletion takes effect after 14 days. During this time, signing in is only to cancel.",
      ),
    ).toBeInTheDocument();
  });

  it("lists sole-member workspaces to be deleted when preview returns them", async () => {
    mockPreviewMutateAsync.mockResolvedValueOnce({
      sole_workspaces: [
        { id: "ws-1", name: "Solo Workspace Alpha" },
        { id: "ws-2", name: "Solo Workspace Beta" },
      ],
      scheduled_at: null,
    });
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "The following workspaces where you are the only member will be deleted:",
        ),
      ).toBeInTheDocument();
      expect(screen.getByText("Solo Workspace Alpha")).toBeInTheDocument();
      expect(screen.getByText("Solo Workspace Beta")).toBeInTheDocument();
    });
  });

  it("shows blocked view with workspace list and no confirm button when preview is blocked by sole_owner", async () => {
    mockPreviewMutateAsync.mockRejectedValueOnce({
      code: "sole_owner",
      workspaces: [{ id: "ws-team", name: "Team Org", member_count: 5 }],
    });
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Cannot delete account" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          "You are the only owner of workspaces with other members. Transfer ownership or delete these workspaces first:",
        ),
      ).toBeInTheDocument();
      expect(screen.getByText("Team Org")).toBeInTheDocument();
    });

    expect(
      screen.getAllByRole("button", { name: "Close" }).length,
    ).toBeGreaterThanOrEqual(1);
    expect(
      screen.queryByRole("button", { name: "Delete account" }),
    ).not.toBeInTheDocument();
  });

  it("shows recent login required view and offers sign-out action", async () => {
    mockPreviewMutateAsync.mockRejectedValueOnce({
      code: "recent_login_required",
    });
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Sign-in required" }),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          "To confirm account deletion, you must sign in again.",
        ),
      ).toBeInTheDocument();
    });

    const signOutBtn = screen.getByRole("button", { name: "Sign out" });
    expect(signOutBtn).toBeInTheDocument();
    fireEvent.click(signOutBtn);
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });

  it("disables confirm button until typed email matches user email exactly", async () => {
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByLabelText(/To confirm, enter your email address/i),
      ).toBeInTheDocument();
    });

    // In the dialog, the confirm button has name "Delete account"
    const buttons = screen.getAllByRole("button", { name: "Delete account" });
    const confirmButton = buttons[buttons.length - 1];
    expect(confirmButton).toBeDisabled();

    const input = screen.getByLabelText(/To confirm, enter your email address/i);
    fireEvent.change(input, { target: { value: "wrong@example.com" } });
    expect(confirmButton).toBeDisabled();

    fireEvent.change(input, { target: { value: "jane@example.com" } });
    expect(confirmButton).not.toBeDisabled();
  });

  it("requests deletion on confirm, displays scheduled date and signs out", async () => {
    mockRequestMutateAsync.mockResolvedValueOnce({
      scheduled_at: "2026-11-29T10:00:00Z",
    });
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByLabelText(/To confirm, enter your email address/i),
      ).toBeInTheDocument();
    });

    const input = screen.getByLabelText(/To confirm, enter your email address/i);
    fireEvent.change(input, { target: { value: "jane@example.com" } });

    const buttons = screen.getAllByRole("button", { name: "Delete account" });
    const confirmButton = buttons[buttons.length - 1];
    expect(confirmButton).toBeDefined();
    fireEvent.click(confirmButton!);

    await waitFor(() => {
      expect(mockRequestMutateAsync).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(
        screen.getByRole("heading", {
          name: "Account scheduled for deletion",
        }),
      ).toBeInTheDocument();
      expect(
        screen.getByText(/Your account will be deleted on/i),
      ).toBeInTheDocument();
      expect(screen.getByText(/You have been signed out\./i)).toBeInTheDocument();
    });

    const signOutBtn = screen.getByRole("button", { name: "Sign out" });
    fireEvent.click(signOutBtn);
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });

  it("handles recent_login_required during deletion request", async () => {
    mockRequestMutateAsync.mockRejectedValueOnce({
      code: "recent_login_required",
    });
    renderAccountTab();

    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));

    await waitFor(() => {
      expect(
        screen.getByLabelText(/To confirm, enter your email address/i),
      ).toBeInTheDocument();
    });

    const input = screen.getByLabelText(/To confirm, enter your email address/i);
    fireEvent.change(input, { target: { value: "jane@example.com" } });

    const buttons = screen.getAllByRole("button", { name: "Delete account" });
    const confirmButton = buttons[buttons.length - 1];
    expect(confirmButton).toBeDefined();
    fireEvent.click(confirmButton!);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Sign-in required" }),
      ).toBeInTheDocument();
    });
  });
});
