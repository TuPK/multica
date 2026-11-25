// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import { parseBatchResults, parseIntent, parseIntentList } from "./schemas";

const intentRow = {
  id: "i-1",
  created_at: "2026-11-25T08:00:00Z",
  expires_at: "2026-11-28T08:00:00Z",
  agent_id: "a-1",
  issue_id: "iss-1",
  source: "connected_app",
  target: "gmail:GMAIL_SEND_EMAIL",
  effect_class: "send",
  state: "pending",
  call: { tool: "GMAIL_SEND_EMAIL", account_id: "acc-1", args: { to: "customer@example.com", subject: "Báo giá", body: "Xin chào" } },
  call_sha256: "abc123",
  preview: {
    kind: "email",
    title: "Báo giá",
    body_markdown: "Xin chào",
    fields: [{ label: "To", before: "", after: "customer@example.com" }],
    recipients: ["customer@example.com"],
    amount_micros: null,
  },
  rationale: "Khách hỏi giá",
  decision_note: "",
  batch_key: "a-1|gmail:GMAIL_SEND_EMAIL|send|2026-11-25",
};

describe("intent schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("reads an intent with its preview and the call it will run", () => {
    const intent = parseIntent(intentRow, "GET /api/intents");
    expect(intent).toMatchObject({
      id: "i-1",
      state: "pending",
      call_sha256: "abc123",
      preview: { kind: "email", title: "Báo giá", recipients: ["customer@example.com"] },
      call: { tool: "GMAIL_SEND_EMAIL", account_id: "acc-1" },
    });
    expect(intent?.call?.args).toMatchObject({ body: "Xin chào" });
    expect(warn).not.toHaveBeenCalled();
  });

  it("never offers a decision on a state it does not know", () => {
    const intent = parseIntent({ ...intentRow, state: "approved_by_robot" }, "GET /api/intents");
    expect(intent?.state).not.toBe("pending");
  });

  it("keeps an intent with no call or preview, with an empty preview", () => {
    const intent = parseIntent({ ...intentRow, call: null, preview: "garbled" }, "GET /api/intents");
    expect(intent).toMatchObject({ call: null, preview: { kind: "generic", fields: [] } });
  });

  it("drops a malformed intent from a list", () => {
    const intents = parseIntentList({ intents: [{ id: 7 }, intentRow] }, "GET /api/intents");
    expect(intents.map((i) => i.id)).toEqual(["i-1"]);
  });

  it("answers no intent for a body that is not an intent", () => {
    expect(parseIntent("nope", "POST /api/intents/:id/approve")).toBeNull();
  });

  it("reads each batch outcome and its status", () => {
    expect(
      parseBatchResults(
        { results: [{ id: "i-1", state: "executed", status: 200 }, { id: "i-2", error: "already decided", status: 409 }] },
        "POST /api/intents/approve-batch",
      ),
    ).toEqual([
      { id: "i-1", state: "executed", error: "", status: 200 },
      { id: "i-2", state: "", error: "already decided", status: 409 },
    ]);
  });
});
