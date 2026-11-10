/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FloorRoom } from "@multica/core/company";
import type { Squad } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { CompanyRoomOverlays } from "./company-room-overlays";
import type { CompanyStage } from "./company-canvas";

const STAGE: CompanyStage = {
  units: { w: 100, h: 100 },
  tank: null,
  rooms: [
    { x: 0, y: 0, w: 50, h: 50 },
    { x: 50, y: 0, w: 50, h: 50 },
  ],
  boards: [],
};

const ROOMS: FloorRoom[] = [
  { slot: 0, squad: { id: "sq-research", name: "Research", archived_at: null } as Squad, agentIds: [], overflow: 0 },
  { slot: 1, squad: null, agentIds: [], overflow: 0 },
];

function renderOverlays(initiativesBySlot: number[], onOpenPipeline = vi.fn()) {
  renderWithI18n(
    <CompanyRoomOverlays
      stage={STAGE}
      rooms={ROOMS}
      pendingBySlot={[0, 0]}
      initiativesBySlot={initiativesBySlot}
      projectBySlot={[]}
      seatableSquads={[]}
      onCreateSquad={vi.fn()}
      onSeatSquad={vi.fn()}
      onOpenSquad={vi.fn()}
      onOpenDecisions={vi.fn()}
      onOpenPipeline={onOpenPipeline}
    />,
  );
  return { onOpenPipeline };
}

describe("CompanyRoomOverlays initiative badge", () => {
  afterEach(cleanup);

  it("shows the owned count on a room whose squad owns initiatives", () => {
    renderOverlays([2, 0]);
    expect(screen.getByRole("button", { name: "2 initiatives owned" }).textContent).toBe("2");
  });

  it("shows no badge for a room with no initiatives", () => {
    renderOverlays([0, 0]);
    expect(screen.queryByRole("button", { name: /initiatives? owned/ })).toBeNull();
  });

  it("opens the pipeline filtered to the room's squad", () => {
    const { onOpenPipeline } = renderOverlays([1, 0]);
    fireEvent.click(screen.getByRole("button", { name: "1 initiative owned" }));
    expect(onOpenPipeline).toHaveBeenCalledWith("sq-research");
  });
});

describe("CompanyRoomOverlays empty room", () => {
  afterEach(cleanup);

  const WAITING = { id: "sq-loose", name: "Loose squad", archived_at: null } as Squad;

  function renderEmpty(seatable: Squad[]) {
    const onCreateSquad = vi.fn();
    const onSeatSquad = vi.fn();
    renderWithI18n(
      <CompanyRoomOverlays
        stage={STAGE}
        rooms={ROOMS}
        pendingBySlot={[0, 0]}
        initiativesBySlot={[0, 0]}
        projectBySlot={[]}
        seatableSquads={seatable}
        onCreateSquad={onCreateSquad}
        onSeatSquad={onSeatSquad}
        onOpenSquad={vi.fn()}
        onOpenDecisions={vi.fn()}
        onOpenPipeline={vi.fn()}
      />,
    );
    return { onCreateSquad, onSeatSquad };
  }

  it("creates a squad straight away when no squad waits outside the company", () => {
    const { onCreateSquad, onSeatSquad } = renderEmpty([]);
    fireEvent.click(screen.getByRole("button", { name: "Create squad" }));
    expect(onCreateSquad).toHaveBeenCalledOnce();
    expect(onSeatSquad).not.toHaveBeenCalled();
  });

  it("seats a waiting squad in the empty room from the menu", async () => {
    const user = userEvent.setup();
    const { onCreateSquad, onSeatSquad } = renderEmpty([WAITING]);

    await user.click(screen.getByRole("button", { name: "Add squad" }));
    await user.click(await screen.findByRole("menuitem", { name: "Loose squad" }));

    expect(onSeatSquad).toHaveBeenCalledWith(WAITING, 1);
    expect(onCreateSquad).not.toHaveBeenCalled();
  });

  it("creates a new squad from the menu when squads wait outside the company", async () => {
    const user = userEvent.setup();
    const { onCreateSquad } = renderEmpty([WAITING]);

    await user.click(screen.getByRole("button", { name: "Add squad" }));
    await user.click(await screen.findByRole("menuitem", { name: "Create new squad" }));

    expect(onCreateSquad).toHaveBeenCalledOnce();
  });
});
