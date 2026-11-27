// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError } from "@multica/core/api";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { GovernancePage } from "../../governance/governance-page";
import { RUNNING } from "../../governance/governance-test-mocks";
import { AccountTab } from "../../settings/components/account-tab";
import { renderWithI18n } from "../../test/i18n";

const flags = vi.hoisted(() => ({ principal: false }));
const apiMock = vi.hoisted(() => ({
  request: vi.fn(),
  updateMe: vi.fn(),
  getCompanyState: vi.fn(),
  listGovernanceEvents: vi.fn(),
  listBudgets: vi.fn(),
  listBudgetHolds: vi.fn(),
  listGoals: vi.fn(),
}));
const currentUser = vi.hoisted(() => ({
  id: "u-1",
  name: "Jane Doe",
  email: "jane@example.com",
  profile_description: "",
  avatar_url: null,
}));

vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (key: string, fallback: boolean) => (key === "durable_principal" ? flags.principal : fallback),
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: apiMock,
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/permissions")>()),
  useCurrentMember: () => ({ userId: "u-1", role: "owner", member: null, isLoading: false }),
}));
vi.mock("@multica/core/workspace/hooks", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/workspace/hooks")>()),
  useActorName: () => ({ getActorName: (type: string, id: string) => `${type} ${id}` }),
}));
vi.mock("@multica/core/audit", () => ({
  securityEventsOptions: (userId: string) => ({
    queryKey: ["security-events", userId],
    queryFn: async () => ({ events: [], next_cursor: null }),
  }),
}));
vi.mock("@multica/core/account", () => ({
  useRevokeAllSessions: () => ({ mutateAsync: vi.fn(), isPending: false }),
  downloadAccountExport: vi.fn(),
  usePreviewAccountDeletion: () => ({ mutateAsync: vi.fn() }),
  useRequestAccountDeletion: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (selector?: (state: { user: typeof currentUser; setUser: ReturnType<typeof vi.fn> }) => unknown) => {
      const state = { user: currentUser, setUser: vi.fn() };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ user: currentUser, setUser: vi.fn() }) },
  );
  return { useAuthStore };
});
vi.mock("../../auth", () => ({ useLogout: () => vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function AppHarness() {
  const [url, setUrl] = useState("/acme/settings");
  const [pathname, rawQuery = ""] = url.split("?");
  const adapter: NavigationAdapter = {
    pathname: pathname || "/acme/settings",
    searchParams: new URLSearchParams(rawQuery),
    hash: "",
    push: (next) => setUrl(next),
    replace: (next) => setUrl(next),
    back: vi.fn(),
    getShareableUrl: (path) => path,
  };

  return (
    <WorkspaceSlugProvider slug="acme">
      <NavigationProvider value={adapter}>
        {adapter.pathname === "/acme/governance" ? <GovernancePage /> : <AccountTab />}
      </NavigationProvider>
    </WorkspaceSlugProvider>
  );
}

function renderHarness() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <AppHarness />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flags.principal = false;
  vi.clearAllMocks();
  apiMock.getCompanyState.mockResolvedValue(RUNNING);
  apiMock.listGovernanceEvents.mockResolvedValue({ events: [], next_cursor: null });
  apiMock.listBudgets.mockResolvedValue({ budgets: [] });
  apiMock.listBudgetHolds.mockResolvedValue([]);
  apiMock.listGoals.mockResolvedValue([]);
  apiMock.request.mockRejectedValue(new ApiError("the person has no principal profile", 404, "Not Found"));
});

afterEach(cleanup);

describe("Account settings to Principal profile navigation", () => {
  it("hides the Principal profile row in Account settings when durable_principal is off", () => {
    renderHarness();
    expect(screen.queryByText("Principal profile")).not.toBeInTheDocument();
  });

  it("navigates from the Account settings row to the Governance principal tab and renders the principal profile page", async () => {
    flags.principal = true;
    renderHarness();

    expect(screen.getByText("Principal profile")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Manage" }));

    expect(await screen.findByRole("heading", { name: "Principal profile is off" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Principal" }).getAttribute("aria-selected")).toBe("true");
  });
});
