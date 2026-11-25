// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Intent } from "./schemas";
import { groupIntents } from "./batch";

function pending(id: string, batchKey: string): Intent {
  return {
    id,
    created_at: "",
    expires_at: "",
    agent_id: null,
    issue_id: null,
    source: "connected_app",
    target: "gmail",
    effect_class: "send",
    state: "pending",
    call: null,
    call_sha256: "",
    preview: { kind: "generic", title: "", body_markdown: "", fields: [], recipients: [], amount_micros: null },
    rationale: "",
    decision_note: "",
    batch_key: batchKey,
  };
}

describe("groupIntents", () => {
  it("shows pending intents that share a batch key as one batch, in the order they first appear", () => {
    const groups = groupIntents([pending("a", "k1"), pending("b", "k2"), pending("c", "k1"), pending("d", "")]);
    expect(groups).toEqual([
      { kind: "batch", key: "k1", intents: [expect.objectContaining({ id: "a" }), expect.objectContaining({ id: "c" })] },
      { kind: "single", intent: expect.objectContaining({ id: "b" }) },
      { kind: "single", intent: expect.objectContaining({ id: "d" }) },
    ]);
  });

  it("leaves a lone intent under a batch key as a single card", () => {
    expect(groupIntents([pending("a", "k1")])).toEqual([{ kind: "single", intent: expect.objectContaining({ id: "a" }) }]);
  });

  it("does not group intents that are no longer pending", () => {
    const decided: Intent = { ...pending("b", "k1"), state: "executed" };
    expect(groupIntents([pending("a", "k1"), decided])).toEqual([
      { kind: "single", intent: expect.objectContaining({ id: "a" }) },
    ]);
  });
});
