// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";
import { ApiError } from "@multica/core/api";
import type { PublishChannel } from "@multica/core/types";

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

function renderTab() {
  const qc = createTestQueryClient();
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <PublishChannelsTab />
    </QueryClientProvider>,
  );
}

const mockCreate = vi.hoisted(() => vi.fn());
const mockDelete = vi.hoisted(() => vi.fn());
const mockConnect = vi.hoisted(() => vi.fn());
const mockCheck = vi.hoisted(() => vi.fn());
const mockDryRun = vi.hoisted(() => vi.fn());
const mockSaveSettings = vi.hoisted(() => vi.fn());

const testState = vi.hoisted(() => ({
  channels: [] as PublishChannel[],
  isLoading: false,
  role: "admin" as "owner" | "admin" | "member",
  settings: {
    ayrshare_configured: false,
    x_configured: false,
  },
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-test-1",
}));

vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ member: { role: testState.role } }),
}));

vi.mock("@multica/core/publish-channels", () => ({
  usePublishChannels: () => ({
    data: testState.channels,
    isLoading: testState.isLoading,
  }),
  useCreatePublishChannel: () => ({
    mutateAsync: mockCreate,
    isPending: false,
  }),
  useDeletePublishChannel: () => ({
    mutateAsync: mockDelete,
    isPending: false,
  }),
  useConnectPublishChannel: () => ({
    mutateAsync: mockConnect,
    isPending: false,
  }),
  useCheckPublishChannel: () => ({
    mutateAsync: mockCheck,
    isPending: false,
  }),
  useDryRunPublishChannel: () => ({
    mutateAsync: mockDryRun,
    isPending: false,
  }),
  usePublishSettings: () => ({
    data: testState.settings,
    isLoading: false,
  }),
  useSavePublishSettings: () => ({
    mutateAsync: mockSaveSettings,
    isPending: false,
  }),
}));

const toastMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("sonner", () => ({ toast: toastMock }));

import { PublishChannelsTab } from "./publish-channels-tab";

describe("PublishChannelsTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    testState.role = "admin";
    testState.channels = [];
    testState.isLoading = false;
    testState.settings = {
      ayrshare_configured: false,
      x_configured: false,
    };
  });

  it("renders the empty state when no channels exist", () => {
    testState.channels = [];
    renderTab();

    expect(screen.getByText("No publishing channels")).toBeInTheDocument();
    expect(
      screen.getByText("Connect social channels for the Content Studio squad."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add channel" })).toBeInTheDocument();
  });

  it("renders channels in all contract states: ready, not_connected, error, expired", () => {
    testState.channels = [
      {
        id: "pc-ready",
        platform: "x",
        market: "intl",
        account_name: "@multica_hq",
        status: "ready",
        status_detail: "",
        last_checked_at: "2026-11-12T08:00:00Z",
        limits: { max_chars: 280, max_video_seconds: 140, aspect: "16:9" },
      },
      {
        id: "pc-not-connected",
        platform: "linkedin",
        market: "intl",
        account_name: "Multica Inc",
        status: "not_connected",
        status_detail: "",
        last_checked_at: null,
        limits: { max_chars: 3000, max_video_seconds: 600, aspect: "1:1" },
      },
      {
        id: "pc-error",
        platform: "facebook",
        market: "vn",
        account_name: "Multica Vietnam",
        status: "error",
        status_detail: "Token revoked or permissions changed",
        last_checked_at: "2026-11-12T07:00:00Z",
        limits: { max_chars: 63206, max_video_seconds: 14400, aspect: "16:9" },
      },
      {
        id: "pc-expired",
        platform: "threads",
        market: "vn",
        account_name: "multica_vn",
        status: "expired",
        status_detail: "OAuth session expired. Reconnect required.",
        last_checked_at: "2026-11-11T12:00:00Z",
        limits: { max_chars: 500, max_video_seconds: 300, aspect: "1:1" },
      },
    ];

    renderTab();

    // Check account names
    expect(screen.getByText("@multica_hq")).toBeInTheDocument();
    expect(screen.getByText("Multica Inc")).toBeInTheDocument();
    expect(screen.getByText("Multica Vietnam")).toBeInTheDocument();
    expect(screen.getByText("multica_vn")).toBeInTheDocument();

    // Check state badges
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.getByText("Expired")).toBeInTheDocument();

    // Check status detail error messages
    expect(
      screen.getByText("Token revoked or permissions changed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("OAuth session expired. Reconnect required."),
    ).toBeInTheDocument();
  });

  it("handles adding a new channel", async () => {
    testState.channels = [];
    mockCreate.mockResolvedValueOnce({
      id: "pc-new",
      platform: "x",
      market: "intl",
      account_name: "@new_account",
      status: "not_connected",
    });

    renderTab();

    // Click "Add channel"
    fireEvent.click(screen.getByRole("button", { name: "Add channel" }));

    // Dialog opens
    expect(screen.getByRole("heading", { name: "Add publishing channel" })).toBeInTheDocument();

    // Type account name
    const accountInput = screen.getByPlaceholderText("@username or page name");
    await userEvent.type(accountInput, "@new_account");

    // Click submit in dialog
    const submitBtn = screen.getByRole("button", { name: "Add channel" });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalledWith({
        platform: "x",
        market: "intl",
        account_name: "@new_account",
      });
    });
  });

  it("handles connecting a channel and opens link in new tab", async () => {
    testState.channels = [
      {
        id: "pc-1",
        platform: "linkedin",
        market: "intl",
        account_name: "Multica Inc",
        status: "not_connected",
        status_detail: "",
        last_checked_at: null,
        limits: { max_chars: 3000, max_video_seconds: 600, aspect: "1:1" },
      },
    ];

    const windowOpenSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    mockConnect.mockResolvedValueOnce({ url: "https://app.ayrshare.com/connect/session_123" });

    renderTab();

    const connectBtn = screen.getByRole("button", { name: "Connect" });
    fireEvent.click(connectBtn);

    await waitFor(() => {
      expect(mockConnect).toHaveBeenCalledWith("pc-1");
      expect(windowOpenSpy).toHaveBeenCalledWith(
        "https://app.ayrshare.com/connect/session_123",
        "_blank",
        "noopener,noreferrer",
      );
    });

    windowOpenSpy.mockRestore();
  });

  it("handles checking status of a channel", async () => {
    testState.channels = [
      {
        id: "pc-1",
        platform: "x",
        market: "intl",
        account_name: "@multica_hq",
        status: "ready",
        status_detail: "",
        last_checked_at: null,
        limits: { max_chars: 280, max_video_seconds: 140, aspect: "16:9" },
      },
    ];

    mockCheck.mockResolvedValueOnce({
      id: "pc-1",
      status: "ready",
      last_checked_at: "2026-11-12T10:00:00Z",
    });

    renderTab();

    const checkBtn = screen.getByRole("button", { name: "Check status" });
    fireEvent.click(checkBtn);

    await waitFor(() => {
      expect(mockCheck).toHaveBeenCalledWith("pc-1");
    });
  });

  it("handles dry-run: opens modal, executes, and renders JSON body output", async () => {
    testState.channels = [
      {
        id: "pc-1",
        platform: "x",
        market: "intl",
        account_name: "@multica_hq",
        status: "ready",
        status_detail: "",
        last_checked_at: null,
        limits: { max_chars: 280, max_video_seconds: 140, aspect: "16:9" },
      },
    ];

    mockDryRun.mockResolvedValueOnce({
      post: "Multica dry-run test body",
      platforms: ["x"],
    });

    renderTab();

    // Click "Dry-run"
    const dryRunBtn = screen.getByRole("button", { name: "Dry-run" });
    fireEvent.click(dryRunBtn);

    // Modal opens
    expect(screen.getByRole("heading", { name: "Dry-run post" })).toBeInTheDocument();

    // Click "Generate payload"
    const generateBtn = screen.getByRole("button", { name: "Generate payload" });
    fireEvent.click(generateBtn);

    await waitFor(() => {
      expect(mockDryRun).toHaveBeenCalledWith({
        id: "pc-1",
        text: expect.any(String),
        media_url: undefined,
      });
      const output = screen.getByTestId("dry-run-json-output");
      expect(output).toBeInTheDocument();
      expect(output.textContent).toContain("Multica dry-run test body");
    });
  });

  it("handles deleting a channel with confirmation dialog", async () => {
    testState.channels = [
      {
        id: "pc-1",
        platform: "x",
        market: "intl",
        account_name: "@multica_hq",
        status: "ready",
        status_detail: "",
        last_checked_at: null,
        limits: { max_chars: 280, max_video_seconds: 140, aspect: "16:9" },
      },
    ];

    mockDelete.mockResolvedValueOnce(undefined);

    renderTab();

    // Click delete icon button
    const deleteBtn = screen.getByRole("button", { name: "Delete" });
    fireEvent.click(deleteBtn);

    // Confirmation AlertDialog opens
    expect(
      screen.getByRole("heading", { name: "Delete publishing channel?" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Removes the channel from Multica. The account stays linked on its network until you unlink it there.",
      ),
    ).toBeInTheDocument();

    // Click Confirm Delete in AlertDialog
    const confirmDeleteBtn = screen.getByRole("button", { name: "Delete channel" });
    fireEvent.click(confirmDeleteBtn);

    await waitFor(() => {
      expect(mockDelete).toHaveBeenCalledWith("pc-1");
    });
  });

  it("names the platform when a channel for it already exists", async () => {
    testState.channels = [];
    mockCreate.mockRejectedValueOnce(
      new ApiError("conflict", 409, "Conflict", {
        code: "platform_taken",
        error: "this workspace already has a channel on that platform",
      }),
    );

    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Add channel" }));
    await userEvent.type(screen.getByPlaceholderText("@username or page name"), "@dup");
    fireEvent.click(screen.getByRole("button", { name: "Add channel" }));

    await waitFor(() => {
      expect(toastMock.error).toHaveBeenCalledWith(
        "This workspace already has a channel on that platform.",
      );
    });
  });

  describe("Ayrshare settings form", () => {
    it("empty state shows 'Not set' for both groups", () => {
      testState.settings = {
        ayrshare_configured: false,
        x_configured: false,
      };
      renderTab();

      const notSetBadges = screen.getAllByText("Not set");
      expect(notSetBadges).toHaveLength(2);
    });

    it("configured state shows 'Connected'", () => {
      testState.settings = {
        ayrshare_configured: true,
        x_configured: true,
      };
      renderTab();

      const connectedBadges = screen.getAllByText("Connected");
      expect(connectedBadges).toHaveLength(2);
    });

    it("saving calls useSavePublishSettings with only the filled fields", async () => {
      mockSaveSettings.mockResolvedValueOnce({
        ayrshare_configured: true,
        x_configured: false,
      });
      renderTab();

      const ayrInput = screen.getByPlaceholderText("Paste the Ayrshare API key");
      await userEvent.type(ayrInput, "ayr_test_key_123");

      const saveBtn = screen.getByRole("button", { name: "Save" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(mockSaveSettings).toHaveBeenCalledWith({
          ayrshare_api_key: "ayr_test_key_123",
        });
      });

      // Field cleared after save
      await waitFor(() => {
        expect(ayrInput).toHaveValue("");
      });
      expect(toastMock.success).toHaveBeenCalledWith("Publishing keys saved");
    });

    it("saving calls useSavePublishSettings with only filled fields when X is expanded", async () => {
      mockSaveSettings.mockResolvedValueOnce({
        ayrshare_configured: false,
        x_configured: true,
      });
      renderTab();

      // Expand X (optional) section
      const expandBtn = screen.getByRole("button", { name: /X \(optional\)/ });
      fireEvent.click(expandBtn);

      const xKeyInput = screen.getByPlaceholderText("Paste the X API key");
      const xSecretInput = screen.getByPlaceholderText("Paste the X API secret");

      await userEvent.type(xKeyInput, "x_key_abc");
      await userEvent.type(xSecretInput, "x_secret_xyz");

      const saveBtn = screen.getByRole("button", { name: "Save" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(mockSaveSettings).toHaveBeenCalledWith({
          x_api_key: "x_key_abc",
          x_api_secret: "x_secret_xyz",
        });
      });

      await waitFor(() => {
        expect(xKeyInput).toHaveValue("");
        expect(xSecretInput).toHaveValue("");
      });
    });

    it("a 403 error shows the message and does not clear the form", async () => {
      mockSaveSettings.mockRejectedValueOnce(
        new ApiError(
          "only an owner or admin may manage publishing channels",
          403,
          "Forbidden",
          { error: "only an owner or admin may manage publishing channels" },
        ),
      );
      renderTab();

      const ayrInput = screen.getByPlaceholderText("Paste the Ayrshare API key");
      await userEvent.type(ayrInput, "forbidden_secret_key");

      const saveBtn = screen.getByRole("button", { name: "Save" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(toastMock.error).toHaveBeenCalledWith(
          "only an owner or admin may manage publishing channels",
        );
      });

      expect(
        screen.getByText("only an owner or admin may manage publishing channels"),
      ).toBeInTheDocument();

      // Form input must not be cleared
      expect(ayrInput).toHaveValue("forbidden_secret_key");
    });
  });
});
