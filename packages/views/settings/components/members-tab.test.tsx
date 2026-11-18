import { useSyncExternalStore, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { configStore } from "@multica/core/config";
import { BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG } from "@multica/core/feature-flags";
import { renderWithI18n } from "../../test/i18n";

const mockCreateMember = vi.hoisted(() => vi.fn());
const mockUpdateMember = vi.hoisted(() => vi.fn());
// The URL the settings router holds; `setSearch` stands in for navigation.
const url = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const state = { search: "" };
  return {
    get: () => state.search,
    set: (search: string) => {
      state.search = search;
      listeners.forEach((listener) => listener());
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
});
const data = vi.hoisted(() => ({
  // The caller's keys from permissions/me (spec 7b.2). Empty by default.
  myPermissions: { role: "member", permissions: [] as string[] } as {
    role: string;
    permissions: string[];
  },
  groups: [] as Array<Record<string, unknown>>,
  members: [] as Array<Record<string, unknown>>,
  invitations: [] as Array<Record<string, unknown>>,
  shareLinks: [] as Array<Record<string, unknown>>,
  projects: [] as Array<Record<string, unknown>>,
  summary: null as Record<string, unknown> | null,
}));

const mockUseQuery = vi.hoisted(() =>
  vi.fn((opts: { queryKey: unknown[]; enabled?: boolean }) => {
    const key = JSON.stringify(opts.queryKey);
    if (opts.enabled === false) return { data: undefined };
    if (key.includes('"permissions"')) return { data: data.myPermissions };
    if (key.includes('"groups"')) return { data: data.groups };
    if (key.includes('"projects"')) return { data: data.projects };
    if (key.includes("invitations")) return { data: data.invitations };
    if (key.includes("share-links")) return { data: data.shareLinks };
    if (key.includes("members")) return { data: data.members };
    return { data: data.summary };
  }),
);

vi.mock("@tanstack/react-query", () => ({
  queryOptions: <T,>(opts: T) => opts,
  useQuery: (opts: { queryKey: unknown[]; enabled?: boolean }) => mockUseQuery(opts),
  useMutation: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useQueryClient: () => ({
    invalidateQueries: vi.fn(),
    fetchQuery: vi.fn(),
  }),
}));
vi.mock("@multica/core/billing", () => ({
  usePreviewWorkspaceSeatPurchase: () => ({ mutateAsync: vi.fn() }),
  usePurchaseWorkspaceSeats: () => ({ mutateAsync: vi.fn() }),
  workspaceSubscriptionSummaryOptions: (wsId: string) => ({
    queryKey: ["billing", wsId, "summary"],
  }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme", slug: "acme" }),
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { createMember: mockCreateMember, updateMember: mockUpdateMember },
}));
vi.mock("@multica/core/auth", () => {
  const state = { user: { id: "user-1" } };
  const useAuthStore = Object.assign(
    (selector?: (s: typeof state) => unknown) => (selector ? selector(state) : state),
    { getState: () => state },
  );
  return { useAuthStore };
});
vi.mock("../../common/actor-avatar", () => ({ ActorAvatar: () => <span /> }));
vi.mock("../../navigation", () => ({
  AppLink: ({ href, children, ...rest }: { href: string; children: ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
  useOptionalNavigation: () => {
    const search = useSyncExternalStore(url.subscribe, url.get);
    const go = (href: string) => url.set(href.split("?")[1] ?? "");
    return {
      pathname: "/acme/settings",
      searchParams: new URLSearchParams(search),
      push: go,
      replace: go,
    };
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { MembersTab } from "./members-tab";

const member = (id: string, name: string, role: string) => ({
  id: `m-${id}`,
  user_id: id,
  workspace_id: "ws-1",
  role,
  name,
  email: `${name.toLowerCase().split(" ")[0]}@acme.dev`,
  avatar_url: null,
  created_at: "2025-01-08T00:00:00Z",
});

beforeEach(() => {
  vi.clearAllMocks();
  url.set("tab=members");
  data.myPermissions = {
    role: "owner",
    permissions: [
      "members.invite",
      "audit.read",
      "billing.view",
      "billing.manage",
      "integrations.manage",
      "runtime_profiles.manage",
    ],
  };
  data.members = [
    member("user-1", "Ada Lovelace", "owner"),
    member("user-2", "Grace Hopper", "admin"),
    member("user-3", "Alan Turing", "member"),
  ];
  data.invitations = [
    {
      id: "inv-1",
      workspace_id: "ws-1",
      inviter_id: "user-1",
      invitee_email: "linus@acme.dev",
      invitee_user_id: null,
      role: "member",
      status: "pending",
      created_at: "2025-03-01T00:00:00Z",
      updated_at: "2025-03-01T00:00:00Z",
      expires_at: "2025-03-08T00:00:00Z",
    },
  ];
  data.shareLinks = [];
  data.projects = [];
  data.summary = null;
  configStore.getState().setFeatureFlags({});
});

describe("MembersTab", () => {
  it("lists members with their role and marks the current user", () => {
    renderWithI18n(<MembersTab />);

    expect(screen.getByRole("tab", { name: /Members\s*3/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.getByText("You")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Actions for Grace Hopper" })).toBeInTheDocument();
    // Nobody manages their own membership from this list.
    expect(screen.queryByRole("button", { name: "Actions for Ada Lovelace" })).toBeNull();
  });

  it("filters members by name or email", () => {
    renderWithI18n(<MembersTab />);
    const search = screen.getByRole("searchbox", { name: "Search name or email" });

    fireEvent.change(search, { target: { value: "grace@" } });
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    expect(screen.queryByText("Alan Turing")).toBeNull();

    fireEvent.change(search, { target: { value: "nobody" } });
    expect(screen.getByText("No members match this search.")).toBeInTheDocument();
  });

  it("shows when each pending invitation was sent and expires", async () => {
    const user = userEvent.setup();
    renderWithI18n(<MembersTab />);

    await user.click(screen.getByRole("tab", { name: /Pending invitations\s*1/ }));
    expect(screen.getByText("linus@acme.dev")).toBeInTheDocument();
    expect(screen.getByText(/Sent .* · Expires /)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Revoke invitation for linus@acme.dev" }),
    ).toBeInTheDocument();
  });

  it("keeps the open list in the URL and follows it while on the page", async () => {
    const user = userEvent.setup();
    renderWithI18n(<MembersTab />);

    await user.click(screen.getByRole("tab", { name: /Pending invitations\s*1/ }));
    expect(url.get()).toBe("tab=members&section=invitations");

    // A search result or back/forward changes the URL without remounting.
    act(() => url.set("tab=members&section=links"));
    expect(screen.getByRole("tab", { name: /Share links\s*0/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    act(() => url.set("tab=members"));
    expect(screen.getByRole("tab", { name: /Members\s*3/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("opens the tab a deep link names", () => {
    url.set("tab=members&section=links");
    renderWithI18n(<MembersTab />);
    expect(screen.getByRole("tab", { name: /Share links\s*0/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("No share links yet.")).toBeInTheDocument();
  });

  it("invites from a dialog and closes it once the invitation is sent", async () => {
    mockCreateMember.mockResolvedValue({});
    const user = userEvent.setup();
    renderWithI18n(<MembersTab />);

    await user.click(screen.getByRole("button", { name: "Invite member" }));
    const dialog = await screen.findByRole("dialog", { name: "Invite member" });
    await user.type(within(dialog).getByRole("textbox", { name: "Email" }), "new@acme.dev");
    await user.click(within(dialog).getByRole("button", { name: "Invite" }));

    await waitFor(() =>
      expect(mockCreateMember).toHaveBeenCalledWith("ws-1", {
        email: "new@acme.dev",
        role: "member",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("keeps the invite dialog open when sending fails", async () => {
    mockCreateMember.mockRejectedValue(new Error("nope"));
    const user = userEvent.setup();
    renderWithI18n(<MembersTab />);

    await user.click(screen.getByRole("button", { name: "Invite member" }));
    const dialog = await screen.findByRole("dialog", { name: "Invite member" });
    await user.type(within(dialog).getByRole("textbox", { name: "Email" }), "new@acme.dev");
    await user.click(within(dialog).getByRole("button", { name: "Invite" }));

    await waitFor(() => expect(mockCreateMember).toHaveBeenCalled());
    expect(screen.getByRole("dialog", { name: "Invite member" })).toBeInTheDocument();
  });

  it("links seat usage to billing when subscriptions are on", () => {
    configStore
      .getState()
      .setFeatureFlags({ [BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG]: true });
    data.summary = { seatCapacity: { used: 3, reserved: 1, purchased: 5 } };
    renderWithI18n(<MembersTab />);

    expect(screen.getByText(/4 of 5 seats used/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage seats" })).toHaveAttribute(
      "href",
      "/acme/settings?tab=billing",
    );
  });

  it("gives members a read-only view without invite or share-link controls", () => {
    data.myPermissions = { role: "member", permissions: [] };
    data.members = [
      member("user-1", "Ada Lovelace", "member"),
      member("user-2", "Grace Hopper", "owner"),
    ];
    renderWithI18n(<MembersTab />);

    expect(screen.getByRole("note")).toHaveTextContent("Ask Grace Hopper");
    expect(screen.queryByRole("button", { name: "Invite member" })).toBeNull();
    expect(screen.queryByRole("tab", { name: /Share links/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Actions for/ })).toBeNull();
  });

  it("does not request the invitation list for a member role", () => {
    data.myPermissions = { role: "member", permissions: [] };
    data.members = [
      member("user-1", "Ada Lovelace", "member"),
      member("user-2", "Grace Hopper", "owner"),
    ];
    renderWithI18n(<MembersTab />);

    const invitationQueryCall = mockUseQuery.mock.calls.find(([opts]) =>
      JSON.stringify(opts.queryKey).includes("invitations"),
    );
    expect(invitationQueryCall).toBeDefined();
    expect(invitationQueryCall?.[0]).toMatchObject({ enabled: false });
  });

  it("requests the invitation list for an owner or admin role", () => {
    data.members = [
      member("user-1", "Ada Lovelace", "owner"),
      member("user-2", "Grace Hopper", "admin"),
    ];
    renderWithI18n(<MembersTab />);

    const invitationQueryCall = mockUseQuery.mock.calls.find(([opts]) =>
      JSON.stringify(opts.queryKey).includes("invitations"),
    );
    expect(invitationQueryCall).toBeDefined();
    expect(invitationQueryCall?.[0]).toMatchObject({ enabled: true });
  });
});

// Account groups (spec 7b.2): what a key or a group lets the caller touch.
describe("MembersTab with account-group keys", () => {
  const allKeys = ["members.invite", "audit.read", "billing.view", "billing.manage", "integrations.manage", "runtime_profiles.manage"];

  beforeEach(() => {
    data.members = [
      member("user-1", "Ada Lovelace", "member"),
      member("user-2", "Grace Hopper", "owner"),
    ];
    data.groups = [{ id: "g1", name: "Accounting", description: "", permissions: [], member_count: 0 }];
    data.invitations = [];
    data.shareLinks = [];
  });

  it("shows the invite control to a member who holds members.invite", () => {
    data.myPermissions = { role: "member", permissions: ["members.invite"] };
    renderWithI18n(<MembersTab />);
    expect(screen.getByRole("button", { name: "Invite member" })).toBeInTheDocument();
  });

  it("offers a member with members.invite the member and viewer roles, never admin", async () => {
    data.myPermissions = { role: "member", permissions: ["members.invite"] };
    const user = userEvent.setup();
    renderWithI18n(<MembersTab />);
    await user.click(screen.getByRole("button", { name: "Invite member" }));
    const dialog = await screen.findByRole("dialog", { name: "Invite member" });
    await user.click(within(dialog).getByRole("combobox", { name: "Role" }));
    expect(screen.queryByRole("option", { name: "Admin" })).toBeNull();
    expect(screen.getByRole("option", { name: "Member" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Viewer" })).toBeInTheDocument();
  });

  it("gives an owner a group control on each other member", () => {
    data.members = [
      member("user-1", "Grace Hopper", "owner"),
      member("user-2", "Ada Lovelace", "member"),
    ];
    data.myPermissions = { role: "owner", permissions: allKeys };
    renderWithI18n(<MembersTab />);
    expect(screen.getByRole("button", { name: /^Groups for Ada Lovelace/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Groups for Grace Hopper/ })).toBeNull();
  });

  it("gives no group control to a member holding members.invite", () => {
    data.myPermissions = { role: "member", permissions: ["members.invite"] };
    renderWithI18n(<MembersTab />);
    expect(screen.queryByRole("button", { name: /^Groups for/ })).toBeNull();
  });

  it("gives a viewer in a group with every key no write control", () => {
    data.myPermissions = { role: "viewer", permissions: allKeys };
    renderWithI18n(<MembersTab />);
    expect(screen.queryByRole("button", { name: "Invite member" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Groups for/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Actions for/ })).toBeNull();
  });
});

describe("MembersTab guest members and guest invitations", () => {
  it("renders guest switch only for non-owner, non-admin members and toggles via updateMember", async () => {
    mockUpdateMember.mockResolvedValue({});
    const user = userEvent.setup();
    data.members = [
      { ...member("user-1", "Ada Lovelace", "owner"), is_guest: false },
      { ...member("user-2", "Grace Hopper", "admin"), is_guest: false },
      { ...member("user-3", "Alan Turing", "member"), is_guest: false },
      { ...member("user-4", "Barbara Liskov", "viewer"), is_guest: true },
    ];
    renderWithI18n(<MembersTab />);

    expect(screen.queryByRole("switch", { name: "Guest access for Ada Lovelace" })).toBeNull();
    expect(screen.queryByRole("switch", { name: "Guest access for Grace Hopper" })).toBeNull();

    const alanSwitch = screen.getByRole("switch", { name: "Guest access for Alan Turing" });
    const barbaraSwitch = screen.getByRole("switch", { name: "Guest access for Barbara Liskov" });
    expect(alanSwitch).not.toBeChecked();
    expect(barbaraSwitch).toBeChecked();

    await user.click(alanSwitch);
    await waitFor(() =>
      expect(mockUpdateMember).toHaveBeenCalledWith("ws-1", "m-user-3", {
        role: "member",
        is_guest: true,
      }),
    );

    await user.click(barbaraSwitch);
    await waitFor(() =>
      expect(mockUpdateMember).toHaveBeenCalledWith("ws-1", "m-user-4", {
        role: "viewer",
        is_guest: false,
      }),
    );
  });

  it("hides guest switches and write controls when viewer cannot manage members", () => {
    data.myPermissions = { role: "member", permissions: [] };
    data.members = [
      { ...member("user-1", "Ada Lovelace", "member"), is_guest: false },
      { ...member("user-2", "Alan Turing", "member"), is_guest: true },
    ];
    renderWithI18n(<MembersTab />);

    expect(screen.queryByRole("switch", { name: /Guest access for/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Invite member" })).toBeNull();
  });

  it("sends is_guest and guest_project_ids from the invite dialog when guest toggle is on", async () => {
    mockCreateMember.mockResolvedValue({});
    const user = userEvent.setup();
    data.projects = [
      { id: "proj-1", title: "Alpha", access: "workspace" },
      { id: "proj-2", title: "Beta", access: "restricted" },
    ];
    renderWithI18n(<MembersTab />);

    await user.click(screen.getByRole("button", { name: "Invite member" }));
    const dialog = await screen.findByRole("dialog", { name: "Invite member" });
    await user.type(within(dialog).getByRole("textbox", { name: "Email" }), "guest@acme.dev");
    await user.click(within(dialog).getByRole("switch", { name: "Invite as guest" }));

    await user.click(await within(dialog).findByRole("checkbox", { name: "Alpha" }));
    await user.click(within(dialog).getByRole("checkbox", { name: "Beta" }));
    await user.click(within(dialog).getByRole("button", { name: "Invite" }));

    await waitFor(() =>
      expect(mockCreateMember).toHaveBeenCalledWith("ws-1", {
        email: "guest@acme.dev",
        role: "member",
        is_guest: true,
        guest_project_ids: ["proj-1", "proj-2"],
      }),
    );
  });
});
