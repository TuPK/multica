// @vitest-environment node
import { describe, expect, it } from "vitest";
import { flowDraftKeys } from "./queries";

describe("flowDraftKeys", () => {
  it("includes wsId in all query keys", () => {
    const wsId = "ws-test-123";
    const squadId = "squad-456";
    const draftId = "draft-789";
    const workflowId = "wf-101";

    expect(flowDraftKeys.all(wsId)).toEqual(["flow-drafts", wsId]);
    expect(flowDraftKeys.list(wsId, squadId)).toEqual(["flow-drafts", wsId, "list", squadId]);
    expect(flowDraftKeys.detail(wsId, squadId, draftId)).toEqual([
      "flow-drafts",
      wsId,
      "detail",
      squadId,
      draftId,
    ]);
    expect(flowDraftKeys.capabilities(wsId)).toEqual(["flow-drafts", wsId, "capabilities"]);
    expect(flowDraftKeys.manifest(wsId, workflowId)).toEqual([
      "flow-drafts",
      wsId,
      "manifest",
      workflowId,
    ]);
  });
});
