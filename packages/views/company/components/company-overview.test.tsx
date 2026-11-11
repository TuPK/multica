import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { CompanyOverview } from "./company-overview";
import { readCompanySelection } from "../lib/company-selection";

type QueryResult = { data?: unknown; isSuccess?: boolean };

const mocks = vi.hoisted(() => ({
  queries: {} as Record<string, QueryResult>,
  admin: false,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey?: readonly unknown[] }) => ({
    isPending: false,
    ...mocks.queries[(options.queryKey ?? []).join("/")],
  }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/company", () => ({
  COMPANY_SLOTS: 6,
  companyFloorsOptions: () => ({ queryKey: ["floors"] }),
  useDeleteCompanyFloor: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateCompanyFloor: () => ({ mutate: vi.fn(), isPending: false }),
  useCreateCompanyFloor: () => ({ mutate: vi.fn(), isPending: false }),
  useSetCompanySeats: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({ queryKey: ["agents"] }),
  squadListOptions: () => ({ queryKey: ["squads"] }),
  workspaceKeys: { squads: () => ["squads"], agents: () => ["agents"] },
}));
vi.mock("../lib/use-company-admin", () => ({ useCompanyAdmin: () => mocks.admin }));
vi.mock("../../issues/surface/peek-context", () => ({ useIssuePeekActions: () => null }));
vi.mock("../../modals/create-squad", () => ({ CreateSquadModal: () => null }));
vi.mock("./company-insights", () => ({
  CompanyInsights: ({ department }: { department: { name: string } | null }) => (
    <div>insights: {department ? department.name : "company"}</div>
  ),
}));
// The detail panes have their own tests; this one checks the drill-down wiring.
vi.mock("./company-squad-detail", () => ({
  CompanySquadDetail: ({ squad }: { squad: { name: string } }) => <div>squad pane: {squad.name}</div>,
}));
vi.mock("./company-agent-detail", () => ({
  CompanyAgentDetail: ({ agentId }: { agentId: string }) => <div>agent pane: {agentId}</div>,
}));

function seed() {
  mocks.queries = {
    floors: {
      data: [
        { id: "f-2", name: "Product", position: 1 },
        { id: "f-1", name: "Executive", position: 0 },
      ],
    },
    squads: {
      isSuccess: true,
      data: [
        { id: "s-1", name: "Planning", floor_id: "f-1", leader_id: "a-1", member_count: 2 },
        { id: "s-2", name: "Discovery", floor_id: "f-2", leader_id: "a-2", member_count: 3 },
        { id: "s-3", name: "Loose squad", floor_id: null, leader_id: "a-1", member_count: 1 },
      ],
    },
    agents: {
      data: [
        { id: "a-1", name: "Secretary", skills: [], instructions: "" },
        { id: "a-2", name: "Researcher", skills: [], instructions: "" },
      ],
    },
  };
}

function renderOverview(search = "") {
  const adapter: NavigationAdapter = {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/company",
    searchParams: new URLSearchParams(search),
    hash: "",
    getShareableUrl: (path) => `https://multica.test${path}`,
  };
  renderWithI18n(
    <NavigationProvider value={adapter}>
      <CompanyOverview selection={readCompanySelection(adapter.searchParams)} />
    </NavigationProvider>,
  );
  return adapter;
}

beforeEach(() => {
  seed();
  mocks.admin = false;
});

describe("CompanyOverview drill-down", () => {
  it("lists departments in floor order, with unseated squads last", () => {
    renderOverview();
    const departments = screen.getByRole("complementary", { name: "Departments" });
    const names = Array.from(departments.querySelectorAll("button")).map((b) => b.textContent ?? "");
    expect(names[0]).toContain("Executive");
    expect(names[1]).toContain("Product");
    expect(names[2]).toContain("Not in a department");
  });

  it("opens a department in the URL without leaving the page", async () => {
    const adapter = renderOverview();
    await userEvent.click(screen.getByRole("button", { name: /Executive/ }));
    expect(adapter.push).toHaveBeenCalledWith("/acme/company?dept=f-1");
  });

  it("opens a squad, which also names its department", async () => {
    const adapter = renderOverview("dept=f-1");
    await userEvent.click(screen.getByRole("button", { name: /Planning/ }));
    expect(adapter.push).toHaveBeenCalledWith("/acme/company?dept=f-1&squad=s-1");
  });

  it("shows the squad pane for a squad in the URL and steps back through the breadcrumb", async () => {
    const adapter = renderOverview("dept=f-1&squad=s-1");
    expect(screen.getByText("squad pane: Planning")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Executive" }));
    expect(adapter.push).toHaveBeenCalledWith("/acme/company?dept=f-1");
  });

  it("shows company insights until a department is chosen, then that department's", () => {
    renderOverview();
    expect(screen.getByText("insights: company")).toBeInTheDocument();
  });

  it("scopes the insights to the open department", () => {
    renderOverview("dept=f-1");
    expect(screen.getByText("insights: Executive")).toBeInTheDocument();
  });

  it("derives the department from a squad named alone in the URL", () => {
    renderOverview("squad=s-2");
    expect(screen.getByText("squad pane: Discovery")).toBeInTheDocument();
  });
});

describe("CompanyOverview department delete", () => {
  it("confirms the delete and names the squads seated in the department", async () => {
    mocks.admin = true;
    renderOverview();

    await userEvent.click(screen.getByRole("button", { name: "Executive options" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete department" }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText("Delete Executive?")).toBeInTheDocument();
    expect(within(dialog).getByText(/1 squad will lose its room\. It is kept\./)).toBeInTheDocument();
  });

  it("keeps delete off until the squad list has loaded", async () => {
    mocks.admin = true;
    mocks.queries.squads = { data: mocks.queries.squads?.data };
    renderOverview();

    await userEvent.click(screen.getByRole("button", { name: "Executive options" }));

    expect((await screen.findByRole("menuitem", { name: "Delete department" })).getAttribute("aria-disabled")).toBe("true");
  });
});
