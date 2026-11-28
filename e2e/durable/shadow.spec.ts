import "../env";

import { createHash } from "node:crypto";
import { expect, test } from "@playwright/test";
import { createTestApi } from "../helpers";
import { apiRaw, sessionOf, type Session } from "../phase4-helpers";

// Spec 17.2 (3). Run with durable_seats, durable_effects and their dependencies enabled.
// The fake daemon drives the real claim and completion routes; no agent CLI is launched.
const PAIRS = 20;

type Task = { id: string; agent_id: string; auth_token: string; shadow: boolean };

async function post(s: Session, path: string, body: unknown, headers: Record<string, string> = {}) {
  const res = await apiRaw(s, "POST", path, body, headers);
  const text = await res.text();
  expect(res.ok, `${path}: ${res.status} ${text}`).toBe(true);
  return text ? JSON.parse(text) : {};
}

async function claim(s: Session, runtime: string, shadowCapable = false): Promise<Task[]> {
  const result = await post(s, "/api/daemon/tasks/claim", {
    daemon_id: "e2e-shadow-daemon", runtime_ids: [runtime], max_tasks: 1,
    force_recheck_runtime_ids: [runtime],
  }, shadowCapable ? { "X-Client-Capabilities": "shadow-v1" } : {});
  return result.tasks;
}

async function start(s: Session, task: Task) {
  await post(s, `/api/daemon/tasks/${task.id}/start`, {});
}

async function complete(s: Session, task: Task, output: string) {
  await post(s, `/api/daemon/tasks/${task.id}/complete`, { output });
}

test("spec 17.2 (3): a shadow trial proposes and applies a runtime change", async () => {
  test.setTimeout(600_000);
  const client = await createTestApi();
  const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const workspace = await client.createWorkspace(`Shadow trial ${suffix}`, `e2e-shadow-${suffix}`);
  const owner = sessionOf(client, workspace);
  try {
    const seatRuntime = await client.seedRuntime(`Shadow seat ${suffix}`);
    const candidateRuntime = await client.seedRuntime(`Shadow candidate ${suffix}`, "openai");
    const jurorRuntime = await client.seedRuntime(`Shadow juror ${suffix}`);
    const seat = await client.createAgent(`Shadow seat ${suffix}`, seatRuntime);
    const juror = await client.createAgent(`Shadow juror ${suffix}`, jurorRuntime);
    const squad = await client.createSquad(`Shadow squad ${suffix}`, seat.id);
    await client.queryDb(`UPDATE agent SET home_squad_id = $2 WHERE id = $1 AND workspace_id = $3`, [seat.id, squad.id, workspace.id]);
    await client.queryDb(
      `UPDATE workspace SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{jury_pool}', $2::jsonb) WHERE id = $1`,
      [workspace.id, JSON.stringify([juror.id])],
    );

    const model = `candidate-${suffix}`;
    const config = await post(owner, `/api/agents/${seat.id}/shadow`, {
      candidate_runtime_id: candidateRuntime, candidate_model: model,
      sample_rate: 1, target_pairs: PAIRS, confirm_cost: true,
    });
    const shadowTasks: string[] = [];

    for (let i = 0; i < PAIRS; i++) {
      const issue = await client.createIssue(`Shadow pair ${i + 1} ${suffix}`, { status: "in_progress" });
      const [queued] = await client.queryDb(
        `INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority)
         VALUES ($1, $2, $3, 'queued', 0) RETURNING id::text`,
        [seat.id, seatRuntime, issue.id],
      );
      const [primary] = await claim(owner, seatRuntime, true);
      expect(primary?.id).toBe(queued.id);
      const [pair] = await client.queryDb(
        `SELECT id::text, shadow_task_id::text FROM shadow_pair WHERE workspace_id = $1 AND config_id = $2 AND primary_task_id = $3`,
        [workspace.id, config.id, primary.id],
      );
      expect(pair).toBeDefined();
      expect(await claim(owner, candidateRuntime)).toHaveLength(0);
      const [shadow] = await claim(owner, candidateRuntime, true);
      expect(shadow?.id).toBe(pair.shadow_task_id);
      expect(shadow.shadow).toBe(true);
      shadowTasks.push(shadow.id);

      await start(owner, primary);
      await start(owner, shadow);
      const declared = await post({ ...owner, token: shadow.auth_token }, "/api/effects/declare", {
        class: "read", target: "e2e/shadow-read", summary: "Read the issue",
      }, { "X-Task-ID": shadow.id, "X-Agent-ID": shadow.agent_id });
      expect(declared.decision).toBe("allow");
      await complete(owner, primary, "Primary answer");
      await complete(owner, shadow, "Candidate answer");

      const [verification] = await client.queryDb(
        `SELECT id::text, agent_id::text FROM agent_task_queue
         WHERE context->>'durable_kind' = 'verification' AND context->>'shadow_pair_id' = $1`,
        [pair.id],
      );
      expect(verification?.agent_id).toBe(juror.id);
      const [judge] = await claim(owner, jurorRuntime);
      expect(judge?.id).toBe(verification.id);
      await start(owner, judge);
      const primaryIsA = (createHash("sha256").update(`shadow-label|${pair.id}`).digest()[0] & 1) === 1;
      await complete(owner, judge, `Candidate answer is better.\nVERDICT: ${primaryIsA ? "B" : "A"}`);
      const [result] = await client.queryDb(`SELECT comparison FROM shadow_pair WHERE workspace_id = $1 AND id = $2`, [workspace.id, pair.id]);
      expect(result.comparison).toBe("shadow");
    }

    const events = await client.queryDb(
      `SELECT task_id::text, effect_class FROM effect_event WHERE workspace_id = $1 AND task_id = ANY($2::uuid[])`,
      [workspace.id, shadowTasks],
    );
    expect(events).toHaveLength(PAIRS);
    expect(events.every((event) => event.effect_class === "read")).toBe(true);
    const listRes = await apiRaw(owner, "GET", `/api/agents/${seat.id}/shadow`);
    expect(listRes.ok).toBe(true);
    const list = await listRes.json();
    const run = list.runs.find((item: { id: string }) => item.id === config.id);
    expect(run).toMatchObject({ state: "completed", report: { pairs: PAIRS, shadow: PAIRS, tie: 0, primary: 0 } });
    expect(run.report.not_losing_lower).toBeGreaterThanOrEqual(0.5);

    const [proposal] = await client.queryDb(
      `SELECT id::text, state, kind, payload FROM proposal
       WHERE workspace_id = $1 AND squad_id = $2 AND payload->>'shadow_config_id' = $3`,
      [workspace.id, squad.id, config.id],
    );
    expect(proposal).toMatchObject({ state: "pending", kind: "org_change", payload: { type: "runtime_change", agent_id: seat.id, to_runtime_id: candidateRuntime, to_model: model } });
    await client.decideProposal(proposal.id as string, "approve", "Approve the tested model");
    const [application] = await client.queryDb(
      `SELECT change_type, subject_id::text, probation_ends_at, applied_at FROM org_change_application
       WHERE workspace_id = $1 AND proposal_id = $2`,
      [workspace.id, proposal.id],
    );
    expect(application).toMatchObject({ change_type: "runtime_change", subject_id: seat.id });
    const probationMs = new Date(application.probation_ends_at as string).getTime() - new Date(application.applied_at as string).getTime();
    expect(Math.abs(probationMs - 14 * 24 * 60 * 60 * 1000)).toBeLessThan(60_000);
    const [updated] = await client.queryDb(`SELECT runtime_id::text, model FROM agent WHERE workspace_id = $1 AND id = $2`, [workspace.id, seat.id]);
    expect(updated).toMatchObject({ runtime_id: candidateRuntime, model });
  } finally {
    await client.cleanup();
    await client.deleteWorkspace();
  }
});
