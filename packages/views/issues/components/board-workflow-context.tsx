"use client";

import { createContext, useContext } from "react";
import type { IssueStatus, IssueWorkflow, Project } from "@multica/core/types";

/**
 * Workflow facts a board column needs (MUL-7420), provided once by BoardView
 * so every column variant can read them without threading props.
 */
export interface BoardWorkflowContextValue {
  /**
   * The workflow that owns a board status column (for column handoff metadata).
   * When multiple room workflows share a status key, the first workflow in
   * appearance order owns the column.
   */
  workflowForStatus: (status: string) => IssueWorkflow | null;
  scopeProject: Project | null;
  /**
   * While a card is dragged, why it cannot be dropped on a status column —
   * its room's workflow does not list the status — or null when it can.
   */
  dropBlockedReason: (status: IssueStatus) => string | null;
  /**
   * While a card is dragged, who dropping it on a status column hands it to
   * (and whether their run starts), or null when the column keeps the
   * assignee.
   */
  dropHint: (status: IssueStatus) => BoardDropHint | null;
}

export interface BoardDropHint {
  name: string;
  actor: { type: "agent" | "squad" | "member"; id: string } | null;
  run: boolean;
}

const NO_WORKFLOW: BoardWorkflowContextValue = {
  workflowForStatus: () => null,
  scopeProject: null,
  dropBlockedReason: () => null,
  dropHint: () => null,
};

export const BoardWorkflowContext = createContext<BoardWorkflowContextValue>(NO_WORKFLOW);

export function useBoardWorkflow(): BoardWorkflowContextValue {
  return useContext(BoardWorkflowContext);
}
