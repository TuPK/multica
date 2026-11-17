import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";

const mockApi = vi.hoisted(() => ({
  getWorkspacePolicy: vi.fn(),
  updateWorkspacePolicy: vi.fn(),
  listSSODomains: vi.fn(),
  listMembers: vi.fn(),
}));

const memberState = vi.hoisted(() => ({
  role: "owner" as "owner" | "admin" | "member" | "viewer",
  isLoading: false,
}));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: mockApi,
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme", slug: "acme" }),
}));

vi.mock("@multica/core/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/permissions")>()),
  useCurrentMember: () => ({
    role: memberState.role,
    isLoading: memberState.isLoading,
  }),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { PolicyTab } from "./policy-tab";

const basePolicy = {
  allowed_invite_domains: [] as string[],
  share_links_enabled: true,
  require_sso: false,
  require_mfa: false,
  session_max_age_hours: 0,
  updated_by: null,
  updated_at: null,
};

function renderPolicyTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <PolicyTab />
    </QueryClientProvider>,
  );
}

describe("PolicyTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    memberState.role = "owner";
    memberState.isLoading = false;
    mockApi.getWorkspacePolicy.mockResolvedValue(basePolicy);
    mockApi.updateWorkspacePolicy.mockImplementation(async (_wsId: string, patch: Record<string, unknown>) => ({
      ...basePolicy,
      ...patch,
    }));
    mockApi.listSSODomains.mockResolvedValue({
      domains: [
        {
          domain: "acme.com",
          txt_name: "_multica.acme.com",
          txt_value: "token-1",
          verified_at: "2026-01-01T00:00:00Z",
        },
        {
          domain: "pending.acme.com",
          txt_name: "_multica.pending.acme.com",
          txt_value: "token-2",
          verified_at: null,
        },
      ],
    });
    mockApi.listMembers.mockResolvedValue([
      { id: "m-1", user_id: "u-1", workspace_id: "ws-1", role: "owner", name: "Ada Lovelace", email: "ada@acme.com", avatar_url: null, created_at: "2026-01-01T00:00:00Z" },
    ]);
  });

  it("renders policy controls with one short consequence line each", async () => {
    renderPolicyTab();

    await waitFor(() => {
      expect(screen.getByRole("switch", { name: /Share links/i })).toBeInTheDocument();
    });

    expect(screen.getByText(/Existing share links stop working/i)).toBeInTheDocument();
    expect(screen.getByText(/Blocks members without two-step sign-in/i)).toBeInTheDocument();
    expect(screen.getByText(/must sign in via SSO; owners are exempt/i)).toBeInTheDocument();
    expect(screen.getByText(/0 turns the limit off/i)).toBeInTheDocument();
    expect(screen.getByText(/Invitations and share links reject other email domains/i)).toBeInTheDocument();
  });

  it("lists verified domains, allows adding/removing verified ones, and rejects unverified domains", async () => {
    const user = userEvent.setup();
    renderPolicyTab();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "acme.com" })).toBeInTheDocument();
    });
    // Unverified domain must not appear in the verified list.
    expect(screen.queryByRole("button", { name: "pending.acme.com" })).toBeNull();

    // Typing an unverified domain shows a clear rejection message.
    const domainInput = screen.getByRole("textbox", { name: /Allowed invite domains/i });
    await user.type(domainInput, "gmail.com");
    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(screen.getByText(/gmail\.com is not a verified domain/i)).toBeInTheDocument();
    expect(mockApi.updateWorkspacePolicy).not.toHaveBeenCalled();

    // Selecting a verified domain adds a row and enables saving.
    await user.click(screen.getByRole("button", { name: "acme.com" }));
    expect(screen.getByRole("button", { name: "Remove acme.com" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(mockApi.updateWorkspacePolicy).toHaveBeenCalledWith("ws-1", {
        allowed_invite_domains: ["acme.com"],
      });
    });
  });

  it("sends only the changed fields when saving", async () => {
    const user = userEvent.setup();
    renderPolicyTab();

    const mfaSwitch = await screen.findByRole("switch", { name: /Require two-step sign-in/i });
    await user.click(mfaSwitch);

    const ageInput = screen.getByRole("spinbutton", { name: /Maximum session age/i });
    fireEvent.change(ageInput, { target: { value: "24" } });

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockApi.updateWorkspacePolicy).toHaveBeenCalledWith("ws-1", {
        require_mfa: true,
        session_max_age_hours: 24,
      });
    });
  });

  it("validates session max age bounds (0-8760)", async () => {
    renderPolicyTab();

    const ageInput = await screen.findByRole("spinbutton", { name: /Maximum session age/i });
    fireEvent.change(ageInput, { target: { value: "9000" } });

    expect(screen.getByText(/between 0 and 8760/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(mockApi.updateWorkspacePolicy).not.toHaveBeenCalled();
  });

  it("shows plan_feature_unavailable (402) and disables controls", async () => {
    mockApi.updateWorkspacePolicy.mockRejectedValue(
      new ApiError("not included", 402, "Payment Required", {
        code: "plan_feature_unavailable",
      }),
    );
    const user = userEvent.setup();
    renderPolicyTab();

    const shareSwitch = await screen.findByRole("switch", { name: /Share links/i });
    await user.click(shareSwitch);
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        /not available on your current plan/i,
      );
    });
    expect(screen.getByRole("switch", { name: /Share links/i })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("switch", { name: /Require two-step sign-in/i })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("switch", { name: /Require SSO/i })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("spinbutton", { name: /Maximum session age/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it.each([
    ["domain_not_verified", 400, /verified by this workspace/i],
    ["invite_domain_not_allowed", 403, /not allowed by workspace policy/i],
    ["share_links_disabled", 403, /disabled by workspace policy/i],
    ["viewer_read_only", 403, /Only workspace owners/i],
    ["something_unknown", 500, /Could not save workspace policy/i],
  ])("maps %s error code on save", async (code, status, pattern) => {
    mockApi.updateWorkspacePolicy.mockRejectedValue(
      new ApiError("err", status, "Error", { code }),
    );
    const user = userEvent.setup();
    renderPolicyTab();

    await user.click(await screen.findByRole("switch", { name: /Require SSO/i }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(pattern);
    });
  });

  it("renders read-only for admins without calling owner-only sso domains", async () => {
    memberState.role = "admin";
    mockApi.getWorkspacePolicy.mockResolvedValue({
      ...basePolicy,
      require_mfa: true,
      allowed_invite_domains: ["acme.com"],
    });
    renderPolicyTab();

    const mfaSwitch = await screen.findByRole("switch", {
      name: /Require two-step sign-in/i,
    });
    expect(screen.getByRole("note")).toBeInTheDocument();
    expect(mockApi.getWorkspacePolicy).toHaveBeenCalledWith("ws-1");
    expect(mockApi.listSSODomains).not.toHaveBeenCalled();
    expect(mfaSwitch).toHaveAttribute("aria-disabled", "true");
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.getByText("acme.com")).toBeInTheDocument();
  });

  it("renders read-only for members without fetching policy or domains", async () => {
    memberState.role = "member";
    renderPolicyTab();

    await waitFor(() => {
      expect(screen.getByRole("note")).toBeInTheDocument();
    });
    expect(mockApi.getWorkspacePolicy).not.toHaveBeenCalled();
    expect(mockApi.listSSODomains).not.toHaveBeenCalled();
    expect(screen.getByRole("switch", { name: /Share links/i })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });
});
