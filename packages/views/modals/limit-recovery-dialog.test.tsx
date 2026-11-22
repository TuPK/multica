import { useState } from "react";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { useModalStore } from "@multica/core/modals";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import enCommon from "../locales/en/common.json";
import enModals from "../locales/en/modals.json";

interface AvailableActions {
  checkout: boolean;
  portal: boolean;
  purchaseSeats: boolean;
}

const mockPush = vi.hoisted(() => vi.fn());
const mockCreatePortal = vi.hoisted(() => vi.fn());
const mockOpenExternal = vi.hoisted(() => vi.fn());
const mockSummaryQuery = vi.hoisted(() => vi.fn());
const featureState = vi.hoisted(() => ({ billingEnabled: true }));
const workspaceState = vi.hoisted(() => ({ id: "ws-test" }));
const summaryState = vi.hoisted(() => ({
  value: null as null | { availableActions: AvailableActions },
  error: null as Error | null,
  pending: null as Promise<{ availableActions: AvailableActions } | null> | null,
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => workspaceState.id,
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    settings: () => `/${workspaceState.id}/settings`,
  }),
}));

vi.mock("@multica/core/config", () => ({
  useFeatureEnabled: () => featureState.billingEnabled,
}));

vi.mock("../navigation/context", () => ({
  useNavigation: () => ({ push: mockPush }),
}));

vi.mock("../platform", () => ({
  openExternal: mockOpenExternal,
}));

vi.mock("@multica/core/billing", () => ({
  workspaceSubscriptionSummaryOptions: (wsId: string) => ({
    queryKey: ["workspace-subscriptions", wsId, "summary"],
    queryFn: mockSummaryQuery,
  }),
  useCreateWorkspaceSubscriptionPortal: () => ({
    mutateAsync: mockCreatePortal,
  }),
}));

import { LimitRecoveryDialog } from "./limit-recovery-dialog";
import { useLimitRecoveryPrompt } from "./use-limit-recovery-prompt";

const TEST_RESOURCES = {
  en: { common: enCommon, modals: enModals },
};

const actions = (
  overrides: Partial<AvailableActions> = {},
): AvailableActions => ({
  checkout: false,
  portal: false,
  purchaseSeats: false,
  ...overrides,
});

function PromptHarness() {
  const showPrompt = useLimitRecoveryPrompt();
  return (
    <>
      <button type="button" onClick={showPrompt}>
        Show recovery
      </button>
      <LimitRecoveryDialog />
    </>
  );
}

function LayeredPromptHarness() {
  const [createOpen, setCreateOpen] = useState(true);
  const showPrompt = useLimitRecoveryPrompt();

  return (
    <>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent showCloseButton={false}>
          <DialogTitle>Create an issue</DialogTitle>
          <DialogDescription>Your draft remains here.</DialogDescription>
          <button type="button" onClick={showPrompt}>
            Show recovery
          </button>
        </DialogContent>
      </Dialog>
      <LimitRecoveryDialog />
    </>
  );
}

function promptTree(client: QueryClient, layered = false) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={client}>
        {layered ? <LayeredPromptHarness /> : <PromptHarness />}
      </QueryClientProvider>
    </I18nProvider>
  );
}

function renderPrompt({ layered = false }: { layered?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: 1 } },
  });
  const view = render(promptTree(client, layered));
  return { client, ...view };
}

async function openPrompt() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Show recovery" }));
  return user;
}

function queryRecoveryDialog() {
  return screen.queryByRole("dialog", {
    name: "This workspace has reached its issue limit",
  });
}

describe("LimitRecoveryDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    featureState.billingEnabled = true;
    workspaceState.id = "ws-test";
    summaryState.value = null;
    summaryState.error = null;
    summaryState.pending = null;
    useModalStore.setState({
      modal: null,
      data: null,
      limitRecoveryWorkspaceId: null,
      limitRecoveryReason: "issue_limit",
    });
    mockSummaryQuery.mockImplementation(async () => {
      if (summaryState.pending) return summaryState.pending;
      if (summaryState.error) throw summaryState.error;
      return summaryState.value;
    });
    mockCreatePortal.mockResolvedValue({
      url: "https://billing.example/portal",
    });
  });

  afterEach(() => {
    cleanup();
    useModalStore.setState({
      modal: null,
      data: null,
      limitRecoveryWorkspaceId: null,
      limitRecoveryReason: "issue_limit",
    });
  });

  it("opens immediately as a centered recovery dialog with one close action", async () => {
    summaryState.pending = new Promise<{
      availableActions: AvailableActions;
    } | null>(() => undefined);
    renderPrompt();

    await openPrompt();

    expect(queryRecoveryDialog()).toBeInTheDocument();
    expect(
      screen.getByText(
        "Checking the billing actions available for this workspace...",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);
  });

  it("uses autopilot-specific copy when Inbox opens the shared recovery surface", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt();

    act(() => {
      useModalStore
        .getState()
        .showLimitRecovery("ws-test", "autopilot_quota");
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its autopilot run limit",
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Upgrade to Pro to raise the monthly limit and restore autopilot runs, or wait for the allowance to reset."),
    ).toBeInTheDocument();

    act(() => useModalStore.getState().dismissLimitRecovery());
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", {
          name: "This workspace has reached its autopilot run limit",
        }),
      ).not.toBeInTheDocument();
    });
  });

  it("closes only recovery when Escape is pressed above a create dialog", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt({ layered: true });
    const user = await openPrompt();

    await user.keyboard("{Escape}");

    await waitFor(() => expect(queryRecoveryDialog()).not.toBeInTheDocument());
    expect(
      screen.getByRole("dialog", { name: "Create an issue" }),
    ).toBeInTheDocument();
  });

  it("closes only recovery when its backdrop is pressed", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt({ layered: true });
    const user = await openPrompt();
    const overlays = document.querySelectorAll<HTMLElement>(
      '[data-slot="dialog-overlay"]',
    );

    expect(overlays).toHaveLength(2);
    await user.click(overlays[overlays.length - 1]!);

    await waitFor(() => expect(queryRecoveryDialog()).not.toBeInTheDocument());
    expect(
      screen.getByRole("dialog", { name: "Create an issue" }),
    ).toBeInTheDocument();
  });

  it("stays dismissed when the Cloud response arrives later", async () => {
    let resolveSummary!: (
      value: { availableActions: AvailableActions } | null,
    ) => void;
    summaryState.pending = new Promise((resolve) => {
      resolveSummary = resolve;
    });
    renderPrompt();
    const user = await openPrompt();

    await user.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(queryRecoveryDialog()).not.toBeInTheDocument());

    await act(async () => {
      resolveSummary({ availableActions: actions({ checkout: true }) });
      await summaryState.pending;
    });

    expect(queryRecoveryDialog()).not.toBeInTheDocument();
  });

  it("keeps the create modal open when recovery is merely dismissed", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    useModalStore.getState().open("create-issue");
    renderPrompt();
    const user = await openPrompt();

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(useModalStore.getState().modal).toBe("create-issue");
  });

  it("dismisses recovery instead of carrying it into another workspace", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    const { client, rerender } = renderPrompt();
    await openPrompt();

    workspaceState.id = "ws-other";
    rerender(promptTree(client));

    await waitFor(() => {
      expect(
        useModalStore.getState().limitRecoveryWorkspaceId,
      ).toBeNull();
    });
    expect(queryRecoveryDialog()).not.toBeInTheDocument();
  });

  it("offers Upgrade to Pro only when Cloud authorizes checkout", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    useModalStore.getState().open("create-issue");
    renderPrompt();
    const user = await openPrompt();

    await user.click(
      await screen.findByRole("button", { name: "Upgrade to Pro" }),
    );

    expect(useModalStore.getState().modal).toBeNull();
    expect(mockPush).toHaveBeenCalledWith("/ws-test/settings?tab=billing");
    await waitFor(() => expect(queryRecoveryDialog()).not.toBeInTheDocument());
  });

  it("opens Billing Portal for a past-due manager authorized for portal", async () => {
    summaryState.value = { availableActions: actions({ portal: true }) };
    useModalStore.getState().open("quick-create-issue");
    renderPrompt();
    const user = await openPrompt();

    await user.click(
      await screen.findByRole("button", { name: "Open Billing Portal" }),
    );

    await waitFor(() => expect(mockCreatePortal).toHaveBeenCalledTimes(1));
    expect(mockCreatePortal.mock.calls[0]?.[0]).toMatch(
      /^issue-limit-portal-ws-test-/,
    );
    await waitFor(() => {
      expect(mockOpenExternal).toHaveBeenCalledWith(
        "https://billing.example/portal",
        { webTarget: "same-tab" },
      );
    });
    expect(useModalStore.getState().modal).toBeNull();
  });

  it("keeps a Billing recovery action when Portal cannot be opened", async () => {
    summaryState.value = { availableActions: actions({ portal: true }) };
    useModalStore.getState().open("create-issue");
    mockCreatePortal.mockRejectedValue(new Error("portal unavailable"));
    renderPrompt();
    const user = await openPrompt();

    await user.click(
      await screen.findByRole("button", { name: "Open Billing Portal" }),
    );

    expect(
      await screen.findByRole("button", { name: "View Billing" }),
    ).toBeInTheDocument();
    expect(useModalStore.getState().modal).toBe("create-issue");
    expect(queryRecoveryDialog()).toBeInTheDocument();
  });

  it("asks for an administrator only when Cloud authorizes no management action", async () => {
    summaryState.value = { availableActions: actions() };
    renderPrompt();
    await openPrompt();

    expect(
      await screen.findByText(
        "Ask a workspace owner or admin to upgrade to Pro.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /Upgrade|Billing Portal|View Billing/,
      }),
    ).not.toBeInTheDocument();
  });

  it("keeps another Cloud-authorized management action reachable in Billing", async () => {
    summaryState.value = {
      availableActions: actions({ purchaseSeats: true }),
    };
    renderPrompt();
    await openPrompt();

    expect(
      await screen.findByRole("button", { name: "View Billing" }),
    ).toBeInTheDocument();
  });

  it("uses one background attempt and keeps Billing as the recovery path", async () => {
    summaryState.error = new Error("cloud unavailable");
    renderPrompt();
    await openPrompt();

    expect(
      await screen.findByRole("button", { name: "View Billing" }),
    ).toBeInTheDocument();
    expect(mockSummaryQuery).toHaveBeenCalledTimes(1);
  });

  it("does not expose a dead Billing link when the Billing surface is disabled", async () => {
    featureState.billingEnabled = false;
    renderPrompt();
    await openPrompt();

    expect(
      screen.getByText(
        "Delete an existing issue to free space, or contact your workspace administrator.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "View Billing" }),
    ).not.toBeInTheDocument();
    expect(mockSummaryQuery).not.toHaveBeenCalled();
  });

  it("opens upgrade dialog with agent-specific copy for agents gate", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt();

    act(() => {
      useModalStore.getState().showLimitRecovery("ws-test", "agents" as any);
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its agent limit",
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Upgrade to Pro to create more agents."),
    ).toBeInTheDocument();
  });

  it("opens upgrade dialog with runtime-specific copy for runtimes gate", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt();

    act(() => {
      useModalStore.getState().showLimitRecovery("ws-test", "runtimes" as any);
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its runtime limit",
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Upgrade to Pro to add more runtimes."),
    ).toBeInTheDocument();
  });

  it("opens upgrade dialog with storage-specific copy for storage_bytes gate", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };
    renderPrompt();

    act(() => {
      useModalStore
        .getState()
        .showLimitRecovery("ws-test", "storage_bytes" as any);
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its storage limit",
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Upgrade to Pro to get more storage."),
    ).toBeInTheDocument();
  });

  it("opens dialog when prompt is invoked with plan_limit_reached error for supported gates, and not for unknown gates", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };

    let promptFn!: (errorOrReason?: unknown) => boolean;
    function CustomHarness() {
      promptFn = useLimitRecoveryPrompt();
      return <LimitRecoveryDialog />;
    }

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <QueryClientProvider client={client}>
          <CustomHarness />
        </QueryClientProvider>
      </I18nProvider>,
    );

    // 1. Unknown gate does not open the dialog
    act(() => {
      const opened = promptFn({
        code: "plan_limit_reached",
        gate: "unknown_gate",
        limit: 10,
        used: 10,
      });
      expect(opened).toBe(false);
    });
    expect(
      screen.queryByRole("dialog", {
        name: /reached its.*limit/,
      }),
    ).not.toBeInTheDocument();

    // 2. Agents gate opens dialog
    act(() => {
      const opened = promptFn({
        code: "plan_limit_reached",
        gate: "agents",
        limit: 5,
        used: 5,
      });
      expect(opened).toBe(true);
    });
    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its agent limit",
      }),
    ).toBeInTheDocument();

    // Dismiss
    act(() => useModalStore.getState().dismissLimitRecovery());
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", {
          name: "This workspace has reached its agent limit",
        }),
      ).not.toBeInTheDocument();
    });

    // 3. Runtimes gate in error.body opens dialog
    act(() => {
      const opened = promptFn({
        body: {
          code: "plan_limit_reached",
          gate: "runtimes",
          limit: 3,
          used: 3,
        },
      });
      expect(opened).toBe(true);
    });
    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its runtime limit",
      }),
    ).toBeInTheDocument();

    // Dismiss
    act(() => useModalStore.getState().dismissLimitRecovery());
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", {
          name: "This workspace has reached its runtime limit",
        }),
      ).not.toBeInTheDocument();
    });

    // 4. Storage bytes gate opens dialog
    act(() => {
      const opened = promptFn({
        code: "plan_limit_reached",
        gate: "storage_bytes",
        limit: 1024,
        used: 1024,
      });
      expect(opened).toBe(true);
    });
    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its storage limit",
      }),
    ).toBeInTheDocument();
  });

  it("handles ApiError instances with plan_limit_reached body", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };

    let promptFn!: (errorOrReason?: unknown) => boolean;
    function ApiErrorHarness() {
      promptFn = useLimitRecoveryPrompt();
      return <LimitRecoveryDialog />;
    }

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <QueryClientProvider client={client}>
          <ApiErrorHarness />
        </QueryClientProvider>
      </I18nProvider>,
    );

    act(() => {
      const opened = promptFn({
        name: "ApiError",
        status: 402,
        statusText: "Payment Required",
        message: "plan limit reached",
        body: {
          code: "plan_limit_reached",
          gate: "agents",
          limit: 3,
          used: 3,
        },
      });
      expect(opened).toBe(true);
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its agent limit",
      }),
    ).toBeInTheDocument();
  });

  it("handles parsed errors carrying gate, limit, and used directly on the error", async () => {
    summaryState.value = { availableActions: actions({ checkout: true }) };

    let promptFn!: (errorOrReason?: unknown) => boolean;
    function ParsedErrorHarness() {
      promptFn = useLimitRecoveryPrompt();
      return <LimitRecoveryDialog />;
    }

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <QueryClientProvider client={client}>
          <ParsedErrorHarness />
        </QueryClientProvider>
      </I18nProvider>,
    );

    // Kiro's parsed error with code, gate, limit, used fields
    act(() => {
      const parsedError = Object.assign(new Error("plan limit reached"), {
        code: "plan_limit_reached",
        gate: "runtimes",
        limit: 2,
        used: 2,
      });
      const opened = promptFn(parsedError);
      expect(opened).toBe(true);
    });

    expect(
      await screen.findByRole("dialog", {
        name: "This workspace has reached its runtime limit",
      }),
    ).toBeInTheDocument();
  });

  it("rejects unknown gates when configured as defaultReason", () => {
    let promptFn!: (errorOrReason?: unknown) => boolean;
    function UnknownHarness() {
      promptFn = useLimitRecoveryPrompt("unknown_gate" as any);
      return <LimitRecoveryDialog />;
    }

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <QueryClientProvider client={client}>
          <UnknownHarness />
        </QueryClientProvider>
      </I18nProvider>,
    );

    act(() => {
      const opened = promptFn();
      expect(opened).toBe(false);
    });

    expect(
      screen.queryByRole("dialog", {
        name: /reached its.*limit/,
      }),
    ).not.toBeInTheDocument();
  });

  it("displays billing disabled descriptions for the new gates", async () => {
    featureState.billingEnabled = false;
    renderPrompt();

    // 1. Agents gate with billing disabled
    act(() => {
      useModalStore.getState().showLimitRecovery("ws-test", "agents" as any);
    });
    expect(
      await screen.findByText(
        "Archive an existing agent to free space, or contact your workspace administrator.",
      ),
    ).toBeInTheDocument();

    // 2. Runtimes gate with billing disabled
    act(() => {
      useModalStore.getState().showLimitRecovery("ws-test", "runtimes" as any);
    });
    expect(
      await screen.findByText(
        "Remove an existing runtime to free space, or contact your workspace administrator.",
      ),
    ).toBeInTheDocument();

    // 3. Storage bytes gate with billing disabled
    act(() => {
      useModalStore.getState().showLimitRecovery("ws-test", "storage_bytes" as any);
    });
    expect(
      await screen.findByText(
        "Delete existing files to free space, or contact your workspace administrator.",
      ),
    ).toBeInTheDocument();
  });
});
