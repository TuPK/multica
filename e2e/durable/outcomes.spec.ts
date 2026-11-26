import "../env";

import { expect, test } from "@playwright/test";
import { createTestApi } from "../helpers";
import { API_BASE, type TestApiClient } from "../fixtures";

// Spec 17.2, scenario 1: one week of a squad at L1 (durable spec 2.4 to 2.7, 3.2, 3.4, 3.5).
// The squad has envelope [1, 2] and 25 work items, each with an acceptance criterion. The
// worker closes 25 items through its task token, the boss reopens one of them, the
// boss records a check on each closed item, and the week's jobs run through the test route
// POST /api/test/durable/jobs (runbook phase 4, section 0). The clock is moved a week on
// for the settlement.
//
// Nothing here runs an agent. The agent's writes are API calls made with the task token a
// running task carries (TestApiClient.seedRunningTask). Setup and teardown go through
// TestApiClient.
//
// The count is 25, not the 12 of the spec text. The track record moves a pair to sampled
// only when the 30-day Wilson lower bound of its outcomes is at least 0.80 (spec 3.3, and
// NextVerificationMode). With one reopened fail, 12 outcomes give 0.646 and 24 give 0.798,
// so 25 is the smallest week in which one reopen still reaches sampled. The same week
// keeps the squad at L1: the squad rise to L2 also needs a proposal acceptance sample, which
// this week does not have (envelopeMayRise).

const DAY_MS = 24 * 60 * 60 * 1000;
const ITEMS = 25;
const PASSED = 24;

type Json = Record<string, any>;

interface Session {
  token: string;
  slug: string;
}

/** One API call. Returns the status and the parsed body; the caller decides what is an error. */
async function call(s: Session, method: string, path: string, body?: unknown): Promise<{ status: number; json: Json }> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${s.token}`,
      "X-Workspace-Slug": s.slug,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  return { status: res.status, json: text ? (JSON.parse(text) as Json) : {} };
}

/** A call that must succeed. */
async function ok(s: Session, method: string, path: string, body?: unknown): Promise<Json> {
  const res = await call(s, method, path, body);
  expect(res.status, `${method} ${path}: ${JSON.stringify(res.json)}`).toBeLessThan(300);
  return res.json;
}

/**
 * Runs the durable jobs once at `now` (default: the server's clock). The route runs the
 * outcome job's transitions and settlement, the track record and the envelope nightly.
 */
async function runDurableJobs(s: Session, now?: string): Promise<{ opened: number; settled: number; track_pairs: number; squads_moved: number }> {
  const query = now ? `?now=${encodeURIComponent(now)}` : "";
  const res = await call(s, "POST", `/api/test/durable/jobs${query}`);
  expect(res.status, "the durable jobs run through the test route").toBe(200);
  return res.json as { opened: number; settled: number; track_pairs: number; squads_moved: number };
}

test("spec 17.2 (1): a week of an L1 squad with 25 criteria items: 24 pass and 1 reopened, the track record moves to sampled, the squad stays at L1", async () => {
  test.setTimeout(180_000);
  const t: TestApiClient = await createTestApi();
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const workspace = await t.createWorkspace(`Outcome week ${id}`, `e2e-outcome-week-${id}`);
  const boss: Session = { token: t.getToken() ?? "", slug: workspace.slug };
  try {
    const runtimeId = await t.seedRuntime(`Outcome week runtime ${id}`);
    const leader = await t.createAgent(`Outcome week leader ${id}`, runtimeId);
    const squad = await t.createSquad(`Outcome week squad ${id}`, leader.id, "Runs one week of criteria work.");
    const worker = await t.createAgent(`Outcome week worker ${id}`, runtimeId);

    // The squad starts at L1 (autonomy_level 1) inside the envelope [1, 2] the boss sets.
    await ok(boss, "PUT", `/api/squads/${squad.id}/autonomy/envelope`, {
      floor_level: 1,
      ceiling_level: 2,
      reason: "E2E outcome week",
    });
    expect((await ok(boss, "GET", `/api/squads/${squad.id}`)).autonomy_level).toBe(1);

    // Each item is an issue the worker owns, with one acceptance criterion. Created in
    // progress, so no outcome opens until the item is done.
    const items: { issueId: string; criterionId: string }[] = [];
    for (let i = 0; i < ITEMS; i++) {
      const issue = await t.createIssue(`Outcome week item ${i + 1} ${id}`, {
        assignee_type: "agent",
        assignee_id: worker.id,
        status: "in_progress",
      });
      const criterion = await ok(boss, "POST", `/api/issues/${issue.id}/criteria`, {
        text: `Item ${i + 1} meets its brief.`,
        check_kind: "manual",
      });
      items.push({ issueId: issue.id, criterionId: criterion.id });
    }

    // The worker closes the items with its task token, as a run does. The last item is
    // closed and then reopened by the boss the same week.
    const task = await t.seedRunningTask(worker.id);
    const agent: Session = { token: task.token, slug: workspace.slug };
    for (const item of items) {
      await ok(agent, "PUT", `/api/issues/${item.issueId}`, { status: "done" });
    }
    const reopened = items[ITEMS - 1];
    await ok(boss, "PUT", `/api/issues/${reopened.issueId}`, { status: "in_progress" });

    // The jobs read the transitions and open one outcome per done item, in full mode.
    await runDurableJobs(boss);
    const outcomes = new Map<string, Json>();
    for (const item of items) {
      const outcome = await ok(boss, "GET", `/api/issues/${item.issueId}/outcome`);
      const row = outcome.outcome ?? outcome;
      expect(row.verification_mode, "a new pair starts at full").toBe("full");
      expect(row.sampled, "a full-mode outcome is verified").toBe(true);
      outcomes.set(item.issueId, row);
    }

    // The boss checks each item that stayed done: a pass on its criterion.
    for (const item of items.slice(0, PASSED)) {
      const row = outcomes.get(item.issueId)!;
      await ok(boss, "POST", `/api/outcomes/${row.id}/verifications`, {
        criterion_id: item.criterionId,
        result: "pass",
        severity: "none",
        note: "",
      });
    }

    // A week on: the settlement runs for every outcome due by then.
    await runDurableJobs(boss, new Date(Date.now() + 8 * DAY_MS).toISOString());

    // The verdicts: 24 pass, and the reopened item is a fail for the reopen.
    const list = await ok(boss, "GET", `/api/outcomes?agent_id=${worker.id}&limit=100`);
    const rows: Json[] = list.outcomes ?? list;
    expect(rows, "one outcome per work item").toHaveLength(ITEMS);
    expect(rows.filter((r) => r.verdict === "pass")).toHaveLength(PASSED);
    const failed = rows.filter((r) => r.verdict === "fail");
    expect(failed).toHaveLength(1);
    expect(failed[0].fail_cause).toBe("reopened");
    expect(failed[0].issue_id).toBe(reopened.issueId);

    // The summary carries the same numbers for the worker.
    const summary = await ok(boss, "GET", `/api/outcomes/summary?agent_id=${worker.id}`);
    const groups: Json[] = summary.groups ?? summary.rows ?? [];
    const total = groups.reduce((n, g) => n + Number(g.total), 0);
    const pass = groups.reduce((n, g) => n + Number(g.pass), 0);
    const fail = groups.reduce((n, g) => n + Number(g.fail), 0);
    expect({ total, pass, fail }).toEqual({ total: ITEMS, pass: PASSED, fail: 1 });

    // The track record of the pair: 30 days, 24 pass and 1 fail, and the mode has moved
    // to sampled (the 30-day lower bound 0.80 is met: 0.80 at 25 outcomes with one fail).
    const tracks = await ok(boss, "GET", `/api/track-records?agent_id=${worker.id}`);
    const records: Json[] = tracks.records ?? tracks.track_records ?? tracks;
    const window30 = records.find((r) => r.window_days === 30);
    expect(window30, "a 30-day track record for the worker").toBeTruthy();
    expect(window30!.n_pass).toBe(PASSED);
    expect(window30!.n_fail).toBe(1);
    expect(window30!.verification_mode).toBe("sampled");
    expect(window30!.score_lower).toBeGreaterThanOrEqual(0.8);

    // The squad stays at L1 inside its envelope. None of these outcomes is the squad's, so
    // its envelope sample is empty and the rise to L2 has nothing to read.
    const view = await ok(boss, "GET", `/api/squads/${squad.id}/autonomy/envelope`);
    expect(view.envelope).toMatchObject({ floor_level: 1, ceiling_level: 2 });
    expect(view.reputation.n, "the squad has no settled outcome of its own").toBe(0);
    expect(view.reputation.acceptance_rate).toBeNull();
    expect((await ok(boss, "GET", `/api/squads/${squad.id}`)).autonomy_level).toBe(1);
  } finally {
    await t.cleanup();
    await t.deleteWorkspace();
  }
});
