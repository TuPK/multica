// @vitest-environment jsdom

import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentPack } from "@multica/core/agent-packs";
import type { Agent } from "@multica/core/types";
import enCommon from "../locales/en/common.json";
import enModals from "../locales/en/modals.json";
import enAgents from "../locales/en/agents.json";

const TEST_RESOURCES = {
  en: { common: enCommon, modals: enModals, agents: enAgents },
};

const pack: AgentPack = {
  id: "market-strategist",
  agent: {
    name: "Market Strategist",
    description: "Analyzes markets",
    instructions: "Do it.",
    max_concurrent_tasks: 2,
  },
  skills: [
    { name: "alpha", files: [{ path: "SKILL.md", content: "" }] },
    { name: "beta", files: [{ path: "SKILL.md", content: "" }] },
  ],
};

const mocks = vi.hoisted(() => ({
  agents: [] as Agent[],
  mutateAsync: vi.fn(),
  isPending: false,
  navigationPush: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (opts: { queryKey?: unknown[] }) => {
    const key = opts.queryKey ?? [];
    if (key.includes("agent-pack")) return { data: pack };
    if (key.includes("agents")) return { data: mocks.agents };
    if (key.includes("runtimes")) {
      return {
        data: [{ id: "rt-1", owner_id: "user-me", status: "online", visibility: "private" }],
        isLoading: false,
      };
    }
    return { data: [] };
  },
}));

vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({ queryKey: ["agents"] }),
  memberListOptions: () => ({ queryKey: ["members"] }),
}));

vi.mock("@multica/core/runtimes", () => ({
  runtimeListOptions: () => ({ queryKey: ["runtimes"] }),
  isRuntimeUsableForUser: () => true,
}));

vi.mock("@multica/core/agent-packs", async () => {
  const actual = await vi.importActual<typeof import("@multica/core/agent-packs")>(
    "@multica/core/agent-packs",
  );
  return {
    ...actual,
    useInstallAgentPack: () => ({
      mutateAsync: mocks.mutateAsync,
      get isPending() {
        return mocks.isPending;
      },
    }),
  };
});

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (s: { user: { id: string } | null }) => unknown) =>
    selector({ user: { id: "user-me" } }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ agentDetail: (id: string) => `/ws/agents/${id}` }),
}));
vi.mock("../navigation", () => ({ useNavigation: () => ({ push: mocks.navigationPush }) }));
vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));
vi.mock("../agents/components/runtime-picker", () => ({
  RuntimePicker: ({ onSelect }: { onSelect: (id: string) => void }) => (
    <button type="button" onClick={() => onSelect("rt-1")}>
      pick runtime
    </button>
  ),
}));

import { AgentPackInstallModal } from "./agent-pack-install";

function renderModal() {
  const onClose = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
  render(<AgentPackInstallModal onClose={onClose} />, { wrapper });
  return { onClose };
}

const readyOutcome = {
  skills: [
    { name: "alpha", id: "s1", status: "created" },
    { name: "beta", id: "s2", status: "kept" },
  ],
  agent: { id: "a1", status: "created" },
  attach: { status: "attached", count: 2 },
};

describe("AgentPackInstallModal", () => {
  beforeEach(() => {
    mocks.agents = [];
    mocks.isPending = false;
    mocks.mutateAsync.mockReset();
  });
  afterEach(cleanup);

  it("shows the pack and needs a runtime when the agent does not exist yet", async () => {
    renderModal();

    expect(screen.getByText("Market Strategist")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Install" })).toBeDisabled();

    await userEvent.setup().click(screen.getByRole("button", { name: "pick runtime" }));

    expect(screen.getByRole("button", { name: "Install" })).toBeEnabled();
  });

  it("installs the pack on the chosen runtime and reports the result", async () => {
    mocks.mutateAsync.mockResolvedValue(readyOutcome);
    renderModal();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "pick runtime" }));
    await user.click(screen.getByRole("button", { name: "Install" }));

    expect(mocks.mutateAsync).toHaveBeenCalledWith({ pack, runtimeId: "rt-1" });
    expect(await screen.findByText("Agent pack installed")).toBeInTheDocument();
    expect(screen.getByText("Already there, left unchanged")).toBeInTheDocument();
    expect(mocks.toastSuccess).toHaveBeenCalled();
  });

  it("asks for no runtime when the agent already exists", async () => {
    mocks.agents = [{ id: "a1", name: "Market Strategist", archived_at: null } as Agent];
    mocks.mutateAsync.mockResolvedValue({ ...readyOutcome, agent: { id: "a1", status: "kept" } });
    renderModal();

    expect(screen.queryByRole("button", { name: "pick runtime" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Install" }));

    expect(mocks.mutateAsync).toHaveBeenCalledWith({ pack, runtimeId: null });
  });

  it("shows what failed and offers to try again", async () => {
    mocks.mutateAsync.mockResolvedValueOnce({
      ...readyOutcome,
      skills: [{ name: "alpha", id: null, status: "failed", error: "413 too large" }, readyOutcome.skills[1]],
    });
    renderModal();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "pick runtime" }));
    await user.click(screen.getByRole("button", { name: "Install" }));

    expect(await screen.findByText("Finished with problems")).toBeInTheDocument();
    expect(screen.getByText("Failed: 413 too large")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});
