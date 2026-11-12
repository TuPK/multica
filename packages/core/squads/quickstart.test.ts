// @vitest-environment node
import { describe, expect, it, vi, type Mock } from "vitest";
import type { Agent, IssueWorkflow, Squad } from "../types";
import type { CompanyFloor } from "../company/types";
import {
  planQuickstartAgents,
  runSquadQuickstart,
  type SquadQuickstartClient,
} from "./quickstart";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { PLANNING_ROLE_AGENT_INSTRUCTIONS } from "./planning-agent-instructions";
import { SDLC_ROLE_AGENT_INSTRUCTIONS } from "./sdlc-agent-instructions";
import { SDLC_BUILTIN_SKILLS } from "../catalog/generated/skills-builtin";
import { SDLC_SKILL_SOURCE } from "./quickstart";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "./templates";
import { SALES_STARTER_SOURCES } from "./sales-starter-sources";
import { KITVIET_P2_FLOOR_NAME_LIST } from "./kitviet-floors";

const greenfield = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_greenfield")!;
const marketTemplate = SQUAD_TEMPLATES.find((t) => t.id === "market_research")!;
const asAgent = (id: string, name: string, archived = false) =>
  ({ id, name, archived_at: archived ? "2026-01-01" : null }) as unknown as Agent;
const asFloor = (id: string, name: string, position = 0) =>
  ({ id, name, position, workspace_id: "ws-1", created_at: "", updated_at: "" }) as CompanyFloor;

type FakeClient = {
  [K in keyof SquadQuickstartClient]: Mock<SquadQuickstartClient[K]>;
};

function fakeClient(over: Partial<FakeClient> = {}): FakeClient {
  let n = 0;
  const client = {
    listAgents: vi.fn(async () => [] as Agent[]),
    createAgent: vi.fn(async (data) => asAgent(`agent-${++n}`, data.name)),
    listSquads: vi.fn(async () => [] as Squad[]),
    createSquad: vi.fn(async () => ({ id: "squad-1" }) as Squad),
    updateSquad: vi.fn(async () => ({}) as Squad),
    updateSquadCharter: vi.fn(async () => ({}) as never),
    createSquadRoutine: vi.fn(async () => ({}) as never),
    addSquadMember: vi.fn(async () => ({}) as never),
    listIssueWorkflows: vi.fn(async () => ({ workflows: [], total: 0 })),
    createIssueWorkflow: vi.fn(async () => ({}) as IssueWorkflow),
    listAutopilots: vi.fn(async () => ({ autopilots: [], total: 0 })),
    getAutopilot: vi.fn(async () => ({ autopilot: {}, triggers: [] }) as never),
    listSquadMembers: vi.fn(async () => [] as never[]),
    createAutopilot: vi.fn(async () => ({ id: "ap-1" }) as never),
    createAutopilotTrigger: vi.fn(async () => ({}) as never),
    updateAutopilot: vi.fn(async () => ({}) as never),
    listCompanyFloors: vi.fn(async () => [] as CompanyFloor[]),
    createCompanyFloor: vi.fn(async (data) => asFloor(`floor-${data.name}`, data.name, 9)),
    setCompanySeats: vi.fn(async () => ({ squads: [] }) as never),
    listSkills: vi.fn(async () => [] as never),
    getSkill: vi.fn(async () => ({}) as never),
    createSkill: vi.fn(async (data) => ({ id: `skill-${data.name}`, name: data.name }) as never),
    updateSkill: vi.fn(async () => ({}) as never),
    listSquadSkills: vi.fn(async () => [] as never),
    setSquadSkills: vi.fn(async () => [] as never),
    addAgentSkills: vi.fn(async () => undefined),
    getKnowledgeStatus: vi.fn(async () => ({ state: "ready" }) as never),
    createKnowledgeDocument: vi.fn(async () => ({ id: "doc-1" }) as never),
    ...over,
  } as FakeClient;
  return client;
}

const options = { runtimeId: "rt-1", createWorkflow: true };

describe("runSquadQuickstart", () => {
  it("creates the agents, the squad with its instructions and roster, and the workflow", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.createAgent).toHaveBeenCalledTimes(greenfield.roles.length);
    expect(client.createAgent.mock.calls[0]![0]).toMatchObject({
      name: SQUAD_ROLES.lead.name,
      runtime_id: "rt-1",
    });
    expect(client.createSquad).toHaveBeenCalledWith(
      expect.objectContaining({ name: greenfield.name, leader_id: "agent-1" }),
    );
    expect(client.updateSquad).toHaveBeenCalledWith("squad-1", {
      instructions: greenfield.instructions,
    });
    expect(client.addSquadMember).toHaveBeenCalledTimes(greenfield.roles.length - 1);
    expect(client.addSquadMember).toHaveBeenCalledWith(
      "squad-1",
      expect.objectContaining({ member_type: "agent", role: SQUAD_ROLES.qa.role }),
    );
    expect(out.squad).toEqual({ id: "squad-1", status: "created" });
    expect(out.workflow.status).toBe("created");
    const steps = client.createIssueWorkflow.mock.calls[0]![0].steps!;
    expect(steps.map((s) => ("status_key" in s ? s.status_key : s.new_status.name))).toEqual([
      "backlog",
      "Analysis",
      "Design",
      "Architecture",
      "Plan review",
      "Security review",
      "Release prep",
      "Environment",
      "Harness",
      "Stage done",
      "Plan ready",
      "todo",
      "in_progress",
      "in_review",
      "QA",
      "blocked",
    ]);
    expect(steps[1]).toMatchObject({ handler: { type: "agent" } });
  });

  it("reuses a live agent with the same name and ignores an archived one", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [
        asAgent("keep", SQUAD_ROLES.analyst.name),
        asAgent("gone", SQUAD_ROLES.qa.name, true),
      ]),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    const status = (name: string) => out.agents.find((a) => a.name === name)?.status;
    expect(status(SQUAD_ROLES.analyst.name)).toBe("reused");
    expect(status(SQUAD_ROLES.qa.name)).toBe("created");
    expect(client.createAgent).toHaveBeenCalledTimes(greenfield.roles.length - 1);
  });

  it("leaves an existing squad and workflow as they are", async () => {
    const client = fakeClient({
      listSquads: vi.fn(async () => [{ id: "s", name: greenfield.name, archived_at: null } as Squad]),
      listIssueWorkflows: vi.fn(async () => ({
        workflows: [{ name: "SDLC Auto" } as IssueWorkflow],
        total: 1,
      })),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.createSquad).not.toHaveBeenCalled();
    expect(client.updateSquad).not.toHaveBeenCalled();
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(out.squad).toEqual({ id: "s", status: "existing" });
    expect(out.workflow.status).toBe("existing");
  });

  it("builds the squad without a member whose agent could not be created", async () => {
    const client = fakeClient({
      createAgent: vi.fn(async (data) => {
        if (data.name === SQUAD_ROLES.ux.name) throw new Error("runtime offline");
        return asAgent(data.name, data.name);
      }),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.agents.find((a) => a.role === "ux")).toMatchObject({
      status: "failed",
      error: "runtime offline",
    });
    expect(client.addSquadMember).toHaveBeenCalledTimes(greenfield.roles.length - 2);
    expect(out.squad.status).toBe("created");
  });

  it("makes no squad when the leader could not be created", async () => {
    const client = fakeClient({
      createAgent: vi.fn(async (data) => {
        if (data.name === SQUAD_ROLES.lead.name) throw new Error("no access");
        return asAgent(data.name, data.name);
      }),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.createSquad).not.toHaveBeenCalled();
    expect(out.squad.status).toBe("failed");
    expect(out.workflow.status).toBe("skipped");
  });

  it("reports member and instruction failures but keeps the squad", async () => {
    const client = fakeClient({
      updateSquad: vi.fn(async () => {
        throw new Error("too long");
      }),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.squad.status).toBe("created");
    expect(out.squad.error).toContain("instructions: too long");
  });

  it("saves the template's charter at revision 0 on the new squad", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, greenfield, options);

    expect(client.updateSquadCharter).toHaveBeenCalledTimes(1);
    const [squadId, request] = client.updateSquadCharter.mock.calls[0]!;
    expect(squadId).toBe("squad-1");
    expect(request).toMatchObject({
      revision: 0,
      mission: greenfield.charter.mission,
      boundaries: greenfield.charter.boundaries,
      escalation: greenfield.charter.escalation,
      intake: greenfield.charter.intake,
      response_hours: 48,
      responsibilities: greenfield.charter.responsibilities,
      outputs: greenfield.charter.outputs.map((o) => ({
        key: o.key,
        kind: o.kind,
        kb_key: "",
        cadence_hint: o.cadenceHint,
      })),
    });
    expect(request).not.toHaveProperty("operating_review_enabled");
  });

  it("reports a charter the server refuses, and keeps the squad", async () => {
    const client = fakeClient({
      updateSquadCharter: vi.fn(async () => {
        throw new Error("mission too long");
      }),
    });

    const out = await runSquadQuickstart(client, marketTemplate, options);

    expect(out.squad.status).toBe("created");
    expect(out.squad.error).toContain("charter: mission too long");
  });

  it("links the template's routine as a squad routine with its purpose", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, marketTemplate, options);

    expect(client.createSquadRoutine).toHaveBeenCalledWith("squad-1", {
      autopilot_id: "ap-1",
      purpose: "Market scan",
      expected_output: "report",
      required: true,
    });
  });

  it("links an existing unlinked routine on a retry and counts a routine already linked as existing", async () => {
    const existing = {
      listAutopilots: vi.fn(async () => ({
        autopilots: [{ id: "old", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1" }] as never[],
        total: 1,
      })),
    };

    const linked = fakeClient(existing);
    const relinked = await runSquadQuickstart(linked, marketTemplate, options);
    expect(linked.createSquadRoutine).toHaveBeenCalledWith("squad-1", expect.objectContaining({ autopilot_id: "old" }));
    expect(relinked.routines).toEqual([{ title: "Market scan", status: "existing" }]);

    const alreadyLinked = fakeClient({
      ...existing,
      createSquadRoutine: vi.fn(async () => {
        throw Object.assign(new Error("already"), { body: { code: "routine_already_linked" } });
      }),
    });
    expect((await runSquadQuickstart(alreadyLinked, marketTemplate, options)).routines).toEqual([
      { title: "Market scan", status: "existing" },
    ]);
  });

  it("reports a routine the server refuses to link, with the autopilot left for the boss", async () => {
    const client = fakeClient({
      createSquadRoutine: vi.fn(async () => {
        throw new Error("autopilot must be assigned to this squad");
      }),
    });

    const out = await runSquadQuickstart(client, marketTemplate, options);

    expect(out.routines).toEqual([
      {
        title: "Market scan",
        status: "failed",
        error: "routine: autopilot must be assigned to this squad",
      },
    ]);
  });

  it("does not build the workflow when the workspace admin has not allowed the new statuses", async () => {
    const forbidden = Object.assign(new Error("admins only"), {
      body: { code: "status_create_forbidden" },
    });
    const createIssueWorkflow = vi.fn().mockRejectedValueOnce(forbidden);
    const client = fakeClient({ createIssueWorkflow });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.workflow.status).toBe("failed");
    expect(out.workflow.error).toContain("Analysis to Plan ready");
    expect(createIssueWorkflow).toHaveBeenCalledTimes(1);
  });

  it("sends each step's next and back status keys and its status category", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, greenfield, options);

    const steps = client.createIssueWorkflow.mock.calls[0]![0].steps!;
    const analysis = steps.find((s) => "new_status" in s && s.new_status.name === "Analysis")!;
    expect(analysis).toMatchObject({ next_status_key: "stage_done", back_status_key: "blocked" });
    const done = steps.find((s) => "new_status" in s && s.new_status.name === "Stage done")!;
    expect(done).toMatchObject({ new_status: { category: "done" } });
  });

  it("skips the workflow when asked not to create it or when the template has none", async () => {
    const client = fakeClient();
    const maintenance = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_maintenance")!;

    expect((await runSquadQuickstart(client, greenfield, { ...options, createWorkflow: false })).workflow.status).toBe("skipped");
    expect((await runSquadQuickstart(client, maintenance, options)).workflow.status).toBe("skipped");
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
  });

  it("reports progress after each step", async () => {
    const onProgress = vi.fn();

    await runSquadQuickstart(fakeClient(), greenfield, options, onProgress);

    expect(onProgress.mock.calls.length).toBeGreaterThan(greenfield.roles.length);
    expect(onProgress.mock.calls[0]![0].agents).toHaveLength(1);
  });
});

describe("runSquadQuickstart for market research", () => {
  const market = SQUAD_TEMPLATES.find((t) => t.id === "market_research")!;

  it("leads the squad with the research lead and creates it without a floor name", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, market, options);

    expect(client.createAgent.mock.calls[0]![0]).toMatchObject({
      name: SQUAD_ROLES.research_lead.name,
      runtime_id: "rt-1",
    });
    expect(client.createSquad).toHaveBeenCalledWith(
      expect.objectContaining({ name: market.name, leader_id: "agent-1" }),
    );
    expect(client.createSquad.mock.calls[0]![0]).not.toHaveProperty("floor_name");
    expect(out.squad).toEqual({ id: "squad-1", status: "created" });
  });

  describe("floor placement", () => {
    const executive = asFloor("floor-exec", "Executive", 0);
    const research = asFloor("floor-research", "Research", 1);
    const seated = (id: string, floorId: string, slot: number) =>
      ({ id, name: `Squad ${id}`, archived_at: null, floor_id: floorId, company_slot: slot }) as unknown as Squad;

    it("seats the squad on the first free slot of the existing Research floor", async () => {
      const client = fakeClient({
        listCompanyFloors: vi.fn(async () => [executive, research]),
        listSquads: vi.fn(async () => [seated("a", research.id, 0), seated("b", research.id, 1)]),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.createCompanyFloor).not.toHaveBeenCalled();
      expect(client.setCompanySeats).toHaveBeenCalledWith([
        { squad_id: "squad-1", floor_id: research.id, slot: 2 },
      ]);
      expect(out.squad).toEqual({ id: "squad-1", status: "created" });
    });

    it("creates the Research floor when the workspace has none, then seats the squad on it", async () => {
      const client = fakeClient({ listCompanyFloors: vi.fn(async () => [executive]) });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.createCompanyFloor).toHaveBeenCalledWith({ name: "Research" });
      expect(client.setCompanySeats).toHaveBeenCalledWith([
        { squad_id: "squad-1", floor_id: "floor-Research", slot: 0 },
      ]);
      expect(out.squad).toEqual({ id: "squad-1", status: "created" });
    });

    it("reports the squad as created without a seat when the Research floor is full", async () => {
      const full = [0, 1, 2, 3, 4, 5].map((slot) => seated(`s${slot}`, research.id, slot));
      const client = fakeClient({
        listCompanyFloors: vi.fn(async () => [research]),
        listSquads: vi.fn(async () => full),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.setCompanySeats).not.toHaveBeenCalled();
      expect(client.updateSquad).toHaveBeenCalledWith("squad-1", expect.anything());
      expect(out.squad).toEqual({
        id: "squad-1",
        status: "created",
        error: "no free seat on the Research floor",
      });
    });

    it("still creates the squad when the floor cannot be set up, and says so", async () => {
      const client = fakeClient({
        createCompanyFloor: vi.fn(async () => {
          throw new Error("admins only");
        }),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.createSquad).toHaveBeenCalledTimes(1);
      expect(client.setCompanySeats).not.toHaveBeenCalled();
      expect(out.squad).toEqual({
        id: "squad-1",
        status: "created",
        error: "could not set up the Research floor: admins only",
      });
    });

    it("reports a seat request the server refuses", async () => {
      const client = fakeClient({
        listCompanyFloors: vi.fn(async () => [research]),
        setCompanySeats: vi.fn(async () => {
          throw new Error("seat is already taken");
        }),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(out.squad).toEqual({
        id: "squad-1",
        status: "created",
        error: "seat on the Research floor: seat is already taken",
      });
    });

    it("does not move a squad the server already seated at the chosen slot", async () => {
      const client = fakeClient({
        listCompanyFloors: vi.fn(async () => [research]),
        createSquad: vi.fn(
          async () => ({ id: "squad-1", floor_id: research.id, company_slot: 0 }) as unknown as Squad,
        ),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.setCompanySeats).not.toHaveBeenCalled();
      expect(out.squad).toEqual({ id: "squad-1", status: "created" });
    });

    it("sends the seat request as one assignment naming the squad, its floor and its slot", async () => {
      const client = fakeClient({ listCompanyFloors: vi.fn(async () => [research]) });

      await runSquadQuickstart(client, market, options);

      expect(client.setCompanySeats).toHaveBeenCalledTimes(1);
      expect(client.setCompanySeats.mock.calls[0]![0]).toEqual([
        { squad_id: "squad-1", floor_id: research.id, slot: 0 },
      ]);
    });

    it("reuses an existing squad without touching its floor or seat", async () => {
      const client = fakeClient({
        listSquads: vi.fn(async () => [{ id: "s", name: market.name, archived_at: null } as Squad]),
      });

      const out = await runSquadQuickstart(client, market, options);

      expect(client.listCompanyFloors).not.toHaveBeenCalled();
      expect(client.createCompanyFloor).not.toHaveBeenCalled();
      expect(client.setCompanySeats).not.toHaveBeenCalled();
      expect(out.squad).toEqual({ id: "s", status: "existing" });
    });
  });

  it("creates a squad-assigned autopilot that opens an issue, then its weekly schedule", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, market, { ...options, timezone: "Europe/Paris" });

    expect(client.createAutopilot).toHaveBeenCalledWith({
      title: "Market scan",
      description: market.routines[0]!.description,
      assignee_type: "squad",
      assignee_id: "squad-1",
      execution_mode: "create_issue",
      issue_title_template: "Market scan, week of {{date}}",
    });
    expect(client.createAutopilotTrigger).toHaveBeenCalledWith("ap-1", {
      kind: "schedule",
      cron_expression: "0 8 * * 1",
      timezone: "Europe/Paris",
    });
    expect(out.routines).toEqual([{ title: "Market scan", status: "created" }]);
  });

  it("uses the browser's timezone when the caller passes none", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, market, options);

    expect(client.createAutopilotTrigger.mock.calls[0]![1].timezone).toBe(
      Intl.DateTimeFormat().resolvedOptions().timeZone,
    );
  });

  it("keeps a routine with the same name on the squad instead of making another", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "old", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "active" },
        ] as never[],
        total: 1,
      })),
      getAutopilot: vi.fn(async () => ({
        autopilot: {},
        triggers: [{ kind: "schedule", enabled: true }],
      }) as never),
    });

    const out = await runSquadQuickstart(client, market, options);

    expect(client.createAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).not.toHaveBeenCalled();
    expect(out.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("adds the schedule to a routine that exists without one, so a rerun finishes the job", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "old", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "paused" },
        ] as never[],
        total: 1,
      })),
    });

    const out = await runSquadQuickstart(client, market, options);

    expect(client.createAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).toHaveBeenCalledWith("old", {
      kind: "schedule",
      cron_expression: market.routines[0]!.cron,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    });
    expect(out.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("leaves a schedule the boss disabled alone, and adds no second one", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "old", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "paused" },
        ] as never[],
        total: 1,
      })),
      getAutopilot: vi.fn(async () => ({
        autopilot: {},
        triggers: [{ kind: "schedule", enabled: false }],
      }) as never),
    });

    const out = await runSquadQuickstart(client, market, options);

    expect(client.createAutopilotTrigger).not.toHaveBeenCalled();
    expect(out.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("flags an active routine that should start paused and has no schedule, when its schedule cannot be set", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "ap-1", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "active" },
        ] as never[],
        total: 1,
      })),
      createAutopilotTrigger: vi.fn(async () => {
        throw new Error("boom");
      }),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
    expect(out.routines).toEqual([
      {
        title: "Market scan",
        status: "failed",
        error: "schedule: boom; the autopilot is active without a schedule, pause or delete it",
      },
    ]);
  });

  it("never unpauses an autopilot the boss made, and reports the schedule it adds to an active one as partial", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "ap-1", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "active" },
        ] as never[],
        total: 1,
      })),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).toHaveBeenCalledTimes(1);
    expect(client.createSquadRoutine).toHaveBeenCalledWith("squad-1", expect.objectContaining({ autopilot_id: "ap-1" }));
    expect(out.routines).toEqual([
      {
        title: "Market scan",
        status: "partial",
        error: "the schedule was added to an active autopilot that should start paused; pause it on the autopilot if it should wait",
      },
    ]);
  });

  it("a rerun after a failed pause re-pauses the quickstart's own unscheduled autopilot before it adds the schedule", async () => {
    const client = fakeClient({
      updateAutopilot: vi.fn(async () => {
        throw new Error("pause refused");
      }),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };
    const first = await runSquadQuickstart(client, paused, options);
    expect(first.routines[0]!.status).toBe("failed");

    // The autopilot the first run made is now listed: active, no trigger, the text the quickstart wrote.
    const routine = market.routines[0]!;
    client.listAutopilots.mockResolvedValue({
      autopilots: [
        {
          id: "ap-1",
          title: "Market scan",
          description: routine.description,
          issue_title_template: routine.issueTitleTemplate,
          assignee_type: "squad",
          assignee_id: "squad-1",
          status: "active",
        },
      ] as never[],
      total: 1,
    });
    client.updateAutopilot.mockResolvedValue({} as never);
    client.createAutopilotTrigger.mockClear();
    client.createAutopilot.mockClear();
    client.updateAutopilot.mockClear();

    const second = await runSquadQuickstart(client, paused, options);

    expect(client.createAutopilot).not.toHaveBeenCalled();
    expect(client.updateAutopilot).toHaveBeenCalledTimes(1);
    expect(client.updateAutopilot).toHaveBeenCalledWith("ap-1", { status: "paused" });
    expect(client.updateAutopilot.mock.invocationCallOrder[0]!).toBeLessThan(
      client.createAutopilotTrigger.mock.invocationCallOrder[0]!,
    );
    expect(client.createAutopilotTrigger).toHaveBeenCalledTimes(1);
    expect(client.createAutopilotTrigger.mock.calls[0]![0]).toBe("ap-1");
    expect(second.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("a rerun whose re-pause fails sets no schedule, so the autopilot cannot fire", async () => {
    const routine = market.routines[0]!;
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          {
            id: "ap-1",
            title: "Market scan",
            description: routine.description,
            issue_title_template: routine.issueTitleTemplate,
            assignee_type: "squad",
            assignee_id: "squad-1",
            status: "active",
          },
        ] as never[],
        total: 1,
      })),
      updateAutopilot: vi.fn(async () => {
        throw new Error("pause refused");
      }),
    });
    const paused = { ...market, routines: [{ ...routine, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.createAutopilotTrigger).not.toHaveBeenCalled();
    expect(client.createSquadRoutine).not.toHaveBeenCalled();
    expect(out.routines).toEqual([
      {
        title: "Market scan",
        status: "failed",
        error: "pause: pause refused; the autopilot is active without a schedule, pause or delete it",
      },
    ]);
  });

  it("leaves an active autopilot the boss edited alone, even with no schedule, and never pauses it", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          {
            id: "ap-1",
            title: "Market scan",
            description: "The boss's own scan brief",
            issue_title_template: market.routines[0]!.issueTitleTemplate,
            assignee_type: "squad",
            assignee_id: "squad-1",
            status: "active",
          },
        ] as never[],
        total: 1,
      })),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).toHaveBeenCalledTimes(1);
    expect(out.routines).toEqual([expect.objectContaining({ title: "Market scan", status: "partial" })]);
  });

  it("keeps the schedule the boss set on an active routine, and changes nothing on a rerun", async () => {
    const routine = market.routines[0]!;
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          {
            id: "ap-1",
            title: "Market scan",
            description: routine.description,
            issue_title_template: routine.issueTitleTemplate,
            assignee_type: "squad",
            assignee_id: "squad-1",
            status: "active",
          },
        ] as never[],
        total: 1,
      })),
      getAutopilot: vi.fn(async () => ({ autopilot: {}, triggers: [{ kind: "schedule", enabled: true }] }) as never),
    });
    const paused = { ...market, routines: [{ ...routine, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).not.toHaveBeenCalled();
    expect(out.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("adds the schedule to a paused autopilot the quickstart made, without a second pause", async () => {
    const routine = market.routines[0]!;
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          {
            id: "ap-1",
            title: "Market scan",
            description: routine.description,
            issue_title_template: routine.issueTitleTemplate,
            assignee_type: "squad",
            assignee_id: "squad-1",
            status: "paused",
          },
        ] as never[],
        total: 1,
      })),
    });
    const paused = { ...market, routines: [{ ...routine, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
    expect(client.createAutopilotTrigger).toHaveBeenCalledTimes(1);
    expect(out.routines).toEqual([{ title: "Market scan", status: "existing" }]);
  });

  it("reports a routine whose schedule could not be set, so the boss can add it", async () => {
    const client = fakeClient({
      createAutopilotTrigger: vi.fn(async () => {
        throw new Error("bad cron");
      }),
    });

    const out = await runSquadQuickstart(client, market, options);

    expect(out.routines).toEqual([{ title: "Market scan", status: "failed", error: "schedule: bad cron" }]);
  });

  it("pauses a routine's autopilot it creates, before the schedule is set", async () => {
    const client = fakeClient();
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).toHaveBeenCalledWith("ap-1", { status: "paused" });
    expect(client.updateAutopilot.mock.invocationCallOrder[0]!).toBeLessThan(
      client.createAutopilotTrigger.mock.invocationCallOrder[0]!,
    );
    expect(out.routines).toEqual([{ title: "Market scan", status: "created" }]);
  });

  it("leaves a routine active when the template does not ask for it paused", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, market, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
  });

  it("never pauses or resumes a routine that already exists", async () => {
    const client = fakeClient({
      listAutopilots: vi.fn(async () => ({
        autopilots: [
          { id: "old", title: "Market scan", assignee_type: "squad", assignee_id: "squad-1", status: "active" },
        ] as never[],
        total: 1,
      })),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    await runSquadQuickstart(client, paused, options);

    expect(client.updateAutopilot).not.toHaveBeenCalled();
  });

  it("reports a pause that fails, and sets no schedule on that autopilot", async () => {
    const client = fakeClient({
      updateAutopilot: vi.fn(async () => {
        throw new Error("pause refused");
      }),
    });
    const paused = { ...market, routines: [{ ...market.routines[0]!, startPaused: true }] };

    const out = await runSquadQuickstart(client, paused, options);

    expect(client.createAutopilotTrigger).not.toHaveBeenCalled();
    expect(out.routines).toEqual([
      {
        title: "Market scan",
        status: "failed",
        error: "pause: pause refused; the autopilot is active without a schedule, pause or delete it",
      },
    ]);
  });

  it("makes no routine for a template without one, or when the squad was not made", async () => {
    const greenfieldClient = fakeClient();
    expect((await runSquadQuickstart(greenfieldClient, greenfield, options)).routines).toEqual([]);
    expect(greenfieldClient.createAutopilot).not.toHaveBeenCalled();

    const failedClient = fakeClient({
      createSquad: vi.fn(async () => {
        throw new Error("nope");
      }),
    });
    expect((await runSquadQuickstart(failedClient, market, options)).routines).toEqual([]);
    expect(failedClient.createAutopilot).not.toHaveBeenCalled();
  });

  it("sends no floor name for the SDLC squads", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, greenfield, options);

    expect(client.createSquad.mock.calls[0]![0]).not.toHaveProperty("floor_name");
    expect(client.listCompanyFloors).not.toHaveBeenCalled();
    expect(client.setCompanySeats).not.toHaveBeenCalled();
  });
});

// Spec 2.10: every squad created from a template has a charter and its routines.
// The charter's first save creates the operating review, which is the SDLC squads' routine.
describe("runSquadQuickstart for every template", () => {
  it.each(SQUAD_TEMPLATES.map((template) => [template.id, template] as const))(
    "%s saves its charter on the squad it creates",
    async (_id, template) => {
      const client = fakeClient();

      const out = await runSquadQuickstart(client, template, options);

      expect(out.squad.status).toBe("created");
      expect(out.squad.error).toBeUndefined();
      expect(client.updateSquadCharter).toHaveBeenCalledTimes(1);
      const [squadId, request] = client.updateSquadCharter.mock.calls[0]!;
      expect(squadId).toBe("squad-1");
      expect(request).toMatchObject({ revision: 0, mission: template.charter.mission });
    },
  );

  it.each(SQUAD_TEMPLATES.map((template) => [template.id, template] as const))(
    "%s has the routine its template names, and no other",
    async (_id, template) => {
      const client = fakeClient();

      const out = await runSquadQuickstart(client, template, options);

      expect(out.routines.map((r) => r.status)).toEqual(template.routines.map(() => "created"));
      expect(client.createAutopilot).toHaveBeenCalledTimes(template.routines.length);
    },
  );
});

// A re-run after an earlier run stopped at the charter save: the squad exists
// with the template's leader and instructions but no charter.
describe("runSquadQuickstart re-run for an existing squad", () => {
  const leaderAgent = asAgent("ag-lead", SQUAD_ROLES.lead.name);
  const quickstartSquad = {
    id: "s",
    name: greenfield.name,
    archived_at: null,
    leader_id: "ag-lead",
    instructions: greenfield.instructions,
  } as unknown as Squad;
  const agentsAll = [leaderAgent] as Agent[];

  it("saves the revision-0 charter on a squad an earlier run made", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => agentsAll),
      listSquads: vi.fn(async () => [quickstartSquad]),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.createSquad).not.toHaveBeenCalled();
    expect(client.updateSquadCharter).toHaveBeenCalledTimes(1);
    expect(client.updateSquadCharter.mock.calls[0]![0]).toBe("s");
    expect(client.updateSquadCharter.mock.calls[0]![1]).toMatchObject({
      revision: 0,
      mission: greenfield.charter.mission,
    });
    expect(out.squad).toEqual({ id: "s", status: "existing" });
  });

  it("treats a 409 on that save as already chartered, with no error", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => agentsAll),
      listSquads: vi.fn(async () => [quickstartSquad]),
      updateSquadCharter: vi.fn(async () => {
        throw Object.assign(new Error("conflict"), { body: { code: "charter_revision_conflict" } });
      }),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.squad).toEqual({ id: "s", status: "existing" });
  });

  it("reports any other charter refusal on the existing squad", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => agentsAll),
      listSquads: vi.fn(async () => [quickstartSquad]),
      updateSquadCharter: vi.fn(async () => {
        throw new Error("mission too long");
      }),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.squad.error).toBe("charter: mission too long");
  });

  it("leaves a hand-made squad with the same name without a charter", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => agentsAll),
      listSquads: vi.fn(async () => [
        { ...quickstartSquad, leader_id: "someone-else", instructions: "my own rules" },
      ]),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.updateSquadCharter).not.toHaveBeenCalled();
    expect(out.squad).toEqual({ id: "s", status: "existing" });
  });
});

describe("runSquadQuickstart SDLC agents", () => {
  it("creates each SDLC agent from its slim SDLC instructions, not the shared template", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, greenfield, options);

    const byName = Object.fromEntries(client.createAgent.mock.calls.map((c) => [c[0].name, c[0].instructions]));
    expect(byName[SQUAD_ROLES.analyst.name]).toBe(SDLC_ROLE_AGENT_INSTRUCTIONS.analyst);
    expect(byName[SQUAD_ROLES.developer.name]).toBe(SDLC_ROLE_AGENT_INSTRUCTIONS.developer);
    expect(byName[SQUAD_ROLES.lead.name]).toBe(SDLC_ROLE_AGENT_INSTRUCTIONS.lead);
  });

  it("keeps the shared template for a market research role", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, marketTemplate, options);

    const scout = client.createAgent.mock.calls.find((c) => c[0].name === SQUAD_ROLES.scout.name)![0];
    expect(scout.instructions).toBe(
      AGENT_INSTRUCTION_TEMPLATES.find((t) => t.id === "market_scout")!.instructions,
    );
  });
});

describe("runSquadQuickstart built-in skills", () => {
  const sdlcNames = greenfield.builtinSkills;
  const bundleOf = (name: string) => SDLC_BUILTIN_SKILLS.find((b) => b.name === name)!;

  it("imports every built-in SDLC skill and attaches it to the new squad", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.createSkill).toHaveBeenCalledTimes(sdlcNames.length);
    const first = client.createSkill.mock.calls[0]![0];
    expect(first).toMatchObject({
      name: sdlcNames[0],
      description: bundleOf(sdlcNames[0]!).description,
      content: bundleOf(sdlcNames[0]!).content,
      config: { source: SDLC_SKILL_SOURCE },
    });
    expect(client.setSquadSkills).toHaveBeenCalledWith(
      "squad-1",
      sdlcNames.map((name) => `skill-${name}`),
    );
    expect(out.skills).toEqual({ status: "ready" });
  });

  it("updates a built-in skill an earlier run made when its text has changed", async () => {
    const stale = {
      id: "skill-old",
      name: sdlcNames[0],
      description: "old",
      content: "old body",
      config: { source: SDLC_SKILL_SOURCE },
      files: [],
    };
    const client = fakeClient({
      listSkills: vi.fn(async () => [stale] as never),
      getSkill: vi.fn(async () => stale as never),
    });

    await runSquadQuickstart(client, greenfield, options);

    expect(client.createSkill.mock.calls.map((c) => c[0].name)).not.toContain(sdlcNames[0]);
    expect(client.updateSkill).toHaveBeenCalledWith("skill-old", {
      description: bundleOf(sdlcNames[0]!).description,
      content: bundleOf(sdlcNames[0]!).content,
      files: bundleOf(sdlcNames[0]!).files,
    });
  });

  it("leaves a skill the boss made under the same name, and reports it", async () => {
    const own = {
      id: "skill-own",
      name: sdlcNames[0],
      description: "mine",
      content: "mine",
      config: {},
      files: [],
    };
    const client = fakeClient({
      listSkills: vi.fn(async () => [own] as never),
      getSkill: vi.fn(async () => own as never),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.updateSkill).not.toHaveBeenCalled();
    expect(client.setSquadSkills).toHaveBeenCalledWith(
      "squad-1",
      sdlcNames.slice(1).map((name) => `skill-${name}`),
    );
    expect(out.skills.status).toBe("partial");
    expect(out.skills.error).toContain(`${sdlcNames[0]}: the workspace has its own skill`);
  });

  it("attaches a boss's own copy of a built-in skill when its text already matches", async () => {
    const bundle = bundleOf(sdlcNames[0]!);
    const copy = {
      id: "skill-copy",
      name: sdlcNames[0],
      description: bundle.description,
      content: bundle.content,
      config: {},
      files: bundle.files.map((f) => ({ ...f, id: `file-${f.path}` })),
    };
    const client = fakeClient({
      listSkills: vi.fn(async () => [copy] as never),
      getSkill: vi.fn(async () => copy as never),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(client.updateSkill).not.toHaveBeenCalled();
    expect(client.setSquadSkills).toHaveBeenCalledWith(
      "squad-1",
      sdlcNames.map((name) => (name === sdlcNames[0] ? "skill-copy" : `skill-${name}`)),
    );
    expect(out.skills).toEqual({ status: "ready" });
  });

  it("keeps the squad's other skills when it attaches the built-in ones", async () => {
    const client = fakeClient({
      listSquadSkills: vi.fn(async () => [{ id: "skill-boss", name: "x", description: "" }] as never),
    });

    await runSquadQuickstart(client, greenfield, options);

    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", [
      "skill-boss",
      ...sdlcNames.map((name) => `skill-${name}`),
    ]);
  });

  it("imports no skill for a template that names none", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, marketTemplate, options);

    expect(client.listSkills).not.toHaveBeenCalled();
    expect(client.setSquadSkills).not.toHaveBeenCalled();
  });

  it("attaches the skills to an existing quickstart squad on a rerun", async () => {
    const client = fakeClient({
      listSquads: vi.fn(async () => [
        {
          id: "squad-1",
          name: greenfield.name,
          leader_id: "agent-1",
          instructions: greenfield.instructions,
          archived_at: null,
        } as unknown as Squad,
      ]),
      listAgents: vi.fn(async () =>
        greenfield.roles.map((role, i) => asAgent(i === 0 ? "agent-1" : `agent-${i + 1}`, SQUAD_ROLES[role].name)),
      ),
    });

    const out = await runSquadQuickstart(client, greenfield, options);

    expect(out.squad.status).toBe("existing");
    expect(client.setSquadSkills).toHaveBeenCalledWith(
      "squad-1",
      sdlcNames.map((name) => `skill-${name}`),
    );
  });
});

describe("planQuickstartAgents", () => {
  it("marks which agents will be reused", () => {
    const plan = planQuickstartAgents(greenfield, [
      asAgent("a", SQUAD_ROLES.lead.name),
      asAgent("b", SQUAD_ROLES.qa.name, true),
    ]);

    expect(plan.find((p) => p.role === "lead")?.reuse).toBe(true);
    expect(plan.find((p) => p.role === "qa")?.reuse).toBe(false);
    expect(plan).toHaveLength(greenfield.roles.length);
  });
});

describe("runSquadQuickstart planning squad", () => {
  const planning = SQUAD_TEMPLATES.find((t) => t.id === "bmad_planning")!;

  it("creates seven agents with the planning instructions, the squad, its workflow and its own skills", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, planning, options);

    expect(client.createAgent).toHaveBeenCalledTimes(7);
    const byName = Object.fromEntries(client.createAgent.mock.calls.map((c) => [c[0].name, c[0].instructions]));
    expect(byName[SQUAD_ROLES.plan_lead.name]).toBe(PLANNING_ROLE_AGENT_INSTRUCTIONS.plan_lead);
    expect(byName[SQUAD_ROLES.plan_pm.name]).toBe(PLANNING_ROLE_AGENT_INSTRUCTIONS.plan_pm);
    expect(client.createSquad).toHaveBeenCalledWith(
      expect.objectContaining({ name: "BMAD Planning", leader_id: "agent-1" }),
    );
    expect(client.addSquadMember).toHaveBeenCalledTimes(6);
    expect(client.createIssueWorkflow.mock.calls[0]![0]).toMatchObject({ name: "BMAD Planning Auto" });
    expect(client.createSkill.mock.calls.map((c) => c[0].name)).toEqual(planning.builtinSkills);
    expect(out.skills).toEqual({ status: "ready" });
    expect(out.workflow.status).toBe("created");
  });

  it("reuses a planning agent that already has its name, and creates the rest", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [asAgent("keep", SQUAD_ROLES.plan_ux.name)]),
    });

    const out = await runSquadQuickstart(client, planning, options);

    expect(out.agents.find((a) => a.role === "plan_ux")).toMatchObject({ status: "reused", id: "keep" });
    expect(client.createAgent).toHaveBeenCalledTimes(6);
  });

  it("names the new statuses of the planning workflow when a workspace admin must allow them", async () => {
    const forbidden = Object.assign(new Error("admins only"), { body: { code: "status_create_forbidden" } });
    const client = fakeClient({ createIssueWorkflow: vi.fn().mockRejectedValueOnce(forbidden) });

    const out = await runSquadQuickstart(client, planning, options);

    expect(out.workflow.error).toContain("Research to Plan ready");
  });
});

describe("runSquadQuickstart for the sales loop", () => {
  const sales = SQUAD_TEMPLATES.find((t) => t.id === "sales_loop")!;

  it("makes the three routines and writes every starter source under its own key", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, sales, options);

    expect(out.routines.map((r) => r.title)).toEqual(["Weekly scan", "Weekly review", "Source health"]);
    expect(out.sources.status).toBe("created");
    expect(out.sources.written).toBe(SALES_STARTER_SOURCES.length);
    const keys = client.createKnowledgeDocument.mock.calls.map((call) => call[0].idempotency_key);
    expect(new Set(keys).size).toBe(SALES_STARTER_SOURCES.length);
    expect(client.createKnowledgeDocument.mock.calls.every((call) => call[0].kind === "sales_source" && call[0].kb === "sales")).toBe(true);
  });

  it("writes the same keys again on a rerun, so the knowledge base keeps one document each", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, sales, options);
    const first = client.createKnowledgeDocument.mock.calls.map((call) => call[0].idempotency_key);
    await runSquadQuickstart(client, sales, options);
    const second = client.createKnowledgeDocument.mock.calls.map((call) => call[0].idempotency_key);

    expect(second).toEqual([...first, ...first]);
  });

  it("skips the sources, with the reason, while the knowledge base is not enabled", async () => {
    const client = fakeClient({
      getKnowledgeStatus: vi.fn(async () => ({ state: "disabled" }) as never),
    });

    const out = await runSquadQuickstart(client, sales, options);

    expect(out.squad.status).toBe("created");
    expect(out.sources.status).toBe("skipped");
    expect(out.sources.error).toMatch(/Enable the company knowledge base/);
    expect(client.createKnowledgeDocument).not.toHaveBeenCalled();
  });

  it("counts a refused document and keeps writing the rest", async () => {
    let calls = 0;
    const client = fakeClient({
      createKnowledgeDocument: vi.fn(async () => {
        calls += 1;
        if (calls === 2) throw new Error("kind sales_source is not in this workspace");
        return { id: `doc-${calls}` } as never;
      }),
    });

    const out = await runSquadQuickstart(client, sales, options);

    expect(out.sources.status).toBe("failed");
    expect(out.sources.failed).toBe(1);
    expect(out.sources.written).toBe(SALES_STARTER_SOURCES.length - 1);
    expect(out.sources.error).toContain("kind sales_source is not in this workspace");
  });
});

describe("runSquadQuickstart for the research lab", () => {
  const lab = SQUAD_TEMPLATES.find((t) => t.id === "research_lab")!;

  it("makes the five lab agents with the lab's own instructions, led by the lab lead", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, lab, options);

    expect(out.agents.map((a) => [a.name, a.status])).toEqual([
      ["Research Lab Lead", "created"],
      ["Literature Scout", "created"],
      ["Relevance Screener", "created"],
      ["Evidence Synthesizer", "created"],
      ["Citation Auditor", "created"],
    ]);
    const created = client.createAgent.mock.calls.map((c) => c[0]);
    expect(created.every((a) => a.runtime_id === "rt-1")).toBe(true);
    expect(created[0]!.instructions).toContain("You are the Research Lab Lead");
    expect(created[1]!.instructions).toContain("You are the Literature Scout");
    expect(created.every((a) => a.instructions!.includes("research-lab-delivery-core"))).toBe(true);
    expect(client.createSquad).toHaveBeenCalledWith(expect.objectContaining({ name: "Research Lab", leader_id: "agent-1" }));
  });

  it("seats the squad on the Research floor and adds the four other members", async () => {
    const client = fakeClient();

    await runSquadQuickstart(client, lab, options);

    expect(client.createCompanyFloor).toHaveBeenCalledWith({ name: "Research" });
    expect(client.addSquadMember).toHaveBeenCalledTimes(4);
    expect(client.addSquadMember.mock.calls.map((c) => c[1].member_id)).toEqual(["agent-2", "agent-3", "agent-4", "agent-5"]);
  });

  it("imports the six research-lab skills, and only those, onto the squad", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, lab, options);

    const names = lab.builtinSkills;
    expect(names).toHaveLength(6);
    expect(client.createSkill.mock.calls.map((c) => c[0].name)).toEqual([...names]);
    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", names.map((n) => `skill-${n}`));
    expect(out.skills).toEqual({ status: "ready" });
  });

  it("opens no routine and no workflow, and writes no knowledge document", async () => {
    const client = fakeClient();

    const out = await runSquadQuickstart(client, lab, options);

    expect(out.routines).toEqual([]);
    expect(out.workflow.status).toBe("skipped");
    expect(client.createSquadRoutine).not.toHaveBeenCalled();
    expect(client.createKnowledgeDocument).not.toHaveBeenCalled();
  });

  it("does not reuse or touch the Market Research agents of the workspace", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [asAgent("m-1", "Research Lead"), asAgent("m-2", "Market Scout"), asAgent("m-3", "Research Critic")]),
    });

    const out = await runSquadQuickstart(client, lab, options);

    expect(out.agents.every((a) => a.status === "created")).toBe(true);
    expect(client.createAgent).toHaveBeenCalledTimes(5);
  });

  it("reuses its own agents on a second run instead of making duplicates", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [asAgent("a-1", "Research Lab Lead"), asAgent("a-2", "Literature Scout")]),
    });

    const out = await runSquadQuickstart(client, lab, options);

    expect(out.agents.map((a) => a.status)).toEqual(["reused", "reused", "created", "created", "created"]);
  });
});

describe("Kit Viet P2 squads on their own floors", () => {
  const p2 = SQUAD_TEMPLATES.filter((template) => KITVIET_P2_FLOOR_NAME_LIST.includes(template.floorName ?? ""));
  const executive = asFloor("floor-exec", "Executive", 0);

  it("has the eleven P2 squads, and each names its own floor except the one documented shared Education floor", () => {
    expect(p2.map((template) => template.id).sort()).toEqual(
      [
        "kv_banking",
        "kv_construction",
        "kv_education",
        "kv_factory",
        "kv_fnb",
        "kv_health",
        "kv_hospitality",
        "kv_insurance",
        "kv_realestate",
        "kv_teaching",
        "kv_trade",
      ].sort(),
    );
    // Teaching Materials sits on the Education floor with Education Desk, by design (spec 9.2).
    const owners = new Map<string, string[]>();
    for (const template of p2) owners.set(template.floorName!, [...(owners.get(template.floorName!) ?? []), template.id]);
    const shared = [...owners].filter(([, ids]) => ids.length > 1).map(([floor, ids]) => [floor, [...ids].sort()]);
    expect(shared).toEqual([["Education", ["kv_education", "kv_teaching"]]]);
  });

  it.each(["kv_health", "kv_construction", "kv_fnb", "kv_insurance", "kv_banking", "kv_realestate", "kv_hospitality", "kv_trade", "kv_factory", "kv_education", "kv_teaching"])(
    "%s creates its floor when the workspace has none, and seats the squad at slot 0 on it",
    async (id) => {
      const template = p2.find((candidate) => candidate.id === id)!;
      const client = fakeClient({ listCompanyFloors: vi.fn(async () => [executive]) });

      const out = await runSquadQuickstart(client, template, options);

      expect(client.createCompanyFloor).toHaveBeenCalledWith({ name: template.floorName });
      expect(client.setCompanySeats).toHaveBeenCalledWith([
        { squad_id: "squad-1", floor_id: `floor-${template.floorName}`, slot: 0 },
      ]);
      expect(out.squad.error ?? "").not.toMatch(/floor|seat/);
    },
  );

  it("reuses the floor of its name when it exists, and does not create a second one", async () => {
    const health = p2.find((candidate) => candidate.id === "kv_health")!;
    const existing = asFloor("floor-health", health.floorName!, 3);
    const client = fakeClient({ listCompanyFloors: vi.fn(async () => [executive, existing]) });

    const out = await runSquadQuickstart(client, health, options);

    expect(client.createCompanyFloor).not.toHaveBeenCalled();
    expect(client.setCompanySeats).toHaveBeenCalledWith([{ squad_id: "squad-1", floor_id: "floor-health", slot: 0 }]);
    expect(out.squad.status).toBe("created");
    expect(out.squad.error ?? "").not.toMatch(/floor|seat/);
  });
});
