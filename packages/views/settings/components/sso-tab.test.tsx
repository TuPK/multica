import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";

const mockApi = vi.hoisted(() => ({
  listSSODomains: vi.fn(),
  addSSODomain: vi.fn(),
  verifySSODomain: vi.fn(),
  removeSSODomain: vi.fn(),
  getSSOConnection: vi.fn(),
  putSSOConnection: vi.fn(),
  deleteSSOConnection: vi.fn(),
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

vi.mock("@multica/ui/lib/clipboard", () => ({
  copyText: vi.fn().mockResolvedValue(true),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { SSOTab } from "./sso-tab";

function renderSSOTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <SSOTab />
    </QueryClientProvider>,
  );
}

const unverifiedDomain = {
  domain: "acme.com",
  txt_name: "_multica.acme.com",
  txt_value: "multica-verify=abc123",
  verified_at: null,
};

const verifiedDomain = {
  ...unverifiedDomain,
  verified_at: "2026-01-01T00:00:00Z",
};

const existingConnection = {
  id: "conn-1",
  workspace_id: "ws-1",
  issuer: "https://idp.acme.com",
  client_id: "client-123",
  client_secret_set: true,
  default_role: "member",
  status: "active",
  redirect_uri: "https://app.multica.ai/auth/sso/callback",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("SSOTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    memberState.role = "owner";
    memberState.isLoading = false;
    mockApi.listSSODomains.mockResolvedValue({ domains: [] });
    mockApi.addSSODomain.mockResolvedValue(unverifiedDomain);
    mockApi.verifySSODomain.mockResolvedValue(verifiedDomain);
    mockApi.removeSSODomain.mockResolvedValue(undefined);
    mockApi.getSSOConnection.mockResolvedValue(null);
    mockApi.putSSOConnection.mockResolvedValue(existingConnection);
    mockApi.deleteSSOConnection.mockResolvedValue(undefined);
    mockApi.listMembers.mockResolvedValue([
      { id: "m-1", user_id: "u-1", workspace_id: "ws-1", role: "owner", name: "Ada Lovelace", email: "ada@acme.com", avatar_url: null, created_at: "2026-01-01T00:00:00Z" },
    ]);
  });

  it("gates non-owners without issuing SSO API calls", async () => {
    memberState.role = "admin";
    renderSSOTab();

    await waitFor(() => {
      expect(screen.getByRole("note")).toBeInTheDocument();
    });
    expect(mockApi.listSSODomains).not.toHaveBeenCalled();
    expect(mockApi.getSSOConnection).not.toHaveBeenCalled();
  });

  it("adds, shows TXT record, verifies, and removes domains", async () => {
    const user = userEvent.setup();
    renderSSOTab();

    await user.type(await screen.findByLabelText("Domain"), "acme.com");
    await user.click(screen.getByRole("button", { name: "Add domain" }));

    await waitFor(() => {
      expect(mockApi.addSSODomain).toHaveBeenCalledWith("ws-1", "acme.com");
    });

    // Simulate list refetch returning the unverified domain.
    mockApi.listSSODomains.mockResolvedValue({ domains: [unverifiedDomain] });
    renderSSOTab();

    await waitFor(() => {
      expect(screen.getByText("_multica.acme.com")).toBeInTheDocument();
    });
    expect(screen.getByText("multica-verify=abc123")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Verify" }));
    await waitFor(() => {
      expect(mockApi.verifySSODomain).toHaveBeenCalledWith("ws-1", "acme.com");
    });

    await user.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => {
      expect(mockApi.removeSSODomain).toHaveBeenCalledWith("ws-1", "acme.com");
    });
  });

  it.each([
    ["domain_already_verified", 409, /already verified by another workspace/i],
    ["public_email_domain", 400, /Public email domains/i],
    ["plan_feature_unavailable", 402, /not available on your current plan/i],
  ])("shows %s domain error plainly", async (code, status, pattern) => {
    mockApi.addSSODomain.mockRejectedValue(
      new ApiError("err", status, "Error", { code }),
    );
    const user = userEvent.setup();
    renderSSOTab();

    await user.type(await screen.findByLabelText("Domain"), "gmail.com");
    await user.click(screen.getByRole("button", { name: "Add domain" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(pattern);
    });
    if (code === "plan_feature_unavailable") {
      expect(screen.getByLabelText("Domain")).toBeDisabled();
      expect(screen.getByRole("button", { name: "Add domain" })).toBeDisabled();
      expect(screen.getByRole("button", { name: "Save connection" })).toBeDisabled();
    }
  });

  it("gates saving the connection until a domain is verified", async () => {
    mockApi.listSSODomains.mockResolvedValue({ domains: [unverifiedDomain] });
    const user = userEvent.setup();
    renderSSOTab();

    await waitFor(() => {
      expect(
        screen.getByText(/Verify at least one domain before saving/i),
      ).toBeInTheDocument();
    });

    await user.type(screen.getByLabelText("Issuer URL"), "https://idp.acme.com");
    await user.type(screen.getByLabelText("Client ID"), "client-123");
    await user.type(screen.getByLabelText("Client secret"), "s3cret");

    expect(screen.getByRole("button", { name: "Save connection" })).toBeDisabled();
    expect(mockApi.putSSOConnection).not.toHaveBeenCalled();
  });

  it("shows stored secret indicator, redirect_uri, allows viewer role, and keeps secret when left blank", async () => {
    mockApi.listSSODomains.mockResolvedValue({ domains: [verifiedDomain] });
    mockApi.getSSOConnection.mockResolvedValue(existingConnection);
    const user = userEvent.setup();
    renderSSOTab();

    await waitFor(() => {
      expect(screen.getByText("Stored")).toBeInTheDocument();
    });
    expect(
      screen.getByText("https://app.multica.ai/auth/sso/callback"),
    ).toBeInTheDocument();

    // Change default_role to viewer and status to disabled, leave client secret blank.
    await user.click(screen.getByRole("combobox", { name: "Default role" }));
    await user.click(await screen.findByRole("option", { name: "Viewer" }));

    await user.click(screen.getByRole("combobox", { name: "Status" }));
    await user.click(await screen.findByRole("option", { name: "Disabled" }));

    await user.click(screen.getByRole("button", { name: "Save connection" }));

    await waitFor(() => {
      expect(mockApi.putSSOConnection).toHaveBeenCalledTimes(1);
    });
    const sent = mockApi.putSSOConnection.mock.calls[0]![1];
    expect(sent).toEqual({
      issuer: "https://idp.acme.com",
      client_id: "client-123",
      default_role: "viewer",
      status: "disabled",
    });
    expect("client_secret" in sent).toBe(false);
  });

  it("maps domain_not_verified on connection save and supports deleting the connection", async () => {
    mockApi.listSSODomains.mockResolvedValue({ domains: [verifiedDomain] });
    mockApi.getSSOConnection.mockResolvedValue(existingConnection);
    mockApi.putSSOConnection.mockRejectedValue(
      new ApiError("verify", 422, "Unprocessable Entity", {
        code: "domain_not_verified",
      }),
    );
    const user = userEvent.setup();
    renderSSOTab();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save connection" })).toBeEnabled();
    });
    await user.click(screen.getByRole("button", { name: "Save connection" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/Verify a domain/i);
    });

    await user.click(screen.getByRole("button", { name: "Remove connection" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    await waitFor(() => {
      expect(mockApi.deleteSSOConnection).toHaveBeenCalledWith("ws-1");
    });
  });
});
