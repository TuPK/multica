import { COMPANY_SLOTS, type FloorRoom } from "@multica/core/company";
import type { Squad } from "@multica/core/types";

/**
 * The squads an empty room can take: every live squad that holds no room of
 * the floor on show. A squad seated on another floor is included. Seating it
 * here moves it, as the seat menu does.
 */
export function roomCandidates(squads: readonly Squad[], rooms: readonly FloorRoom[]): Squad[] {
  const inRoom = new Set(rooms.flatMap((room) => (room.squad ? [room.squad.id] : [])));
  return squads.filter((squad) => !squad.archived_at && !inRoom.has(squad.id));
}

/**
 * The first room of `floorId` that no active squad holds, or null when the
 * floor is full. `squads` may include the squad being seated: it is not
 * counted against itself by the caller excluding it.
 */
export function firstFreeSlot(squads: readonly Squad[], floorId: string): number | null {
  const taken = new Set(
    squads
      .filter((squad) => !squad.archived_at && squad.floor_id === floorId && typeof squad.company_slot === "number")
      .map((squad) => squad.company_slot as number),
  );
  for (let slot = 0; slot < COMPANY_SLOTS; slot += 1) {
    if (!taken.has(slot)) return slot;
  }
  return null;
}
