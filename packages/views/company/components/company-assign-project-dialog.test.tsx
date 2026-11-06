/**
 * @vitest-environment jsdom
 *
 * A squad works one project at a time. Once it has one, the dialog only offers to
 * unassign it; the mutations are stubbed so these tests cover what the dialog shows
 * and which action it fires.
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { projectKeys } from "@multica/core/projects/queries";
import { workspaceKeys } from "@multica/core/workspace/queries";
import type { Project, Squad } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { CompanyAssignProjectDialog } from "./company-assign-project-dialog";

const mutations = vi.hoisted(() => ({
  assign: vi.fn(),
  unassign: vi.fn(),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/company", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/company")>()),
  useAssignSquadProject: () => ({ mutate: mutations.assign, isPending: false }),
  useUnassignSquadProject: () => ({ mutate: mutations.unassign, isPending: false }),
}));

const SQUAD = { id: "squad-1", name: "SDLC Greenfield" } as Squad;
const project = (id: string, title: string, squadId: string | null = null) =>
  ({ id, title, status: "in_progress", squad_id: squadId }) as Project;

function setup(projects: Project[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  queryClient.setQueryData(projectKeys.list("ws-1"), { projects, total: projects.length });
  queryClient.setQueryData(workspaceKeys.squads("ws-1"), [SQUAD]);
  const onClose = vi.fn();
  renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <CompanyAssignProjectDialog squad={SQUAD} onClose={onClose} onAssigned={vi.fn()} onOpenSquad={vi.fn()} />
    </QueryClientProvider>,
  );
  return { onClose };
}

describe("CompanyAssignProjectDialog", () => {
  afterEach(() => {
    cleanup();
    mutations.assign.mockReset();
    mutations.unassign.mockReset();
  });

  it("offers the squad's projects to pick from while it has none", () => {
    setup([project("p1", "Mua Ngon"), project("p2", "Secretary desk")]);

    expect(screen.getByRole("textbox", { name: "Search projects" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Assign project" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Unassign project" })).toBeNull();
  });

  it("shows only the assigned project once the squad has one, and unassigns it on request", () => {
    const { onClose } = setup([project("p1", "Mua Ngon", SQUAD.id), project("p2", "Secretary desk")]);

    expect(screen.queryByRole("textbox", { name: "Search projects" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Assign project" })).toBeNull();
    expect(screen.getByText("Mua Ngon")).toBeTruthy();
    expect(screen.queryByText("Secretary desk")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Unassign project" }));

    expect(mutations.unassign).toHaveBeenCalledTimes(1);
    expect(mutations.assign).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});
