// @vitest-environment node
import { describe, expect, it } from "vitest";
import { roomColumns, roomLookFor } from "./room-theme";

describe("roomLookFor", () => {
  it("gives each built-in status its own room", () => {
    expect(roomLookFor("backlog", "unstarted").theme).toBe("archive");
    expect(roomLookFor("todo", "unstarted").theme).toBe("intake");
    expect(roomLookFor("in_progress", "started").theme).toBe("work");
    expect(roomLookFor("in_review", "started").theme).toBe("review");
    expect(roomLookFor("blocked", "started").theme).toBe("alert");
    expect(roomLookFor("done", "done").theme).toBe("lounge");
    expect(roomLookFor("cancelled", "closed").theme).toBe("closed");
  });

  it("moves a custom status into the room of its category", () => {
    expect(roomLookFor("qa", "started").theme).toBe("work");
    expect(roomLookFor("triage", "unstarted").theme).toBe("intake");
    expect(roomLookFor("shipped", "done").theme).toBe("lounge");
  });
});

describe("roomColumns", () => {
  it("widens only the In Progress room", () => {
    expect(roomColumns("in_progress")).toBe(2);
    expect(roomColumns("todo")).toBe(1);
    expect(roomColumns("qa")).toBe(1);
  });
});
