// Fork-owned schemas, split from schemas.ts so upstream edits there never conflict with them
// (docs/specs/upstream-sync/spec.md section 3.9). schemas.ts re-exports everything below.
import { WORKSPACE_ORDER_EINVOICE_STATES, type ScheduledPriceChange, type WorkspaceOrderEinvoice, type WorkspaceOrderEinvoiceState } from "../billing/workspace-orders-types";
import { z } from "zod";
import type { FlowCapabilitiesResponse, FlowCapabilityFeatures, FlowDoc, FlowDraft, FlowInsights, FlowIssue, FlowLimits, FlowManifestResponse, FlowTraceResponse, FlowValidationResponse, HubActivityPage, HubApplyUpdateResult, HubBundle, HubForkResult, HubImportCommitResult, HubImportPreview, HubItemMeta, HubQuota, HubSource, HubSyncResult, HubUpdateCheck, HubUpdateDiff, Invitation, IssueFlowAction, IssueFlowAdvanceResponse, IssueFlowNext, IssueFlowPending, IssueFlowState, IssueFlowSwitchValue, IssueFlowTask, IssueWorkflowBlocker, IssueWorkflowSwitch, IssueWorkflowUsage, ListFlowDraftsResponse, ListIssueWorkflowTemplatesResponse, ListIssueWorkflowUsageResponse, PackCommitResponse, PackIssue, PackPreview, PackScanSeverity, PluginVersionFileListResponse, PolicyFloor, QuotaRequest, QuotaRequestListResponse, RuntimeQuickstartRequest, SaveHubSkillFileResult, SetSquadActiveWorkflowResponse, SetSquadRuntimeResponse, SquadPlugin, SquadSkill, SquadWorkflowsResponse, WorkflowCondition, Workspace, WorkspacePendingPlanChangeCleared, WorkspaceSubscriptionCancelState, WorkspaceSubscriptionUsage, WorkspaceUsageMeter, WorkspaceUsagePeriod } from "../types";
import { normalizeHubSourceType } from "../hub/items";
import type { IssueFlowStopInfo, IssueFlowViewer } from "../types/issue-flow";
import type { IssueLimitUsage, WorkspaceSubscriptionPrice, WorkspaceSubscriptionPrices, Project, MemberWithUser } from "../types";

/** One file of a published version. `path` is required: a listing entry without one is malformed. */
export const PluginVersionFileSchema = z.object({
  path: z.string(),
  size_bytes: z.number().default(0),
  sha256: z.string().default(""),
}).loose();

export const PluginVersionFileListResponseSchema = z.object({
  files: z.array(PluginVersionFileSchema).default([]),
}).loose();

export const EMPTY_PLUGIN_VERSION_FILE_LIST: PluginVersionFileListResponse = {
  files: [],
};

export const PluginVersionFileContentSchema = z.object({
  path: z.string(),
  content: z.string().nullable().default(null),
  is_binary: z.boolean().default(false),
  size_bytes: z.number().default(0),
  sha256: z.string().default(""),
}).loose();

// Routing transitions (workflow-tab §3). A branch the schema cannot read is
// dropped on its own, so one bad branch never blanks the workflow list around
// it. Field and op stay strings: a condition this build does not know still
// reaches the designer, where it matches nothing.
export const WorkflowConditionSchema: z.ZodType<WorkflowCondition> = z.lazy(() =>
  z.union([
    z.object({ all: z.array(WorkflowConditionSchema).min(1) }).loose(),
    z.object({ any: z.array(WorkflowConditionSchema).min(1) }).loose(),
    z.object({ not: WorkflowConditionSchema }).loose(),
    z.object({
      field: z.string(),
      op: z.string(),
      value: z.unknown().optional(),
    }).loose(),
  ]),
);

// A canvas position. A layout this build cannot read is dropped on its own, so
// the step still shows and the designer places it.
export const IssueWorkflowLayoutSchema = z.object({
  x: z.number(),
  y: z.number(),
}).loose().optional().catch(undefined);

const IssueWorkflowSwitchSchema = z.object({
  key: z.string().min(1),
  type: z.string().default("bool"),
  values: z.array(z.string()).optional().catch(undefined),
  default: z.union([z.boolean(), z.string()]).nullable().default(null).catch(null),
}).loose();

export const IssueWorkflowFlowConfigSchema = z.object({
  version: z.number().default(1).catch(1),
  task_timeout_minutes: z.number().optional().catch(undefined),
  task_hard_cap_minutes: z.number().optional().catch(undefined),
  task_max_attempts: z.number().optional().catch(undefined),
  switches: z.unknown().transform((value): IssueWorkflowSwitch[] => {
    if (!Array.isArray(value)) return [];
    return value.flatMap((item) => {
      const parsed = IssueWorkflowSwitchSchema.safeParse(item);
      return parsed.success ? [parsed.data] : [];
    });
  }),
  messages: z.object({
    stop1: z.string().optional().catch(undefined),
    human: z.string().optional().catch(undefined),
    stop2: z.string().optional().catch(undefined),
    stop3: z.string().optional().catch(undefined),
    stalled: z.string().optional().catch(undefined),
  }).loose().optional().catch(undefined),
}).loose();

export const EMPTY_SQUAD_WORKFLOWS_RESPONSE: SquadWorkflowsResponse = {
  active_workflow_id: null,
  workflows: [],
  total: 0,
};

// System templates for a new workflow. A template id this build does not know
// is skipped, not shown: the frontend has no name or description to show for it.
// A template with a malformed step is dropped whole, since a template with a
// column missing would route issues to statuses that do not exist.
export const ISSUE_WORKFLOW_TEMPLATE_IDS: ReadonlySet<string> = new Set<string>([
  "delivery", "content", "community", "sdlc_auto", "bmad_planning_auto", "sales_loop", "quick_fix",
]);

export const EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE: ListIssueWorkflowTemplatesResponse = {
  templates: [],
};

// Where each workflow is assigned, for the manage view. A usage row that does
// not parse is dropped alone, as a transition is.
const IssueWorkflowUsageItemSchema = z.object({
  workflow_id: z.string(),
  squad_ids: z.array(z.string()).default([]),
  agent_ids: z.array(z.string()).default([]),
}).loose();

export const ListIssueWorkflowUsageResponseSchema = z.object({
  usage: z.unknown().transform((value): IssueWorkflowUsage[] => {
    if (!Array.isArray(value)) return [];
    return value.flatMap((item) => {
      const parsed = IssueWorkflowUsageItemSchema.safeParse(item);
      return parsed.success ? [parsed.data] : [];
    });
  }),
  total: z.number().default(0),
}).loose();

export const EMPTY_LIST_ISSUE_WORKFLOW_USAGE_RESPONSE: ListIssueWorkflowUsageResponse = {
  usage: [],
  total: 0,
};

const IssueWorkflowBlockerSchema = z.object({
  code: z.string().default(""),
  count: z.number().default(0),
  ids: z.array(z.string()).default([]),
  message: z.string().default(""),
}).loose();

/**
 * The blocking reasons in a failed workflow delete's body (`reasons`). A body
 * without a readable list reads as no reasons, and the caller falls back to the
 * message.
 */
export function parseIssueWorkflowBlockers(body: unknown): IssueWorkflowBlocker[] {
  const parsed = z.object({ reasons: z.unknown() }).loose().safeParse(body);
  if (!parsed.success || !Array.isArray(parsed.data.reasons)) return [];
  return parsed.data.reasons.flatMap((reason) => {
    const item = IssueWorkflowBlockerSchema.safeParse(reason);
    return item.success ? [item.data] : [];
  });
}

export const EMPTY_SET_SQUAD_ACTIVE_WORKFLOW_RESPONSE: SetSquadActiveWorkflowResponse = {
  squad_id: "",
  active_workflow_id: "",
  updated_issues: 0,
  plan: { required: [], unchanged: [], total_issues: 0, affected_issues: 0 },
};

// GET /api/issues/{id}/flow and the advance answer. Every list is read item by
// item, so one task or action this build cannot read never hides the rest.
const IssueFlowClosedBySchema = z.object({
  type: z.string().default(""),
  id: z.string().default(""),
  on_behalf: z.boolean().default(false).catch(false),
}).loose();

const IssueFlowSlotStatusSchema = z.object({
  slot: z.string().default(""),
  min: z.number().default(0).catch(0),
  max: z.number().default(0).catch(0),
  count: z.number().default(0).catch(0),
}).loose();

const IssueFlowLinkSchema = z.object({
  issue_id: z.string().default(""),
  identifier: z.string().default(""),
  task: z.string().default(""),
  slot: z.string().default(""),
}).loose();

const IssueFlowStopInfoSchema = z.object({
  code: z.string().default(""),
  legacy_no: z.number().default(0).catch(0),
  from: z.string().default(""),
  edge: z.string().default(""),
  at: z.string().default(""),
  reason: z.string().default("").catch(""),
}).loose();

// The stop of a state is an object {code, legacy_no, from, edge, at}, null while the request is not
// parked (spec 7.1). A value that is not an object is a stop this build cannot read: it still parks
// the request, so it reads as "stalled" and a person looks at it.
const IssueFlowStateStopSchema = z.unknown().transform((value): IssueFlowStopInfo | null => {
  if (value === null || value === undefined) return null;
  const parsed = IssueFlowStopInfoSchema.safeParse(value);
  if (parsed.success) return parsed.data as IssueFlowStopInfo;
  return { code: "stalled", legacy_no: 0, from: "", edge: "", at: "" };
});

const IssueFlowCheckpointSchema = z.object({
  step: z.string().default(""),
  activation: z.number().default(0).catch(0),
  by: z.string().optional().catch(undefined),
  at: z.string().optional().catch(undefined),
}).loose();

const IssueFlowTrailEntrySchema = z.object({
  step: z.string().default(""),
  activation: z.number().default(0).catch(0),
  edge: z.string().optional().catch(undefined),
  at: z.string().optional().catch(undefined),
}).loose();

const IssueFlowVerdictSchema = z.object({
  key: z.string().min(1),
  class: z.string().default("neutral").catch("neutral"),
}).loose();

const IssueFlowTaskSchema = z.object({
  issue_id: z.string().default(""),
  identifier: z.string().default(""),
  step: z.string().default(""),
  task: z.string().min(1),
  round: z.number().default(0).catch(0),
  kind: z.string().default("").catch(""),
  status: z.string().default(""),
  outcome: z.string().nullable().default(null).catch(null),
  assignee_id: z.string().nullable().default(null).catch(null),
  closed_at: z.string().nullable().default(null).catch(null),
  closed_by: IssueFlowClosedBySchema.nullable().default(null).catch(null),
  instance: z.number().default(0).catch(0),
  class: z.string().default("").catch(""),
  slot_status: tolerantList(IssueFlowSlotStatusSchema),
  inputs: tolerantList(IssueFlowLinkSchema),
  feedback: tolerantList(IssueFlowLinkSchema),
  sets: z.array(z.string()).default([]).catch([]),
  verdicts: tolerantList(IssueFlowVerdictSchema),
}).loose();

const IssueFlowNextSchema = z.object({
  id: z.string().default(""),
  label: z.string().default(""),
  to: z.string().default(""),
  rework: z.boolean().default(false).catch(false),
  trigger: z.string().default("auto").catch("auto"),
}).loose();

function tolerantList<T>(schema: z.ZodType<T>) {
  return z.unknown().transform((value): T[] => {
    if (!Array.isArray(value)) return [];
    return value.flatMap((item) => {
      const parsed = schema.safeParse(item);
      return parsed.success ? [parsed.data] : [];
    });
  });
}

// What the reader may do. An older server sends none: the reader may do nothing.
export const EMPTY_ISSUE_FLOW_VIEWER: IssueFlowViewer = { can_manage: false, closable_task_ids: [] };

const IssueFlowViewerSchema = z.object({
  can_manage: z.boolean().default(false).catch(false),
  closable_task_ids: z.array(z.string()).default([]).catch([]),
}).loose();

export const IssueFlowStateSchema = z.object({
  managed: z.boolean().default(true).catch(true),
  role: z.unknown().transform((value): IssueFlowState["role"] => (value === "task" ? "task" : "request")),
  workflow_id: z.string().default(""),
  workflow_name: z.string().default(""),
  step: z.string().default(""),
  rework: z.number().default(0).catch(0),
  stop: IssueFlowStateStopSchema,
  closed: z.boolean().default(false).catch(false),
  switches: z.record(z.string(), z.union([z.boolean(), z.string()])).default({}).catch({}),
  auto: z.boolean().default(false).catch(false),
  entered_at: z.string().default(""),
  tasks: tolerantList(IssueFlowTaskSchema as unknown as z.ZodType<IssueFlowTask>),
  next: tolerantList(IssueFlowNextSchema as unknown as z.ZodType<IssueFlowNext>),
  rework_step: z.string().default("").catch(""),
  viewer: z.unknown().transform((value): IssueFlowViewer => {
    const parsed = IssueFlowViewerSchema.safeParse(value ?? {});
    return parsed.success ? parsed.data : { ...EMPTY_ISSUE_FLOW_VIEWER, closable_task_ids: [] };
  }),
  schema: z.number().default(1).catch(1),
  loops: z.record(z.string(), z.number()).default({}).catch({}),
  loop_max: z.record(z.string(), z.number()).default({}).catch({}),
  activations: z.record(z.string(), z.number()).default({}).catch({}),
  vars: z.record(z.string(), z.unknown()).default({}).catch({}),
  facts: z.record(z.string(), z.unknown()).default({}).catch({}),
  checkpoints: tolerantList(IssueFlowCheckpointSchema),
  unreviewed: z.boolean().default(false).catch(false),
  trail: tolerantList(IssueFlowTrailEntrySchema),
  trial: z.boolean().default(false).catch(false),
}).loose();

export const EMPTY_ISSUE_FLOW_STATE: IssueFlowState = {
  managed: false,
  role: "request",
  workflow_id: "",
  workflow_name: "",
  step: "",
  rework: 0,
  stop: null,
  switches: {} as Record<string, IssueFlowSwitchValue>,
  auto: false,
  entered_at: "",
  tasks: [],
  next: [],
  rework_step: "",
  viewer: EMPTY_ISSUE_FLOW_VIEWER,
  trial: false,
};

const IssueFlowActionSchema = z.object({
  kind: z.string().min(1),
  step: z.string().default(""),
  tasks: z.array(z.string()).default([]).catch([]),
  code: z.string().optional().catch(undefined),
  agent_id: z.string().optional().catch(undefined),
}).loose();

const IssueFlowPendingSchema = z.object({
  task: z.string().min(1),
  issue_id: z.string().default(""),
  status: z.string().default(""),
}).loose();

export const IssueFlowAdvanceResponseSchema = z.object({
  state: IssueFlowStateSchema,
  actions: tolerantList(IssueFlowActionSchema as unknown as z.ZodType<IssueFlowAction>),
  pending: tolerantList(IssueFlowPendingSchema as unknown as z.ZodType<IssueFlowPending>),
}).loose();

export const EMPTY_ISSUE_FLOW_ADVANCE_RESPONSE: IssueFlowAdvanceResponse = {
  state: EMPTY_ISSUE_FLOW_STATE,
  actions: [],
  pending: [],
};

export const DetachFlowResponseSchema = z.object({
  detached: z.boolean().default(false).catch(false),
}).loose();

export const PROJECT_ACCESS_VALUES = ["workspace", "restricted"] as const;

export type ProjectAccess = (typeof PROJECT_ACCESS_VALUES)[number];

export interface ProjectWithAccess extends Project {
  access?: ProjectAccess;
  can_manage_access?: boolean;
}

export const SquadSkillSchema = z.object({
  id: z.string(),
  name: z.string().default(""),
  description: z.string().default(""),
}).loose();

export const SquadSkillListSchema = z.array(SquadSkillSchema);

export const EMPTY_SQUAD_SKILL_LIST: SquadSkill[] = [];

export const SquadPluginSchema = z.object({
  name: z.string(),
  install_hint: z.string().default(""),
}).loose();

export const SquadPluginListSchema = z.array(SquadPluginSchema);

export const EMPTY_SQUAD_PLUGIN_LIST: SquadPlugin[] = [];

const SquadRuntimeAgentResultSchema = z.object({
  agent_id: z.string(),
  agent_name: z.string().default(""),
  reason: z.string().default(""),
}).loose();

export const SetSquadRuntimeResponseSchema = z.object({
  runtime_id: z.string().default(""),
  updated: z.array(SquadRuntimeAgentResultSchema).default([]),
  unchanged: z.array(SquadRuntimeAgentResultSchema).default([]),
  skipped: z.array(SquadRuntimeAgentResultSchema).default([]),
}).loose();

export const EMPTY_SET_SQUAD_RUNTIME_RESPONSE: SetSquadRuntimeResponse = {
  runtime_id: "",
  updated: [],
  unchanged: [],
  skipped: [],
};

// The 402 a creation gets when the workspace's plan has no room left (platform
// spec 10). `gate` is a free string: a newer server may name a gate this
// client does not know, and the caller decides what that means.
export const PlanLimitReachedBodySchema = z.object({
  code: z.literal("plan_limit_reached"),
  error: z.string().optional(),
  gate: z.string().min(1),
  limit: z.number().int().nonnegative(),
  used: z.number().int().nonnegative(),
}).loose();

export type PlanLimitReachedBody = z.infer<typeof PlanLimitReachedBodySchema>;

// Usage of a pay as you go plan (spec 7b.3). The summary and the usage read
// share the period shape. Rates are optional here: the summary sends only
// amounts, while the usage read also carries each meter's price.
const WorkspaceUsageMeterSchema = z
  .object({
    key: z.string().min(1),
    quantity: z.number().int().nonnegative(),
    unit_price: z.number().int().nonnegative().default(0),
    per: z.number().int().positive().default(1),
    discount_pct: z.number().int().min(0).max(100).default(0),
    amount: z.number().int().nonnegative(),
  })
  .loose()
  .transform(
    (value): WorkspaceUsageMeter => ({
      key: value.key,
      quantity: value.quantity,
      unitPrice: value.unit_price,
      per: value.per,
      discountPct: value.discount_pct,
      amount: value.amount,
    }),
  );

export const WorkspaceUsagePeriodSchema = z
  .object({
    period_start: z.string().min(1),
    period_end: z.string().min(1),
    currency: z.string().regex(/^[a-z]{3}$/),
    accrued_amount: z.number().int().nonnegative(),
    credit_limit: z.number().int().nonnegative(),
    unpaid_order_id: z.string().nullable().optional(),
    meters: z.array(WorkspaceUsageMeterSchema).default([]),
  })
  .loose()
  .transform(
    (value): WorkspaceUsagePeriod => ({
      periodStart: value.period_start,
      periodEnd: value.period_end,
      currency: value.currency,
      accruedAmount: value.accrued_amount,
      creditLimit: value.credit_limit,
      unpaidOrderId: value.unpaid_order_id ?? null,
      meters: value.meters,
    }),
  );

// `GET /api/cloud-subscriptions/usage`: a seat plan answers with no period.
export const WorkspaceSubscriptionUsageSchema = z
  .object({
    billing_model: z.string().default("seat"),
    usage: WorkspaceUsagePeriodSchema.nullable().default(null),
  })
  .loose()
  .transform(
    (value): WorkspaceSubscriptionUsage => ({
      // An unknown model reads as seat, which shows no usage card.
      billingModel: value.billing_model === "usage" ? "usage" : "seat",
      usage: value.usage,
    }),
  );

// Every paid plan must be priced: a response missing one plan reads as "prices
// unavailable" as a whole rather than offering one plan without the other.
// A day count from a newer or malformed Cloud reads as unknown, never as a
// number: the plan labels it feeds are hints, and a wrong one would point a
// user at the wrong plan.
export const PlanDaysSchema = z.number().int().positive().nullable().catch(null).default(null);

export const IssueLimitUsageSchema = z
  .object({
    used: z.number().int().nonnegative(),
    limit: z.number().int().positive(),
  })
  .loose()
  .transform(
    (value): IssueLimitUsage => ({
      used: value.used,
      limit: value.limit,
    }),
  );

const WorkspaceSubscriptionPriceSchema = (
  expected: "month" | "year",
) =>
  z
    .object({
      currency: z.string().regex(/^[a-z]{3}$/),
      // Reject 0 and negatives: a free or malformed Price must read as
      // "price unavailable", not as a real amount shown next to a purchase
      // button.
      unit_amount: z.number().int().positive(),
      // Pinned to the slot it arrived in. Cloud validates this too, but a
      // schema that accepted a yearly Price under `month` would let the UI
      // quote a yearly amount as a monthly one — the schema is an independent
      // boundary, so it checks the correspondence itself.
      interval: z.literal(expected),
      interval_count: z.literal(1),
    })
    .loose()
    .transform(
      (value): WorkspaceSubscriptionPrice => ({
        currency: value.currency,
        unitAmount: value.unit_amount,
        interval: value.interval,
        intervalCount: value.interval_count,
      }),
    );

export const WorkspaceSubscriptionPricesSchema = z
  .object({
    month: WorkspaceSubscriptionPriceSchema("month"),
    year: WorkspaceSubscriptionPriceSchema("year"),
  })
  .loose()
  .transform(
    (value): WorkspaceSubscriptionPrices => ({
      month: value.month,
      year: value.year,
    }),
  );

export const ClearWorkspaceScheduledPlanChangeResponseSchema = z
  .object({
    pending_change: z.null(),
    version: z.number().int().nonnegative(),
    changed: z.boolean(),
  })
  .loose()
  .transform(
    (value): WorkspacePendingPlanChangeCleared => ({
      pendingChange: null,
      version: value.version,
      changed: value.changed,
    }),
  );

// Answer of both cancel and resume.
export const WorkspaceSubscriptionCancelStateSchema = z
  .object({
    cancel_at_period_end: z.boolean(),
    status: z.string(),
    current_period_end: z.string().nullable(),
    version: z.number().int().nonnegative(),
    changed: z.boolean(),
  })
  .loose()
  .transform(
    (value): WorkspaceSubscriptionCancelState => ({
      cancelAtPeriodEnd: value.cancel_at_period_end,
      status: value.status,
      currentPeriodEnd: value.current_period_end,
      version: value.version,
      changed: value.changed,
    }),
  );

export const ScheduledPriceChangeSchema = z
  .object({
    effective_from: z.string().min(1),
    seat_price_month: z.number().int().nonnegative().nullable().default(null),
    seat_price_year: z.number().int().nonnegative().nullable().default(null),
  })
  .loose()
  .transform(
    (value): ScheduledPriceChange => ({
      effectiveFrom: value.effective_from,
      seatPriceMonth: value.seat_price_month,
      seatPriceYear: value.seat_price_year,
    }),
  )
  .nullable()
  .default(null);

const KNOWN_EINVOICE_STATE_SET = new Set<string>(WORKSPACE_ORDER_EINVOICE_STATES);

function parseEinvoiceState(value: string): WorkspaceOrderEinvoiceState {
  return KNOWN_EINVOICE_STATE_SET.has(value)
    ? (value as WorkspaceOrderEinvoiceState)
    : "unknown";
}

function sanitizeHttpsUrl(value: string): string {
  const trimmed = value.trim();
  if (!trimmed.startsWith("https://")) return "";
  try {
    return new URL(trimmed).protocol === "https:" ? trimmed : "";
  } catch {
    return "";
  }
}

export const WorkspaceOrderEinvoiceSchema = z
  .object({
    state: z.string().min(1).transform(parseEinvoiceState),
    number: z.string().optional().default(""),
    lookup_url: z.string().optional().default(""),
    missing_fields: z
      .array(z.string())
      .nullable()
      .optional()
      .transform((value) => value ?? []),
  })
  .loose()
  .transform(
    (value): WorkspaceOrderEinvoice => ({
      state: value.state,
      number: value.number,
      lookupUrl: sanitizeHttpsUrl(value.lookup_url),
      missingFields: value.missing_fields,
    }),
  )
  .nullable()
  .default(null);

export const RuntimeQuickstartStepSchema = z
  .object({
    name: z.string().default(""),
    status: z
      .enum(["installed", "already_installed", "failed"])
      .catch("failed"),
    detail: z.string().optional(),
  })
  .loose();

export const RuntimeQuickstartRequestSchema = z
  .object({
    id: z.string().default(""),
    runtime_id: z.string().default(""),
    status: z
      .enum(["pending", "running", "completed", "failed"])
      .catch("failed"),
    steps: z.array(RuntimeQuickstartStepSchema).default([]),
    error: z.string().optional(),
    created_at: z.string().default(""),
    updated_at: z.string().default(""),
  })
  .loose();

// Fallback for an unparseable quickstart response. It reads as a failed
// request so the UI shows an error instead of polling forever.
export const MALFORMED_RUNTIME_QUICKSTART_REQUEST: RuntimeQuickstartRequest = {
  id: "",
  runtime_id: "",
  status: "failed",
  steps: [],
  error: "invalid quickstart response",
  created_at: "",
  updated_at: "",
};

export interface MemberWithGuest extends MemberWithUser {
  is_guest?: boolean;
}

export const InvitationStatusSchema = z
  .enum(["pending", "accepted", "declined", "expired"])
  .catch("pending");

export interface InvitationWithGuest extends Invitation {
  is_guest?: boolean;
  guest_project_ids?: string[];
}

export const EMPTY_INVITATION: InvitationWithGuest = {
  id: "",
  workspace_id: "",
  inviter_id: "",
  invitee_email: "",
  invitee_user_id: null,
  role: "viewer",
  status: "pending",
  created_at: "",
  updated_at: "",
  expires_at: "",
  is_guest: false,
  guest_project_ids: [],
};

export const WakeupConditionSchema = z.union([
  z.object({ type: z.literal("issue_field"), field: z.literal("status"), value: z.string() }),
  z.object({ type: z.literal("issue_field"), field: z.literal("assignee"), assignee_type: z.enum(["member", "agent", "squad"]), assignee_id: z.string() }),
  z.object({ type: z.literal("issue_field"), field: z.literal("label"), label_id: z.string() }),
  z.object({ type: z.literal("issue_field"), field: z.literal("property"), property_id: z.string(), value: z.unknown() }),
  z.object({ type: z.literal("children_done"), stage: z.number().int().nullish() }),
  z.object({ type: z.literal("pull_request"), event: z.enum(["checks_finished", "merged", "conflicts"]) }),
  z.object({ type: z.literal("other_issue"), issue_id: z.string(), state: z.enum(["done", "ended", "in_review", "blocked"]), identifier: z.string().optional() }),
]);

export const RunQueuePreviewSchema = z.object({
  release_now: z.number().int().nonnegative().default(0),
  run_count: z.number().int().nonnegative().default(0),
});

export const RunQueueWaitSchema = z.object({
  issue_id: z.string(),
  waiting_on_issue_ids: z.array(z.string()).default([]),
});

const WorkspaceRepoSchema = z.object({
  url: z.string(),
  description: z.string().optional(),
}).loose();

// Optional tenant ID: present only when the response carries a non-empty string.
// Missing or malformed values degrade to undefined rather than failing the workspace parse.
export const OptionalTenantIdSchema = z
  .string()
  .refine((value) => value.trim().length > 0)
  .optional()
  .catch(undefined);

export const WorkspaceSchema: z.ZodType<Workspace> = z.object({
  id: z.string().min(1),
  name: z.string(),
  slug: z.string(),
  description: z.string().nullable().default(null),
  context: z.string().nullable().default(null),
  settings: z.record(z.string(), z.unknown()).default({}).catch({}),
  repos: z.array(WorkspaceRepoSchema).default([]).catch([]),
  issue_prefix: z.string().default(""),
  avatar_url: z.string().nullable().default(null),
  secretary_agent_id: z.string().nullable().optional().catch(undefined),
  tenant_id: OptionalTenantIdSchema,
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
  pinned_at: z.iso.datetime({ offset: true }).nullable().optional().catch(undefined),
}).loose();

export const EMPTY_WORKSPACE: Workspace = {
  id: "",
  name: "",
  slug: "",
  description: null,
  context: null,
  settings: {},
  repos: [],
  issue_prefix: "",
  avatar_url: null,
  created_at: "",
  updated_at: "",
};

export const HubSourceSchema = z.object({
  id: z.string().default(""),
  name: z.string().default(""),
  type: z.string().default("").transform((value) => normalizeHubSourceType(value)),
  url: z.string().default(""),
  config: z.record(z.string(), z.unknown()).catch({}),
  enabled: z.boolean().default(true),
  auto_sync_interval_sec: z.number().default(0),
  last_sync_at: z.string().nullable().default(null),
  last_sync_status: z.string().default(""),
  last_sync_error: z.string().nullable().default(null),
  item_count: z.number().default(0),
  has_credential: z.boolean().default(false),
  created_at: z.string().default(""),
  updated_at: z.string().default(""),
}).loose();

export const HubSourceListSchema = z.array(HubSourceSchema);

export const EMPTY_HUB_SOURCE: HubSource = {
  id: "",
  name: "",
  type: "manual",
  url: "",
  config: {},
  enabled: true,
  auto_sync_interval_sec: 0,
  last_sync_at: null,
  last_sync_status: "",
  last_sync_error: null,
  item_count: 0,
  has_credential: false,
  created_at: "",
  updated_at: "",
};

const HubFindingSchema = z.object({
  rule: z.string().default(""),
  severity: z.enum(["info", "warn", "block"]).catch("info"),
  path: z.string().default(""),
  message: z.string().default(""),
}).loose();

const HubImportScanSchema = z.object({
  status: z.enum(["clean", "warn", "blocked"]).catch("warn"),
  findings: z.array(HubFindingSchema).default([]),
}).loose();

const HubImportConflictSchema = z.object({
  exists: z.boolean().default(false),
  item_id: z.string().optional(),
  kind: z.enum(["skill", "plugin"]).optional().catch(undefined),
}).loose();

const HubRegistryItemSchema = z.object({
  kind: z.enum(["skill", "plugin"]).catch("skill"),
  name: z.string().default(""),
  description: z.string().default(""),
  version: z.string().nullable().default(null),
  url: z.string().default(""),
  sha256: z.string().default(""),
  tags: z.array(z.string()).default([]),
}).loose();

const HubRegistryIndexSchema = z.object({
  name: z.string().default(""),
  items: z.array(HubRegistryItemSchema).default([]),
}).loose();

export const HubImportPreviewSchema = z.object({
  preview_id: z.string().default(""),
  kind: z.enum(["skill", "plugin", "registry"]).catch("skill"),
  name: z.string().default(""),
  description: z.string().default(""),
  version: z.string().nullable().default(null),
  files: z.array(z.object({
    path: z.string().default(""),
    size_bytes: z.number().default(0),
  }).loose()).default([]),
  scan: HubImportScanSchema.default({ status: "warn", findings: [] }),
  conflict: HubImportConflictSchema.default({ exists: false }),
  registry: HubRegistryIndexSchema.optional(),
}).loose();

export const EMPTY_HUB_IMPORT_PREVIEW: HubImportPreview = {
  preview_id: "",
  kind: "skill",
  name: "",
  description: "",
  version: null,
  files: [],
  scan: { status: "warn", findings: [] },
  conflict: { exists: false },
};

export const HubImportCommitResultSchema = z.object({
  kind: z.enum(["skill", "plugin"]).catch("skill"),
  id: z.string().default(""),
  hub_item_id: z.string().default(""),
  skipped: z.boolean().default(false),
}).loose();

export const EMPTY_HUB_IMPORT_COMMIT_RESULT: HubImportCommitResult = {
  kind: "skill",
  id: "",
  hub_item_id: "",
  skipped: false,
};

const HubItemKindSchema = z.enum(["skill", "plugin"]).catch("skill");

const HubCountSchema = z.number().catch(0);

export const HubItemMetaSchema = z.object({
  id: z.string().catch(""),
  kind: HubItemKindSchema,
  ref_id: z.string().catch(""),
  source_id: z.string().nullable().catch(null),
  source_ref: z.string().catch(""),
  tracking: z
    .string()
    .catch("manual")
    .transform((value): "tracking" | "forked" | "manual" =>
      value === "tracking" || value === "forked" ? value : "manual",
    ),
  latest_version: z.string().catch(""),
  bundle_digest: z.string().catch(""),
  size_bytes: HubCountSchema,
  file_count: HubCountSchema,
  scan_status: z.enum(["clean", "warn", "blocked"]).catch("warn"),
  update_available: z.boolean().catch(false),
  remote_digest: z.string().catch(""),
  checked_at: z.string().nullable().catch(null),
  local_modified: z.boolean().catch(false),
  created_at: z.string().catch(""),
  updated_at: z.string().catch(""),
}).loose();

export const HubItemMetaListSchema = z.array(HubItemMetaSchema);

export const EMPTY_HUB_ITEM_META: HubItemMeta = {
  id: "",
  kind: "skill",
  ref_id: "",
  source_id: null,
  source_ref: "",
  tracking: "manual",
  latest_version: "",
  bundle_digest: "",
  size_bytes: 0,
  file_count: 0,
  scan_status: "warn",
  update_available: false,
  remote_digest: "",
  checked_at: null,
  local_modified: false,
  created_at: "",
  updated_at: "",
};

/** The save response of a skill file write. `etag` is the file's new etag. */
export const HubSaveSkillFileResultSchema = z.object({
  path: z.string().default(""),
  etag: z.string().default(""),
}).loose();

export const EMPTY_HUB_SAVE_SKILL_FILE_RESULT: SaveHubSkillFileResult = { path: "", etag: "" };

export const HubBundleSchema = z.object({
  url: z.string().catch(""),
  expires_in: HubCountSchema,
  digest: z.string().catch(""),
  size_bytes: HubCountSchema,
  version: z.string().catch(""),
}).loose();

const HubSyncItemSchema = z.object({
  kind: HubItemKindSchema,
  id: z.string().catch(""),
  name: z.string().catch(""),
  change: z.enum(["update_available", "unchanged", "error"]).catch("error"),
  error: z.string().optional().catch(undefined),
}).loose();

export const HubSyncResultSchema = z.object({
  status: z.enum(["ok", "error"]).catch("error"),
  checked: HubCountSchema,
  updates_available: HubCountSchema,
  unchanged: HubCountSchema,
  error: z.string().optional().catch(undefined),
  items: z.array(HubSyncItemSchema).catch([]),
}).loose();

export const EMPTY_HUB_SYNC_RESULT: HubSyncResult = {
  status: "error",
  checked: 0,
  updates_available: 0,
  unchanged: 0,
  items: [],
};

const HubUpdateFileSchema = z.object({
  path: z.string().catch(""),
  // An unknown status is shown as modified, so it is reviewed rather than hidden.
  status: z.enum(["added", "modified", "removed", "unchanged"]).catch("modified"),
  old_size: z.number().nullable().catch(null),
  new_size: z.number().nullable().catch(null),
}).loose();

export const HubUpdateCheckSchema = z.object({
  update_available: z.boolean().catch(false),
  current_digest: z.string().catch(""),
  remote_digest: z.string().catch(""),
  local_modified: z.boolean().catch(false),
  files: z.array(HubUpdateFileSchema).catch([]),
}).loose();

export const EMPTY_HUB_UPDATE_CHECK: HubUpdateCheck = {
  update_available: false,
  current_digest: "",
  remote_digest: "",
  local_modified: false,
  files: [],
};

export const HubUpdateDiffSchema = z.object({
  path: z.string().catch(""),
  old: z.string().nullable().catch(null),
  new: z.string().nullable().catch(null),
  binary: z.boolean().catch(false),
}).loose();

export const HubApplyUpdateResultSchema = z.object({
  kind: HubItemKindSchema,
  id: z.string().catch(""),
  version: z.string().catch(""),
  tracking: z
    .string()
    .catch("manual")
    .transform((value): "tracking" | "forked" | "manual" =>
      value === "tracking" || value === "forked" ? value : "manual",
    ),
}).loose();

export const HubForkResultSchema = z.object({
  kind: HubItemKindSchema,
  id: z.string().catch(""),
  tracking: z.literal("forked").catch("forked"),
}).loose();

export const HubActivityEntrySchema = z.object({
  id: z.string().catch(""),
  action: z.string().catch(""),
  actor_type: z.string().catch(""),
  actor_id: z.string().nullable().catch(null),
  actor_name: z.string().catch(""),
  kind: z.enum(["skill", "plugin"]).nullable().catch(null),
  ref_id: z.string().nullable().catch(null),
  item_name: z.string().catch(""),
  source_id: z.string().nullable().catch(null),
  detail: z.record(z.string(), z.unknown()).catch({}),
  created_at: z.string().catch(""),
}).loose();

export const HubActivityPageSchema = z.object({
  entries: z.array(HubActivityEntrySchema).catch([]),
  next_cursor: z.string().nullable().catch(null),
}).loose();

export const EMPTY_HUB_ACTIVITY_PAGE: HubActivityPage = { entries: [], next_cursor: null };

const HubQuotaUsageSchema = z.object({
  used: HubCountSchema,
  limit: HubCountSchema,
}).loose();

export const HubQuotaSchema = z.object({
  items: HubQuotaUsageSchema.catch({ used: 0, limit: 0 }),
  storage_bytes: HubQuotaUsageSchema.catch({ used: 0, limit: 0 }),
}).loose();

export const EMPTY_HUB_QUOTA: HubQuota = {
  items: { used: 0, limit: 0 },
  storage_bytes: { used: 0, limit: 0 },
};

export const EMPTY_HUB_BUNDLE: HubBundle = { url: "", expires_in: 0, digest: "", size_bytes: 0, version: "" };

export const EMPTY_HUB_APPLY_UPDATE_RESULT: HubApplyUpdateResult = {
  kind: "skill",
  id: "",
  version: "",
  tracking: "manual",
};

export const EMPTY_HUB_FORK_RESULT: HubForkResult = { kind: "skill", id: "", tracking: "forked" };

export const EMPTY_HUB_UPDATE_DIFF: HubUpdateDiff = { path: "", old: null, new: null, binary: false };

export const EMPTY_FLOW_DRAFT: FlowDraft = {
  id: "",
  squad_id: "",
  name: "",
  lineage_id: null,
  base_workflow_id: null,
  revision: 1,
  doc: {
    version: 2,
    start: "plan",
    roles: [],
    terminals: [{ key: "done", status: "done" }],
    steps: [
      {
        key: "plan",
        title: "Plan",
        kind: "lead",
        status: { key: "todo" },
        edges: [{ id: "plan_done", label: "Done", to: "done" }],
      },
    ],
  },
  origin: "manual",
  source_ref: null,
  created_by: "",
  updated_by: "",
  created_at: "",
  updated_at: "",
};

export const EMPTY_LIST_FLOW_DRAFTS_RESPONSE: ListFlowDraftsResponse = {
  drafts: [],
  total: 0,
};

export const FlowDraftSummarySchema = z.object({
  id: z.string().default("").catch(""),
  squad_id: z.string().default("").catch(""),
  name: z.string().default("").catch(""),
  lineage_id: z.string().nullable().default(null).catch(null),
  base_workflow_id: z.string().nullable().default(null).catch(null),
  revision: z.number().default(1).catch(1),
  origin: z.string().default("manual").catch("manual"),
  created_by: z.string().default("").catch(""),
  updated_by: z.string().default("").catch(""),
  created_at: z.string().default("").catch(""),
  updated_at: z.string().default("").catch(""),
  summary: z.object({
    steps: z.number().default(0).catch(0),
    tasks: z.number().default(0).catch(0),
    gates: z.number().default(0).catch(0),
  }).default({ steps: 0, tasks: 0, gates: 0 }).catch({ steps: 0, tasks: 0, gates: 0 }),
}).loose();

export const ListFlowDraftsResponseSchema = z.object({
  drafts: tolerantList(FlowDraftSummarySchema),
  total: z.number().default(0).catch(0),
}).loose();

export const FlowDocRoleSchema = z.object({
  key: z.string().default(""),
  label: z.string().optional().catch(undefined),
  kind: z.string().default("agent").catch("agent"),
  pool: z.number().optional().catch(undefined),
  bind: z.object({
    agent_ids: z.array(z.string()).optional().catch(undefined),
    member_ids: z.array(z.string()).optional().catch(undefined),
  }).optional().catch(undefined),
  source: z.object({
    bundle: z.string().optional().catch(undefined),
    agent_key: z.string().optional().catch(undefined),
  }).optional().catch(undefined),
}).loose();

export const FlowDocVarSchema = z.object({
  key: z.string().default(""),
  type: z.string().default("string").catch("string"),
  values: z.array(z.string()).optional().catch(undefined),
  min: z.number().optional().catch(undefined),
  max: z.number().optional().catch(undefined),
  max_len: z.number().optional().catch(undefined),
  max_items: z.number().optional().catch(undefined),
  default: z.unknown(),
  set_by: z.array(z.string()).optional().catch(undefined),
  set_at: z.array(z.string()).optional().catch(undefined),
}).loose();

export const FlowDocVerdictSchema = z.object({
  key: z.string().default(""),
  class: z.string().default("pass").catch("pass"),
  label: z.string().optional().catch(undefined),
}).loose();

export const FlowDocVerdictSetSchema = z.object({
  key: z.string().default(""),
  verdicts: tolerantList(FlowDocVerdictSchema),
}).loose();

export const FlowDocLoopSchema = z.object({
  key: z.string().default(""),
  header: z.string().default(""),
  max: z.union([
    z.number(),
    z.object({ var: z.string(), cap: z.number() }).loose(),
  ]).default(1).catch(1),
  reset: z.string().optional().catch(undefined),
}).loose();

export const FlowDocStopKindSchema = z.object({
  code: z.string().default(""),
  class: z.string().default("custom").catch("custom"),
  label: z.string().optional().catch(undefined),
  message: z.string().optional().catch(undefined),
  status: z.string().optional().catch(undefined),
  resume_by: z.string().optional().catch(undefined),
  legacy_no: z.number().optional().catch(undefined),
}).loose();

export const FlowDocTerminalSchema = z.object({
  key: z.string().default(""),
  status: z.string().default(""),
  leaves_workspace: z.boolean().optional().catch(undefined),
}).loose();

export const FlowDocTaskSchema = z.object({
  key: z.string().default(""),
  title: z.string().default(""),
  kind: z.string().default("work").catch("work"),
  role: z.string().default(""),
  instances: z.object({
    count: z.number().optional().catch(undefined),
    from_var: z.string().optional().catch(undefined),
    min: z.number().optional().catch(undefined),
    max: z.number().optional().catch(undefined),
    distinct: z.boolean().optional().catch(undefined),
  }).optional().catch(undefined),
  after: z.array(z.string()).optional().catch(undefined),
  when: z.unknown().optional().catch(undefined),
  verdicts: z.string().optional().catch(undefined),
  requires: z.object({
    files: z.array(z.object({
      slot: z.string().default(""),
      min: z.number().default(1),
      max: z.number().optional().catch(undefined),
      ext: z.array(z.string()).optional().catch(undefined),
      content_types: z.array(z.string()).optional().catch(undefined),
    }).loose()).optional().catch(undefined),
    verdict: z.boolean().nullable().optional().catch(undefined),
    vars: z.array(z.string()).optional().catch(undefined),
  }).optional().catch(undefined),
  inputs: z.array(z.object({
    from: z.string().default(""),
    slot: z.string().optional().catch(undefined),
    required: z.boolean().optional().catch(undefined),
  }).loose()).optional().catch(undefined),
  sets: z.array(z.string()).optional().catch(undefined),
  skills: z.array(z.string()).optional().catch(undefined),
  deadline: z.object({
    idle_minutes: z.number().optional().catch(undefined),
    hard_cap_minutes: z.number().optional().catch(undefined),
    max_attempts: z.number().optional().catch(undefined),
    remind_after_minutes: z.number().optional().catch(undefined),
    stall_after_minutes: z.number().optional().catch(undefined),
  }).optional().catch(undefined),
  instructions: z.string().optional().catch(undefined),
}).loose();

export const FlowDocEdgeSchema = z.object({
  id: z.string().default(""),
  label: z.string().optional().catch(undefined),
  to: z.string().optional().catch(undefined),
  when: z.unknown().optional().catch(undefined),
  loop: z.string().optional().catch(undefined),
  stop: z.string().optional().catch(undefined),
  trigger: z.string().optional().catch(undefined),
}).loose();

export const FlowDocStepSchema = z.object({
  key: z.string().default(""),
  title: z.string().optional().catch(undefined),
  status: z.object({
    key: z.string().optional().catch(undefined),
    name: z.string().optional().catch(undefined),
    category: z.string().optional().catch(undefined),
  }).default({}).catch({}),
  kind: z.string().default("tasks").catch("tasks"),
  tasks: tolerantList(FlowDocTaskSchema).optional().catch(undefined),
  join: z.object({
    over: z.union([z.string(), z.array(z.string())]).default("gates").catch("gates"),
    mode: z.string().optional().catch(undefined),
    k: z.number().optional().catch(undefined),
    block: z.string().optional().catch(undefined),
  }).optional().catch(undefined),
  lead: z.object({
    sets: z.array(z.string()).optional().catch(undefined),
    verdicts: z.string().optional().catch(undefined),
    requires: z.unknown().optional().catch(undefined),
  }).optional().catch(undefined),
  exit_role: z.string().optional().catch(undefined),
  notice: z.string().optional().catch(undefined),
  edges: tolerantList(FlowDocEdgeSchema),
  layout: z.object({
    x: z.number().default(0),
    y: z.number().default(0),
  }).optional().catch(undefined),
}).loose();

export const FlowDocSchema = z.object({
  version: z.number().default(2).catch(2),
  start: z.string().default("plan"),
  roles: tolerantList(FlowDocRoleSchema).optional().catch([]),
  vars: tolerantList(FlowDocVarSchema).optional().catch([]),
  verdicts: tolerantList(FlowDocVerdictSetSchema).optional().catch([]),
  loops: tolerantList(FlowDocLoopSchema).optional().catch([]),
  stops: tolerantList(FlowDocStopKindSchema).optional().catch([]),
  terminals: tolerantList(FlowDocTerminalSchema),
  steps: tolerantList(FlowDocStepSchema),
  messages: z.record(z.string(), z.string()).optional().catch({}),
  policy: z.object({
    task_idle_minutes: z.number().optional().catch(undefined),
    task_hard_cap_minutes: z.number().optional().catch(undefined),
    task_max_attempts: z.number().optional().catch(undefined),
    human_remind_minutes: z.number().optional().catch(undefined),
    human_stall_minutes: z.number().optional().catch(undefined),
    max_activations: z.number().optional().catch(undefined),
    max_task_issues: z.number().optional().catch(undefined),
    lead_may_work: z.boolean().optional().catch(undefined),
    allow_unattended_release: z.boolean().optional().catch(undefined),
    acks: z.array(z.object({
      kind: z.string().default(""),
      by: z.string().optional().catch(undefined),
      at: z.string().optional().catch(undefined),
    }).loose()).optional().catch(undefined),
  }).optional().catch(undefined),
}).loose();

export const FlowDraftSchema = z.object({
  id: z.string().default("").catch(""),
  squad_id: z.string().default("").catch(""),
  name: z.string().default("").catch(""),
  lineage_id: z.string().nullable().default(null).catch(null),
  base_workflow_id: z.string().nullable().default(null).catch(null),
  revision: z.number().default(1).catch(1),
  doc: z.unknown().transform((val): FlowDoc => {
    if (!val || typeof val !== "object") return EMPTY_FLOW_DRAFT.doc;
    const res = FlowDocSchema.safeParse(val);
    return res.success ? (res.data as FlowDoc) : EMPTY_FLOW_DRAFT.doc;
  }),
  origin: z.string().default("manual").catch("manual"),
  source_ref: z.unknown().nullable().default(null).catch(null),
  created_by: z.string().default("").catch(""),
  updated_by: z.string().default("").catch(""),
  created_at: z.string().default("").catch(""),
  updated_at: z.string().default("").catch(""),
}).loose();

export const FlowIssueSchema = z.object({
  code: z.string().default(""),
  severity: z.unknown().transform((v): "error" | "warning" | "info" => {
    if (v === "error" || v === "info") return v;
    return "warning";
  }),
  path: z.string().optional().catch(undefined),
  message: z.string().default(""),
  params: z.record(z.string(), z.unknown()).optional().catch(undefined),
  ack: z.string().optional().catch(undefined),
}).loose();

export const EMPTY_FLOW_VALIDATION_RESPONSE: FlowValidationResponse = {
  ok: false,
  revision: 0,
  issues: [],
  analysis: null,
  fact_catalog: {},
};

export const FlowValidationResponseSchema = z.object({
  ok: z.boolean().default(false).catch(false),
  revision: z.number().default(0).catch(0),
  issues: z.unknown().transform((v): FlowIssue[] => {
    if (!Array.isArray(v)) return [];
    return v.flatMap((item) => {
      const parsed = FlowIssueSchema.safeParse(item);
      return parsed.success ? [parsed.data as FlowIssue] : [];
    });
  }),
  analysis: z.object({
    worst_case_task_issues: z.number().default(0).catch(0),
    worst_case_activations: z.number().default(0).catch(0),
    steps: tolerantList(z.object({
      step: z.string().default(""),
      max_entries: z.number().default(0).catch(0),
      max_tasks: z.number().default(0).catch(0),
    }).loose()),
    loops: tolerantList(z.object({
      key: z.string().default(""),
      header: z.string().default(""),
      max: z.number().default(0).catch(0),
      body: z.array(z.string()).default([]).catch([]),
    }).loose()),
    stops: tolerantList(z.object({
      code: z.string().default(""),
      class: z.string().optional().catch(undefined),
      status: z.string().default(""),
      edges: z.array(z.string()).default([]).catch([]),
    }).loose()),
    release_paths_without_human: tolerantList(z.object({
      from: z.string().default(""),
      terminal: z.string().default(""),
      path: z.array(z.string()).default([]).catch([]),
    }).loose()),
  }).loose().nullable().default(null).catch(null),
  fact_catalog: z.record(z.string(), tolerantList(z.object({
    key: z.string().default(""),
    type: z.string().default(""),
    source: z.string().default(""),
    ops: z.array(z.string()).default([]).catch([]),
    values: z.array(z.string()).optional().catch(undefined),
  }).loose())).default({}).catch({}),
}).loose();

export const FlowTraceHopSchema = z.object({
  n: z.number().default(0).catch(0),
  step: z.string().default(""),
  activation: z.number().default(1).catch(1),
  opened: z.array(z.string()).optional().catch(undefined),
  closed: z.record(z.string(), z.string()).optional().catch(undefined),
  join: z.string().optional().catch(undefined),
  edge: z.string().optional().catch(undefined),
  to: z.string().optional().catch(undefined),
  loops: z.record(z.string(), z.number()).optional().catch(undefined),
}).loose();

export const FlowTraceEndSchema = z.object({
  kind: z.string().default("stalled").catch("stalled"),
  code: z.string().optional().catch(undefined),
  from: z.string().optional().catch(undefined),
}).loose();

export const EMPTY_FLOW_TRACE_RESPONSE: FlowTraceResponse = {
  hops: [],
  end: { kind: "stalled" },
  truncated: false,
};

export const FlowTraceResponseSchema = z.object({
  hops: tolerantList(FlowTraceHopSchema),
  end: FlowTraceEndSchema.default({ kind: "stalled" }).catch({ kind: "stalled" }),
  truncated: z.boolean().default(false).catch(false),
}).loose();

export const EMPTY_FLOW_MANIFEST_RESPONSE: FlowManifestResponse = {
  format: "multica.flow-manifest/v1",
  name: "",
  doc: EMPTY_FLOW_DRAFT.doc,
};

export const FlowManifestResponseSchema = z.object({
  format: z.string().default("multica.flow-manifest/v1").catch("multica.flow-manifest/v1"),
  name: z.string().default("").catch(""),
  doc: z.unknown().transform((val): FlowDoc => {
    if (!val || typeof val !== "object") return EMPTY_FLOW_DRAFT.doc;
    const res = FlowDocSchema.safeParse(val);
    return res.success ? (res.data as FlowDoc) : EMPTY_FLOW_DRAFT.doc;
  }),
}).loose();

export const EMPTY_ISSUE_FLOW_TASK: IssueFlowTask = {
  issue_id: "",
  identifier: "",
  step: "",
  task: "",
  round: 0,
  kind: "",
  status: "",
  outcome: null,
  assignee_id: null,
  closed_at: null,
  closed_by: null,
};

export const CloseIssueFlowTaskResponseSchema = z.object({
  task: IssueFlowTaskSchema,
  state: IssueFlowStateSchema,
}).loose();

export const EMPTY_CLOSE_ISSUE_FLOW_TASK_RESPONSE: { task: IssueFlowTask; state: IssueFlowState } = {
  task: EMPTY_ISSUE_FLOW_TASK,
  state: EMPTY_ISSUE_FLOW_STATE,
};

export const EMPTY_FLOW_CAPABILITIES_RESPONSE: FlowCapabilitiesResponse = {
  schemas: [1, 2],
  limits: {
    doc_bytes: 524288,
    steps: 40,
    status_names: 12,
    tasks_per_step: 12,
    edges_per_step: 10,
    roles: 16,
    vars: 16,
    verdict_sets: 8,
    loops: 8,
    loop_max: 20,
    instances: 20,
    expr_depth: 4,
    after_depth: 6,
    message_runes: 4000,
    instructions_runes: 8000,
    inputs_per_task: 12,
    sets_per_task: 8,
    skills_per_task: 8,
    default_max_task_issues: 300,
    max_task_issues: 500,
    default_max_activations: 100,
    schema1_steps: 12,
    schema1_tasks_per_step: 8,
    schema1_rework_max: 5,
    drafts_per_squad: 20,
    versions_per_lineage: 50,
    draft_name_runes: 56,
    workflow_name_runes: 64,
    task_title_runes: 80,
    default_task_idle_minutes: 120,
  },
  step_kinds: ["tasks", "lead", "manual"],
  task_kinds: ["work", "gate"],
  join_modes: ["all", "any", "quorum"],
  join_over: ["gates", "all", "tasks"],
  join_block: ["immediate", "at_close"],
  role_kinds: ["agent", "member", "lead", "requester", "managers"],
  verdict_classes: ["pass", "fail", "neutral"],
  var_types: ["bool", "enum", "int", "string", "list_string"],
  stop_classes: ["missing_grounds", "loop_exhausted", "custom"],
  edge_triggers: ["auto", "manual"],
  loop_resets: ["on_entry", "never"],
  ops_by_type: {},
  builtin_facts: [],
  features: {
    packs: false,
    proposals: false,
    insights: false,
    system_performer: false,
    subflow: false,
    fork_join: false,
    authoring: false,
    graph_v2: false,
  },
};

export const FlowLimitsSchema = z.object({
  doc_bytes: z.number().default(524288).catch(524288),
  steps: z.number().default(40).catch(40),
  status_names: z.number().default(12).catch(12),
  tasks_per_step: z.number().default(12).catch(12),
  edges_per_step: z.number().default(10).catch(10),
  roles: z.number().default(16).catch(16),
  vars: z.number().default(16).catch(16),
  verdict_sets: z.number().default(8).catch(8),
  loops: z.number().default(8).catch(8),
  loop_max: z.number().default(20).catch(20),
  instances: z.number().default(20).catch(20),
  expr_depth: z.number().default(4).catch(4),
  after_depth: z.number().default(6).catch(6),
  message_runes: z.number().default(4000).catch(4000),
  instructions_runes: z.number().default(8000).catch(8000),
  inputs_per_task: z.number().default(12).catch(12),
  sets_per_task: z.number().default(8).catch(8),
  skills_per_task: z.number().default(8).catch(8),
  default_max_task_issues: z.number().default(300).catch(300),
  max_task_issues: z.number().default(500).catch(500),
  default_max_activations: z.number().default(100).catch(100),
  schema1_steps: z.number().default(12).catch(12),
  schema1_tasks_per_step: z.number().default(8).catch(8),
  schema1_rework_max: z.number().default(5).catch(5),
  drafts_per_squad: z.number().default(20).catch(20),
  versions_per_lineage: z.number().default(50).catch(50),
  draft_name_runes: z.number().default(56).catch(56),
  workflow_name_runes: z.number().default(64).catch(64),
  task_title_runes: z.number().default(80).catch(80),
  default_task_idle_minutes: z.number().default(120).catch(120),
}).loose();

export const FlowCapabilityFeaturesSchema = z.object({
  packs: z.boolean().default(false).catch(false),
  proposals: z.boolean().default(false).catch(false),
  insights: z.boolean().default(false).catch(false),
  system_performer: z.boolean().default(false).catch(false),
  subflow: z.boolean().default(false).catch(false),
  fork_join: z.boolean().default(false).catch(false),
  authoring: z.boolean().default(false).catch(false),
  graph_v2: z.boolean().default(false).catch(false),
}).loose();

export const FlowCapabilitiesResponseSchema = z.object({
  schemas: z.array(z.number()).default([1, 2]).catch([1, 2]),
  limits: z.unknown().transform((v): FlowLimits => {
    const res = FlowLimitsSchema.safeParse(v ?? {});
    return res.success ? (res.data as FlowLimits) : EMPTY_FLOW_CAPABILITIES_RESPONSE.limits;
  }),
  step_kinds: z.array(z.string()).default(["tasks", "lead", "manual"]).catch(["tasks", "lead", "manual"]),
  task_kinds: z.array(z.string()).default(["work", "gate"]).catch(["work", "gate"]),
  join_modes: z.array(z.string()).default(["all", "any", "quorum"]).catch(["all", "any", "quorum"]),
  join_over: z.array(z.string()).default(["gates", "all", "tasks"]).catch(["gates", "all", "tasks"]),
  join_block: z.array(z.string()).default(["immediate", "at_close"]).catch(["immediate", "at_close"]),
  role_kinds: z.array(z.string()).default(["agent", "member", "lead", "requester", "managers"]).catch(["agent", "member", "lead", "requester", "managers"]),
  verdict_classes: z.array(z.string()).default(["pass", "fail", "neutral"]).catch(["pass", "fail", "neutral"]),
  var_types: z.array(z.string()).default(["bool", "enum", "int", "string", "list_string"]).catch(["bool", "enum", "int", "string", "list_string"]),
  stop_classes: z.array(z.string()).default(["missing_grounds", "loop_exhausted", "custom"]).catch(["missing_grounds", "loop_exhausted", "custom"]),
  edge_triggers: z.array(z.string()).default(["auto", "manual"]).catch(["auto", "manual"]),
  loop_resets: z.array(z.string()).default(["on_entry", "never"]).catch(["on_entry", "never"]),
  ops_by_type: z.record(z.string(), z.array(z.string())).default({}).catch({}),
  builtin_facts: tolerantList(z.object({
    key: z.string().default(""),
    type: z.string().default(""),
    source: z.string().default(""),
    ops: z.array(z.string()).default([]).catch([]),
    values: z.array(z.string()).optional().catch(undefined),
  }).loose()),
  features: z.unknown().transform((v): FlowCapabilityFeatures => {
    const res = FlowCapabilityFeaturesSchema.safeParse(v ?? {});
    return res.success ? (res.data as FlowCapabilityFeatures) : EMPTY_FLOW_CAPABILITIES_RESPONSE.features;
  }),
}).loose();

const text = () => z.string().default("").catch("");

const count = () => z.number().default(0).catch(0);

const optionalId = () => z.string().nullable().default(null).catch(null);

const PackIssueSchema = z.object({
  code: text(),
  // The server knows error and info; a severity this build does not know is a warning, so that the person looks.
  severity: z.unknown().transform((v): PackIssue["severity"] => (v === "error" || v === "info" ? v : "warning")),
  path: z.string().optional().catch(undefined),
  message: text(),
  params: z.record(z.string(), z.unknown()).optional().catch(undefined),
}).loose();

const PackPreviewAgentSchema = z.object({
  key: text(),
  name: text(),
  description: text(),
  instructions_bytes: count(),
  skills: z.array(z.string()).default([]).catch([]),
  skills_declared: z.boolean().default(false).catch(false),
  existing_agent_id: optionalId(),
}).loose();

const PackPreviewSkillSchema = z.object({
  name: text(),
  files: count(),
  existing_skill_id: optionalId(),
  same_content: z.boolean().default(false).catch(false),
}).loose();

const PackPreviewReferenceSchema = z.object({ path: text(), bytes: count(), kind: text() }).loose();

const PackPreviewFindingSchema = z.object({
  path: text(),
  // A severity this build does not know is a warning: it asks for a tick instead of passing unseen.
  severity: z.unknown().transform((v): PackScanSeverity => (v === "info" || v === "block" ? v : "warn")),
  rule: text(),
}).loose();

const PackIgnoredSchema = z.object({
  agent: z.string().optional().catch(undefined),
  skill: z.string().optional().catch(undefined),
  key: text(),
}).loose();

export const EMPTY_PACK_PREVIEW: PackPreview = {
  digest: "",
  format: "",
  id: "",
  name: "",
  version: "",
  agents: [],
  skills: [],
  leader: null,
  flow: { present: false, validation: null, issues: [] },
  references: [],
  references_omitted: 0,
  ignored: [],
  scan: [],
  issues: [],
};

export const PackPreviewSchema = z.object({
  digest: text(),
  format: text(),
  id: text(),
  name: text(),
  version: text(),
  agents: tolerantList(PackPreviewAgentSchema),
  skills: tolerantList(PackPreviewSkillSchema),
  leader: optionalId(),
  flow: z.unknown().transform((v): PackPreview["flow"] => {
    const obj = v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
    const validation = FlowValidationResponseSchema.safeParse(obj.validation);
    return {
      present: obj.present === true,
      validation: validation.success ? (validation.data as FlowValidationResponse) : null,
      issues: z.array(z.unknown()).safeParse(obj.issues).success
        ? (obj.issues as unknown[]).flatMap((item) => {
            const parsed = PackIssueSchema.safeParse(item);
            return parsed.success ? [parsed.data as PackIssue] : [];
          })
        : [],
    };
  }),
  references: tolerantList(PackPreviewReferenceSchema),
  references_omitted: count(),
  ignored: tolerantList(PackIgnoredSchema),
  scan: tolerantList(PackPreviewFindingSchema),
  issues: tolerantList(PackIssueSchema),
}).loose() as unknown as z.ZodType<PackPreview>;

export const EMPTY_PACK_COMMIT_RESPONSE: PackCommitResponse = {
  install_id: "",
  already_installed: false,
  squad_id: null,
  agents: [],
  skills: [],
  draft_id: null,
  flow_issues: [],
};

export const PackCommitResponseSchema = z.object({
  install_id: text(),
  already_installed: z.boolean().default(false).catch(false),
  squad_id: optionalId(),
  agents: tolerantList(z.object({
    key: text(),
    agent_id: text(),
    created: z.boolean().default(false).catch(false),
  }).loose()),
  skills: tolerantList(z.object({
    name: text(),
    skill_id: text(),
    created: z.boolean().default(false).catch(false),
  }).loose()),
  draft_id: optionalId(),
  flow_issues: tolerantList(FlowIssueSchema),
}).loose() as unknown as z.ZodType<PackCommitResponse>;

export const EMPTY_FLOW_INSIGHTS: FlowInsights = {
  requests: { total: 0, open: 0, by_end: {} },
  steps: [],
  edges: [],
  loops: [],
  stalls: [],
  manual_closes: 0,
  truncated: false,
};

const nullableCount = () => z.number().nullable().default(null).catch(null);

export const FlowInsightsSchema = z.object({
  requests: z.unknown().transform((v): FlowInsights["requests"] => {
    const obj = v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
    const byEnd: Record<string, number> = {};
    if (obj.by_end && typeof obj.by_end === "object" && !Array.isArray(obj.by_end)) {
      for (const [key, n] of Object.entries(obj.by_end)) if (typeof n === "number") byEnd[key] = n;
    }
    return {
      total: typeof obj.total === "number" ? obj.total : 0,
      open: typeof obj.open === "number" ? obj.open : 0,
      by_end: byEnd,
    };
  }),
  steps: tolerantList(z.object({
    key: text(),
    entered: count(),
    median_minutes: nullableCount(),
    p90_minutes: nullableCount(),
  }).loose()),
  edges: tolerantList(z.object({ step: text(), edge: text(), taken: count() }).loose()),
  loops: tolerantList(z.object({
    key: text(),
    max: count(),
    histogram: z.unknown().transform((v): number[] => (Array.isArray(v) ? v.map((n) => (typeof n === "number" ? n : 0)) : [])),
  }).loose()),
  stalls: tolerantList(z.object({ reason: text(), count: count() }).loose()),
  manual_closes: count(),
  truncated: z.boolean().default(false).catch(false),
}).loose() as unknown as z.ZodType<FlowInsights>;

export const DEFAULT_POLICY_FLOOR: PolicyFloor = {
  allowed_invite_domains: [],
  share_links_enabled: true,
  require_sso: false,
  require_mfa: false,
  session_max_age_hours: 0,
};

export const PolicyFloorSchema = z
  .object({
    allowed_invite_domains: z
      .array(z.string())
      .nullish()
      .transform((v) => v ?? []),
    share_links_enabled: z.boolean().default(true),
    require_sso: z.boolean().default(false),
    require_mfa: z.boolean().default(false),
    session_max_age_hours: z.number().min(0).max(8760).default(0),
  })
  .loose();

export const EMPTY_QUOTA_REQUEST: QuotaRequest = {
  id: "",
  workspace_id: "",
  gate: "",
  current_value: null,
  requested_value: 0,
  approved_value: null,
  state: "",
  reason: "",
  decision_reason: "",
  requested_by: "",
  requested_by_label: "",
  decided_by: "",
  created_at: "",
  decided_at: null,
};

export const EMPTY_QUOTA_REQUEST_LIST: QuotaRequestListResponse = {
  requests: [],
};

const nullableTextDefaultEmpty = () =>
  z
    .string()
    .nullish()
    .transform((v) => v ?? "");

export const QuotaRequestSchema = z
  .object({
    id: nullableTextDefaultEmpty(),
    workspace_id: nullableTextDefaultEmpty(),
    gate: nullableTextDefaultEmpty(),
    current_value: z.number().nullable().default(null),
    requested_value: z.number().default(0),
    approved_value: z.number().nullable().default(null),
    state: nullableTextDefaultEmpty(),
    reason: nullableTextDefaultEmpty(),
    decision_reason: nullableTextDefaultEmpty(),
    requested_by: nullableTextDefaultEmpty(),
    requested_by_label: nullableTextDefaultEmpty(),
    decided_by: nullableTextDefaultEmpty(),
    created_at: nullableTextDefaultEmpty(),
    decided_at: z.string().nullable().default(null),
  })
  .loose();

export const QuotaRequestListResponseSchema = z
  .object({
    requests: z
      .array(QuotaRequestSchema)
      .nullish()
      .transform((v) => v ?? []),
  })
  .loose();

// The room-owned workflow policy and its write preview (squad-room-workflow spec, sections 4 to 6).
const RoomWorkflowCodeSchema = z.enum(["room_workflow_required", "room_no_workflow_for_non_request"]);

export const RoomWorkflowPolicyResponseSchema = z.object({
  non_request_workflow: z
    .object({
      kind: z.enum(["no_action", "default", "workflow"]).catch("no_action"),
      workflow_id: z.string().nullish(),
    })
    .catch({ kind: "no_action" }),
});

export const RoomWorkflowCheckSchema = z.object({
  needs_workflow: z.boolean().catch(false),
  code: RoomWorkflowCodeSchema.nullish(),
  message: z.string().nullish(),
  squad_id: z.string().nullish(),
});

// Principal profile, delegation, prediction, evidence candidate, and queued attention schemas.
export {
  DelegationEnableResultSchema,
  EMPTY_PRINCIPAL_DELEGATIONS,
  EMPTY_PRINCIPAL_EVIDENCE_CANDIDATES,
  PrincipalDelegationSchema,
  PrincipalDelegationsSchema,
  PrincipalEvidenceCandidateSchema,
  PrincipalEvidenceCandidatesSchema,
  PrincipalEvidenceSchema,
  PrincipalInterviewSchema,
  PrincipalKindAccuracySchema,
  PrincipalPredictionSchema,
  PrincipalProfileSchema,
  PrincipalTraitSchema,
} from "../durable/principal/schemas";
export {
  EMPTY_QUEUED_ATTENTION,
  QueuedAttentionAskSchema,
  QueuedAttentionListSchema,
} from "../governance/attention";
