/**
 * The workflow rules for rooms (squad-room-workflow spec, sections 3 to 6).
 *
 * An issue in a project follows its room's workflow. A write that would leave an
 * issue in a room with no workflow is refused with one of these codes, and the
 * form asks the user to choose a workflow before it is sent.
 */

/** What issues that are not requests of a flow room follow (spec section 4). */
export type RoomNonRequestKind = "no_action" | "default" | "workflow";

export interface RoomWorkflowPolicy {
  kind: RoomNonRequestKind;
  /** Set only for the workflow kind. */
  workflowId: string | null;
}

export type RoomWorkflowCode = "room_workflow_required" | "room_no_workflow_for_non_request";

/** The answer to a preview: whether the write would be refused, and why. */
export interface RoomWorkflowCheck {
  needsWorkflow: boolean;
  code: RoomWorkflowCode | null;
  message: string;
  /** The room the refusal is about, when there is one. */
  squadId: string | null;
}

/** The inputs of a preview. They match the write the form is about to send. */
export interface RoomWorkflowCheckParams {
  projectId?: string | null;
  assigneeType?: "squad" | "agent" | "member" | null;
  assigneeId?: string | null;
  parentIssueId?: string | null;
}
