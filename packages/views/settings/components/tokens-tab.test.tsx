import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../../test/i18n";

const mockApi = vi.hoisted(() => ({
  listPersonalAccessTokens: vi.fn(),
  createPersonalAccessToken: vi.fn(),
  revokePersonalAccessToken: vi.fn(),
  listWorkspaces: vi.fn(),
}));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: mockApi,
}));

vi.mock("@multica/ui/lib/clipboard", () => ({
  copyText: vi.fn().mockResolvedValue(true),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { TokensTab } from "./tokens-tab";

function renderTokensTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <TokensTab />
    </QueryClientProvider>,
  );
}

describe("TokensTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApi.listWorkspaces.mockResolvedValue([
      { id: "ws-1", name: "Acme", slug: "acme", description: null, context: null, settings: {}, repos: [], issue_prefix: "ACM", avatar_url: null, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
      { id: "ws-2", name: "Beta", slug: "beta", description: null, context: null, settings: {}, repos: [], issue_prefix: "BET", avatar_url: null, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
    ]);
    mockApi.listPersonalAccessTokens.mockResolvedValue([
      {
        id: "tok-1",
        name: "Deploy CI",
        token_prefix: "mul_abc",
        expires_at: null,
        last_used_at: null,
        created_at: "2026-01-02T00:00:00Z",
        workspace_id: "ws-1",
        read_only: true,
      },
      {
        id: "tok-2",
        name: "Local CLI",
        token_prefix: "mul_def",
        expires_at: null,
        last_used_at: null,
        created_at: "2026-01-02T00:00:00Z",
        workspace_id: null,
        read_only: false,
      },
    ]);
    mockApi.createPersonalAccessToken.mockResolvedValue({
      id: "tok-3",
      name: "New Token",
      token_prefix: "mul_new",
      expires_at: null,
      last_used_at: null,
      created_at: "2026-01-03T00:00:00Z",
      token: "mul_secret_value",
    });
  });

  it("shows bound workspace name and read-only badge in the token list", async () => {
    renderTokensTab();

    await waitFor(() => {
      expect(screen.getByText("Deploy CI")).toBeInTheDocument();
    });

    expect(screen.getByText("Acme")).toBeInTheDocument();
    expect(screen.getAllByText("Read-only")).toHaveLength(2); // Switch label + badge
  });

  it("sends neither workspace_id nor read_only when left at defaults", async () => {
    const user = userEvent.setup();
    renderTokensTab();

    await waitFor(() => {
      expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveTextContent(
        "Any workspace",
      );
    });

    await user.type(screen.getByPlaceholderText(/Token name/i), "Default CLI");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockApi.createPersonalAccessToken).toHaveBeenCalledTimes(1);
    });
    const payload = mockApi.createPersonalAccessToken.mock.calls[0]![0];
    expect(payload).toEqual({ name: "Default CLI", expires_in_days: 90 });
    expect("workspace_id" in payload).toBe(false);
    expect("read_only" in payload).toBe(false);
  });

  it("sends workspace_id and read_only: true when selected", async () => {
    const user = userEvent.setup();
    renderTokensTab();

    await waitFor(() => {
      expect(screen.getByRole("combobox", { name: "Workspace" })).toBeInTheDocument();
    });

    await user.type(screen.getByPlaceholderText(/Token name/i), "Scoped Reader");
    await user.click(screen.getByRole("combobox", { name: "Workspace" }));
    await user.click(await screen.findByRole("option", { name: "Beta" }));
    await user.click(screen.getByRole("switch", { name: "Read-only" }));
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockApi.createPersonalAccessToken).toHaveBeenCalledWith({
        name: "Scoped Reader",
        expires_in_days: 90,
        workspace_id: "ws-2",
        read_only: true,
      });
    });
  });
});
