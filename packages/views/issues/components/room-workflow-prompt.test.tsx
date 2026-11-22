/**
 * @vitest-environment jsdom
 */
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError } from "@multica/core/api/client";
import type { IssueWorkflow, RoomWorkflowCheck, RoomWorkflowPolicy } from "@multica/core/types";
import { useRoomWorkflowCheck, useRoomWorkflowPolicy } from "@multica/core/issue-workflows/hooks";
import { renderWithI18n } from "../../test/i18n";
import {
  isRoomWorkflowCode,
  parseRoomWorkflowRefusal,
  RoomWorkflowPrompt,
  shouldPromptForRoomWorkflow,
} from "./room-workflow-prompt";

const mocks = vi.hoisted(() => ({
  getRoomWorkflowPolicy: vi.fn(),
  setRoomWorkflowPolicy: vi.fn(),
  checkRoomWorkflow: vi.fn(),
  listIssueWorkflows: vi.fn(),
  setSquadActiveWorkflow: vi.fn(),
  listMembers: vi.fn(),
  listSquads: vi.fn(),
  push: vi.fn(),
  user: { id: "user-1", name: "Ada", email: "ada@example.test" },
}));

vi.mock("@multica/core/api", async () => {
  const { ApiError: RealApiError } = await import("@multica/core/api/client");
  return {
    ApiError: RealApiError,
    api: {
      getRoomWorkflowPolicy: mocks.getRoomWorkflowPolicy,
      setRoomWorkflowPolicy: mocks.setRoomWorkflowPolicy,
      checkRoomWorkflow: mocks.checkRoomWorkflow,
      listIssueWorkflows: mocks.listIssueWorkflows,
      setSquadActiveWorkflow: mocks.setSquadActiveWorkflow,
      listMembers: mocks.listMembers,
      listSquads: mocks.listSquads,
    },
  };
});

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    workflow: () => "/acme/workflow",
  }),
}));

vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (selector?: (state: { user: typeof mocks.user }) => unknown) => {
      const state = { user: mocks.user };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ user: mocks.user }) },
  );
  return { useAuthStore };
});

vi.mock("../../navigation/context", () => ({
  useNavigation: () => ({
    pathname: "/acme/issues",
    searchParams: new URLSearchParams(),
    hash: "",
    push: mocks.push,
    replace: vi.fn(),
    back: vi.fn(),
    getShareableUrl: (p: string) => p,
  }),
}));

const WORKFLOWS: IssueWorkflow[] = [
  {
    id: "wf-default",
    workspace_id: "ws-1",
    name: "Default",
    description: "",
    is_default: true,
    owner_id: null,
    steps: [],
    created_at: "",
    updated_at: "",
  },
  {
    id: "wf-delivery",
    workspace_id: "ws-1",
    name: "Delivery",
    description: "",
    is_default: false,
    owner_id: null,
    steps: [],
    created_at: "",
    updated_at: "",
  },
  {
    id: "wf-content",
    workspace_id: "ws-1",
    name: "Content",
    description: "",
    is_default: false,
    owner_id: null,
    steps: [],
    created_at: "",
    updated_at: "",
  },
];

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function renderPrompt(
  props: {
    open?: boolean;
    refusal: RoomWorkflowCheck | null;
    onOpenChange?: (open: boolean) => void;
    onResolved?: () => void;
    onNavigateAway?: () => void;
  },
  qc = makeQueryClient(),
) {
  return {
    qc,
    ...renderWithI18n(
      <QueryClientProvider client={qc}>
        <RoomWorkflowPrompt
          open={props.open ?? true}
          onOpenChange={props.onOpenChange ?? vi.fn()}
          refusal={props.refusal}
          onResolved={props.onResolved ?? vi.fn()}
          onNavigateAway={props.onNavigateAway}
        />
      </QueryClientProvider>,
    ),
  };
}

beforeEach(() => {
  mocks.user = { id: "user-1", name: "Ada", email: "ada@example.test" };
  mocks.getRoomWorkflowPolicy.mockResolvedValue({
    kind: "no_action",
    workflowId: null,
  } satisfies RoomWorkflowPolicy);
  mocks.setRoomWorkflowPolicy.mockImplementation(
    async (_wsId: string, policy: RoomWorkflowPolicy) => policy,
  );
  mocks.listIssueWorkflows.mockResolvedValue({ workflows: WORKFLOWS });
  mocks.setSquadActiveWorkflow.mockResolvedValue({ active_workflow_id: "wf-delivery" });
  mocks.listMembers.mockResolvedValue([
    { user_id: "user-1", role: "admin", name: "Ada" },
  ]);
  mocks.listSquads.mockResolvedValue([
    { id: "squad-1", name: "Planning Squad", creator_id: "user-2", leader_id: "agent-1" },
  ]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("room-workflow refusal helpers", () => {
  it("accepts only the two room-workflow codes", () => {
    expect(isRoomWorkflowCode("room_workflow_required")).toBe(true);
    expect(isRoomWorkflowCode("room_no_workflow_for_non_request")).toBe(true);
    expect(isRoomWorkflowCode("other_error")).toBe(false);
    expect(isRoomWorkflowCode(null)).toBe(false);
  });

  it("parses only HTTP 422 responses with one of the two codes", () => {
    const err422 = new ApiError("Workflow needed", 422, "Unprocessable Entity", {
      code: "room_workflow_required",
      error: "Squad has no workflow",
      squad_id: "squad-1",
    });
    expect(parseRoomWorkflowRefusal(err422)).toEqual({
      needsWorkflow: true,
      code: "room_workflow_required",
      message: "Squad has no workflow",
      squadId: "squad-1",
    });

    const other422 = new ApiError("Agent unavailable", 422, "Unprocessable Entity", {
      code: "agent_unavailable",
    });
    expect(parseRoomWorkflowRefusal(other422)).toBeNull();

    const err400 = new ApiError("Bad request", 400, "Bad Request", {
      code: "room_workflow_required",
    });
    expect(parseRoomWorkflowRefusal(err400)).toBeNull();
  });

  it("stops prompting for non-request once a policy is stored", () => {
    const check: RoomWorkflowCheck = {
      needsWorkflow: true,
      code: "room_no_workflow_for_non_request",
      message: "",
      squadId: "squad-1",
    };
    expect(shouldPromptForRoomWorkflow(check, { kind: "no_action", workflowId: null })).toBe(true);
    expect(shouldPromptForRoomWorkflow(check, { kind: "default", workflowId: null })).toBe(false);
    expect(shouldPromptForRoomWorkflow(check, { kind: "workflow", workflowId: "wf-delivery" })).toBe(false);
  });
});

describe("RoomWorkflowPrompt", () => {
  it("does not render for non-matching codes or when needsWorkflow is false", () => {
    renderPrompt({
      refusal: {
        needsWorkflow: false,
        code: "room_workflow_required",
        message: "",
        squadId: "squad-1",
      },
    });
    expect(screen.queryByRole("dialog")).toBeNull();

    cleanup();
    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: null,
        message: "",
        squadId: null,
      },
    });
    expect(screen.queryByRole("dialog")).toBeNull();

    cleanup();
    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "something_else" as unknown as RoomWorkflowCheck["code"],
        message: "",
        squadId: null,
      },
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("saves Default policy on OK, mentions workspace settings, and resubmits", async () => {
    const user = userEvent.setup();
    const onResolved = vi.fn();
    const onOpenChange = vi.fn();
    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_no_workflow_for_non_request",
        message: "",
        squadId: "squad-1",
      },
      onResolved,
      onOpenChange,
    });

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("saved to workspace settings and can be changed later");

    await user.click(screen.getByRole("button", { name: "OK" }));

    await waitFor(() => {
      expect(mocks.setRoomWorkflowPolicy).toHaveBeenCalledWith("ws-1", {
        kind: "default",
        workflowId: null,
      });
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onResolved).toHaveBeenCalledTimes(1);
  });

  it("saves a chosen existing workflow policy on OK and resubmits", async () => {
    const user = userEvent.setup();
    const onResolved = vi.fn();
    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_no_workflow_for_non_request",
        message: "",
        squadId: "squad-1",
      },
      onResolved,
    });

    await screen.findByRole("dialog");
    await user.click(screen.getByRole("radio", { name: "Existing workflow" }));
    await user.click(await screen.findByRole("combobox", { name: "Existing workflow" }));
    await user.click(await screen.findByRole("option", { name: "Content" }));
    await user.click(screen.getByRole("button", { name: "OK" }));

    await waitFor(() => {
      expect(mocks.setRoomWorkflowPolicy).toHaveBeenCalledWith("ws-1", {
        kind: "workflow",
        workflowId: "wf-content",
      });
    });
    expect(onResolved).toHaveBeenCalledTimes(1);
  });

  it("shows a clear admin-only message when saving the policy returns 403", async () => {
    const user = userEvent.setup();
    const onResolved = vi.fn();
    const onOpenChange = vi.fn();
    mocks.setRoomWorkflowPolicy.mockRejectedValueOnce(
      new ApiError("Forbidden", 403, "Forbidden", { error: "admin required" }),
    );

    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_no_workflow_for_non_request",
        message: "",
        squadId: "squad-1",
      },
      onResolved,
      onOpenChange,
    });

    await screen.findByRole("dialog");
    await user.click(screen.getByRole("button", { name: "OK" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Only workspace admins can change this setting.");
    expect(onResolved).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it("never re-opens once the choice is stored", async () => {
    const user = userEvent.setup();
    const submitSpy = vi.fn();
    let storedPolicy: RoomWorkflowPolicy = { kind: "no_action", workflowId: null };
    mocks.getRoomWorkflowPolicy.mockImplementation(async () => storedPolicy);
    mocks.setRoomWorkflowPolicy.mockImplementation(async (_wsId: string, next: RoomWorkflowPolicy) => {
      storedPolicy = next;
      return next;
    });
    mocks.checkRoomWorkflow.mockImplementation(async () =>
      storedPolicy.kind === "no_action"
        ? {
            needsWorkflow: true,
            code: "room_no_workflow_for_non_request",
            message: "",
            squadId: "squad-1",
          }
        : {
            needsWorkflow: false,
            code: null,
            message: "",
            squadId: null,
          },
    );

    function WriteHarness() {
      const [promptRefusal, setPromptRefusal] = useState<RoomWorkflowCheck | null>(null);
      const { check } = useRoomWorkflowCheck("ws-1", { assigneeType: "squad", assigneeId: "squad-1" }, true);
      const { policy } = useRoomWorkflowPolicy("ws-1");

      const handleSubmit = () => {
        if (shouldPromptForRoomWorkflow(check, policy)) {
          setPromptRefusal(check);
          return;
        }
        submitSpy();
      };

      return (
        <>
          <button type="button" onClick={handleSubmit}>Submit</button>
          <RoomWorkflowPrompt
            open={promptRefusal !== null}
            onOpenChange={(open) => { if (!open) setPromptRefusal(null); }}
            refusal={promptRefusal}
            onResolved={() => {
              setPromptRefusal(null);
              submitSpy();
            }}
          />
        </>
      );
    }

    const qc = makeQueryClient();
    renderWithI18n(
      <QueryClientProvider client={qc}>
        <WriteHarness />
      </QueryClientProvider>,
    );

    // Wait for initial check to report needsWorkflow.
    await waitFor(() => expect(mocks.checkRoomWorkflow).toHaveBeenCalledTimes(1));

    // First submit opens the prompt instead of submitting.
    await user.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(submitSpy).not.toHaveBeenCalled();

    // OK saves policy and resubmits.
    await user.click(screen.getByRole("button", { name: "OK" }));
    await waitFor(() => expect(submitSpy).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    // Second submit goes straight through without re-opening the prompt.
    await user.click(screen.getByRole("button", { name: "Submit" }));
    expect(submitSpy).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers squad workflow picker and workflow tab link to squad managers", async () => {
    const user = userEvent.setup();
    const onResolved = vi.fn();
    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_workflow_required",
        message: "",
        squadId: "squad-1",
      },
      onResolved,
    });

    const dialog = await screen.findByRole("dialog");
    await waitFor(() => {
      expect(dialog).toHaveTextContent("Planning Squad has no active workflow yet.");
    });

    const link = screen.getByRole("link", { name: "Open Workflows" });
    expect(link).toHaveAttribute("href", "/acme/workflow");

    await user.click(screen.getByRole("combobox", { name: "Squad workflow" }));
    await user.click(await screen.findByRole("option", { name: "Delivery" }));
    await user.click(screen.getByRole("button", { name: "OK" }));

    await waitFor(() => {
      expect(mocks.setSquadActiveWorkflow).toHaveBeenCalledWith("squad-1", {
        workflow_id: "wf-delivery",
      });
    });
    expect(onResolved).toHaveBeenCalledTimes(1);
  });

  it("shows server refusal error when setting squad workflow fails", async () => {
    const user = userEvent.setup();
    const onResolved = vi.fn();
    mocks.setSquadActiveWorkflow.mockRejectedValueOnce(
      new ApiError("Workflow incompatible", 409, "Conflict", {
        error: "Workflow statuses require mapping first.",
      }),
    );

    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_workflow_required",
        message: "",
        squadId: "squad-1",
      },
      onResolved,
    });

    await screen.findByRole("dialog");
    await waitFor(() => expect(screen.getByRole("button", { name: "OK" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "OK" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Workflow statuses require mapping first.",
    );
    expect(onResolved).not.toHaveBeenCalled();
  });

  it("hides the squad picker for members who cannot manage the squad", async () => {
    mocks.listMembers.mockResolvedValue([
      { user_id: "user-1", role: "member", name: "Ada" },
    ]);
    mocks.listSquads.mockResolvedValue([
      { id: "squad-1", name: "Planning Squad", creator_id: "user-other", leader_id: "agent-1" },
    ]);

    renderPrompt({
      refusal: {
        needsWorkflow: true,
        code: "room_workflow_required",
        message: "",
        squadId: "squad-1",
      },
    });

    await screen.findByRole("dialog");
    await waitFor(() => {
      expect(screen.getByText("Ask a squad manager or workspace admin to assign one.")).toBeInTheDocument();
    });
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: "OK" })).toBeNull();
    expect(screen.getByRole("link", { name: "Open Workflows" })).toHaveAttribute(
      "href",
      "/acme/workflow",
    );
  });
});
