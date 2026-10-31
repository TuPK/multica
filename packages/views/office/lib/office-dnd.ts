import { pointerWithin, type CollisionDetection, type Collision } from "@dnd-kit/core";

/** A desk's size is shared with the walk-path anchor, so keep them together. */
export const DESK_WIDTH = 216;
export const DESK_HEIGHT = 92;

export type OfficeDragData =
  | { type: "issue"; issueId: string }
  | {
      type: "actor";
      actorType: "member" | "agent";
      actorId: string;
      name: string;
    };

export type OfficeDropData =
  | { type: "room"; status: string }
  | { type: "desk"; issueId: string; status: string };

export const roomDropId = (status: string) => `room:${status}`;
export const deskDropId = (issueId: string) => `desk:${issueId}`;

function dropTypeOf(collision: Collision): OfficeDropData["type"] | undefined {
  const container = collision.data?.droppableContainer as
    | { data: { current?: OfficeDropData } }
    | undefined;
  return container?.data.current?.type;
}

/**
 * An issue lands on a room (a desk under the pointer resolves to its room);
 * a staff member lands on a desk to take the issue, or on a room to open a new
 * one there. Dropping anywhere else, such as the corridor, cancels the drag.
 */
export const officeCollision: CollisionDetection = (args) => {
  const dragType = (args.active.data.current as OfficeDragData | undefined)?.type;
  const hits = pointerWithin(args);
  if (dragType === "actor") {
    const desks = hits.filter((hit) => dropTypeOf(hit) === "desk");
    if (desks.length > 0) return desks;
  }
  return hits.filter((hit) => dropTypeOf(hit) === "room");
};
