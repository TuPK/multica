/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { renderWithI18n } from "../../test/i18n";
import { HandoffSection } from "./handoff-section";

const api = vi.hoisted(() => ({
  getIssueHandoff: vi.fn(),
  handOffIssue: vi.fn(),
  declineIssueHandoff: vi.fn(),
}));
const admin = vi.hoisted(() => ({ value: true }));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api,
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/paths")>();
  return { ...actual, useWorkspacePaths: () => actual.paths.workspace("acme") };
});
vi.mock("../../company/lib/use-company-admin", () => ({ useCompanyAdmin: () => admin.value }));

const adapter: NavigationAdapter = {
  pathname: "/acme/issues/i-1",
  searchParams: new URLSearchParams(),
  hash: "",
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  getShareableUrl: (path) => path,
};

const rule = (mode: string) => ({ squad_id: "sq-1", target_squad_id: "sq-2", target_squad_name: "SDLC Greenfield", mode });
const handoff = (status: string, targetIssue: string | null = null) => ({
  id: "h-1",
  source_issue_id: "i-1",
  target_issue_id: targetIssue,
  target_squad_id: "sq-2",
  target_squad_name: "SDLC Greenfield",
  mode: "ask",
  status,
  created_at: "2026-11-10T01:00:00Z",
  decided_at: null,
});

function renderSection() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <NavigationProvider value={adapter}>
        <HandoffSection issueId="i-1" />
      </NavigationProvider>
    </QueryClientProvider>,
  );
}

describe("HandoffSection", () => {
  beforeEach(() => {
    admin.value = true;
    for (const fn of Object.values(api)) fn.mockReset();
  });
  afterEach(cleanup);

  it("shows nothing for a request whose squad has no handoff rule", async () => {
    api.getIssueHandoff.mockResolvedValue({ status: "none", rule: null, handoff: null, reason: "" });
    const { container } = renderSection();
    await waitFor(() => expect(api.getIssueHandoff).toHaveBeenCalled());
    expect(container.textContent).toBe("");
  });

  it("lets the boss approve a pending handoff or decline it", async () => {
    api.getIssueHandoff.mockResolvedValue({ status: "pending", rule: rule("ask"), handoff: handoff("pending"), reason: "" });
    api.handOffIssue.mockResolvedValue({
      status: "handed_off", rule: rule("ask"), handoff: handoff("handed_off", "i-2"), reason: "",
    });
    renderSection();
    expect(await screen.findByText("Waiting for approval to hand off to SDLC Greenfield.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Decline" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Approve and hand off" }));
    await waitFor(() => expect(api.handOffIssue).toHaveBeenCalledWith("i-1"));
  });

  it("offers a hand-off by hand when the rule is off, and no buttons to a plain member", async () => {
    api.getIssueHandoff.mockResolvedValue({ status: "none", rule: rule("off"), handoff: null, reason: "" });
    renderSection();
    expect(await screen.findByRole("button", { name: "Hand off to SDLC Greenfield" })).toBeTruthy();
    cleanup();

    admin.value = false;
    renderSection();
    expect(await screen.findByText("Not handed off yet. Next: SDLC Greenfield.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Hand off to SDLC Greenfield" })).toBeNull();
  });

  it("links a handed-off request to the other squad's request", async () => {
    api.getIssueHandoff.mockResolvedValue({
      status: "handed_off", rule: rule("auto"), handoff: handoff("handed_off", "i-2"), reason: "",
    });
    renderSection();
    expect(await screen.findByText("Handed off to SDLC Greenfield.")).toBeTruthy();
    const link = screen.getByRole("link", { name: "Open its request" });
    expect(link.getAttribute("href")).toBe("/acme/issues/i-2");
    expect(screen.queryByRole("button", { name: /hand off/i })).toBeNull();
  });
});
