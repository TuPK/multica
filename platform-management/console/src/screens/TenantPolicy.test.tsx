// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TenantPolicy } from "./TenantPolicy";
import { PlatformApiError } from "../api/client";
import { findAlert } from "../test/alert";

const mockUseMe = vi.fn();
const mockUseTenants = vi.fn();
const mockUseTenantPolicy = vi.fn();
const mockUpdateTenantPolicyMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useTenants: () => mockUseTenants(),
  useTenantPolicy: (tenantId?: string, enabled = true) =>
    mockUseTenantPolicy(tenantId, enabled),
  useUpdateTenantPolicy: () => ({
    mutateAsync: mockUpdateTenantPolicyMutateAsync,
    isPending: false,
  }),
}));

const samplePolicyResponse = {
  policy: {
    allowed_invite_domains: ["acme.com"],
    share_links_enabled: false,
    require_sso: true,
    require_mfa: false,
    session_max_age_hours: 24,
  },
  version: 2,
  effective_policy: {
    allowed_invite_domains: ["acme.com"],
    share_links_enabled: false,
    require_sso: true,
    require_mfa: true,
    session_max_age_hours: 24,
  },
  sources: {
    allowed_invite_domains: "tenant" as const,
    share_links_enabled: "tenant" as const,
    require_sso: "tenant" as const,
    require_mfa: "workspace" as const,
    session_max_age_hours: "tenant" as const,
  },
  tenant_fields: [
    "allowed_invite_domains",
    "share_links_enabled",
    "require_sso",
    "session_max_age_hours",
  ],
};

describe("TenantPolicy screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseTenants.mockReturnValue({
      data: [{ id: "t-1", name: "Acme Tenant" }, { id: "t-2", name: "Beta Tenant" }],
      isLoading: false,
      isError: false,
    });
    mockUseTenantPolicy.mockReturnValue({
      data: samplePolicyResponse,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("shows merged effective policy and which value comes from the tenant", () => {
    render(<TenantPolicy tenantId="t-1" userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: /Effective policy/i })).toBeInTheDocument();

    const ssoRow = screen.getByTestId("effective-policy-require_sso");
    expect(ssoRow).toHaveTextContent("Required");
    expect(ssoRow).toHaveTextContent("From tenant");

    const mfaRow = screen.getByTestId("effective-policy-require_mfa");
    expect(mfaRow).toHaveTextContent("Required");
    expect(mfaRow).not.toHaveTextContent("From tenant");

    const domainsRow = screen.getByTestId("effective-policy-allowed_invite_domains");
    expect(domainsRow).toHaveTextContent("acme.com");
    expect(domainsRow).toHaveTextContent("From tenant");

    const shareRow = screen.getByTestId("effective-policy-share_links_enabled");
    expect(shareRow).toHaveTextContent("Disabled");
    expect(shareRow).toHaveTextContent("From tenant");

    const sessionRow = screen.getByTestId("effective-policy-session_max_age_hours");
    expect(sessionRow).toHaveTextContent("24 hours");
    expect(sessionRow).toHaveTextContent("From tenant");
  });

  it("admin edits own tenant policy only", async () => {
    const user = userEvent.setup();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" },
    });
    mockUpdateTenantPolicyMutateAsync.mockResolvedValueOnce({
      policy: {
        ...samplePolicyResponse.policy,
        require_mfa: true,
      },
      version: 3,
      effective_policy: samplePolicyResponse.effective_policy,
      sources: samplePolicyResponse.sources,
      tenant_fields: samplePolicyResponse.tenant_fields,
    });

    const { rerender } = render(<TenantPolicy tenantId="t-1" userRole="admin" />);

    expect(mockUseTenantPolicy).toHaveBeenCalledWith("t-1", true);
    expect(screen.queryByRole("combobox", { name: /Tenant/i })).not.toBeInTheDocument();

    const mfaSwitch = screen.getByRole("switch", { name: /Require MFA/i });
    await user.click(mfaSwitch);

    fireEvent.click(screen.getByRole("button", { name: /Save policy/i }));

    await waitFor(() => {
      expect(mockUpdateTenantPolicyMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-1",
        expected_version: 2,
        allowed_invite_domains: ["acme.com"],
        share_links_enabled: false,
        require_sso: true,
        require_mfa: true,
        session_max_age_hours: 24,
      });
    });

    rerender(<TenantPolicy tenantId="t-other" userRole="admin" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
    expect(mockUseTenantPolicy).toHaveBeenLastCalledWith("t-other", false);
  });

  it("support sees read-only policy and billing gets not found", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-supp", email: "support@example.com", role: "support", tenant_id: null },
    });
    const { rerender } = render(<TenantPolicy tenantId="t-1" userRole="support" />);
    expect(screen.queryByRole("button", { name: /Save policy/i })).not.toBeInTheDocument();
    expect(screen.getByRole("switch", { name: /Require SSO/i })).toHaveAttribute("aria-disabled", "true");

    mockUseMe.mockReturnValue({
      data: { user_id: "u-bill", email: "billing@example.com", role: "billing", tenant_id: null },
    });
    rerender(<TenantPolicy tenantId="t-1" userRole="billing" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });

  it("shows 402 plan_feature_unavailable and 409 policy_version_conflict errors", async () => {
    mockUpdateTenantPolicyMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(402, "plan_feature_unavailable", "no workspace of the tenant has the plan feature", {
        feature: "workspace_policies",
      }),
    );

    render(<TenantPolicy tenantId="t-1" userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: /Save policy/i }));
    await waitFor(() => {
      expect(findAlert()).toHaveTextContent(/workspace policies/i);
    });
  });
});
