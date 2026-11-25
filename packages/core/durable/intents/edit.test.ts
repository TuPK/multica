// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Intent } from "./schemas";
import { buildEditedCall, canEditIntent, editableArgKey } from "./edit";

const call = {
  tool: "GMAIL_SEND_EMAIL",
  account_id: "acc-1",
  args: { to: "customer@example.com", subject: "Báo giá", body: "Xin chào" },
};

function intentWith(patch: Partial<Intent>): Intent {
  return {
    id: "i-1",
    created_at: "",
    expires_at: "",
    agent_id: null,
    issue_id: null,
    source: "connected_app",
    target: "gmail",
    effect_class: "send",
    state: "pending",
    call,
    call_sha256: "h",
    preview: { kind: "email", title: "Báo giá", body_markdown: "Xin chào", fields: [], recipients: [], amount_micros: null },
    rationale: "",
    decision_note: "",
    batch_key: "",
    ...patch,
  };
}

describe("editable arguments", () => {
  it("picks the message text the server lets a person change", () => {
    expect(editableArgKey(call)).toBe("body");
    expect(editableArgKey({ tool: "X", account_id: "a", args: { text: "hi" } })).toBe("text");
  });

  it("has no editable text for a call without one", () => {
    expect(editableArgKey({ tool: "X", account_id: "a", args: { to: "a@b" } })).toBeNull();
    expect(editableArgKey(null)).toBeNull();
  });

  it("keeps every other argument and the tool when the text changes", () => {
    expect(buildEditedCall(call, "Xin chào, đây là bản sửa")).toEqual({
      tool: "GMAIL_SEND_EMAIL",
      account_id: "acc-1",
      args: { to: "customer@example.com", subject: "Báo giá", body: "Xin chào, đây là bản sửa" },
    });
  });

  it("offers an edit only on a pending message or post that has editable text", () => {
    expect(canEditIntent(intentWith({}))).toBe(true);
    expect(canEditIntent(intentWith({ state: "approved" }))).toBe(false);
    expect(canEditIntent(intentWith({ preview: { kind: "record", title: "", body_markdown: "", fields: [], recipients: [], amount_micros: null } }))).toBe(false);
    expect(canEditIntent(intentWith({ call: null }))).toBe(false);
  });
});
