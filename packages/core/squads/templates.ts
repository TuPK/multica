import type { AgentInstructionTemplate } from "../agents/instruction-templates";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { KV_ADMIN_ROLES, KV_ADMIN_TEMPLATE, type KvAdminRoleKey } from "./kitviet-templates/admin";
import { KV_B2B_ROLES, KV_B2B_TEMPLATE, type KvB2bRoleKey } from "./kitviet-templates/b2b";
import { KV_ACCOUNTING_ROLES, KV_ACCOUNTING_TEMPLATE, type AcctRoleKey } from "./kitviet-templates/accounting";
import { LEGAL_ROLES, KV_LEGAL_TEMPLATE, type LegalRoleKey } from "./kitviet-templates/legal";
import { MARKETING_ROLES, KV_MARKETING_TEMPLATE, type MarketingRoleKey } from "./kitviet-templates/marketing";
import { KV_TALENT_ROLES, KV_TALENT_TEMPLATE, type KvTalentRoleKey } from "./kitviet-templates/talent";
import { PR_ROLES, KV_PR_TEMPLATE, type PrRoleKey } from "./kitviet-templates/pr";
import { KV_ECOM_ROLES, KV_ECOM_TEMPLATE, type KvEcomRoleKey } from "./kitviet-templates/ecom";
import { KV_OFFICEOPS_ROLES, KV_OFFICEOPS_TEMPLATE, type KvOfficeOpsRoleKey } from "./kitviet-templates/officeops";
import { ACADEMIC_ROLES, KV_ACADEMIC_TEMPLATE, type AcademicRoleKey } from "./kitviet-templates/academic";

/**
 * Ready-made squads a user can start from in the Squads quickstart. A template
 * names the roles it needs, the instructions its leader follows, and the
 * workflow its build project uses. Card names and descriptions are translated
 * in the views package, keyed by `id`; the agents it creates keep the English
 * names below so that running the quickstart twice reuses them.
 */

export type SquadTemplateId =
  | "sdlc_greenfield"
  | "sdlc_maintenance"
  | "market_research"
  | "bmad_planning"
  | "sales_loop"
  | "kv_legal"
  | "kv_accounting"
  | "kv_b2b"
  | "kv_marketing"
  | "kv_admin"
  | "kv_talent"
  | "kv_pr"
  | "kv_ecom"
  | "kv_officeops"
  | "kv_academic";

/** Role keys of the SDLC, research, planning and Sales Loop squads. */
type BaseSquadRoleKey =
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
  | "critic"
  | "plan_lead"
  | "plan_researcher"
  | "plan_analyst"
  | "plan_pm"
  | "plan_ux"
  | "plan_architect"
  | "plan_reviewer"
  | "sales_lead"
  | "sales_scout"
  | "sales_writer"
  | "sales_analyst"
  | "sales_checker";

/** Every role key: the base keys above and the roles of each Kit Viet squad. */
export type SquadRoleKey =
  | BaseSquadRoleKey
  | KvAdminRoleKey
  | KvB2bRoleKey
  | MarketingRoleKey
  | LegalRoleKey
  | AcctRoleKey
  | KvTalentRoleKey
  | PrRoleKey
  | KvEcomRoleKey
  | KvOfficeOpsRoleKey
  | AcademicRoleKey;

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

const BASE_SQUAD_ROLES: Record<BaseSquadRoleKey, SquadRole> = {
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
  plan_lead: {
    key: "plan_lead",
    agentTemplate: "tech_lead",
    name: "Blueprint Lead",
    description: "Leads planning squads: reads the request, maps its impact, delegates the documents, and verifies them.",
    role: "Squad leader",
  },
  plan_researcher: {
    key: "plan_researcher",
    agentTemplate: "market_scout",
    name: "Blueprint Researcher",
    description: "Researches markets, domains, competitors, technology, and users, with a dated source for each claim.",
    role: "Market, domain, competitor, technical and user research",
  },
  plan_analyst: {
    key: "plan_analyst",
    agentTemplate: "product_analyst",
    name: "Blueprint Analyst",
    description: "Turns a request and its research into a product brief or PRFAQ, and tests weak ideas.",
    role: "Product brief, PRFAQ, requirement clarification",
  },
  plan_pm: {
    key: "plan_pm",
    agentTemplate: "product_analyst",
    name: "Blueprint PM",
    description: "Writes or extends the PRD, epics and stories, and corrects course when scope changes.",
    role: "PRD, epics and stories, readiness check, course correction",
  },
  plan_ux: {
    key: "plan_ux",
    agentTemplate: "ux_designer",
    name: "Blueprint UX Designer",
    description: "Designs or extends the screens, flows, states, and design system the requirements need.",
    role: "UX design, screens and flows",
  },
  plan_architect: {
    key: "plan_architect",
    agentTemplate: "architect",
    name: "Blueprint Architect",
    description: "Designs the simplest system that fits, analyses the impact of a change, and records decisions.",
    role: "Architecture, ADRs, impact analysis, project context",
  },
  sales_lead: {
    key: "sales_lead",
    agentTemplate: "research_lead",
    name: "Sales Lead",
    description: "Leads Sales Loop squads: calibrates lanes, routes requests, verifies reports, and writes the weekly snapshot.",
    role: "Squad leader",
  },
  sales_scout: {
    key: "sales_scout",
    agentTemplate: "market_scout",
    name: "Sales Scout",
    description: "Runs the lane's sources, scores organisations, and onboards new sources with a sample before they go live.",
    role: "Source collection and scoring",
  },
  sales_writer: {
    key: "sales_writer",
    agentTemplate: "growth_lead",
    name: "Sales Writer",
    description: "Drafts messages, follow-ups and public replies for approval. Sends nothing.",
    role: "Outreach drafts",
  },
  sales_analyst: {
    key: "sales_analyst",
    agentTemplate: "opportunity_analyst",
    name: "Sales Deal Analyst",
    description: "Turns meeting notes into opportunities and quotes from the offer, and writes lessons from outcomes.",
    role: "Opportunities, quotes and lessons",
  },
  sales_checker: {
    key: "sales_checker",
    agentTemplate: "critic",
    name: "Sales Checker",
    description: "Checks every draft and quote against the channel and compliance rules. Writes no drafts of its own.",
    role: "Independent review",
  },
  plan_reviewer: {
    key: "plan_reviewer",
    agentTemplate: "code_reviewer",
    name: "Blueprint Reviewer",
    description: "Independent review of the planning documents for consistency and readiness to build.",
    role: "Independent review, readiness verdict",
  },
};

/** Every role the quickstart can create, by key. A Kit Viet role's agent text is loaded on demand. */
export const SQUAD_ROLES: Record<SquadRoleKey, SquadRole> = {
  ...BASE_SQUAD_ROLES,
  ...KV_ADMIN_ROLES,
  ...KV_B2B_ROLES,
  ...MARKETING_ROLES,
  ...LEGAL_ROLES,
  ...KV_ACCOUNTING_ROLES,
  ...KV_TALENT_ROLES,
  ...PR_ROLES,
  ...KV_ECOM_ROLES,
  ...KV_OFFICEOPS_ROLES,
  ...ACADEMIC_ROLES,
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

/**
 * The lead's brief for a blocked planning issue. A blocked step with no handler
 * leaves the issue with nobody to unblock it.
 */
export const PLANNING_BLOCKED_INSTRUCTIONS =
  "This issue is blocked. Load the planning-delivery-core and planning-squad-lead skills and follow the lead's Blocked work (Step 6). Read the blocker report, route its class to the member who owns it with an unblock issue, verify the answer, and resume the issue at its Blocked-from status. Do not write the member's document yourself, do not leave the issue here, and do not wait for a person. Only scope-reversal, irreversible, or a capability the host must change goes to a person, once, with your recommended default. " +
  "Never change an acceptance criterion or an accepted decision to get past a blocker.";

/** The brief a planning stage gives its owner. The owner's skill holds the procedure. */
const stageBrief = (skill: string, done: string) =>
  `This issue is either a planning stage of a request or an unblock answer; its description says which. Load the planning-delivery-core and ${skill} skills. For a planning stage, do the work the issue names, edit only the documents you own and only the parts its impact map part names, and record what you did. ${done} For an unblock answer, answer the one question with evidence and end with the answer comment. If you cannot proceed, set the issue to blocked with the blocker report from planning-delivery-core.`;

/**
 * The BMAD Planning Auto workflow of a planning project. Each stage hands the issue to the
 * member who owns that document. Every stage ends at Stage done, which counts as finished
 * for the queue and the stage barrier. The request ends at Plan ready, which nobody picks
 * up, so the boss decides what happens to the plan next.
 */
export const PLANNING_AUTO_WORKFLOW: SquadWorkflowTemplate = {
  name: "BMAD Planning Auto",
  description:
    "Planning by agents: research, brief, PRD, design, architecture, epics, then an independent readiness review. A stage hands the issue to the member who owns that document.",
  steps: [
    { status: { key: "backlog" }, handler: null, instructions: "", next: "todo" },
    { status: { key: "todo" }, handler: null, instructions: "" },
    {
      status: { name: "Research" },
      handler: "plan_researcher",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-research", "When the report is written, sourced and committed, move the issue to Stage done."),
    },
    {
      status: { name: "Brief" },
      handler: "plan_analyst",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-product", "Work the Brief stage. When the brief is written and committed, move the issue to Stage done."),
    },
    {
      status: { name: "PRD" },
      handler: "plan_pm",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-product", "Work the PRD stage. When the PRD is written or extended, its ids are stable, and its Change log is updated, move the issue to Stage done."),
    },
    {
      status: { name: "Design" },
      handler: "plan_ux",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-ux", "When every screen in the issue has its states and is registered, move the issue to Stage done."),
    },
    {
      status: { name: "Architecture" },
      handler: "plan_architect",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-architecture", "When the design, ADRs, and component list are committed and posted, move the issue to Stage done."),
    },
    {
      status: { name: "Epics" },
      handler: "plan_pm",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-product", "Work the Epics stage. When the epics, stories, traceability table, and readiness check are written, move the issue to Stage done."),
    },
    {
      status: { name: "Plan review" },
      handler: "plan_reviewer",
      next: "stage_done",
      back: "blocked",
      instructions: stageBrief("planning-readiness", "Give one verdict. PASS: move the issue to Stage done. CONCERNS or FAIL: set the issue to blocked and list each gap with its id, so the squad lead can create the gap issues. Do not move it to Stage done on CONCERNS."),
    },
    { status: { name: "Stage done", category: "done" }, handler: null, instructions: "" },
    { status: { name: "Plan ready" }, handler: null, instructions: "" },
    { status: { key: "blocked" }, handler: "plan_lead", instructions: PLANNING_BLOCKED_INSTRUCTIONS },
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
  /**
   * Create the autopilot paused. A rerun re-pauses only the quickstart's own autopilot that a
   * failed pause left active and unscheduled (ensureRoutine); any other existing autopilot keeps its
   * status, so a rerun never resumes a routine the boss paused or enabled.
   */
  startPaused?: boolean;
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
  /** The recurring autopilots the template creates for its squad. Empty when it has none. */
  routines: readonly SquadRoutineTemplate[];
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
  /**
   * Built-in skill names the leader agent gets on top of the squad's skills. The
   * quickstart attaches them to the leader alone with addAgentSkills.
   */
  leaderSkills?: readonly string[];
  /**
   * Built-in skill names each role's agent gets on top of the squad's skills, attached
   * to that agent alone with addAgentSkills. Roles without an entry get none.
   */
  roleSkills?: Partial<Record<SquadRoleKey, readonly string[]>>;
}

/**
 * The built-in skills the planning squad imports. The SDLC squads do not: the planning
 * procedure skills and the research and design tools below ship in the same bundle folder
 * but belong to the planning template alone.
 */
export const PLANNING_SKILL_NAMES: readonly string[] = [
  "planning-delivery-core",
  "planning-squad-lead",
  "planning-research",
  "planning-product",
  "planning-ux",
  "planning-architecture",
  "planning-readiness",
  "bmad-method:bmad-product-brief",
  "bmad-method:bmad-prfaq",
  "bmad-method:bmad-prd",
  "bmad-method:bmad-ux",
  "bmad-method:bmad-architecture",
  "bmad-method:bmad-create-epics-and-stories",
  "bmad-method:bmad-sprint-planning",
  "bmad-method:bmad-correct-course",
  "bmad-method:bmad-project-context",
  "bmad-method:bmad-retrospective",
  "bmad-method:bmad-walkthrough",
  "bmad-method:bmad-spec",
  "bmad-method:bmad-agent-analyst",
  "bmad-method:bmad-agent-pm",
  "bmad-method:bmad-agent-architect",
  "bmad-method:bmad-agent-ux-designer",
  "bmad-toolbox:bmad-deep-recon",
  "bmad-toolbox:bmad-forge-idea",
  "bmad-toolbox:bmad-review",
  "bmad-toolbox:bmad-advanced-elicitation",
  "frontend-design:frontend-design",
  "superpowers:brainstorming",
  "superpowers:writing-plans",
  "superpowers:dispatching-parallel-agents",
  "superpowers:verification-before-completion",
];

/** The built-in skills of the sales squad. They ship in the bundle folder with the sales- prefix. */
export const SALES_SKILL_NAMES: readonly string[] = [
  "sales-delivery-core",
  "sales-squad-lead",
  "sales-scouting",
  "sales-source-builder",
  "sales-writing",
  "sales-deal-analysis",
  "sales-review",
];

/** Skills that ship in the bundle for other squads only: planning and sales. No SDLC squad imports them. */
const NON_SDLC_SKILLS: readonly string[] = SDLC_BUILTIN_SKILLS.map((skill) => skill.name).filter(
  (name) =>
    name.startsWith("planning-") ||
    name.startsWith("sales-") ||
    name.startsWith("bmad-toolbox:") ||
    name === "frontend-design:frontend-design",
);

/** The built-in SDLC skills, by name. Every SDLC squad imports all of them, and none of the planning or sales ones. */
export const SDLC_SKILL_NAMES: readonly string[] = SDLC_BUILTIN_SKILLS.map((skill) => skill.name).filter(
  (name) => !NON_SDLC_SKILLS.includes(name),
);

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

const SALES_LOOP_INSTRUCTIONS = `You lead a Sales Loop squad. A request arrives as an issue assigned to this squad, or a routine opens the week's issue. You carry it to the end without asking the requester again.

Before any action on an issue, load the sales-delivery-core and sales-squad-lead skills. They hold the procedure: the route (calibration, weekly scan, after a meeting, weekly review, source onboarding, share and dig), delegation by sub-issue, verification against the knowledge base, and the weekly snapshot. Follow them. Where this text and a skill differ, the skill wins.

This squad finds and cares for business opportunities across industries. Its knowledge base is the base "sales": accounts, opportunities, drafts, lessons and sources are documents there. Nothing is sent by the squad: drafts wait for a person, and a person sends them.

Delegate only to the members listed on your Squad Roster. Each member loads its own sales skill, which the squad shares with all of them.`;

const BMAD_PLANNING_INSTRUCTIONS = `You lead a planning squad. A request arrives as an issue assigned to this squad, and you carry it to reviewed planning documents without asking the requester again.

Before any action on an issue, load the planning-delivery-core and planning-squad-lead skills. They hold the procedure: choose new or extend mode, write the impact map, split the work into queued issues, verify each report, route blocked work, and close with the planning handoff packet. Follow them. Where this text and a skill differ, the skill wins.

This squad writes planning documents only: research, brief, PRD, UX design, architecture, epics and stories. It writes no product code. A request names the project with the repository and the "BMAD Planning Auto" workflow.

Delegate only to the members listed on your Squad Roster. Each member loads its own planning skill, which the squad shares with all of them.`;

const BASE_SQUAD_TEMPLATES: readonly SquadTemplate[] = [
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
    routines: [],
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
    routines: [],
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
    routines: [
      {
        title: "Market scan",
      issueTitleTemplate: "Market scan, week of {{date}}",
      cron: "0 8 * * 1",
      description:
        "Weekly market scan. Scope: the markets, customer segments, and named competitors in the research notes; if none are named, start from the company knowledge base (multica knowledge search) and say which markets you chose. Look for changes in the last seven days: pricing, product launches, customer wins and complaints, regulation, and funding. Bar for a proposal: a change that touches a decision the company can make this quarter, backed by at least two dated sources, with its cost and risk stated. File zero to three proposals; no proposal this week is a valid result. Write the report in English.",
      },
    ],
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
  {
    id: "sales_loop",
    name: "Sales Loop",
    description:
      "Finds business opportunities for your lanes each week, drafts outreach for a person to approve, and learns from the results. Works from the knowledge base; sources come ready to use.",
    roles: ["sales_lead", "sales_scout", "sales_writer", "sales_analyst", "sales_checker"],
    leader: "sales_lead",
    floorName: null,
    routines: [
      {
        title: "Weekly scan",
        issueTitleTemplate: "Weekly scan, week of {{date}}",
        cron: "0 8 * * 1",
        description:
          "Weekly scan of every active lane in the sales base. For each lane: run its active sources (multica sales collect), score the organisations, write their accounts, draft outreach only for qualified accounts, have the checker review each draft, then write the sales report. Scope and caps come from each sales_lane document. A lane still in draft is skipped and listed. No new qualified account this week is a valid result.",
      },
      {
        title: "Weekly review",
        issueTitleTemplate: "Weekly review, week of {{date}}",
        cron: "0 20 * * 0",
        description:
          "Weekly review of the sales base: accounts won, lost and stale; the sources that are degraded or blocked; the drafts a person changed before approval. Write lessons at low confidence with their evidence counts. Only a confirmed lesson may change a weight, filter or template. Close with the sales report.",
      },
      {
        title: "Source health",
        issueTitleTemplate: "Source health, {{date}}",
        cron: "0 7 * * *",
        description:
          "Daily source health. Run multica sales collect with --dry-run for each active source that is not a paid provider or a connector, record last_run, last_http, hits_7d and status on each source document, and repair at most two sources a week. Report every source that failed twice in a row.",
      },
    ],
    charter: {
      mission: "Find, reach, track and learn from business opportunities for each lane, from sources that are ready to use, with every draft approved by a person before it is sent.",
      responsibilities: [
        { key: "calibration", text: "Calibrate each lane's offer, ICP and sources with the boss, once, before any outreach is drafted." },
        { key: "discovery", text: "Scan the lane's sources, score the organisations, and keep the sales base current with a source and a date for each claim." },
        { key: "outreach", text: "Draft outreach for qualified accounts in the channel the lane allows, check each draft, and never send it." },
        { key: "learning", text: "Review outcomes weekly and write lessons whose confidence matches their evidence." },
      ],
      outputs: [
        { key: "sales_report", kind: "report", cadenceHint: "weekly" },
        { key: "sales_documents", kind: "kb_document", cadenceHint: "per request" },
        { key: "calibration_proposal", kind: "proposal", cadenceHint: "once per lane" },
      ],
      boundaries:
        "Never sends, posts, quotes or contacts anyone. Quotes come only from the lane's offer. Spending money, a paid provider beyond its cap, and any connection need the boss.",
      escalation: "A decision the boss must make goes to a proposal; a blocked request goes to the requester, named on the request.",
      intake: "Requests arrive as issues assigned to this squad, and the weekly routines open the scan and review issues.",
    },
    instructions: SALES_LOOP_INSTRUCTIONS,
    workflow: null,
    builtinSkills: SALES_SKILL_NAMES,
  },
  {
    id: "bmad_planning",
    name: "BMAD Planning",
    description:
      "Turns a request into reviewed planning documents: research, brief, PRD, UX design, architecture, epics and stories. Extends the documents a product already has.",
    roles: [
      "plan_lead",
      "plan_researcher",
      "plan_analyst",
      "plan_pm",
      "plan_ux",
      "plan_architect",
      "plan_reviewer",
    ],
    leader: "plan_lead",
    floorName: null,
    routines: [],
    charter: {
      mission: "Turn a request into planning documents the build squad can follow, and keep the documents of an existing product current.",
      responsibilities: [
        { key: "research", text: "Research the market, domain, competitors and users, with a dated source for each claim." },
        { key: "documents", text: "Write or extend the brief, PRD, UX design, architecture, epics and stories, keeping existing ids stable." },
        { key: "readiness", text: "Have an independent reviewer check the documents agree and can be built before anyone calls the plan ready." },
      ],
      outputs: [
        { key: "planning_handoff", kind: "report", cadenceHint: "per request" },
        { key: "planning_documents", kind: "kb_document", cadenceHint: "per request" },
      ],
      boundaries:
        "Writes planning documents only, no product code. Does not reverse an accepted decision or change a criterion of work already built without the requester. Spending money needs the boss.",
      escalation: "A change that reverses an accepted decision or a built criterion goes to the requester, named on the request.",
      intake: "Requests arrive as issues assigned to this squad.",
    },
    instructions: BMAD_PLANNING_INSTRUCTIONS,
    workflow: PLANNING_AUTO_WORKFLOW,
    builtinSkills: PLANNING_SKILL_NAMES,
  },
];

/** The templates the Squads quickstart offers. */
export const SQUAD_TEMPLATES: readonly SquadTemplate[] = [
  ...BASE_SQUAD_TEMPLATES,
  KV_ADMIN_TEMPLATE,
  KV_B2B_TEMPLATE,
  KV_MARKETING_TEMPLATE,
  KV_LEGAL_TEMPLATE,
  KV_ACCOUNTING_TEMPLATE,
  KV_TALENT_TEMPLATE,
  KV_PR_TEMPLATE,
  KV_ECOM_TEMPLATE,
  KV_OFFICEOPS_TEMPLATE,
  KV_ACADEMIC_TEMPLATE,
];
