import type { AgentInstructionTemplate } from "../agents/instruction-templates";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";

/**
 * Ready-made squads a user can start from in the Squads quickstart. A template
 * names the roles it needs, the instructions its leader follows, and the
 * workflow its build project uses. Card names and descriptions are translated
 * in the views package, keyed by `id`; the agents it creates keep the English
 * names below so that running the quickstart twice reuses them.
 */

export type SquadTemplateId = "sdlc_greenfield" | "sdlc_maintenance" | "market_research";

export type SquadRoleKey =
  | "lead"
  | "analyst"
  | "ux"
  | "architect"
  | "developer"
  | "fixer"
  | "reviewer"
  | "qa"
  | "security"
  | "devops"
  | "research_lead"
  | "scout"
  | "competitor"
  | "critic";

export interface SquadRole {
  key: SquadRoleKey;
  /** The agent instruction template the created agent starts from. */
  agentTemplate: AgentInstructionTemplate["id"];
  /** Name of the created agent. A non-archived agent with this name is reused. */
  name: string;
  description: string;
  /** Roster text the leader reads; context only, it schedules nothing. */
  role: string;
}

export const SQUAD_ROLES: Record<SquadRoleKey, SquadRole> = {
  lead: {
    key: "lead",
    agentTemplate: "tech_lead",
    name: "SDLC Lead",
    description: "Leads SDLC squads: delegates, verifies evidence, and reports.",
    role: "Squad leader",
  },
  analyst: {
    key: "analyst",
    agentTemplate: "product_analyst",
    name: "SDLC Product Analyst",
    description: "Turns requests and sources into briefs, PRDs, epics, and stories.",
    role: "Briefs, PRD, epics and stories, requirement changes, build queue",
  },
  ux: {
    key: "ux",
    agentTemplate: "ux_designer",
    name: "SDLC UX Designer",
    description: "Designs screens, states, and the design system for the requirements.",
    role: "UX design, screen specs, mockups",
  },
  architect: {
    key: "architect",
    agentTemplate: "architect",
    name: "SDLC Architect",
    description: "Designs the simplest system that fits, records decisions, and analyses impact.",
    role: "Architecture, impact analysis, project context",
  },
  developer: {
    key: "developer",
    agentTemplate: "fullstack_developer",
    name: "SDLC Developer",
    description: "Builds stories end to end and verifies them on the running app.",
    role: "Implementation of epics, stories, and features",
  },
  fixer: {
    key: "fixer",
    agentTemplate: "bug_fixer",
    name: "SDLC Bug Fixer",
    description: "Reproduces a defect and fixes it with the smallest change.",
    role: "Defects and small changes in one component",
  },
  reviewer: {
    key: "reviewer",
    agentTemplate: "code_reviewer",
    name: "SDLC Code Reviewer",
    description: "Independent review of changes and plans, with a verdict first.",
    role: "Independent review, planning readiness check",
  },
  qa: {
    key: "qa",
    agentTemplate: "qa_tester",
    name: "SDLC QA Tester",
    description: "Designs tests from acceptance criteria and runs them on the real system.",
    role: "Test design, regression, final verification",
  },
  security: {
    key: "security",
    agentTemplate: "security_reviewer",
    name: "SDLC Security Reviewer",
    description: "Gates a change or release on verified security controls.",
    role: "Security review of changes and of the whole product",
  },
  devops: {
    key: "devops",
    agentTemplate: "devops_engineer",
    name: "SDLC DevOps",
    description: "Packages and releases one verified digest; production needs approval.",
    role: "Packaging, run instructions, release notes",
  },
  research_lead: {
    key: "research_lead",
    agentTemplate: "research_lead",
    name: "Research Lead",
    description: "Leads market research: sets the scan, delegates, and files only evidenced proposals.",
    role: "Squad leader",
  },
  scout: {
    key: "scout",
    agentTemplate: "market_scout",
    name: "Market Scout",
    description: "Finds and reads market, customer, and trend sources, with a date and link for each claim.",
    role: "Market scan",
  },
  competitor: {
    key: "competitor",
    agentTemplate: "competitor_analyst",
    name: "Competitor Analyst",
    description: "Compares competitors' products, pricing, and moves from primary sources.",
    role: "Competitor analysis",
  },
  critic: {
    key: "critic",
    agentTemplate: "critic",
    name: "Research Critic",
    description: "Challenges the evidence before anything is filed, and writes no findings of its own.",
    role: "Challenges the evidence",
  },
};

export interface SquadWorkflowStep {
  /**
   * An existing status key, or a status to create under this name. The key of a
   * created status is its name in lower case with underscores, so `next` and
   * `back` can name it before it exists.
   */
  status: { key: string } | { name: string; category?: "started" | "done" };
  /** The role that takes the issue when it enters the step; null keeps the assignee. */
  handler: SquadRoleKey | null;
  instructions: string;
  /** The status key the handler moves the issue to when its step is done. */
  next?: string;
  back?: string;
}

export interface SquadWorkflowTemplate {
  name: string;
  description: string;
  steps: SquadWorkflowStep[];
}

/**
 * The lead's brief for a blocked build issue. A blocked step with no handler
 * leaves the issue with nobody to unblock it.
 */
export const SDLC_BLOCKED_INSTRUCTIONS =
  "This issue is blocked. Load the sdlc-delivery-core and sdlc-squad-lead skills and follow the lead's Blocked work (Step 7). Read the blocker report, route its class to the specialist who owns it with an unblock issue, verify the ruling, and resume the issue at its Blocked-from status. Do not do the specialist's work yourself, do not leave the issue here, and do not wait for a person. Only an irreversible action, a person's credential, a capability the host must change, or a reached limit goes to a person, with a proposal that carries your recommendation. " +
  "Never set a build issue or its Epic done while a check it needs has not run; name that check as unverified in the report instead. Never change acceptance criteria to get past a blocker.";

/** The brief a planning step gives its owner. The owner's skill holds the procedure. */
const planningBrief = (skill: string, done: string) =>
  `This issue is either a planning stage of a request or an unblock ruling; its description says which. Load the sdlc-delivery-core and ${skill} skills. For a planning stage, do the work the issue names. ${done} For an unblock ruling, follow your skill's Ruling mode and end with the Ruling comment. If you cannot proceed, set the issue to blocked with the blocker report from sdlc-delivery-core.`;

/**
 * The SDLC Auto workflow of a build project. Build statuses hand the issue to the
 * developer, reviewer, QA tester, or lead. The planning statuses (Analysis to
 * Release prep) hand a planning sub-issue to the member who owns that stage, so a
 * request can be planned in the build project itself. Every planning step ends at
 * Stage done, which counts as finished for the queue and the stage barrier. The
 * request ends at Plan ready, which nobody picks up, so the boss decides when it is done.
 */
export const SDLC_AUTO_WORKFLOW: SquadWorkflowTemplate = {
  name: "SDLC Auto",
  description:
    "Planning and build by agents: a planning step hands the sub-issue to its owner, and a build issue goes through developer, independent review, then QA. A rejected step goes back to the developer.",
  steps: [
    { status: { key: "backlog" }, handler: null, instructions: "", next: "todo" },
    {
      status: { name: "Analysis" },
      handler: "analyst",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-product-analysis", "When the requirements, epics, or ledger in the issue are written and posted, move the issue to Stage done."),
    },
    {
      status: { name: "Design" },
      handler: "ux",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-ux-design", "When every screen in the issue has its states and is registered, move the issue to Stage done."),
    },
    {
      status: { name: "Architecture" },
      handler: "architect",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-architecture", "When the design, ADRs, and component list are committed and posted, move the issue to Stage done."),
    },
    {
      status: { name: "Plan review" },
      handler: "reviewer",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-code-review", "Give one verdict. PASS: move the issue to Stage done. CONCERNS or FAIL: set the issue to blocked and list each gap with its id, so the squad lead can create the gap issues. Do not move it to Stage done on CONCERNS."),
    },
    {
      status: { name: "Security review" },
      handler: "security",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-security-review", "PASS: move the issue to Stage done. FAIL or NO_GO: set the issue to blocked and list each finding with its id."),
    },
    {
      status: { name: "Release prep" },
      handler: "devops",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-devops-release", "Move the issue to Stage done only when the report lists the version, digests, and rollback steps. Production waits for the named approval: set the issue to blocked and say what the approver must decide."),
    },
    {
      status: { name: "Environment" },
      handler: "devops",
      next: "stage_done",
      back: "blocked",
      instructions: planningBrief("sdlc-devops-release", "Work in Environment mode. When the environment runs, `docs/sdlc/environment.md` and the one command that rebuilds it are written, and the health output is posted, move the issue to Stage done."),
    },
    { status: { name: "Stage done", category: "done" }, handler: null, instructions: "" },
    { status: { name: "Plan ready" }, handler: null, instructions: "" },
    {
      status: { key: "todo" },
      handler: "developer",
      next: "in_progress",
      instructions:
        "Pick up this issue from To Do. Read its spec, stories, and acceptance criteria, then move it to In Progress to start the build.",
    },
    {
      status: { key: "in_progress" },
      handler: "developer",
      next: "in_review",
      back: "todo",
      instructions:
        "Build everything this issue asks for, story by story, following the story flow in your instructions. Hand it to review only when every story is implemented and verified on the running app. Record what you verified in the spec. If this issue came back from Blocked, read the lead's comment first and re-run the check it names before you hand it to review.",
    },
    {
      status: { key: "in_review" },
      handler: "reviewer",
      next: "qa",
      back: "in_progress",
      instructions:
        "Review the whole change for this issue against its acceptance criteria. Send it back to In Progress with the blocking findings, or pass it on to QA with your verdict. If you cannot finish the review, or a blocking finding is not the developer's to fix, set the issue to blocked with the blocker report from sdlc-delivery-core and give each blocking finding an Owner-role.",
    },
    {
      status: { name: "QA" },
      handler: "qa",
      next: "done",
      back: "in_progress",
      instructions:
        "Verify the change on the running system against the acceptance criteria. NO GO sends it back to In Progress with the defects. GO or CONDITIONAL GO finishes the issue. Run the environment's env-ensure command first when docs/sdlc/environment.md has one. If you cannot run the verification (a service is down, a required check cannot run) or a defect is not the developer's to fix, set the issue to blocked with the blocker report from sdlc-delivery-core and give each defect an Owner-role.",
    },
    { status: { key: "blocked" }, handler: "lead", back: "in_progress", instructions: SDLC_BLOCKED_INSTRUCTIONS },
  ],
};

/** A recurring autopilot the template creates for its squad. */
export interface SquadRoutineTemplate {
  /** Name of the autopilot. A non-archived autopilot with this name on the squad is reused. */
  title: string;
  /** Title of each issue the routine opens. Supports {{date}}. */
  issueTitleTemplate: string;
  /** Five-field cron expression, in the timezone the quickstart passes. */
  cron: string;
  /** The issue body each run starts from: the scan scope and the bar for a proposal. */
  description: string;
}

/** The charter a template saves for its squad on creation. */
export interface SquadCharterTemplate {
  mission: string;
  responsibilities: { key: string; text: string }[];
  outputs: { key: string; kind: "issue" | "proposal" | "kb_document" | "report"; cadenceHint: string }[];
  boundaries: string;
  escalation: string;
  intake: string;
}

export interface SquadTemplate {
  id: SquadTemplateId;
  /** Name of the created squad. */
  name: string;
  description: string;
  /** Roles in the squad; the leader leads and the others are members. */
  roles: SquadRoleKey[];
  /** The role that leads. It is created first and is the squad's leader. */
  leader: SquadRoleKey;
  /**
   * Floor name passed as the squad's floor_name. The server uses it only when
   * it has to make a new floor, so the squad may land on an existing floor.
   */
  floorName: string | null;
  /** The recurring autopilot the template creates for its squad, when it has one. */
  routine: SquadRoutineTemplate | null;
  /** The charter saved on creation. Saving it also creates the squad's operating review. */
  charter: SquadCharterTemplate;
  /** Squad instructions: what the leader follows. */
  instructions: string;
  /** The workflow its build project needs, when it has one. */
  workflow: SquadWorkflowTemplate | null;
  /**
   * Built-in skill names the quickstart imports into the workspace and attaches
   * to the squad, so every member of the squad gets them.
   */
  builtinSkills: readonly string[];
}

/** The built-in SDLC skills, by name. Every SDLC squad imports all of them. */
export const SDLC_SKILL_NAMES: readonly string[] = SDLC_BUILTIN_SKILLS.map((skill) => skill.name);

const SDLC_CHARTER_BOUNDARIES =
  "No production deploys and no deletions without the boss. Spending money needs the boss.";
const SDLC_CHARTER_ESCALATION =
  "A blocked request or an open decision goes to the requester, named on the request.";

/**
 * The SDLC procedure lives in the built-in SDLC skills (sdlc-skills folder), which
 * the quickstart attaches to the squad, so every member gets it. The leader's
 * instructions only say what to load and what the squad is.
 */
const SDLC_SQUAD_INSTRUCTIONS = (kind: string) => `You lead an SDLC squad. A request arrives as an issue assigned to this squad, and you carry it to the end without asking the requester again.

Before any action on an issue, load the sdlc-delivery-core and sdlc-squad-lead skills. They hold the procedure: triage and route, delegation by sub-issue, verification of each report, blocked work, the build queue, and the final report. Follow them. Where this text and a skill differ, the skill wins.

${kind}

Delegate only to the members listed on your Squad Roster. Each member loads its own sdlc-<role> skill, which the squad shares with all of them.`;

const GREENFIELD_INSTRUCTIONS = SDLC_SQUAD_INSTRUCTIONS(
  "This is a Greenfield squad: a product request, which names the Build project with the repository and the \"SDLC Auto\" workflow. Its route (A, B, C, or D) decides the planning stages.",
);

const MAINTENANCE_INSTRUCTIONS = SDLC_SQUAD_INSTRUCTIONS(
  "This is a Maintenance squad: changes to an existing product. Triage each bug or change into a route and a risk tier first, as sdlc-squad-lead describes.",
);

const RESEARCH_COMMON_INSTRUCTIONS = `You lead an SDLC squad. A request arrives as an issue assigned to this squad. You carry it to the end without asking the requester again. You coordinate and verify; the members do the work.

These instructions decide how you delegate here, in place of @mentioning members in a comment. They override the squad protocol's mention rule for this squad: delegation here is by sub-issue only.

How you delegate:
- Your assignees come only from the Squad Roster in your briefing. Take each member's id from the UUID in that member's mention link there. Do not list the workspace's agents (no multica agent list, no searching agents by name or skill), and do not look up ids anywhere else: an agent that is not on the roster is not yours to assign, even when its name fits the work. If no roster member fits a piece, split it across the members who are on the roster, or set the request to blocked and comment the gap.
- Create one sub-issue of the request per piece of work: multica issue create --title "..." --parent <request> --assignee-id <member id> --stage <N> --status todo|backlog --description-file <file>. Assign by that id, never by name: a name can match an agent outside this squad. Create later stages up front as backlog, and promote a stage to todo only when every sub-issue of the earlier stage is done.
- Each description holds: the goal; the input paths or links; the output paths; the skill to run; the acceptance check; the words "This run is unattended. Do not wait for approval; take the recommended default and record it as an assumption".
- Post at most one short comment per turn that links the sub-issues. Do not @mention a member in it: a mention and an assignment would start the same member twice.
- Match the piece to the roster member who owns that kind of work: building an epic or story goes to SDLC Developer, a defect to SDLC Bug Fixer, a design to SDLC UX Designer, and so on. Use that member's id from the roster, and only a member listed there. An assignment request that names a build skill, such as "/bmad-build-auto Implement epic 01", is a build piece: it becomes a sub-issue for the developer, and the skill is named in that sub-issue's description for the developer to run.

You never build:
- Do not run bmad-build, bmad-build-auto, or any other skill, command, or script that writes product code, specs, or designs, even when the request names it. Those belong to the member who owns the piece. If a request names a build skill, delegate it as above and do not run it yourself.

How you verify:
- When a sub-issue reaches in_review, read its report and open the files and evidence it names. If it meets the acceptance check, set it done: multica issue status <sub-issue> done. If not, comment the specific gap on the sub-issue and set it back to todo so its assignee reworks it. A report is a claim until the evidence supports it.
- A build issue in Blocked is unblocked by its workflow step's brief, which overrides the request rule for that issue: route its blocker class to the specialist who owns it with an unblock issue, verify the ruling, then resume the issue (the brief says how), and do not do a member's work yourself. The request rule above still applies to the request.
- Treat an empty, malformed, or failed report as no progress. Retry the same piece at most twice, then change the member or the approach, or split it. Never set a sub-issue done to get past a stage.

Unattended:
- Members take the recommended default for every open question and record it as an assumption (id, decision, why, who can reverse it) in docs/sdlc/assumptions.md. You add any default you took yourself.
- Never wait for a person to approve a brief, PRD, design, plan, approach, epic, or story, and never ask a person to choose between options. This overrides any skill or workflow that asks for approval or a choice, including superpowers:brainstorming and the approval steps of the bmad skills: take its recommended default, record it as an assumption, and continue.
- Stop only for what cannot be undone: deleting data, breaking a released contract, a production release, or spending money. Then set the request to blocked, comment what you need, and mention the requester.
- Never deploy to production, and never delete anything, on your own.
- A business, product, or naming choice that the documents leave open is the member's to make, not a reason to stop. Each sub-issue description says so. The member researches the decision records, PRD, architecture and code, lists two or three options with evidence, chooses the one that changes no released behavior, database schema or API contract, records the decision (options, evidence, choice, why, reversal cost), and continues.
- Stop-loss: a member that fails the same command or edit three times changes the approach. An epic is done only when its member's retrospective is accepted, so set it done only after that verdict.

Records: every turn, run multica squad activity on the issue this turn runs on with outcome "action" when you created or moved a sub-issue, and "no_action" otherwise. Keep the request in progress while work remains. Do not do a member's work yourself.

Final report, one comment on the request when everything is verified:
- Outcome in one line, then what was built or changed and where.
- Evidence: the verification results with links, and anything not verified.
- Decisions and assumptions you or the members took, with ids.
- Open risks and follow-ups, and how to run and roll back.
Then set the request to in_review. Leave done to a person.`;

const MARKET_RESEARCH_INSTRUCTIONS = `${RESEARCH_COMMON_INSTRUCTIONS}

Market research: a weekly scan of markets, customers, and competitors. The routine opens one issue each week; the leader carries it to a report and proposals.

Plan:
- Restate the scan's scope: the markets, the customer segments, and the named competitors. Take the scope from the issue, or else from the company's knowledge base (multica knowledge search "<query>"). Record any scope you choose as an assumption.
- In the first round, create two sub-issues for research members: the market scout (sources and changes in the window) and the competitor analyst (named competitors, their moves and pricing). Give each the question, the start sources, where to post the report, and the acceptance check: every claim has a dated source link.
- When both reports are verified, create one sub-issue for the research critic. It challenges each claim in both reports and in the draft proposals. Each blocking objection is answered with evidence or the claim is dropped.

Proposals:
- File zero to three proposals with multica proposal create --squad "Market Research", after the critic has reviewed them. Each one states the recommendation, the evidence links, the options considered, the cost and risk, and the decision the boss needs. Propose nothing you cannot support with evidence. "No proposal this week" is a valid result.
- Do not write product code, make product decisions, or contact anyone outside the workspace. A proposal is a request for the boss's decision.

Final report, one comment on the request: the window scanned, the proposals filed with their ids, what the critic objected to, the assumptions you took, and what is not verified.`;

export const SQUAD_TEMPLATES: readonly SquadTemplate[] = [
  {
    id: "sdlc_greenfield",
    name: "SDLC Greenfield",
    description:
      "Builds a new product from a request: brief, PRD, UX, architecture, epics, a queued build, final verification, and a report.",
    roles: [
      "lead",
      "analyst",
      "ux",
      "architect",
      "developer",
      "reviewer",
      "qa",
      "security",
      "devops",
    ],
    leader: "lead",
    floorName: null,
    routine: null,
    charter: {
      mission: "Take a product request from brief to a verified build, with a report the boss can act on.",
      responsibilities: [
        { key: "analysis", text: "Turn each request into a brief, PRD, epics and stories the build can follow." },
        { key: "build", text: "Build the queued epics and verify them on the running app." },
        { key: "verification", text: "Run the final regression, security and packaging checks before anyone calls it done." },
      ],
      outputs: [{ key: "final_report", kind: "report", cadenceHint: "per request" }],
      boundaries: SDLC_CHARTER_BOUNDARIES,
      escalation: SDLC_CHARTER_ESCALATION,
      intake: "Requests arrive as issues assigned to this squad.",
    },
    instructions: GREENFIELD_INSTRUCTIONS,
    workflow: SDLC_AUTO_WORKFLOW,
    builtinSkills: SDLC_SKILL_NAMES,
  },
  {
    id: "sdlc_maintenance",
    name: "SDLC Maintenance",
    description:
      "Changes an existing product: triages a bug, feature, optimization, or upgrade, routes it, verifies it, and reports.",
    roles: [
      "lead",
      "analyst",
      "ux",
      "architect",
      "developer",
      "fixer",
      "reviewer",
      "qa",
      "security",
      "devops",
    ],
    leader: "lead",
    floorName: null,
    routine: null,
    charter: {
      mission: "Keep an existing product correct, safe and current: fix bugs, ship changes, and keep it up to date.",
      responsibilities: [
        { key: "triage", text: "Triage each bug or change: type, risk tier, and the route it takes." },
        { key: "delivery", text: "Deliver the change through review and QA, with evidence for each claim." },
        { key: "upkeep", text: "Keep dependencies, the build and the documentation current." },
      ],
      outputs: [{ key: "change_report", kind: "report", cadenceHint: "per change" }],
      boundaries: SDLC_CHARTER_BOUNDARIES,
      escalation: SDLC_CHARTER_ESCALATION,
      intake: "Bugs and changes arrive as issues assigned to this squad.",
    },
    instructions: MAINTENANCE_INSTRUCTIONS,
    workflow: null,
    builtinSkills: SDLC_SKILL_NAMES,
  },
  {
    id: "market_research",
    name: "Market Research",
    description:
      "Scans markets and competitors each week, has a critic challenge the evidence, and files only the proposals it supports.",
    roles: ["research_lead", "scout", "competitor", "critic"],
    leader: "research_lead",
    floorName: "Research",
    routine: {
      title: "Market scan",
      issueTitleTemplate: "Market scan, week of {{date}}",
      cron: "0 8 * * 1",
      description:
        "Weekly market scan. Scope: the markets, customer segments, and named competitors in the research notes; if none are named, start from the company knowledge base (multica knowledge search) and say which markets you chose. Look for changes in the last seven days: pricing, product launches, customer wins and complaints, regulation, and funding. Bar for a proposal: a change that touches a decision the company can make this quarter, backed by at least two dated sources, with its cost and risk stated. File zero to three proposals; no proposal this week is a valid result. Write the report in English.",
    },
    charter: {
      mission: "Scan markets, customers and competitors each week, and file only the proposals the evidence supports.",
      responsibilities: [
        { key: "market_scan", text: "Watch markets and customers for changes in the last seven days, with a dated source for each claim." },
        { key: "competitors", text: "Track named competitors' products, pricing and moves from primary sources." },
        { key: "challenge", text: "Have the critic challenge the evidence before anything is filed." },
      ],
      outputs: [
        { key: "weekly_report", kind: "report", cadenceHint: "weekly" },
        { key: "market_proposals", kind: "proposal", cadenceHint: "0 to 3 a week" },
      ],
      boundaries:
        "Does not write product code, make product decisions, or contact anyone outside the workspace.",
      escalation: "Questions for the boss go in a proposal, not in a chat.",
      intake: "The weekly market scan routine opens each week's issue.",
    },
    instructions: MARKET_RESEARCH_INSTRUCTIONS,
    workflow: null,
    builtinSkills: [],
  },
];
