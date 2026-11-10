/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";
import { CompanySquadHandoff } from "./company-squad-handoff";

const api = vi.hoisted(() => ({
  getSquadHandoffRule: vi.fn(),
  setSquadHandoffRule: vi.fn(),
  listSquads: vi.fn(),
}));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api,
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const squad = (id: string, name: string) => ({
  id,
  workspace_id: "ws-1",
  name,
  description: "",
  instructions: "",
  avatar_url: null,
  leader_id: "a-1",
  creator_id: "u-1",
  created_at: "2026-11-01T10:00:00Z",
  updated_at: "2026-11-01T10:00:00Z",
  archived_at: null,
  archived_by: null,
  autonomy_level: 1,
});

function renderPanel(canEdit: boolean) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <CompanySquadHandoff squadId="sq-1" canEdit={canEdit} />
    </QueryClientProvider>,
  );
}

describe("CompanySquadHandoff", () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset();
    api.listSquads.mockResolvedValue([squad("sq-1", "Planning"), squad("sq-2", "SDLC Greenfield")]);
  });
  afterEach(cleanup);

  it("gives the boss a target and a mode, with the consequence of automatic handoff", async () => {
    api.getSquadHandoffRule.mockResolvedValue({
      squad_id: "sq-1", target_squad_id: "sq-2", target_squad_name: "SDLC Greenfield", mode: "auto",
    });
    renderPanel(true);
    expect(await screen.findByRole("combobox", { name: "Hand finished requests to" })).toBeTruthy();
    expect(await screen.findByRole("combobox", { name: "How" })).toBeTruthy();
    expect(await screen.findByText("Starts the other squad's work as soon as this squad's leader asks, without waiting for you.")).toBeTruthy();
  });

  it("shows only the target picker while no squad is chosen", async () => {
    api.getSquadHandoffRule.mockResolvedValue({ squad_id: "sq-1", target_squad_id: null, target_squad_name: null, mode: "off" });
    renderPanel(true);
    expect(await screen.findByRole("combobox", { name: "Hand finished requests to" })).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "How" })).toBeNull();
  });

  it("tells a viewer where finished requests go, and hides the block when nothing does", async () => {
    api.getSquadHandoffRule.mockResolvedValue({
      squad_id: "sq-1", target_squad_id: "sq-2", target_squad_name: "SDLC Greenfield", mode: "ask",
    });
    renderPanel(false);
    expect(await screen.findByText("Hands finished requests to SDLC Greenfield (Ask me first).")).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();
    cleanup();

    api.getSquadHandoffRule.mockResolvedValue({ squad_id: "sq-1", target_squad_id: null, target_squad_name: null, mode: "off" });
    const { container } = renderPanel(false);
    await waitFor(() => expect(api.getSquadHandoffRule).toHaveBeenCalledTimes(2));
    expect(container.textContent).toBe("");
  });
});
