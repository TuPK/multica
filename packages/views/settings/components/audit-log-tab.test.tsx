// @vitest-environment jsdom

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";

const currentMemberState = vi.hoisted(() => ({
  role: "owner" as "owner" | "admin" | "member",
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme" }),
}));

vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({
    role: currentMemberState.role,
    isLoading: false,
  }),
}));

const mockAuditState = vi.hoisted(() => ({
  isError: false,
  pages: [
    {
      events: [
        {
          id: "evt-1",
          workspace_id: "ws-1",
          actor_type: "member",
          actor_id: "usr-1",
          actor_label: "alice@example.com",
          action: "member.role_changed",
          target_type: "member",
          target_id: "usr-2",
          target_label: "bob@example.com",
          detail: { old_role: "member", new_role: "admin" },
          ip: "192.168.1.1",
          user_agent: "Mozilla/5.0",
          request_id: "req-1",
          created_at: "2026-11-15T12:00:00Z",
        },
        {
          id: "evt-2",
          workspace_id: "ws-1",
          actor_type: "member",
          actor_id: "usr-1",
          actor_label: "alice@example.com",
          action: "invitation.created",
          target_type: "invitation",
          target_id: "inv-1",
          target_label: "carol@example.com",
          detail: {},
          ip: "192.168.1.1",
          user_agent: "Mozilla/5.0",
          request_id: "req-2",
          created_at: "2026-11-14T15:30:00Z",
        },
      ],
      next_cursor: "cursor-page-2" as string | null,
    },
  ],
  secondPage: {
    events: [
      {
        id: "evt-3",
        workspace_id: "ws-1",
        actor_type: "member",
        actor_id: "usr-1",
        actor_label: "alice@example.com",
        action: "workspace.updated",
        target_type: "workspace",
        target_id: "ws-1",
        target_label: "Acme",
        detail: {},
        ip: "192.168.1.1",
        user_agent: "Mozilla/5.0",
        request_id: "req-3",
        created_at: "2026-11-13T09:00:00Z",
      },
    ],
    next_cursor: null as string | null,
  },
}));

const mockExportCsv = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/audit", () => ({
  AUDIT_ACTIONS: [
    "member.role_changed",
    "member.removed",
    "member.left",
    "invitation.created",
    "invitation.revoked",
    "invitation.accepted",
    "invitation.declined",
    "share_link.created",
    "share_link.revoked",
    "share_link.joined",
    "workspace.created",
    "workspace.updated",
    "workspace.deleted",
    "auth.login",
    "auth.sessions_revoked",
    "pat.created",
    "pat.revoked",
  ],
  auditEventsOptions: (wsId: string, filters: unknown) => ({
    queryKey: ["audit-events", wsId, filters],
    queryFn: ({ pageParam }: { pageParam?: unknown }) => {
      if (mockAuditState.isError) {
        return Promise.reject(new Error("Failed to load audit events"));
      }
      if (pageParam === "cursor-page-2") {
        return Promise.resolve(mockAuditState.secondPage);
      }
      return Promise.resolve(mockAuditState.pages[0]);
    },
    initialPageParam: undefined,
    getNextPageParam: (lastPage: { next_cursor: string | null }) =>
      lastPage.next_cursor ?? undefined,
  }),
  exportAuditCsv: mockExportCsv,
}));

const mockToast = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: mockToast,
}));

import { AuditLogTab } from "./audit-log-tab";

function renderAuditLogTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <AuditLogTab />
    </QueryClientProvider>,
  );
}

describe("AuditLogTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    currentMemberState.role = "owner";
    mockAuditState.isError = false;
    mockAuditState.pages[0]!.next_cursor = "cursor-page-2";
    mockAuditState.pages[0]!.events = [
      {
        id: "evt-1",
        workspace_id: "ws-1",
        actor_type: "member",
        actor_id: "usr-1",
        actor_label: "alice@example.com",
        action: "member.role_changed",
        target_type: "member",
        target_id: "usr-2",
        target_label: "bob@example.com",
        detail: { old_role: "member", new_role: "admin" },
        ip: "192.168.1.1",
        user_agent: "Mozilla/5.0",
        request_id: "req-1",
        created_at: "2026-11-15T12:00:00Z",
      },
      {
        id: "evt-2",
        workspace_id: "ws-1",
        actor_type: "member",
        actor_id: "usr-1",
        actor_label: "alice@example.com",
        action: "invitation.created",
        target_type: "invitation",
        target_id: "inv-1",
        target_label: "carol@example.com",
        detail: {},
        ip: "192.168.1.1",
        user_agent: "Mozilla/5.0",
        request_id: "req-2",
        created_at: "2026-11-14T15:30:00Z",
      },
    ];
    mockExportCsv.mockResolvedValue({
      text: "time,actor,action,target,ip\n",
      truncated: false,
    });
  });

  it("renders table headers, filters, and export button", async () => {
    renderAuditLogTab();

    expect(
      screen.getByRole("button", { name: "Export CSV" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Action" }),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText("From"),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText("To"),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("Time")).toBeInTheDocument();
      expect(screen.getByText("Actor")).toBeInTheDocument();
      expect(screen.getByText("Action")).toBeInTheDocument();
      expect(screen.getByText("Target")).toBeInTheDocument();
      expect(screen.getByText("IP")).toBeInTheDocument();
    });
  });

  it("displays events with monospace action catalog codes", async () => {
    renderAuditLogTab();

    await waitFor(() => {
      expect(screen.getByText("member.role_changed")).toBeInTheDocument();
      expect(screen.getByText("invitation.created")).toBeInTheDocument();
    });

    const actionCode = screen.getByText("member.role_changed");
    expect(actionCode.tagName.toLowerCase()).toBe("code");
    expect(actionCode.className).toContain("font-mono");

    expect(screen.getAllByText("alice@example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
    expect(screen.getByText("carol@example.com")).toBeInTheDocument();
    expect(screen.getAllByText("192.168.1.1").length).toBeGreaterThan(0);
  });

  it("allows selecting action filter", async () => {
    renderAuditLogTab();

    const select = screen.getByRole("combobox", { name: "Action" });
    expect(select).toBeInTheDocument();
    expect(select).toHaveTextContent("All actions");
  });

  it("updates from and to date filter values", () => {
    renderAuditLogTab();

    const fromInput = screen.getByLabelText("From");
    const toInput = screen.getByLabelText("To");

    fireEvent.change(fromInput, { target: { value: "2026-11-01" } });
    expect(fromInput).toHaveValue("2026-11-01");

    fireEvent.change(toInput, { target: { value: "2026-11-15" } });
    expect(toInput).toHaveValue("2026-11-15");
  });

  it("renders load more button and fetches next page", async () => {
    renderAuditLogTab();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Load more" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));

    await waitFor(() => {
      expect(screen.getByText("workspace.updated")).toBeInTheDocument();
    });
  });

  it("calls export helper and creates download when clicking Export CSV", async () => {
    renderAuditLogTab();

    const exportBtn = screen.getByRole("button", { name: "Export CSV" });
    fireEvent.click(exportBtn);

    await waitFor(() => {
      expect(mockExportCsv).toHaveBeenCalledWith("ws-1", {});
    });
  });

  it("shows error toast when export fails", async () => {
    mockExportCsv.mockRejectedValueOnce(new Error("Export network error"));
    renderAuditLogTab();

    const exportBtn = screen.getByRole("button", { name: "Export CSV" });
    fireEvent.click(exportBtn);

    await waitFor(() => {
      expect(mockToast.error).toHaveBeenCalledWith(
        "Failed to export audit events",
      );
    });
  });

  it("displays empty state when there are no events", async () => {
    mockAuditState.pages[0]!.events = [];
    mockAuditState.pages[0]!.next_cursor = null;
    renderAuditLogTab();

    await waitFor(() => {
      expect(screen.getByText("No audit events")).toBeInTheDocument();
    });
  });

  it("displays error state when audit query fails", async () => {
    mockAuditState.isError = true;
    renderAuditLogTab();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load audit events"),
      ).toBeInTheDocument();
    });
  });

  it("renders read only notice when user is member", () => {
    currentMemberState.role = "member";
    renderAuditLogTab();

    expect(screen.getByRole("note")).toBeInTheDocument();
    expect(screen.getByText("View only")).toBeInTheDocument();
  });
});
