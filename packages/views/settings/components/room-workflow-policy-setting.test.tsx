/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError } from "@multica/core/api/client";
import type { IssueWorkflow, RoomWorkflowPolicy } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { RoomWorkflowPolicySetting } from "./room-workflow-policy-setting";

const mocks = vi.hoisted(() => ({
  getRoomWorkflowPolicy: vi.fn(),
  setRoomWorkflowPolicy: vi.fn(),
  listIssueWorkflows: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@multica/core/api", async () => {
  const { ApiError: RealApiError } = await import("@multica/core/api/client");
  return {
    ApiError: RealApiError,
    api: {
      getRoomWorkflowPolicy: mocks.getRoomWorkflowPolicy,
      setRoomWorkflowPolicy: mocks.setRoomWorkflowPolicy,
      listIssueWorkflows: mocks.listIssueWorkflows,
    },
  };
});

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
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
];

function renderSetting(canManage = true) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <RoomWorkflowPolicySetting wsId="ws-1" canManage={canManage} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mocks.getRoomWorkflowPolicy.mockResolvedValue({
    kind: "no_action",
    workflowId: null,
  } satisfies RoomWorkflowPolicy);
  mocks.setRoomWorkflowPolicy.mockImplementation(
    async (_wsId: string, policy: RoomWorkflowPolicy) => policy,
  );
  mocks.listIssueWorkflows.mockResolvedValue({ workflows: WORKFLOWS });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RoomWorkflowPolicySetting", () => {
  it("defaults to No action and lets an admin switch to Default", async () => {
    const user = userEvent.setup();
    renderSetting(true);

    const modeSelect = screen.getByRole("combobox", {
      name: "Issues that are not requests of a flow squad",
    });
    expect(modeSelect).toHaveTextContent("No action");

    await user.click(modeSelect);
    await user.click(await screen.findByRole("option", { name: "Default" }));

    await waitFor(() => {
      expect(mocks.setRoomWorkflowPolicy).toHaveBeenCalledWith("ws-1", {
        kind: "default",
        workflowId: null,
      });
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Workflow setting saved");
  });

  it("lets an admin pick an existing workflow from the dropdown", async () => {
    const user = userEvent.setup();
    renderSetting(true);

    await waitFor(() => expect(mocks.listIssueWorkflows).toHaveBeenCalled());

    await user.click(
      screen.getByRole("combobox", {
        name: "Issues that are not requests of a flow squad",
      }),
    );
    await user.click(await screen.findByRole("option", { name: "Existing workflow" }));

    const workflowSelect = await screen.findByRole("combobox", { name: "Workflow" });
    await user.click(workflowSelect);
    await user.click(await screen.findByRole("option", { name: "Delivery" }));

    await waitFor(() => {
      expect(mocks.setRoomWorkflowPolicy).toHaveBeenCalledWith("ws-1", {
        kind: "workflow",
        workflowId: "wf-delivery",
      });
    });
  });

  it("renders read-only for a regular member", async () => {
    mocks.getRoomWorkflowPolicy.mockResolvedValue({
      kind: "workflow",
      workflowId: "wf-delivery",
    } satisfies RoomWorkflowPolicy);

    renderSetting(false);

    expect(
      screen.getByText("Issues that are not requests of a flow squad"),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText("Delivery")).toBeInTheDocument();
    });
    expect(screen.queryByRole("combobox")).toBeNull();
  });

  it("shows the admin-only error message when a save returns 403", async () => {
    const user = userEvent.setup();
    mocks.setRoomWorkflowPolicy.mockRejectedValueOnce(
      new ApiError("Forbidden", 403, "Forbidden"),
    );

    renderSetting(true);

    await user.click(
      screen.getByRole("combobox", {
        name: "Issues that are not requests of a flow squad",
      }),
    );
    await user.click(await screen.findByRole("option", { name: "Default" }));

    await waitFor(() => {
      expect(mocks.toastError).toHaveBeenCalledWith(
        "Only workspace admins can change this setting.",
      );
    });
  });
});
