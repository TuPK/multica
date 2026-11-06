// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { AgentTask } from "@multica/core/types";
import { pickAgentRunTask } from "./agent-run-transcript";

function task(overrides: Partial<AgentTask> & Pick<AgentTask, "id" | "status">): AgentTask {
  return {
    agent_id: "ada",
    issue_id: "issue-1",
    priority: 0,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  } as AgentTask;
}

describe("pickAgentRunTask", () => {
  it("prefers the running run over a newer finished one", () => {
    const tasks = [
      task({ id: "done", status: "completed", created_at: "2026-01-03T00:00:00Z" }),
      task({ id: "live", status: "running", created_at: "2026-01-01T00:00:00Z" }),
    ];
    expect(pickAgentRunTask(tasks, "ada")?.id).toBe("live");
  });

  it("falls back to the most recent finished run when nothing is running", () => {
    const tasks = [
      task({ id: "old", status: "failed", created_at: "2026-01-01T00:00:00Z" }),
      task({ id: "newest", status: "completed", created_at: "2026-01-05T00:00:00Z" }),
      task({ id: "middle", status: "cancelled", created_at: "2026-01-03T00:00:00Z" }),
    ];
    expect(pickAgentRunTask(tasks, "ada")?.id).toBe("newest");
  });

  it("ignores runs of other agents", () => {
    const tasks = [
      task({ id: "other-live", agent_id: "grace", status: "running" }),
      task({ id: "other-done", agent_id: "grace", status: "completed" }),
    ];
    expect(pickAgentRunTask(tasks, "ada")).toBeNull();
  });

  it("skips runs that have not produced output yet", () => {
    const tasks = [
      task({ id: "waiting", status: "queued", created_at: "2026-01-09T00:00:00Z" }),
      task({ id: "dispatched", status: "dispatched", created_at: "2026-01-08T00:00:00Z" }),
      task({ id: "finished", status: "completed", created_at: "2026-01-01T00:00:00Z" }),
    ];
    expect(pickAgentRunTask(tasks, "ada")?.id).toBe("finished");
  });

  it("returns null when the agent has no run to show", () => {
    expect(pickAgentRunTask([], "ada")).toBeNull();
    expect(
      pickAgentRunTask([task({ id: "waiting", status: "queued" })], "ada"),
    ).toBeNull();
  });
});
