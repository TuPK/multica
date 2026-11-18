// @vitest-environment jsdom
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { Agent, Skill } from "../types";
import { useCurrentMember } from "./use-current-member";
import { useAgentPermissions, useSkillPermissions } from "./use-resource-permissions";
import { useCan } from "./use-can";

const mockListMembers = vi.hoisted(() => vi.fn());
const mockGetMyPermissions = vi.hoisted(() => vi.fn());
const authState = vi.hoisted(() => ({
  user: { id: "user-1" } as { id: string } | null,
}));

vi.mock("../api", () => ({
  api: {
    listMembers: mockListMembers,
    getMyPermissions: mockGetMyPermissions,
  },
}));

vi.mock("../auth", () => {
  const useAuthStore = Object.assign(
    (selector?: (s: typeof authState) => unknown) =>
      selector ? selector(authState) : authState,
    { getState: () => authState },
  );
  return { useAuthStore };
});

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);
}

const OWNED_AGENT: Agent = {
  id: "agent-1",
  workspace_id: "ws-1",
  runtime_id: "rt-1",
  name: "Agent One",
  description: "",
  instructions: "",
  avatar_url: null,
  runtime_mode: "local",
  runtime_config: {},
  custom_args: [],
  visibility: "workspace",
  permission_mode: "public_to",
  invocation_targets: [{ target_type: "workspace", target_id: null }],
  status: "idle",
  max_concurrent_tasks: 1,
  model: "default",
  owner_id: "user-1",
  skills: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  archived_at: null,
  archived_by: null,
};

const OWNED_SKILL: Skill = {
  id: "skill-1",
  workspace_id: "ws-1",
  name: "Skill One",
  description: "",
  content: "",
  config: {},
  files: [],
  created_by: "user-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  authState.user = { id: "user-1" };
  mockListMembers.mockResolvedValue([]);
  mockGetMyPermissions.mockResolvedValue({ role: "member", permissions: [] });
});

afterEach(() => {
  cleanup();
});

describe("useCurrentMember", () => {
  it("returns isGuest true when the current member has is_guest true", async () => {
    mockListMembers.mockResolvedValue([
      {
        id: "m-1",
        workspace_id: "ws-1",
        user_id: "user-1",
        role: "member",
        name: "Guest User",
        email: "guest@acme.dev",
        avatar_url: null,
        created_at: "2026-01-01T00:00:00Z",
        is_guest: true,
      },
    ]);

    const { result } = renderHook(() => useCurrentMember("ws-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.member).not.toBeNull());
    expect(result.current.userId).toBe("user-1");
    expect(result.current.role).toBe("member");
    expect(result.current.isGuest).toBe(true);
  });

  it("returns isGuest false when is_guest is absent from an older response", async () => {
    mockListMembers.mockResolvedValue([
      {
        id: "m-1",
        workspace_id: "ws-1",
        user_id: "user-1",
        role: "member",
        name: "Legacy Member",
        email: "legacy@acme.dev",
        avatar_url: null,
        created_at: "2026-01-01T00:00:00Z",
      },
    ]);

    const { result } = renderHook(() => useCurrentMember("ws-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.member).not.toBeNull());
    expect(result.current.role).toBe("member");
    expect(result.current.isGuest).toBe(false);
  });

  it("returns isGuest false when there is no current member", async () => {
    mockListMembers.mockResolvedValue([
      {
        id: "m-2",
        workspace_id: "ws-1",
        user_id: "user-2",
        role: "member",
        name: "Other User",
        email: "other@acme.dev",
        avatar_url: null,
        created_at: "2026-01-01T00:00:00Z",
        is_guest: true,
      },
    ]);

    const { result } = renderHook(() => useCurrentMember("ws-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.member).toBeNull();
    expect(result.current.role).toBeNull();
    expect(result.current.isGuest).toBe(false);
  });
});

describe("permission hooks guest write denial", () => {
  it("denies resource and capability writes for a guest member", async () => {
    mockListMembers.mockResolvedValue([
      {
        id: "m-1",
        workspace_id: "ws-1",
        user_id: "user-1",
        role: "member",
        name: "Guest Member",
        email: "guest@acme.dev",
        avatar_url: null,
        created_at: "2026-01-01T00:00:00Z",
        is_guest: true,
      },
    ]);
    mockGetMyPermissions.mockResolvedValue({
      role: "member",
      permissions: ["members.invite", "audit.read"],
    });

    const { result } = renderHook(
      () => ({
        agent: useAgentPermissions(OWNED_AGENT, "ws-1"),
        skill: useSkillPermissions(OWNED_SKILL, "ws-1"),
        canInviteWrite: useCan("ws-1", "members.invite", { write: true }),
        canAuditRead: useCan("ws-1", "audit.read"),
      }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.agent.isLoading).toBe(false));
    await waitFor(() => expect(result.current.canAuditRead.allowed).toBe(true));

    expect(result.current.agent.canEdit.allowed).toBe(false);
    expect(result.current.agent.canAssign.allowed).toBe(false);
    expect(result.current.skill.canEdit.allowed).toBe(false);
    expect(result.current.skill.canDelete.allowed).toBe(false);
    expect(result.current.canInviteWrite.allowed).toBe(false);
  });
});
