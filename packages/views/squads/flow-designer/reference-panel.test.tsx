// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { renderWithI18n } from "../../test/i18n";
import { ReferencePanel } from "./reference-panel";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const fakeApi = { listSquadMembers: vi.fn(), listAgents: vi.fn(), listSquadSkills: vi.fn(), getSkill: vi.fn() };

// Text that would be markup if anything rendered it as HTML.
const HOSTILE = '<img src="x" onerror="window.__pwned = 1"><b>bold</b><script>window.__pwned = 2</script>';

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <ReferencePanel squadId="sq-1" />
    </QueryClientProvider>,
  );
}

describe("ReferencePanel", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.listSquadMembers.mockResolvedValue([
      { member_type: "agent", member_id: "ag-1", role: "leader" },
      { member_type: "agent", member_id: "ag-2", role: "member" },
      { member_type: "member", member_id: "u-1", role: "member" },
    ]);
    fakeApi.listAgents.mockResolvedValue([
      { id: "ag-1", name: "Lead", instructions: "Plan the work.", archived_at: null },
      { id: "ag-2", name: "Writer", instructions: HOSTILE, archived_at: null },
      { id: "ag-3", name: "Outsider", instructions: "Not in the squad.", archived_at: null },
    ]);
    fakeApi.listSquadSkills.mockResolvedValue([{ id: "sk-1", name: "style", description: "House style" }]);
    fakeApi.getSkill.mockResolvedValue({
      id: "sk-1",
      name: "style",
      content: HOSTILE,
      files: [{ id: "f-1", skill_id: "sk-1", path: "rules.md", content: "No exclamation marks." }],
    });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    delete (window as unknown as Record<string, unknown>).__pwned;
  });

  it("starts folded and reads nothing until it is opened", () => {
    renderPanel();
    expect(screen.getByRole("button", { name: "Show reference" })).toBeInTheDocument();
    expect(fakeApi.listSquadSkills).not.toHaveBeenCalled();
    expect(fakeApi.listAgents).not.toHaveBeenCalled();
  });

  it("shows the instructions of the agents of the squad and its skills, and only those", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Show reference" }));

    expect(await screen.findByText("Plan the work.")).toBeInTheDocument();
    expect(screen.getByText("Writer")).toBeInTheDocument();
    expect(screen.queryByText("Outsider")).toBeNull();
    expect(screen.getByRole("heading", { name: "Agent instructions" })).toBeInTheDocument();
    expect(await screen.findByText("style")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hide reference" })).toBeInTheDocument();
  });

  it("shows what the files say as text and never as HTML", async () => {
    const { container } = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Show reference" }));
    await screen.findByText("Plan the work.");
    fireEvent.click(await screen.findByText("style"));
    await waitFor(() => expect(fakeApi.getSkill).toHaveBeenCalledWith("sk-1"));
    await screen.findByText("No exclamation marks.");

    // The instruction of the agent and the skill are on the page as characters, twice over.
    expect(screen.getAllByText(HOSTILE, { exact: false }).length).toBeGreaterThanOrEqual(2);
    expect(container.querySelector("img, script, b")).toBeNull();
    expect((window as unknown as Record<string, unknown>).__pwned).toBeUndefined();
  });

  it("says there is nothing to read for a squad with no instructions and no skills", async () => {
    fakeApi.listSquadMembers.mockResolvedValue([]);
    fakeApi.listSquadSkills.mockResolvedValue([]);
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Show reference" }));
    expect(await screen.findByText("Nothing to read yet")).toBeInTheDocument();
  });
});
