import type { Issue, IssueStatus } from "@multica/core/types";
import {
  computePosition,
  getMoveAnchors,
  insertIdByPosition,
  type DragMoveUpdates,
} from "../../issues/utils/drag-utils";

/**
 * The write for moving an issue into another room: it joins the end of that
 * room. Under a non-manual sort the server keeps its own order, so the issue
 * keeps its position and only the anchors say where it lands.
 */
export function planRoomMove({
  issue,
  status,
  targetIds,
  manualOrder,
  issueMap,
}: {
  issue: Issue;
  status: IssueStatus;
  /** Issue ids already in the target room, in display order. */
  targetIds: readonly string[];
  manualOrder: boolean;
  issueMap: Map<string, Issue>;
}): DragMoveUpdates {
  const others = targetIds.filter((id) => id !== issue.id);
  const ids = manualOrder
    ? [...others, issue.id]
    : insertIdByPosition(others, issue.id, issue.position, issueMap);
  return {
    status,
    position: manualOrder ? computePosition(ids, issue.id, issueMap) : issue.position,
    ...getMoveAnchors(ids, issue.id),
  };
}
