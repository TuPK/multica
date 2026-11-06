/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { projectKeys } from "@multica/core/projects/queries";
import type { Agent, Issue, Project } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { CompanyAgentNewIssue } from "./company-agent-new-issue";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const create = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));

vi.mock("@multica/core/issues/mutations", () => ({
  useCreateIssue: () => ({ mutate: create.mutate, isPending: create.isPending }),
  useSetIssueRunQueue: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBatchSetRunQueue: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@multica/core/api", () => ({
  api: { listIssues: () => Promise.resolve({ issues: [] }), previewRunQueue: () => Promise.resolve(null) },
}));

// The project picker reads the workspace query cache; the form only needs the project id it starts from.
vi.mock("../../projects/components/project-picker", () => ({
  ProjectPicker: ({ projectId }: { projectId: string | null }) => <button type="button">Project {projectId ?? "none"}</button>,
}));

// The real picker reads the workspace providers; the form only needs to hand it a status.
vi.mock("../../issues/components/pickers/status-picker", () => ({
  StatusPicker: ({ status, onUpdate }: { status: string; onUpdate: (u: { status: string }) => void }) => (
    <button type="button" onClick={() => onUpdate({ status: "in_review" })}>
      Pick in review (was {status})
    </button>
  ),
}));

const AGENT = { id: "ada", name: "Ada" } as Agent;

function setup(overrides: Partial<Parameters<typeof CompanyAgentNewIssue>[0]> = {}, projects: Project[] = []) {
  const props = { agent: AGENT, onCreated: vi.fn(), onCancel: vi.fn(), ...overrides };
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  queryClient.setQueryData(projectKeys.list("ws-1"), { projects, total: projects.length });
  renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <CompanyAgentNewIssue {...props} />
    </QueryClientProvider>,
  );
  return props;
}

const typeTitle = (value: string) => fireEvent.change(screen.getByLabelText("Title"), { target: { value } });

describe("CompanyAgentNewIssue", () => {
  beforeEach(() => {
    create.mutate.mockReset();
    create.isPending = false;
  });

  afterEach(() => {
    cleanup();
  });

  it("cannot be created without a title", () => {
    setup();

    expect(screen.getByRole("button", { name: "Create issue" }).hasAttribute("disabled")).toBe(true);
    typeTitle("   ");
    expect(screen.getByRole("button", { name: "Create issue" }).hasAttribute("disabled")).toBe(true);
  });

  it("hands the issue to the agent as todo by default", () => {
    setup();
    typeTitle(" Ship it ");
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "All of it" } });
    fireEvent.click(screen.getByRole("button", { name: /High/ }));

    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    expect(create.mutate).toHaveBeenCalledWith(
      {
        title: "Ship it",
        description: "All of it",
        priority: "high",
        status: "todo",
        assignee_type: "agent",
        assignee_id: "ada",
      },
      expect.any(Object),
    );
  });

  it("creates the issue in the status picked, keeping the agent handover", () => {
    setup();
    typeTitle("Review me");
    fireEvent.click(screen.getByRole("button", { name: "Pick in review (was todo)" }));

    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    expect(create.mutate.mock.calls[0]![0]).toMatchObject({
      status: "in_review",
      assignee_type: "agent",
      assignee_id: "ada",
    });
  });

  it("follows the assign switch for the default status until a status is picked", () => {
    setup();
    typeTitle("Later");
    fireEvent.click(screen.getByRole("switch", { name: "Assign to Ada" }));
    expect(screen.getByRole("button", { name: "Pick in review (was backlog)" })).toBeTruthy();
  });

  it("leaves the issue in the backlog, unassigned, when it is not handed over", () => {
    setup();
    typeTitle("Later");
    fireEvent.click(screen.getByRole("switch", { name: "Assign to Ada" }));

    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    expect(create.mutate).toHaveBeenCalledWith(
      { title: "Later", priority: "none", status: "backlog" },
      expect.any(Object),
    );
  });

  it("files the issue under the project it was opened from", () => {
    setup({ projectId: "proj-1" });
    typeTitle("In project");

    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    expect(create.mutate.mock.calls[0]![0]).toMatchObject({ project_id: "proj-1" });
  });

  it("files the issue under its squad's project, which it cannot change", () => {
    const squadProject = { id: "proj-squad", title: "Shop", status: "in_progress", squad_id: "squad-1" } as Project;
    setup({ projectId: "proj-1", agent: { ...AGENT, home_squad_id: "squad-1" } as Agent }, [squadProject]);
    typeTitle("In squad project");

    expect(screen.getByRole("button", { name: "Project proj-squad" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    expect(create.mutate.mock.calls[0]![0]).toMatchObject({ project_id: "proj-squad" });
  });

  it("reports the new issue once it exists", () => {
    const props = setup();
    typeTitle("Done deal");
    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    const options = create.mutate.mock.calls[0]![1] as { onSuccess: (issue: Issue) => void };
    options.onSuccess({ id: "new-1", identifier: "MUL-9" } as Issue);

    expect(props.onCreated).toHaveBeenCalledWith("new-1");
  });

  it("stays open, keeping what was typed, when the issue cannot be created", () => {
    const props = setup();
    typeTitle("Keep me");
    fireEvent.click(screen.getByRole("button", { name: "Create issue" }));

    (create.mutate.mock.calls[0]![1] as { onError: () => void }).onError();

    expect(props.onCreated).not.toHaveBeenCalled();
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Keep me");
  });

  it("cancels", () => {
    const props = setup();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(props.onCancel).toHaveBeenCalled();
  });
});
