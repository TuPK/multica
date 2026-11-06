import type { AgentTask } from "@multica/core/types";
import { compareActiveIssueTasks } from "./active-task-order";

// Runs that have not started streaming output yet: there is no transcript to open.
const NO_OUTPUT_YET: ReadonlySet<AgentTask["status"]> = new Set([
  "queued",
  "dispatched",
  "waiting_local_directory",
  "deferred",
]);

/**
 * The run an agent's transcript icon opens for one issue: its running run when
 * one is live, otherwise its most recent run that has produced output. `tasks`
 * is the issue's task list in any order. Returns null when the agent has no run
 * worth opening yet.
 */
export function pickAgentRunTask(
  tasks: readonly AgentTask[],
  agentId: string,
): AgentTask | null {
  let running: AgentTask | null = null;
  let latest: AgentTask | null = null;
  let latestAt = -Infinity;

  for (const task of tasks) {
    if (task.agent_id !== agentId) continue;
    if (task.status === "running") {
      if (!running || compareActiveIssueTasks(task, running) < 0) running = task;
      continue;
    }
    if (NO_OUTPUT_YET.has(task.status)) continue;
    const at = Date.parse(task.created_at);
    const time = Number.isFinite(at) ? at : -Infinity;
    if (!latest || time > latestAt) {
      latest = task;
      latestAt = time;
    }
  }

  return running ?? latest;
}
