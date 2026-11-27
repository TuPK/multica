// Fork-owned ApiClient methods. They live in a base class of ApiClient so upstream edits to
// client.ts never conflict with them (docs/specs/upstream-sync/spec.md section 3.9). They
// reach the transport through the protected members declared below, which ApiClient provides.
import type { WeixinLogin } from "../types/weixin";
import { WeixinLoginSchema, INVALID_WEIXIN_LOGIN } from "../weixin/schema";
import { AcceptConsentsResponseSchema, EMPTY_ACCEPT_CONSENTS_RESPONSE, EMPTY_REVOKE_ALL_SESSIONS_RESPONSE, RevokeAllSessionsResponseSchema, type AcceptConsentItem, type AcceptConsentsResponse, type RevokeAllSessionsResponse } from "../account/schema";
import { AccountDeletionCanceledSchema, AccountDeletionPreviewSchema, AccountDeletionScheduledSchema, EMPTY_ACCOUNT_DELETION_CANCELED, EMPTY_ACCOUNT_DELETION_PREVIEW, EMPTY_ACCOUNT_DELETION_SCHEDULED, type AccountDeletionCanceled, type AccountDeletionPreview, type AccountDeletionScheduled } from "../account/deletion-schema";
import { AuditEventPageSchema, EMPTY_AUDIT_EVENT_PAGE, type AuditEventPage } from "../audit/schema";
import { auditQueryString, type AuditFilters, type AuditPageParams } from "../audit/params";
import { WorkspaceOrderSchema, WorkspaceOrderUsageSchema } from "../billing/workspace-orders-schemas";
import type { WorkspaceOrder, WorkspaceOrderBuyer, WorkspaceOrderUsage } from "../billing/workspace-orders-types";
import { RunQueuePreviewSchema, RunQueueWaitSchema } from "./schemas";
import type { ListIssueWorkflowTemplatesResponse } from "../types/issue-workflow";
import type { RoomWorkflowCheck, RoomWorkflowCheckParams, RoomWorkflowPolicy } from "../types/room-workflow";
import { RoomWorkflowCheckSchema, RoomWorkflowPolicyResponseSchema } from "./schemas";
import type { CreatePublishChannelInput, SavePublishSettingsInput } from "../types/publish";
import type { UpdateResearchSettingsInput } from "../types/research";
import { AddMemberGroupMembersResponseSchema, MemberGroupDetailSchema, MemberGroupListSchema, MemberGroupSchema, MyPermissionsSchema, PermissionCatalogueSchema, emptyGroup, type MemberGroup, type MemberGroupDetail, type MyPermissions } from "../groups/schema";
import { EMPTY_COMPANY_FLOOR, EMPTY_COMPANY_FLOOR_LIST, EMPTY_COMPANY_SEATS_RESPONSE, CompanyFloorListSchema, CompanyFloorSchema, CompanySeatsResponseSchema, AssignSquadProjectResponseSchema, EMPTY_ASSIGN_SQUAD_PROJECT_RESPONSE } from "../company/schemas";
import type { CreateCompanyFloorRequest, CompanyFloor, CompanySeatAssignment, CompanySeatsResponse, UpdateCompanyFloorRequest, AssignSquadProjectRequest, AssignSquadProjectResponse } from "../company/types";
import type { Issue, RunQueuePreview, RunQueueSetting, RunQueueWait, Workspace, SkillSummary, RuntimeQuickstartRequest, IssueWorkflow, IssueWorkflowWriteRequest, IssueWorkflowDryRunResponse, ListIssueWorkflowUsageResponse, Autopilot, AutopilotTrigger, AutopilotRun, AutopilotQuotaUsage, CreateAutopilotRequest, UpdateAutopilotRequest, CreateAutopilotTriggerRequest, UpdateAutopilotTriggerRequest, ListAutopilotsResponse, CronPreviewResponse, GetAutopilotResponse, AutopilotCollaboratorsResponse, ListAutopilotRunsResponse, ListWebhookDeliveriesResponse, WebhookDelivery, IssuePullRequestsResponse, IssueEnvironmentsResponse, IssueEnvironmentAction, IssueEnvironmentCommand, StartIssueEnvironmentRequest, IssuePullRequestSummariesResponse, ListGitHubInstallationsResponse, ListGitHubRepositoriesResponse, GitHubConnectResponse, ListVCSConnectionsResponse, ConnectVCSRequest, ConnectVCSResponse, ListLarkInstallationsResponse, BeginLarkInstallResponse, LarkInstallStatusResponse, RedeemLarkBindingTokenResponse, ComposioToolkit, ComposioConnection, ComposioConnectInitResponse, AppsOverview, AppCatalogResponse, AppRule, AppToolCall, IssueAppCall, AppRequestList, TeamAppImpact, PersonalAppImpact, AppRequest, AgentApp, AppAccountScope, AppActionClass, AppActionPolicy, AppCatalogMode, UpdateAgentAppsRequest, SlackInstallation, ListSlackInstallationsResponse, RegisterSlackBYORequest, RedeemSlackBindingTokenResponse, DingTalkInstallation, ListDingTalkInstallationsResponse, ListDingTalkGroupsResponse, ListDingTalkGroupsParams, RegisterDingTalkBYORequest, RedeemDingTalkBindingTokenResponse, WecomInstallation, ListWecomInstallationsResponse, RegisterWecomBYORequest, BeginWecomInstallResponse, WecomInstallStatusResponse, Squad, SquadHandoffMode, IssueFlowState, IssueFlowTask, SquadMember, SquadMemberStatusListResponse, SquadSkill, SquadPlugin, SetSquadRuntimeResponse, WorkspaceSubscriptionUsage, IssueLimitUsage, PurchaseWorkspaceSeatsRequest, PurchaseWorkspaceSeatsResponse } from "../types";
import type { WorkspacePendingPlanChangeCleared, WorkspaceSubscriptionCancelState } from "../types/billing";
import type { FlowDraft, FlowValidationResponse, CompleteIssueFlowStepRequest } from "../types/flow-draft";
import type { PackPreview } from "../types/pack";
import type { CreateQuotaRequestInput, QuotaRequest, QuotaRequestListResponse } from "../types/quota-request";
import { createSafeId } from "../utils";
import { parseWithFallback } from "./schema";
import { LoginAttemptSchema, LoginResponseSchema, type LoginAttempt } from "./login-schemas";
import { NO_SSO, SSOConnectionSchema, SSODiscoverySchema, type SSOConnection, type SSOConnectionInput, type SSODiscovery } from "./sso-schemas";
import { DEFAULT_WORKSPACE_POLICY, MFAConfirmResponseSchema, MFAEnrollResponseSchema, NO_SSO_DOMAINS, SSODomainListSchema, SSODomainSchema, WorkspacePolicySchema, type MFAConfirmResponse, type MFAEnrollResponse, type SSODomain, type SSODomainList, type WorkspacePolicy, type WorkspacePolicyInput } from "./security-schemas";
import type { HubItemKind } from "../hub/items";
import { mapHubSourceDeleteError, mapHubSyncError } from "../hub/errors";
import { HubItemMetaSchema, HubSyncResultSchema, EMPTY_HUB_SYNC_RESULT, HubUpdateCheckSchema, EMPTY_HUB_UPDATE_CHECK, HubActivityPageSchema, EMPTY_HUB_ACTIVITY_PAGE, EMPTY_HUB_ITEM_META } from "./schemas";
import type { HubActivityFilter, HubActivityPage, HubItemMeta, HubSourceDeleteItems, HubSyncResult, HubUpdateCheck } from "../types/hub";
import { EMPTY_ORG_MAP, OrgMapSchema, type OrgMap } from "../company/org-map-schema";
import { parseInstructionRevisions, type InstructionRevision } from "../agents/instruction-revisions";
import { BriefListSchema, BriefSchema, FALLBACK_BRIEF, type Brief, type ListBriefsParams, type PublishBriefRequest } from "../company/brief-schema";
import { DigestSchema, FALLBACK_DIGEST, type Digest, type DigestParams } from "../company/digest-schema";
import { BriefPushBindingListSchema, BriefPushPreferenceSchema, EMPTY_BRIEF_PUSH, type BriefPushBinding, type BriefPushPreference, type UpdateBriefPushRequest } from "../company/brief-push-schema";
import { BriefFeedbackOrNoneSchema, BriefFeedbackSummarySchema, type BriefFeedback, type BriefFeedbackSummary, type SubmitBriefFeedbackRequest } from "../company/brief-feedback-schema";
import { SquadAttentionSchema, type PinSquadAttentionRequest, type SquadAttention } from "../company/squad-attention-schema";
import { EMPTY_SQUAD_ROUTINE, SquadCharterSchema, SquadRoutineListSchema, SquadRoutineSchema, emptySquadCharter, type CreateSquadRoutineRequest, type SquadCharter, type SquadRoutine, type UpdateSquadCharterRequest } from "../company/charter-schema";
import { FALLBACK_COMPANY_SETUP_READINESS, CompanySetupReadinessSchema, type CompanySetupReadiness, type CompanySetupRequest } from "../company/setup-schema";
import { CompanyTemplateApplyResultSchema, FALLBACK_COMPANY_TEMPLATE_RESULT } from "../company/company-template-schema";
import type { CompanyTemplateApplyRequest, CompanyTemplateApplyResult } from "../company/company-template-types";
import { FALLBACK_KNOWLEDGE_BASES, FALLBACK_KNOWLEDGE_DOCUMENT, FALLBACK_KNOWLEDGE_DOCUMENT_LIST, FALLBACK_KNOWLEDGE_SEARCH, FALLBACK_KNOWLEDGE_STATUS, KnowledgeBasesSchema, KnowledgeDocumentListSchema, KnowledgeDocumentSchema, KnowledgeSearchResultSchema, KnowledgeStatusSchema } from "../knowledge/schemas";
import { FALLBACK_KNOWLEDGE_ACCESS, FALLBACK_KNOWLEDGE_REPO_SYNC, FALLBACK_KNOWLEDGE_SOURCES, KnowledgeAccessSchema, KnowledgeRepoSyncSchema, KnowledgeSourcesSchema } from "../knowledge/platform-schemas";
import type { KnowledgeAccess, KnowledgeRepoSync, KnowledgeRepoSyncRequest, KnowledgeSources, UploadKnowledgeDocumentParams } from "../knowledge/platform-types";
import { FALLBACK_KNOWLEDGE_EVAL_RUN, FALLBACK_KNOWLEDGE_EVAL_RUNS, FALLBACK_KNOWLEDGE_HEALTH, FALLBACK_KNOWLEDGE_POLICIES, FALLBACK_KNOWLEDGE_POLICY, KnowledgeEvalRunSchema, KnowledgeEvalRunsSchema, FALLBACK_KNOWLEDGE_COLLECTION, KnowledgeCollectionSchema, KnowledgeHealthSchema, KnowledgePublishPoliciesSchema, KnowledgePublishPolicySchema } from "../knowledge/taxonomy";
import { FALLBACK_SALES_INTAKE, SalesIntakeSchema, type SalesIntake } from "../squads/sales-intake";
import type { KnowledgeCollection, KnowledgeEvalRun, KnowledgeEvalRuns, KnowledgeHealth, KnowledgePublishPolicies, KnowledgePublishPolicy, SetKnowledgePolicyRequest } from "../knowledge/taxonomy-types";
import type { CreateKnowledgeDocumentRequest, KnowledgeBases, KnowledgeDocument, KnowledgeDocumentList, KnowledgeSearchResult, KnowledgeStatus, ListKnowledgeDocumentsParams, SearchKnowledgeParams } from "../knowledge/types";
import { FALLBACK_GOAL, GoalResponseSchema } from "../governance/goals-schemas";
import type { Goal } from "../governance/goals-types";
import { FALLBACK_COMPANY_STATE, CompanyStateSchema } from "../governance/schemas";
import { parseHealthReport, type HealthReport } from "../governance/health";
import { parseQueuedAttention, type QueuedAttentionList } from "../governance/attention";
import { parseIssueHandoff, parseSquadHandoffRule, type IssueHandoff, type SetSquadHandoffRuleRequest, type SquadHandoffRule } from "../squads/handoff";
import { parseApprovalPolicies, parseApprovalPolicy, parsePolicyAutoApprovals, type ApprovalPolicy, type PolicyAutoApprovalHistory } from "../governance/approval-policy";
import { parseCompanyPulse, type CompanyPulse } from "../governance/scale";
import { BudgetListSchema, FALLBACK_BUDGET_LIST, parseBudget } from "../governance/budgets-schemas";
import type { Budget, BudgetList, CreateBudgetRequest } from "../governance/budgets-types";
import type { CompanyStateInfo } from "../governance/types";
import { FALLBACK_PROPOSAL, ProposalListSchema, ProposalRevisionListSchema, ProposalSchema, type ListProposalsParams, type Proposal, type ProposalRevision } from "../company/proposal-schema";
import { EMPTY_SQUAD, EMPTY_SQUAD_SKILL_LIST, EMPTY_SQUAD_PLUGIN_LIST, EMPTY_SET_SQUAD_RUNTIME_RESPONSE, SetSquadRuntimeResponseSchema, SquadSkillListSchema, SquadPluginListSchema, EMPTY_SQUAD_MEMBER_STATUS_LIST, EMPTY_LIST_WEBHOOK_DELIVERIES_RESPONSE, EMPTY_WEBHOOK_DELIVERY, ListAutopilotsResponseSchema, EMPTY_LIST_AUTOPILOTS_RESPONSE, AutopilotRunSchema, AutopilotQuotaUsageSchema, FALLBACK_AUTOPILOT_RUN, CronPreviewResponseSchema, UNREADABLE_CRON_PREVIEW_RESPONSE, ListWebhookDeliveriesResponseSchema, SquadSchema, IssueFlowStateSchema, EMPTY_ISSUE_FLOW_STATE, FlowDraftSchema, EMPTY_FLOW_DRAFT, FlowValidationResponseSchema, EMPTY_FLOW_VALIDATION_RESPONSE, CloseIssueFlowTaskResponseSchema, EMPTY_CLOSE_ISSUE_FLOW_TASK_RESPONSE, SquadMemberStatusListResponseSchema, WebhookDeliveryResponseSchema, WorkspaceSubscriptionUsageSchema, IssueLimitUsageSchema, PurchaseWorkspaceSeatsResponseSchema, ClearWorkspaceScheduledPlanChangeResponseSchema, WorkspaceSubscriptionCancelStateSchema, DingTalkInstallationSchema, ListDingTalkInstallationsResponseSchema, ListDingTalkGroupsResponseSchema, RedeemDingTalkBindingTokenResponseSchema, EMPTY_DINGTALK_INSTALLATION, EMPTY_LIST_DINGTALK_INSTALLATIONS_RESPONSE, EMPTY_LIST_DINGTALK_GROUPS_RESPONSE, EMPTY_REDEEM_DINGTALK_BINDING_TOKEN_RESPONSE, WecomInstallationSchema, ListWecomInstallationsResponseSchema, BeginWecomInstallResponseSchema, MALFORMED_BEGIN_WECOM_INSTALL_RESPONSE, WecomInstallStatusResponseSchema, MALFORMED_WECOM_INSTALL_STATUS_RESPONSE, EMPTY_WECOM_INSTALLATION, EMPTY_LIST_WECOM_INSTALLATIONS_RESPONSE, ListLarkInstallationsResponseSchema, EMPTY_LIST_LARK_INSTALLATIONS_RESPONSE, AgentAppsResponseSchema, AppActivityResponseSchema, AppCatalogResponseSchema, AppConnectInitSchema, AppRequestListSchema, AppRequestSchema, AppRulesResponseSchema, AppSettingsResponseSchema, AppsOverviewSchema, EMPTY_APP_CATALOG, EMPTY_APPS_OVERVIEW, EMPTY_TEAM_APP_IMPACT, IssueAppCallsResponseSchema, PersonalAppImpactSchema, TeamAppImpactSchema, ListIssueWorkflowUsageResponseSchema, ListIssueWorkflowTemplatesResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE, EMPTY_LIST_ISSUE_WORKFLOW_USAGE_RESPONSE, IssueWorkflowSchema, IssueWorkflowDryRunResponseSchema, EMPTY_ISSUE_WORKFLOW, EMPTY_ISSUE_WORKFLOW_DRY_RUN, EMPTY_ISSUE_PULL_REQUESTS_RESPONSE, IssuePullRequestsResponseSchema, EMPTY_ISSUE_ENVIRONMENTS_RESPONSE, IssueEnvironmentsResponseSchema, IssueEnvironmentCommandSchema, EMPTY_ISSUE_PULL_REQUEST_SUMMARIES_RESPONSE, IssuePullRequestSummariesResponseSchema, GitHubConnectResponseSchema, ListGitHubInstallationsResponseSchema, ListGitHubRepositoriesResponseSchema, EMPTY_GITHUB_CONNECT_RESPONSE, EMPTY_LIST_GITHUB_INSTALLATIONS_RESPONSE, EMPTY_LIST_GITHUB_REPOSITORIES_RESPONSE, RuntimeQuickstartRequestSchema, MALFORMED_RUNTIME_QUICKSTART_REQUEST, QuotaRequestSchema, QuotaRequestListResponseSchema, EMPTY_QUOTA_REQUEST, EMPTY_QUOTA_REQUEST_LIST } from "./schemas";
import { ApiError, packPreviewFromResponse, workspaceHeader, type ApiClient, type ApiRequestInit, type LoginResponse } from "./client";
import { configStore } from "../config";
import type { RoomNonRequestKind, RoomWorkflowCode } from "../types/room-workflow";

function unverifiedDomain(domain: string): SSODomain {
  return { domain, txt_name: "", txt_value: "", verified_at: null };
}

function assertWebhookPayloadConditionsWriteSupported(data: Pick<UpdateAutopilotTriggerRequest, "event_filters">): void {
  if (data.event_filters?.some((filter) => filter.conditions?.length) && !configStore.getState().webhookPayloadConditionsSupported) {
    throw new Error("This server version does not support webhook payload conditions. Update the server before saving them.");
  }
}

function dingTalkGroupSearch(params: ListDingTalkGroupsParams): string {
  const search = new URLSearchParams();
  if (params.activity) search.set("activity", params.activity);
  if (params.installationId) search.set("installation_id", params.installationId);
  if (params.offset !== undefined) search.set("offset", String(params.offset));
  if (params.limit !== undefined) search.set("limit", String(params.limit));
  const encoded = search.toString();
  return encoded ? `?${encoded}` : "";
}

// The wire shapes of the room workflow endpoints (squad-room-workflow spec, section 6).
type RoomPolicyBody = { non_request_workflow: { kind: RoomNonRequestKind; workflow_id?: string | null } };

type RoomCheckBody = { needs_workflow: boolean; code?: RoomWorkflowCode | null; message?: string | null; squad_id?: string | null };

function toRoomWorkflowPolicy(body: RoomPolicyBody["non_request_workflow"]): RoomWorkflowPolicy {
  return { kind: body.kind, workflowId: body.kind === "workflow" ? (body.workflow_id ?? null) : null };
}

export abstract class ForkApiMethods {
  protected abstract baseUrl: string;
  abstract getToken(): string | null;
  protected abstract authHeaders(): Record<string, string>;
  protected abstract fetch<T>(path: string, init?: ApiRequestInit): Promise<T>;
  protected abstract fetchRaw(...args: Parameters<ApiClient["fetchRaw"]>): ReturnType<ApiClient["fetchRaw"]>;
  protected abstract hubItemPath(kind: HubItemKind, refId: string): string;
  protected abstract parseErrorMessage(res: Response, fallback: string): Promise<string>;
  protected abstract handleUnauthorized(credentialUsed: string | null): void;
  /**
   * First step of sign-in. Answers a session, or an MFA challenge when the
   * user has two-step sign-in. A malformed answer throws: a login must never
   * be silently accepted without a token and user.
   */
  async attemptLoginCode(email: string, code: string): Promise<LoginAttempt> {
    const raw = await this.fetch<unknown>("/auth/verify-code", {
      method: "POST",
      body: JSON.stringify({ email, code }),
    });
    const parsed = parseWithFallback<LoginAttempt | null>(raw, LoginAttemptSchema, null, {
      endpoint: "POST /auth/verify-code",
    });
    if (!parsed) throw new Error("Malformed response from POST /auth/verify-code");
    return parsed;
  }
  /** Second step: exchanges the pending MFA token and a TOTP or recovery code for a session. */
  async verifyMFALogin(mfaToken: string, code: string): Promise<LoginResponse> {
    const raw = await this.fetch<unknown>("/auth/mfa/verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, code }),
    });
    const parsed = parseWithFallback<LoginResponse | null>(raw, LoginResponseSchema, null, {
      endpoint: "POST /auth/mfa/verify",
    });
    if (!parsed) throw new Error("Malformed response from POST /auth/mfa/verify");
    return parsed;
  }
  /**
   * Sign the user out of every device. The response carries the caller's
   * replacement session, because the one this request used is revoked by it.
   */
  async revokeAllSessions(): Promise<RevokeAllSessionsResponse> {
    const raw = await this.fetch<unknown>("/api/me/sessions/revoke-all", {
      method: "POST",
    });
    return parseWithFallback<RevokeAllSessionsResponse>(
      raw,
      RevokeAllSessionsResponseSchema,
      EMPTY_REVOKE_ALL_SESSIONS_RESPONSE,
      { endpoint: "POST /api/me/sessions/revoke-all" },
    );
  }
  /** Record acceptance of legal documents; answers with what is still outstanding. */
  async acceptConsents(documents: AcceptConsentItem[]): Promise<AcceptConsentsResponse> {
    const raw = await this.fetch<unknown>("/api/me/consents", {
      method: "POST",
      body: JSON.stringify({ documents }),
    });
    return parseWithFallback<AcceptConsentsResponse>(
      raw,
      AcceptConsentsResponseSchema,
      EMPTY_ACCEPT_CONSENTS_RESPONSE,
      { endpoint: "POST /api/me/consents" },
    );
  }
  /**
   * What requesting deletion would do, without doing it. Needs a login from the
   * last 10 minutes (403 `recent_login_required`) and is refused with 409
   * `sole_owner` while the user owns a workspace that has other members.
   */
  async previewAccountDeletion(): Promise<AccountDeletionPreview> {
    const raw = await this.fetch<unknown>("/api/me/deletion?preview=1", { method: "POST" });
    return parseWithFallback<AccountDeletionPreview>(
      raw,
      AccountDeletionPreviewSchema,
      EMPTY_ACCOUNT_DELETION_PREVIEW,
      { endpoint: "POST /api/me/deletion?preview=1" },
    );
  }
  /**
   * Schedules the account for deletion in 14 days. The server ends every
   * session of the user, this one included, so the caller signs out after it.
   */
  async requestAccountDeletion(): Promise<AccountDeletionScheduled> {
    const raw = await this.fetch<unknown>("/api/me/deletion", { method: "POST" });
    return parseWithFallback<AccountDeletionScheduled>(
      raw,
      AccountDeletionScheduledSchema,
      EMPTY_ACCOUNT_DELETION_SCHEDULED,
      { endpoint: "POST /api/me/deletion" },
    );
  }
  /** Cancels a pending deletion. 409 `not_pending_deletion` when none is pending. */
  async cancelAccountDeletion(): Promise<AccountDeletionCanceled> {
    const raw = await this.fetch<unknown>("/api/me/deletion", { method: "DELETE" });
    return parseWithFallback<AccountDeletionCanceled>(
      raw,
      AccountDeletionCanceledSchema,
      EMPTY_ACCOUNT_DELETION_CANCELED,
      { endpoint: "DELETE /api/me/deletion" },
    );
  }
  /** The account export file. At most one per hour; a second call is a 429. */
  async exportAccount(): Promise<Blob> {
    const res = await this.fetchRaw("/api/me/export");
    return res.blob();
  }
  /** Owner/admin only. Newest first; `cursor` comes from the previous page. */
  async listWorkspaceAuditEvents(
    wsId: string,
    filters: AuditFilters = {},
    page: AuditPageParams = {},
  ): Promise<AuditEventPage> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${wsId}/audit-events${auditQueryString(filters, page)}`,
    );
    return parseWithFallback<AuditEventPage>(raw, AuditEventPageSchema, EMPTY_AUDIT_EVENT_PAGE, {
      endpoint: "GET /api/workspaces/{id}/audit-events",
    });
  }
  /** The CSV export, as text. `truncated` is set when the server cut it at its row cap. */
  async exportWorkspaceAuditCsv(
    wsId: string,
    filters: AuditFilters = {},
  ): Promise<{ text: string; truncated: boolean }> {
    const res = await this.fetchRaw(
      `/api/workspaces/${wsId}/audit-events${auditQueryString(filters, {}, "csv")}`,
    );
    return {
      text: await res.text(),
      truncated: res.headers.get("X-Audit-Truncated") === "true",
    };
  }
  /** The caller's own account-level events (sign-ins, token changes, ...). */
  async listMySecurityEvents(page: AuditPageParams = {}): Promise<AuditEventPage> {
    const raw = await this.fetch<unknown>(
      `/api/me/security-events${auditQueryString({}, page)}`,
    );
    return parseWithFallback<AuditEventPage>(raw, AuditEventPageSchema, EMPTY_AUDIT_EVENT_PAGE, {
      endpoint: "GET /api/me/security-events",
    });
  }
  /** Queues an issue behind `afterIssueIds`, or takes it out with queued false. */
  async setIssueRunQueue(id: string, queue: RunQueueSetting): Promise<Issue> {
    return this.fetch(`/api/issues/${id}/run-queue`, {
      method: "PUT",
      body: JSON.stringify({ queued: queue.queued, after_issue_ids: queue.afterIssueIds }),
    });
  }
  async batchSetRunQueue(ids: string[], queue: RunQueueSetting): Promise<{ updated: number }> {
    return this.fetch(`/api/issues/batch-run-queue`, {
      method: "POST",
      body: JSON.stringify({ issue_ids: ids, queued: queue.queued, after_issue_ids: queue.afterIssueIds }),
    });
  }
  async previewRunQueue(ids: string[], queue: RunQueueSetting): Promise<RunQueuePreview> {
    const raw = await this.fetch<unknown>(`/api/issues/run-queue/preview`, {
      method: "POST",
      body: JSON.stringify({ issue_ids: ids, queued: queue.queued, after_issue_ids: queue.afterIssueIds }),
    });
    return parseWithFallback(raw, RunQueuePreviewSchema, { release_now: 0, run_count: 0 }, { endpoint: "POST /api/issues/run-queue/preview" });
  }
  async listRunQueueWaits(): Promise<RunQueueWait[]> {
    const raw = await this.fetch<unknown>(`/api/issues/run-queue/waits`);
    return parseWithFallback(raw, RunQueueWaitSchema.array(), [] as RunQueueWait[], { endpoint: "GET /api/issues/run-queue/waits" });
  }
  async getIssueLimitUsage(): Promise<IssueLimitUsage | null> {
    const raw = await this.fetch<unknown>("/api/issues/limit-usage");
    if (raw == null) return null;
    return parseWithFallback<IssueLimitUsage | null>(
      raw,
      IssueLimitUsageSchema,
      null,
      { endpoint: "GET /api/issues/limit-usage" },
    );
  }
  /** An agent's instruction history, newest first (spec 4.6). */
  async listAgentInstructionRevisions(id: string): Promise<InstructionRevision[]> {
    const raw = await this.fetch<unknown>(`/api/agents/${encodeURIComponent(id)}/instruction-revisions`);
    return parseInstructionRevisions(raw);
  }
  /** Restores an earlier revision. The server answers with the new revision, which the caller does not read. */
  async restoreAgentInstructionRevision(id: string, revision: number): Promise<void> {
    await this.fetch<unknown>(
      `/api/agents/${encodeURIComponent(id)}/instruction-revisions/${revision}/restore`,
      { method: "POST" },
    );
  }
  // The workflow chosen for issues that are not requests of a flow room (spec
  // section 4). A response that cannot be read is the server default: no_action.
  async getRoomWorkflowPolicy(workspaceId: string): Promise<RoomWorkflowPolicy> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/room-workflow`);
    const parsed = parseWithFallback<RoomPolicyBody>(raw, RoomWorkflowPolicyResponseSchema, { non_request_workflow: { kind: "no_action" } }, {
      endpoint: "GET /api/workspaces/:id/room-workflow",
    });
    return toRoomWorkflowPolicy(parsed.non_request_workflow);
  }
  // Admins only; the server refuses a member's write.
  async setRoomWorkflowPolicy(workspaceId: string, policy: RoomWorkflowPolicy): Promise<RoomWorkflowPolicy> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/room-workflow`, {
      method: "PUT",
      body: JSON.stringify({
        non_request_workflow: { kind: policy.kind, workflow_id: policy.workflowId ?? undefined },
      }),
    });
    const sent: RoomPolicyBody = { non_request_workflow: { kind: policy.kind, workflow_id: policy.workflowId } };
    const parsed = parseWithFallback<RoomPolicyBody>(raw, RoomWorkflowPolicyResponseSchema, sent, {
      endpoint: "PUT /api/workspaces/:id/room-workflow",
    });
    return toRoomWorkflowPolicy(parsed.non_request_workflow);
  }
  // Previews the room rules for a write the form is about to send. A response that
  // cannot be read says the write needs nothing, and the server still checks it.
  async checkRoomWorkflow(workspaceId: string, params: RoomWorkflowCheckParams): Promise<RoomWorkflowCheck> {
    const query = new URLSearchParams();
    if (params.projectId) query.set("project_id", params.projectId);
    if (params.assigneeType) query.set("assignee_type", params.assigneeType);
    if (params.assigneeId) query.set("assignee_id", params.assigneeId);
    if (params.parentIssueId) query.set("parent_issue_id", params.parentIssueId);
    const suffix = query.toString() ? `?${query.toString()}` : "";
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/room-workflow/check${suffix}`);
    const parsed = parseWithFallback<RoomCheckBody>(raw, RoomWorkflowCheckSchema, { needs_workflow: false }, {
      endpoint: "GET /api/workspaces/:id/room-workflow/check",
    });
    return {
      needsWorkflow: parsed.needs_workflow,
      code: parsed.code ?? null,
      message: parsed.message ?? "",
      squadId: parsed.squad_id ?? null,
    };
  }
  /** The current period of a pay as you go plan; null when the body does not match. */
  async getWorkspaceSubscriptionUsage(): Promise<WorkspaceSubscriptionUsage | null> {
    const raw = await this.fetch<unknown>("/api/cloud-subscriptions/usage");
    return parseWithFallback<WorkspaceSubscriptionUsage | null>(
      raw,
      WorkspaceSubscriptionUsageSchema,
      null,
      { endpoint: "GET /api/cloud-subscriptions/usage" },
    );
  }
  /** The day and meter lines of a usage invoice; null when the body does not match. */
  async getWorkspaceOrderUsage(
    wsId: string,
    orderId: string,
  ): Promise<WorkspaceOrderUsage | null> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${wsId}/orders/${orderId}/usage`,
    );
    return parseWithFallback<WorkspaceOrderUsage | null>(
      raw,
      WorkspaceOrderUsageSchema,
      null,
      { endpoint: "GET /api/workspaces/{id}/orders/{orderId}/usage" },
    );
  }
  private async submitWorkspaceOrderBuyerUpdate(
    wsId: string,
    orderId: string,
    subpath: "buyer" | "invoice-buyer",
    buyer: WorkspaceOrderBuyer,
  ): Promise<WorkspaceOrder | null> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${wsId}/orders/${orderId}/${subpath}`,
      {
        method: "PUT",
        body: JSON.stringify({
          name: buyer.name,
          tax_code: buyer.taxCode,
          address: buyer.address,
          invoice_email: buyer.invoiceEmail,
        }),
      },
    );
    return parseWithFallback<WorkspaceOrder | null>(
      raw,
      WorkspaceOrderSchema,
      null,
      { endpoint: `PUT /api/workspaces/{id}/orders/{orderId}/${subpath}` },
    );
  }
  /** Replaces the invoice details of a pending order; 409 `order_not_pending` otherwise. */
  async updateWorkspaceOrderBuyer(
    wsId: string,
    orderId: string,
    buyer: WorkspaceOrderBuyer,
  ): Promise<WorkspaceOrder | null> {
    return this.submitWorkspaceOrderBuyerUpdate(wsId, orderId, "buyer", buyer);
  }
  /** Updates invoice buyer details for an order whose e-invoice needs info or failed. */
  async updateWorkspaceOrderInvoiceBuyer(
    wsId: string,
    orderId: string,
    buyer: WorkspaceOrderBuyer,
  ): Promise<WorkspaceOrder | null> {
    return this.submitWorkspaceOrderBuyerUpdate(
      wsId,
      orderId,
      "invoice-buyer",
      buyer,
    );
  }
  // Workspace quota requests (R6 S32).

  async listQuotaRequests(wsId: string): Promise<QuotaRequest[]> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/quota-requests`);
    return parseWithFallback<QuotaRequestListResponse>(
      raw,
      QuotaRequestListResponseSchema,
      EMPTY_QUOTA_REQUEST_LIST,
      { endpoint: "GET /api/workspaces/{id}/quota-requests" },
    ).requests;
  }
  async createQuotaRequest(
    wsId: string,
    input: CreateQuotaRequestInput,
  ): Promise<QuotaRequest> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/quota-requests`, {
      method: "POST",
      body: JSON.stringify(input),
    });
    return parseWithFallback<QuotaRequest>(
      raw,
      QuotaRequestSchema,
      EMPTY_QUOTA_REQUEST,
      { endpoint: "POST /api/workspaces/{id}/quota-requests" },
    );
  }
  // Single sign-on (platform spec 6.3). A body that does not match the contract
  // reads as "no SSO" or "no connection", so the sign-in page keeps working.

  /** Asks whether an email signs in through SSO. Any failure answers "no". */
  async discoverSSO(email: string): Promise<SSODiscovery> {
    const raw = await this.fetch<unknown>("/auth/sso/discover", {
      method: "POST",
      body: JSON.stringify({ email }),
    }).catch(() => null);
    return parseWithFallback<SSODiscovery>(raw, SSODiscoverySchema, NO_SSO, {
      endpoint: "POST /auth/sso/discover",
    });
  }
  /** The workspace's connection, or null when it has none. */
  async getSSOConnection(wsId: string): Promise<SSOConnection | null> {
    let raw: unknown;
    try {
      raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/sso/connection`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
    return parseWithFallback<SSOConnection | null>(raw, SSOConnectionSchema, null, {
      endpoint: "GET /api/workspaces/{id}/sso/connection",
    });
  }
  /** Creates or updates the connection. Errors carry a `code` such as `domain_not_verified`. */
  async putSSOConnection(wsId: string, input: SSOConnectionInput): Promise<SSOConnection | null> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/sso/connection`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
    return parseWithFallback<SSOConnection | null>(raw, SSOConnectionSchema, null, {
      endpoint: "PUT /api/workspaces/{id}/sso/connection",
    });
  }
  async deleteSSOConnection(wsId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${wsId}/sso/connection`, { method: "DELETE" });
  }
  async listSSODomains(wsId: string): Promise<SSODomainList> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/sso/domains`);
    return parseWithFallback<SSODomainList>(raw, SSODomainListSchema, NO_SSO_DOMAINS, {
      endpoint: "GET /api/workspaces/{id}/sso/domains",
    });
  }
  /** A malformed answer reads as an unverified claim with no TXT record to show. */
  async addSSODomain(wsId: string, domain: string): Promise<SSODomain> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/sso/domains`, {
      method: "POST",
      body: JSON.stringify({ domain }),
    });
    return parseWithFallback<SSODomain>(raw, SSODomainSchema, unverifiedDomain(domain), {
      endpoint: "POST /api/workspaces/{id}/sso/domains",
    });
  }
  /** Errors carry a `code` such as `domain_not_verified`. */
  async verifySSODomain(wsId: string, domain: string): Promise<SSODomain> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${wsId}/sso/domains/${encodeURIComponent(domain)}/verify`,
      { method: "POST" },
    );
    return parseWithFallback<SSODomain>(raw, SSODomainSchema, unverifiedDomain(domain), {
      endpoint: "POST /api/workspaces/{id}/sso/domains/{domain}/verify",
    });
  }
  async removeSSODomain(wsId: string, domain: string): Promise<void> {
    await this.fetch(`/api/workspaces/${wsId}/sso/domains/${encodeURIComponent(domain)}`, {
      method: "DELETE",
    });
  }
  // Workspace policy and MFA (platform spec R3). Errors keep the API `code` on
  // `ApiError.body` (invite_domain_not_allowed, plan_feature_unavailable,
  // mfa_already_enabled).

  async getWorkspacePolicy(wsId: string): Promise<WorkspacePolicy> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/policy`);
    return parseWithFallback<WorkspacePolicy>(raw, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY, {
      endpoint: "GET /api/workspaces/{id}/policy",
    });
  }
  async updateWorkspacePolicy(wsId: string, input: WorkspacePolicyInput): Promise<WorkspacePolicy> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${wsId}/policy`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
    return parseWithFallback<WorkspacePolicy>(raw, WorkspacePolicySchema, DEFAULT_WORKSPACE_POLICY, {
      endpoint: "PUT /api/workspaces/{id}/policy",
    });
  }
  async enrollMFA(): Promise<MFAEnrollResponse> {
    const raw = await this.fetch<unknown>("/api/me/mfa/enroll", { method: "POST" });
    // Without the secret there is nothing to scan, so a malformed answer is an error.
    const parsed = parseWithFallback<MFAEnrollResponse | null>(raw, MFAEnrollResponseSchema, null, {
      endpoint: "POST /api/me/mfa/enroll",
    });
    if (!parsed) throw new Error("The server did not return a multi-factor secret");
    return parsed;
  }
  async confirmMFA(code: string): Promise<MFAConfirmResponse> {
    const raw = await this.fetch<unknown>("/api/me/mfa/confirm", {
      method: "POST",
      body: JSON.stringify({ code }),
    });
    // The codes are shown once and cannot be fetched again: an answer without
    // them is an error, never an empty list the user would confirm past.
    const parsed = parseWithFallback<MFAConfirmResponse | null>(raw, MFAConfirmResponseSchema, null, {
      endpoint: "POST /api/me/mfa/confirm",
    });
    if (!parsed) throw new Error("The server confirmed two-step sign-in but returned no recovery codes");
    return parsed;
  }
  /** `code` is a TOTP code or a recovery code. */
  async disableMFA(code: string): Promise<void> {
    await this.fetch("/api/me/mfa/disable", {
      method: "POST",
      body: JSON.stringify({ code }),
    });
  }
  async purchaseWorkspaceSeats(
    data: PurchaseWorkspaceSeatsRequest,
  ): Promise<PurchaseWorkspaceSeatsResponse | null> {
    const res = await this.fetchRaw(
      "/api/cloud-subscriptions/seats/purchases",
      {
        method: "POST",
        body: JSON.stringify({
          additional_seats: data.additionalSeats,
          expected_current_seats: data.expectedCurrentSeats,
          expected_purchase_version: data.expectedPurchaseVersion,
          accepted_proration_amount: data.acceptedProrationAmount,
          currency: data.currency,
          idempotency_key: data.idempotencyKey,
        }),
        extraHeaders: {
          "Content-Type": "application/json",
          "Idempotency-Key": data.idempotencyKey,
        },
      },
    );
    const raw = (await res.json()) as unknown;
    return parseWithFallback<PurchaseWorkspaceSeatsResponse | null>(
      raw,
      PurchaseWorkspaceSeatsResponseSchema,
      null,
      { endpoint: "POST /api/cloud-subscriptions/seats/purchases" },
    );
  }
  async cancelWorkspaceScheduledPlanChange(idempotencyKey: string): Promise<WorkspacePendingPlanChangeCleared | null> {
    const res = await this.fetchRaw("/api/cloud-subscriptions/plan-changes/scheduled", {
      method: "DELETE",
      extraHeaders: { "Idempotency-Key": idempotencyKey },
    });
    const raw = (await res.json()) as unknown;
    return parseWithFallback<WorkspacePendingPlanChangeCleared | null>(
      raw,
      ClearWorkspaceScheduledPlanChangeResponseSchema,
      null,
      { endpoint: "DELETE /api/cloud-subscriptions/plan-changes/scheduled" },
    );
  }
  async cancelWorkspaceSubscription(idempotencyKey: string): Promise<WorkspaceSubscriptionCancelState | null> {
    const res = await this.fetchRaw("/api/cloud-subscriptions/cancel", {
      method: "POST",
      extraHeaders: { "Idempotency-Key": idempotencyKey },
    });
    const raw = (await res.json()) as unknown;
    return parseWithFallback<WorkspaceSubscriptionCancelState | null>(
      raw,
      WorkspaceSubscriptionCancelStateSchema,
      null,
      { endpoint: "POST /api/cloud-subscriptions/cancel" },
    );
  }
  async resumeWorkspaceSubscription(idempotencyKey: string): Promise<WorkspaceSubscriptionCancelState | null> {
    const res = await this.fetchRaw("/api/cloud-subscriptions/resume", {
      method: "POST",
      extraHeaders: { "Idempotency-Key": idempotencyKey },
    });
    const raw = (await res.json()) as unknown;
    return parseWithFallback<WorkspaceSubscriptionCancelState | null>(
      raw,
      WorkspaceSubscriptionCancelStateSchema,
      null,
      { endpoint: "POST /api/cloud-subscriptions/resume" },
    );
  }
  async initiateRuntimeQuickstart(
    runtimeId: string,
  ): Promise<RuntimeQuickstartRequest> {
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/quickstart`,
      { method: "POST" },
    );
    return parseWithFallback<RuntimeQuickstartRequest>(
      raw,
      RuntimeQuickstartRequestSchema,
      { ...MALFORMED_RUNTIME_QUICKSTART_REQUEST, runtime_id: runtimeId },
      { endpoint: "POST /api/runtimes/{id}/quickstart" },
    );
  }
  async getRuntimeQuickstart(
    runtimeId: string,
    requestId: string,
  ): Promise<RuntimeQuickstartRequest> {
    const raw = await this.fetch<unknown>(
      `/api/runtimes/${runtimeId}/quickstart/${requestId}`,
    );
    return parseWithFallback<RuntimeQuickstartRequest>(
      raw,
      RuntimeQuickstartRequestSchema,
      {
        ...MALFORMED_RUNTIME_QUICKSTART_REQUEST,
        id: requestId,
        runtime_id: runtimeId,
      },
      { endpoint: "GET /api/runtimes/{id}/quickstart/{requestId}" },
    );
  }
  async getWorkspace(id: string): Promise<Workspace> {
    return this.fetch(`/api/workspaces/${id}`);
  }
  // ---- Account groups (platform spec 7b.2) ----

  /** The fixed permission keys. Empty when the call fails, so nothing is granted. */
  async getPermissionCatalogue(workspaceId: string): Promise<string[]> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/permission-catalogue`);
    return parseWithFallback(raw, PermissionCatalogueSchema, { permissions: [] }, {
      endpoint: "GET /api/workspaces/{id}/permission-catalogue",
    }).permissions;
  }
  /** The caller's role and effective keys. An empty grant when the call fails. */
  async getMyPermissions(workspaceId: string): Promise<MyPermissions> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/permissions/me`);
    return parseWithFallback<MyPermissions>(raw, MyPermissionsSchema, { role: "", permissions: [] }, {
      endpoint: "GET /api/workspaces/{id}/permissions/me",
    });
  }
  async listMemberGroups(workspaceId: string): Promise<MemberGroup[]> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups`);
    return parseWithFallback<MemberGroup[]>(raw, MemberGroupListSchema, [], {
      endpoint: "GET /api/workspaces/{id}/groups",
    });
  }
  async getMemberGroup(workspaceId: string, groupId: string): Promise<MemberGroupDetail> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups/${groupId}`);
    return parseWithFallback<MemberGroupDetail>(raw, MemberGroupDetailSchema, { ...emptyGroup(), members: [] }, {
      endpoint: "GET /api/workspaces/{id}/groups/{groupId}",
    });
  }
  async createMemberGroup(workspaceId: string, data: { name: string; description?: string }): Promise<MemberGroup> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback<MemberGroup>(raw, MemberGroupSchema, emptyGroup(), {
      endpoint: "POST /api/workspaces/{id}/groups",
    });
  }
  async updateMemberGroup(
    workspaceId: string,
    groupId: string,
    data: { name?: string; description?: string },
  ): Promise<MemberGroup> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups/${groupId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
    return parseWithFallback<MemberGroup>(raw, MemberGroupSchema, emptyGroup(), {
      endpoint: "PATCH /api/workspaces/{id}/groups/{groupId}",
    });
  }
  async deleteMemberGroup(workspaceId: string, groupId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/groups/${groupId}`, { method: "DELETE" });
  }
  /** Replaces the keys of a group. The server returns the group with its new keys. */
  async setMemberGroupPermissions(workspaceId: string, groupId: string, permissions: string[]): Promise<MemberGroup> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups/${groupId}/permissions`, {
      method: "PUT",
      body: JSON.stringify({ permissions }),
    });
    return parseWithFallback<MemberGroup>(raw, MemberGroupSchema, emptyGroup(), {
      endpoint: "PUT /api/workspaces/{id}/groups/{groupId}/permissions",
    });
  }
  /** Adds people to a group (100 at most per call). Returns the ids the server added. */
  async addMemberGroupMembers(workspaceId: string, groupId: string, userIds: string[]): Promise<string[]> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/groups/${groupId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_ids: userIds }),
    });
    return parseWithFallback(raw, AddMemberGroupMembersResponseSchema, { added: [] }, {
      endpoint: "POST /api/workspaces/{id}/groups/{groupId}/members",
    }).added;
  }
  async removeMemberGroupMember(workspaceId: string, groupId: string, userId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/groups/${groupId}/members/${userId}`, { method: "DELETE" });
  }
  async listAgentSkills(agentId: string): Promise<SkillSummary[]> {
    return this.fetch(`/api/agents/${agentId}/skills`);
  }
  /** Where each workflow is assigned: its projects, squads and agents (read-only). */
  async listIssueWorkflowUsage(): Promise<ListIssueWorkflowUsageResponse> {
    const raw = await this.fetch<unknown>(`/api/issue-workflows/usage`);
    return parseWithFallback(raw, ListIssueWorkflowUsageResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_USAGE_RESPONSE, {
      endpoint: "GET /api/issue-workflows/usage",
    });
  }
  /** System templates a new workflow can start from. Unknown template ids are dropped. */
  async listIssueWorkflowTemplates(): Promise<ListIssueWorkflowTemplatesResponse> {
    const raw = await this.fetch<unknown>(`/api/issue-workflows/templates`);
    return parseWithFallback(raw, ListIssueWorkflowTemplatesResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE, {
      endpoint: "GET /api/issue-workflows/templates",
    });
  }
  async getIssueWorkflow(id: string): Promise<IssueWorkflow> {
    const raw = await this.fetch<unknown>(`/api/issue-workflows/${id}`);
    return parseWithFallback(raw, IssueWorkflowSchema, EMPTY_ISSUE_WORKFLOW, {
      endpoint: "GET /api/issue-workflows/{id}",
    });
  }
  async dryRunIssueWorkflowUpdate(id: string, data: IssueWorkflowWriteRequest): Promise<IssueWorkflowDryRunResponse> {
    const raw = await this.fetch<unknown>(`/api/issue-workflows/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ ...data, dry_run: true }),
    });
    return parseWithFallback(raw, IssueWorkflowDryRunResponseSchema, EMPTY_ISSUE_WORKFLOW_DRY_RUN, {
      endpoint: "PATCH /api/issue-workflows/{id} (dry run)",
    });
  }
  /** Gets one flow draft with its document. */
  async getFlowDraft(squadId: string, draftId: string): Promise<FlowDraft> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/flow-drafts/${draftId}`);
    return parseWithFallback(raw, FlowDraftSchema, EMPTY_FLOW_DRAFT, {
      endpoint: "GET /api/squads/{id}/flow-drafts/{draftId}",
    });
  }
  /** Reads a zip the person uploads. The JSON Content-Type is left off so the browser sets the boundary. */
  async previewPackUpload(file: File): Promise<PackPreview> {
    const formData = new FormData();
    formData.append("file", file, file.name || "upload.zip");
    const res = await this.fetchRaw("/api/squads/packs/preview", { method: "POST", body: formData });
    return packPreviewFromResponse(await res.json());
  }
  /** Completes a lead step in a flow. */
  async completeIssueFlowStep(issueId: string, step: string, data: CompleteIssueFlowStepRequest): Promise<IssueFlowState> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/flow/steps/${step}/complete`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, IssueFlowStateSchema, EMPTY_ISSUE_FLOW_STATE, {
      endpoint: "POST /api/issues/{id}/flow/steps/{step}/complete",
    });
  }
  /** Sets a flow variable on a request or on an open task. */
  async setIssueFlowVar(issueIdOrTaskId: string, key: string, value: unknown): Promise<IssueFlowState> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueIdOrTaskId}/flow/vars/${key}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    });
    return parseWithFallback(raw, IssueFlowStateSchema, EMPTY_ISSUE_FLOW_STATE, {
      endpoint: "PUT /api/issues/{id}/flow/vars/{key}",
    });
  }
  /** Chooses an exit edge of a manual flow step. */
  async exitIssueFlow(issueId: string, edgeId: string): Promise<IssueFlowState> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/flow/exits/${edgeId}`, {
      method: "POST",
    });
    return parseWithFallback(raw, IssueFlowStateSchema, EMPTY_ISSUE_FLOW_STATE, {
      endpoint: "POST /api/issues/{id}/flow/exits/{edgeId}",
    });
  }
  /** Closes an open task on behalf of its assignee. Squad managers only. */
  async closeIssueFlowTask(taskId: string, data: { verdict?: string; note?: string }): Promise<{ task: IssueFlowTask; state: IssueFlowState }> {
    const raw = await this.fetch<unknown>(`/api/issues/${taskId}/flow/close`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CloseIssueFlowTaskResponseSchema, EMPTY_CLOSE_ISSUE_FLOW_TASK_RESPONSE, {
      endpoint: "POST /api/issues/{taskId}/flow/close",
    });
  }
  /** Stateless manifest validation (members and agents). */
  async validateFlowStateless(data: { manifest: unknown; squad_id?: string }): Promise<FlowValidationResponse> {
    const raw = await this.fetch<unknown>("/api/flow/validate", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, FlowValidationResponseSchema, EMPTY_FLOW_VALIDATION_RESPONSE, {
      endpoint: "POST /api/flow/validate",
    });
  }
  async createSquad(data: {
    name: string;
    description?: string;
    leader_id: string;
    avatar_url?: string;
    floor_name?: string;
    template_key?: string;
  }): Promise<Squad> {
    const raw = await this.fetch<unknown>("/api/squads", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, SquadSchema, EMPTY_SQUAD, {
      endpoint: "POST /api/squads",
    }) as Squad;
  }
  async updateSquad(id: string, data: { name?: string; description?: string; instructions?: string; leader_id?: string; avatar_url?: string; handoff_mode?: SquadHandoffMode }): Promise<Squad> {
    const raw = await this.fetch<unknown>(`/api/squads/${id}`, { method: "PUT", body: JSON.stringify(data) });
    return parseWithFallback(raw, SquadSchema, EMPTY_SQUAD, {
      endpoint: "PUT /api/squads/:id",
    }) as Squad;
  }
  async deleteSquad(id: string): Promise<void> {
    await this.fetch(`/api/squads/${id}`, { method: "DELETE" });
  }
  async listSquadMembers(squadId: string): Promise<SquadMember[]> {
    return this.fetch(`/api/squads/${squadId}/members`);
  }
  async addSquadMember(squadId: string, data: { member_type: string; member_id: string; role?: string }): Promise<SquadMember> {
    return this.fetch(`/api/squads/${squadId}/members`, { method: "POST", body: JSON.stringify(data) });
  }
  async removeSquadMember(squadId: string, data: { member_type: string; member_id: string }): Promise<void> {
    await this.fetch(`/api/squads/${squadId}/members`, { method: "DELETE", body: JSON.stringify(data) });
  }
  async updateSquadMemberRole(squadId: string, data: { member_type: string; member_id: string; role: string }): Promise<SquadMember> {
    return this.fetch(`/api/squads/${squadId}/members/role`, { method: "PATCH", body: JSON.stringify(data) });
  }
  async listSquadSkills(squadId: string): Promise<SquadSkill[]> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/skills`);
    return parseWithFallback(raw, SquadSkillListSchema, EMPTY_SQUAD_SKILL_LIST, {
      endpoint: "GET /api/squads/:id/skills",
    }) as SquadSkill[];
  }
  async setSquadSkills(squadId: string, skillIds: string[]): Promise<SquadSkill[]> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/skills`, {
      method: "PUT",
      body: JSON.stringify({ skill_ids: skillIds }),
    });
    return parseWithFallback(raw, SquadSkillListSchema, EMPTY_SQUAD_SKILL_LIST, {
      endpoint: "PUT /api/squads/:id/skills",
    }) as SquadSkill[];
  }
  async listSquadPlugins(squadId: string): Promise<SquadPlugin[]> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/plugins`);
    return parseWithFallback(raw, SquadPluginListSchema, EMPTY_SQUAD_PLUGIN_LIST, {
      endpoint: "GET /api/squads/:id/plugins",
    }) as SquadPlugin[];
  }
  // The plugins of the team's quickstart bundle, offered in the squad picker.
  async listSquadPluginCatalog(): Promise<SquadPlugin[]> {
    const raw = await this.fetch<unknown>(`/api/squads/plugin-catalog`);
    return parseWithFallback(raw, SquadPluginListSchema, EMPTY_SQUAD_PLUGIN_LIST, {
      endpoint: "GET /api/squads/plugin-catalog",
    }) as SquadPlugin[];
  }
  // Points every live agent member at one runtime in one request. The
  // response says which agents moved, which were already there, and which were
  // skipped with a reason.
  async setSquadRuntime(squadId: string, runtimeId: string): Promise<SetSquadRuntimeResponse> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/runtime`, {
      method: "PUT",
      body: JSON.stringify({ runtime_id: runtimeId }),
    });
    return parseWithFallback(raw, SetSquadRuntimeResponseSchema, EMPTY_SET_SQUAD_RUNTIME_RESPONSE, {
      endpoint: "PUT /api/squads/:id/runtime",
    }) as SetSquadRuntimeResponse;
  }
  async setSquadPlugins(squadId: string, plugins: SquadPlugin[]): Promise<SquadPlugin[]> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/plugins`, {
      method: "PUT",
      body: JSON.stringify({ plugins }),
    });
    return parseWithFallback(raw, SquadPluginListSchema, EMPTY_SQUAD_PLUGIN_LIST, {
      endpoint: "PUT /api/squads/:id/plugins",
    }) as SquadPlugin[];
  }
  // The org map: floors, squads, leaders and open work (any member may read it).
  async getCompanyOrgMap(params?: { floorId?: string }): Promise<OrgMap> {
    const search = new URLSearchParams();
    if (params?.floorId) search.set("floor_id", params.floorId);
    const qs = search.toString();
    const raw = await this.fetch<unknown>(`/api/company/org-map${qs ? `?${qs}` : ""}`);
    return parseWithFallback(raw, OrgMapSchema, EMPTY_ORG_MAP, { endpoint: "GET /api/company/org-map" });
  }
  // Company floors and squad seats (the digital company).
  async listCompanyFloors(): Promise<CompanyFloor[]> {
    const raw = await this.fetch<unknown>("/api/company/floors");
    return parseWithFallback(raw, CompanyFloorListSchema, EMPTY_COMPANY_FLOOR_LIST, {
      endpoint: "GET /api/company/floors",
    }) as CompanyFloor[];
  }
  async createCompanyFloor(data: CreateCompanyFloorRequest): Promise<CompanyFloor> {
    const raw = await this.fetch<unknown>("/api/company/floors", { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, CompanyFloorSchema, EMPTY_COMPANY_FLOOR, {
      endpoint: "POST /api/company/floors",
    }) as CompanyFloor;
  }
  async updateCompanyFloor(id: string, data: UpdateCompanyFloorRequest): Promise<CompanyFloor> {
    const raw = await this.fetch<unknown>(`/api/company/floors/${id}`, { method: "PUT", body: JSON.stringify(data) });
    return parseWithFallback(raw, CompanyFloorSchema, EMPTY_COMPANY_FLOOR, {
      endpoint: "PUT /api/company/floors/:id",
    }) as CompanyFloor;
  }
  /** Deletes the floor with its rooms and chief. Squads seated on it are unseated, not deleted. */
  async deleteCompanyFloor(id: string): Promise<void> {
    await this.fetch(`/api/company/floors/${id}`, { method: "DELETE" });
  }
  /**
   * Applies seat moves in one transaction (a swap is two entries). A taken seat
   * is a 409 whose `ApiError.body` names the holder; see `parseCompanySeatConflict`.
   */
  /**
   * Gives a project to a squad: the project takes the SDLC Auto workflow and the
   * squad's previous project is unassigned. A 409 `workflow_status_mapping_required`
   * (see `parseSquadProjectMapping`) changed nothing; retry with `status_mapping`.
   */
  async assignSquadProject(squadId: string, data: AssignSquadProjectRequest): Promise<AssignSquadProjectResponse> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/project`, { method: "POST", body: JSON.stringify(data) });
    return parseWithFallback(raw, AssignSquadProjectResponseSchema, EMPTY_ASSIGN_SQUAD_PROJECT_RESPONSE, {
      endpoint: "POST /api/squads/:id/project",
    }) as AssignSquadProjectResponse;
  }
  async unassignSquadProject(squadId: string): Promise<void> {
    await this.fetch(`/api/squads/${squadId}/project`, { method: "DELETE" });
  }
  async setCompanySeats(seats: CompanySeatAssignment[]): Promise<CompanySeatsResponse> {
    const raw = await this.fetch<unknown>("/api/company/seats", { method: "PUT", body: JSON.stringify(seats) });
    return parseWithFallback(raw, CompanySeatsResponseSchema, EMPTY_COMPANY_SEATS_RESPONSE, {
      endpoint: "PUT /api/company/seats",
    }) as CompanySeatsResponse;
  }
  // The caller's chat push for the daily brief (spec 2.7). Member-only: an agent
  // is refused by the server.
  async getBriefPushPreference(): Promise<BriefPushPreference> {
    const raw = await this.fetch<unknown>("/api/company/brief-push");
    return parseWithFallback(raw, BriefPushPreferenceSchema, EMPTY_BRIEF_PUSH, {
      endpoint: "GET /api/company/brief-push",
    });
  }
  async updateBriefPushPreference(data: UpdateBriefPushRequest): Promise<BriefPushPreference> {
    const raw = await this.fetch<unknown>("/api/company/brief-push", {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, BriefPushPreferenceSchema, EMPTY_BRIEF_PUSH, {
      endpoint: "PUT /api/company/brief-push",
    });
  }
  // The caller's own chats in this workspace that can take brief pushes. Empty
  // until a chat is linked.
  async listBriefPushBindings(): Promise<BriefPushBinding[]> {
    const raw = await this.fetch<unknown>("/api/company/brief-push/bindings");
    return parseWithFallback<BriefPushBinding[]>(raw, BriefPushBindingListSchema, [], {
      endpoint: "GET /api/company/brief-push/bindings",
    });
  }
  // A member rates a brief; a second rating replaces the first. The read returns
  // the caller's own rating, or null when there is none.
  async submitBriefFeedback(briefId: string, data: SubmitBriefFeedbackRequest): Promise<BriefFeedback | null> {
    const raw = await this.fetch<unknown>(`/api/briefs/${encodeURIComponent(briefId)}/feedback`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback<BriefFeedback | null>(raw, BriefFeedbackOrNoneSchema, null, {
      endpoint: "POST /api/briefs/:id/feedback",
    });
  }
  async getMyBriefFeedback(briefId: string): Promise<BriefFeedback | null> {
    const raw = await this.fetch<unknown>(`/api/briefs/${encodeURIComponent(briefId)}/feedback`);
    return parseWithFallback<BriefFeedback | null>(raw, BriefFeedbackOrNoneSchema, null, {
      endpoint: "GET /api/briefs/:id/feedback",
    });
  }
  // The workspace's usefulness figure. Owners and admins only; other callers get
  // a 403 ApiError, which the caller treats as "not shown".
  async getBriefFeedbackSummary(): Promise<BriefFeedbackSummary | null> {
    const raw = await this.fetch<unknown>("/api/briefs/feedback/summary");
    return parseWithFallback<BriefFeedbackSummary | null>(raw, BriefFeedbackSummarySchema, null, {
      endpoint: "GET /api/briefs/feedback/summary",
    });
  }
  // A squad's adaptive limits and the reason they are what they are (spec 2.6).
  async getSquadAttention(squadId: string): Promise<SquadAttention | null> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/attention`);
    return parseWithFallback<SquadAttention | null>(raw, SquadAttentionSchema, null, {
      endpoint: "GET /api/squads/:id/attention",
    });
  }
  // Pins or clears a squad's limits (owner or admin).
  async setSquadAttentionPin(squadId: string, data: PinSquadAttentionRequest): Promise<SquadAttention | null> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/attention/pin`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback<SquadAttention | null>(raw, SquadAttentionSchema, null, {
      endpoint: "PUT /api/squads/:id/attention/pin",
    });
  }
  // Squad charter and routines. A refused charter save is a 409 whose body
  // carries the current revision; see `parseCharterRevisionConflict`.
  async getSquadCharter(squadId: string): Promise<SquadCharter> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/charter`);
    return parseWithFallback(raw, SquadCharterSchema, emptySquadCharter(squadId), {
      endpoint: "GET /api/squads/:id/charter",
    });
  }
  async updateSquadCharter(squadId: string, data: UpdateSquadCharterRequest): Promise<SquadCharter> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/charter`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, SquadCharterSchema, emptySquadCharter(squadId), {
      endpoint: "PUT /api/squads/:id/charter",
    });
  }
  async listSquadRoutines(squadId: string): Promise<SquadRoutine[]> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/routines`);
    return parseWithFallback(raw, SquadRoutineListSchema, [] as SquadRoutine[], {
      endpoint: "GET /api/squads/:id/routines",
    });
  }
  async createSquadRoutine(squadId: string, data: CreateSquadRoutineRequest): Promise<SquadRoutine> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/routines`, {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, SquadRoutineSchema, EMPTY_SQUAD_ROUTINE, {
      endpoint: "POST /api/squads/:id/routines",
    });
  }
  /** Unlinks a routine; `pause` also pauses its autopilot. `autopilotId` is the routine id. */
  async deleteSquadRoutine(squadId: string, autopilotId: string, options: { pause: boolean }): Promise<void> {
    const query = options.pause ? "?pause=true" : "";
    await this.fetch(`/api/squads/${squadId}/routines/${autopilotId}${query}`, { method: "DELETE" });
  }
  // Per-squad members status snapshot: one row per member with derived
  // working/idle/offline/unstable plus the issues each agent is currently
  // running. Parsed with a lenient schema so a new server-side status
  // value or extra field can't white-screen the Squad page (#2143).
  async getSquadMemberStatus(squadId: string): Promise<SquadMemberStatusListResponse> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/members/status`);
    return parseWithFallback(raw, SquadMemberStatusListResponseSchema, EMPTY_SQUAD_MEMBER_STATUS_LIST, {
      endpoint: "GET /api/squads/:id/members/status",
    }) as SquadMemberStatusListResponse;
  }
  // Autopilots
  async listAutopilots(params?: { status?: string }): Promise<ListAutopilotsResponse> {
    const search = new URLSearchParams();
    if (params?.status) search.set("status", params.status);
    const raw = await this.fetch<unknown>(`/api/autopilots?${search}`);
    return parseWithFallback(
      raw,
      ListAutopilotsResponseSchema,
      EMPTY_LIST_AUTOPILOTS_RESPONSE as ListAutopilotsResponse,
      { endpoint: "GET /api/autopilots" },
    );
  }
  async getAutopilot(id: string): Promise<GetAutopilotResponse> {
    return this.fetch(`/api/autopilots/${id}`);
  }
  async createAutopilot(data: CreateAutopilotRequest): Promise<Autopilot> {
    return this.fetch("/api/autopilots", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }
  async updateAutopilot(id: string, data: UpdateAutopilotRequest): Promise<Autopilot> {
    return this.fetch(`/api/autopilots/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }
  async deleteAutopilot(id: string): Promise<void> {
    await this.fetch(`/api/autopilots/${id}`, { method: "DELETE" });
  }
  // Grant a workspace member explicit write access to the autopilot. Both
  // grant and revoke return the full updated collaborator list so callers can
  // refresh without a second round-trip.
  async grantAutopilotAccess(id: string, userId: string): Promise<AutopilotCollaboratorsResponse> {
    return this.fetch(`/api/autopilots/${id}/collaborators`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId }),
    });
  }
  async revokeAutopilotAccess(id: string, userId: string): Promise<AutopilotCollaboratorsResponse> {
    return this.fetch(`/api/autopilots/${id}/collaborators/${userId}`, {
      method: "DELETE",
    });
  }
  async triggerAutopilot(id: string): Promise<AutopilotRun> {
    // Manual "run now" usually returns a run, including downstream admission
    // failures. Quota rejection is the exception and returns 429. The UI
    // handles both paths without presenting a false-success toast.
    const raw = await this.fetch<unknown>(`/api/autopilots/${id}/trigger`, {
      method: "POST",
      headers: { "Idempotency-Key": createSafeId() },
    });
    return parseWithFallback(raw, AutopilotRunSchema, FALLBACK_AUTOPILOT_RUN, {
      endpoint: "POST /api/autopilots/:id/trigger",
    });
  }
  async getAutopilotQuotaUsage(): Promise<AutopilotQuotaUsage> {
    const raw = await this.fetch<unknown>("/api/autopilots/usage");
    return parseWithFallback(
      raw,
      AutopilotQuotaUsageSchema,
      {
        action: "off",
        used: null,
        reserved: null,
        total: null,
        limit: null,
        reached: null,
        period_start: null,
        period_end: null,
        reset_at: null,
        blocked_counts: null,
      },
      { endpoint: "GET /api/autopilots/usage" },
    );
  }
  async listAutopilotRuns(id: string, params?: { limit?: number; offset?: number }): Promise<ListAutopilotRunsResponse> {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", params.limit.toString());
    if (params?.offset) search.set("offset", params.offset.toString());
    return this.fetch(`/api/autopilots/${id}/runs?${search}`);
  }
  // Returns a single run including its full trigger_payload. List responses
  // omit trigger_payload to keep them small (a webhook envelope can be
  // up to 256 KiB × limit rows), so the detail view fetches via this route.
  async getAutopilotRun(autopilotId: string, runId: string): Promise<AutopilotRun> {
    return this.fetch(`/api/autopilots/${autopilotId}/runs/${runId}`);
  }
  async createAutopilotTrigger(autopilotId: string, data: CreateAutopilotTriggerRequest): Promise<AutopilotTrigger> {
    assertWebhookPayloadConditionsWriteSupported(data);
    return this.fetch(`/api/autopilots/${autopilotId}/triggers`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }
  async updateAutopilotTrigger(autopilotId: string, triggerId: string, data: UpdateAutopilotTriggerRequest): Promise<AutopilotTrigger> {
    assertWebhookPayloadConditionsWriteSupported(data);
    return this.fetch(`/api/autopilots/${autopilotId}/triggers/${triggerId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }
  async deleteAutopilotTrigger(autopilotId: string, triggerId: string): Promise<void> {
    await this.fetch(`/api/autopilots/${autopilotId}/triggers/${triggerId}`, { method: "DELETE" });
  }
  async cronPreview(params: { expr: string; tz: string }): Promise<CronPreviewResponse> {
    const search = new URLSearchParams();
    search.set("expr", params.expr);
    search.set("tz", params.tz);
    const raw = await this.fetch<unknown>(`/api/autopilots/cron-preview?${search}`);
    return parseWithFallback(
      raw,
      CronPreviewResponseSchema,
      UNREADABLE_CRON_PREVIEW_RESPONSE,
      { endpoint: "GET /api/autopilots/cron-preview" },
    );
  }
  async rotateAutopilotTriggerWebhookToken(
    autopilotId: string,
    triggerId: string,
  ): Promise<AutopilotTrigger> {
    return this.fetch(
      `/api/autopilots/${autopilotId}/triggers/${triggerId}/rotate-webhook-token`,
      { method: "POST" },
    );
  }
  // Webhook deliveries — list is slim (no raw_body / selected_headers /
  // response_body); detail returns the full row. Both responses are parsed
  // through a lenient schema so an unknown server-side `status` /
  // `signature_status` value degrades to a generic row instead of dropping
  // the whole list.
  async listAutopilotDeliveries(
    autopilotId: string,
    params?: { limit?: number; offset?: number },
  ): Promise<ListWebhookDeliveriesResponse> {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", params.limit.toString());
    if (params?.offset) search.set("offset", params.offset.toString());
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries?${search}`,
    );
    return parseWithFallback(
      raw,
      ListWebhookDeliveriesResponseSchema,
      EMPTY_LIST_WEBHOOK_DELIVERIES_RESPONSE,
      { endpoint: "GET /api/autopilots/:id/deliveries" },
    );
  }
  async getAutopilotDelivery(
    autopilotId: string,
    deliveryId: string,
  ): Promise<WebhookDelivery> {
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries/${deliveryId}`,
    );
    return parseWithFallback(
      raw,
      WebhookDeliveryResponseSchema,
      { ...EMPTY_WEBHOOK_DELIVERY, id: deliveryId, autopilot_id: autopilotId },
      { endpoint: "GET /api/autopilots/:id/deliveries/:deliveryId" },
    );
  }
  // Replay creates a NEW delivery row referencing the original via
  // `replayed_from_delivery_id`. Server rejects replays of
  // signature-invalid / rejected deliveries with 400 — the UI keeps the
  // button disabled for those rows, but the server is the source of truth.
  async replayAutopilotDelivery(
    autopilotId: string,
    deliveryId: string,
  ): Promise<WebhookDelivery> {
    const raw = await this.fetch<unknown>(
      `/api/autopilots/${autopilotId}/deliveries/${deliveryId}/replay`,
      { method: "POST", headers: { "Idempotency-Key": createSafeId() } },
    );
    return parseWithFallback(
      raw,
      WebhookDeliveryResponseSchema,
      { ...EMPTY_WEBHOOK_DELIVERY, autopilot_id: autopilotId },
      { endpoint: "POST /api/autopilots/:id/deliveries/:deliveryId/replay" },
    );
  }
  // GitHub integration
  async getGitHubConnectURL(
    workspaceId: string,
    options?: { desktop?: boolean },
  ): Promise<GitHubConnectResponse> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/github/connect${options?.desktop ? "?desktop=1" : ""}`,
    );
    return parseWithFallback(
      raw,
      GitHubConnectResponseSchema,
      EMPTY_GITHUB_CONNECT_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/github/connect" },
    );
  }
  async listGitHubInstallations(workspaceId: string): Promise<ListGitHubInstallationsResponse> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/github/installations`,
    );
    return parseWithFallback(
      raw,
      ListGitHubInstallationsResponseSchema,
      EMPTY_LIST_GITHUB_INSTALLATIONS_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/github/installations" },
    );
  }
  async listGitHubInstallationRepositories(
    workspaceId: string,
    installationId: string,
    params: { page?: number; per_page?: number } = {},
  ): Promise<ListGitHubRepositoriesResponse> {
    const search = new URLSearchParams();
    if (params.page !== undefined) search.set("page", String(params.page));
    if (params.per_page !== undefined) search.set("per_page", String(params.per_page));
    const suffix = search.size > 0 ? `?${search.toString()}` : "";
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/github/installations/${installationId}/repositories${suffix}`,
    );
    return parseWithFallback(
      raw,
      ListGitHubRepositoriesResponseSchema,
      EMPTY_LIST_GITHUB_REPOSITORIES_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/github/installations/:installationId/repositories" },
    );
  }
  async deleteGitHubInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/github/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  async listIssuePullRequests(issueId: string): Promise<IssuePullRequestsResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/pull-requests`);
    return parseWithFallback(
      raw,
      IssuePullRequestsResponseSchema,
      EMPTY_ISSUE_PULL_REQUESTS_RESPONSE,
      { endpoint: "GET /api/issues/:id/pull-requests" },
    );
  }
  /** Environments the issue's runs started, live ones plus recently ended. */
  async listIssueEnvironments(issueId: string): Promise<IssueEnvironmentsResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/environments`);
    return parseWithFallback(
      raw,
      IssueEnvironmentsResponseSchema,
      EMPTY_ISSUE_ENVIRONMENTS_RESPONSE,
      { endpoint: "GET /api/issues/:id/environments" },
    );
  }
  /** Ask the environment's machine to stop it. Resolves once queued. */
  async stopIssueEnvironment(issueId: string, environmentId: string): Promise<IssueEnvironmentCommand> {
    return this.issueEnvironmentCommand(`/api/issues/${issueId}/environments/${environmentId}/stop`, "stop", { method: "POST" });
  }
  /** Ask the environment's machine to start it again from the same code. */
  async restartIssueEnvironment(issueId: string, environmentId: string): Promise<IssueEnvironmentCommand> {
    return this.issueEnvironmentCommand(`/api/issues/${issueId}/environments/${environmentId}/restart`, "restart", { method: "POST" });
  }
  /** Ask the environment's machine for the end of its log; poll the command for it. */
  async readIssueEnvironmentLogs(issueId: string, environmentId: string): Promise<IssueEnvironmentCommand> {
    return this.issueEnvironmentCommand(`/api/issues/${issueId}/environments/${environmentId}/logs`, "logs", { method: "POST" });
  }
  /** Start an environment the latest run on a machine declared. */
  async startIssueEnvironment(issueId: string, body: StartIssueEnvironmentRequest): Promise<IssueEnvironmentCommand> {
    return this.issueEnvironmentCommand(`/api/issues/${issueId}/environments/start`, "start", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }
  async getIssueEnvironmentCommand(issueId: string, commandId: string): Promise<IssueEnvironmentCommand> {
    return this.issueEnvironmentCommand(`/api/issues/${issueId}/environment-commands/${commandId}`, "logs");
  }
  private async issueEnvironmentCommand(path: string, action: IssueEnvironmentAction, init?: RequestInit): Promise<IssueEnvironmentCommand> {
    const raw = await this.fetch<unknown>(path, init);
    const fallback: IssueEnvironmentCommand = {
      id: "", action, status: "failed", error: "", environment_id: null, runtime_id: "", task_id: null, repo: "", name: "",
      actor_type: "member", actor_id: null, created_at: new Date().toISOString(), completed_at: null, output: null,
    };
    return parseWithFallback(raw, IssueEnvironmentCommandSchema, fallback, { endpoint: `${init?.method ?? "GET"} ${path.replace(/[0-9a-f-]{36}/g, ":id")}` });
  }
  /** Every linked PR in the workspace, for PR chips on issue cards and rows. */
  async listIssuePullRequestSummaries(): Promise<IssuePullRequestSummariesResponse> {
    const raw = await this.fetch<unknown>("/api/issues/pull-requests");
    return parseWithFallback(
      raw,
      IssuePullRequestSummariesResponseSchema,
      EMPTY_ISSUE_PULL_REQUEST_SUMMARIES_RESPONSE,
      { endpoint: "GET /api/issues/pull-requests" },
    );
  }
  /** Link a PR the workspace already mirrors, by pasted URL or by id (undo). */
  async linkIssuePullRequest(
    issueId: string,
    body: { url: string } | { pull_request_id: string },
  ): Promise<IssuePullRequestsResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/pull-requests`, {
      method: "POST",
      body: JSON.stringify(body),
    });
    return parseWithFallback(
      raw,
      IssuePullRequestsResponseSchema,
      EMPTY_ISSUE_PULL_REQUESTS_RESPONSE,
      { endpoint: "POST /api/issues/:id/pull-requests" },
    );
  }
  /** Remove a PR from an issue; later webhooks will not link it again. */
  async unlinkIssuePullRequest(issueId: string, pullRequestId: string): Promise<IssuePullRequestsResponse> {
    const raw = await this.fetch<unknown>(
      `/api/issues/${issueId}/pull-requests/${pullRequestId}`,
      { method: "DELETE" },
    );
    return parseWithFallback(
      raw,
      IssuePullRequestsResponseSchema,
      EMPTY_ISSUE_PULL_REQUESTS_RESPONSE,
      { endpoint: "DELETE /api/issues/:id/pull-requests/:prId" },
    );
  }
  /** Turn PR auto-complete off (or back on) for one issue. */
  async setIssuePRAutoComplete(issueId: string, disabled: boolean): Promise<IssuePullRequestsResponse> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/pr-auto-complete`, {
      method: "PUT",
      body: JSON.stringify({ disabled }),
    });
    return parseWithFallback(
      raw,
      IssuePullRequestsResponseSchema,
      EMPTY_ISSUE_PULL_REQUESTS_RESPONSE,
      { endpoint: "PUT /api/issues/:id/pr-auto-complete" },
    );
  }
  // VCS integration (Forgejo / Gitea / GitLab)
  async listVCSConnections(workspaceId: string): Promise<ListVCSConnectionsResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/vcs/connections`);
  }
  async connectVCS(
    workspaceId: string,
    body: ConnectVCSRequest,
  ): Promise<ConnectVCSResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/vcs/connections`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }
  async deleteVCSConnection(workspaceId: string, connectionId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/vcs/connections/${connectionId}`, {
      method: "DELETE",
    });
  }
  async rotateVCSWebhook(
    workspaceId: string,
    connectionId: string,
  ): Promise<ConnectVCSResponse> {
    return this.fetch(
      `/api/workspaces/${workspaceId}/vcs/connections/${connectionId}/rotate-webhook`,
      { method: "POST" },
    );
  }
  // Lark integration
  async listLarkInstallations(workspaceId: string): Promise<ListLarkInstallationsResponse> {
    const endpoint = `/api/workspaces/${workspaceId}/lark/installations`;
    return parseWithFallback(
      await this.fetch(endpoint),
      ListLarkInstallationsResponseSchema,
      EMPTY_LIST_LARK_INSTALLATIONS_RESPONSE,
      { endpoint },
    );
  }
  async beginLarkInstall(
    workspaceId: string,
    agentId: string,
    region: "feishu" | "lark",
  ): Promise<BeginLarkInstallResponse> {
    // The user picks the cloud explicitly in the UI ("Bind to Feishu"
    // vs "Bind to Lark"), and the backend POSTs the device-flow `begin`
    // against the corresponding accounts host (accounts.feishu.cn vs
    // accounts.larksuite.com) so the QR renders against the right
    // cloud up front. Empty / omitted region still resolves to Feishu
    // server-side (RegionOrDefault) — we surface region as a required
    // arg here so every call site is forced to make a deliberate
    // choice rather than silently defaulting to mainland.
    const search = new URLSearchParams({ agent_id: agentId, region });
    return this.fetch(`/api/workspaces/${workspaceId}/lark/install/begin?${search.toString()}`, {
      method: "POST",
    });
  }
  async getLarkInstallStatus(workspaceId: string, sessionId: string): Promise<LarkInstallStatusResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/lark/install/${sessionId}/status`);
  }
  async deleteLarkInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/lark/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  async redeemLarkBindingToken(token: string): Promise<RedeemLarkBindingTokenResponse> {
    return this.fetch(`/api/lark/binding/redeem`, {
      method: "POST",
      body: JSON.stringify({ token }),
    });
  }
  // Composio integration (MUL-3720). All routes are user-scoped (a connection
  // belongs to a user, not a workspace), so none take a workspaceId.

  /** The project's connectable Composio toolkits (those with an enabled auth
   * config). Since MUL-4009 the backend filters out non-connectable toolkits,
   * so every entry has `connectable: true`. A resolver/upstream failure is a
   * 502 rather than an empty list. */
  async listComposioToolkits(): Promise<ComposioToolkit[]> {
    return this.fetch(`/api/integrations/composio/toolkits`);
  }
  /** The caller's active Composio connections. */
  async listComposioConnections(): Promise<ComposioConnection[]> {
    return this.fetch(`/api/integrations/composio/connections`);
  }
  /** Disconnects a Composio connection the caller owns. */
  async deleteComposioConnection(connectionId: string): Promise<void> {
    await this.fetch(`/api/integrations/composio/connections/${connectionId}`, {
      method: "DELETE",
    });
  }
  // Publishing channels (Content Studio). Responses are returned raw; the
  // publish-channels hooks parse them with their zod schemas.

  // Connected Apps (MUL-8516). Workspace-scoped: team accounts belong to the
  // workspace, personal accounts to the caller.
  /** The workspace's publishing channels. */
  async listPublishChannels(): Promise<unknown> {
    return this.fetch(`/api/publish-channels`);
  }
  /** Adds a publishing channel that is not connected yet. */
  async createPublishChannel(input: CreatePublishChannelInput): Promise<unknown> {
    return this.fetch(`/api/publish-channels`, {
      method: "POST",
      body: JSON.stringify(input),
    });
  }
  /** Which Ayrshare keys the workspace has set. No key material is returned. */
  async getPublishSettings(): Promise<unknown> {
    return this.fetch(`/api/publish-channels/settings`);
  }
  /** Saves the workspace's Ayrshare keys. Write-only; a field left out is unchanged. */
  async savePublishSettings(input: SavePublishSettingsInput): Promise<unknown> {
    return this.fetch(`/api/publish-channels/settings`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  }
  /** Removes a publishing channel from the workspace. */
  async deletePublishChannel(id: string): Promise<void> {
    await this.fetch(`/api/publish-channels/${id}`, { method: "DELETE" });
  }
  /** A link the user opens to connect the channel's account. */
  async connectPublishChannel(id: string): Promise<unknown> {
    return this.fetch(`/api/publish-channels/${id}/connect`, { method: "POST" });
  }
  /** Asks Ayrshare whether the channel's account is linked, and returns the channel. */
  async checkPublishChannel(id: string): Promise<unknown> {
    return this.fetch(`/api/publish-channels/${id}/check`, { method: "POST" });
  }
  /** The exact request body a post would send. Nothing is sent. */
  async dryRunPublishChannel(
    id: string,
    input: { text: string; media_url?: string; hashtags?: string[] },
  ): Promise<unknown> {
    return this.fetch(`/api/publish-channels/${id}/dry-run`, {
      method: "POST",
      body: JSON.stringify(input),
    });
  }
  // Research Lab source settings. Responses are returned raw; the research hooks parse them
  // with their zod schema. The API keys are write-only here: the server never returns one.

  /** Which Research Lab sources have a key, and the contact address. */
  async getResearchSettings(): Promise<unknown> {
    return this.fetch(`/api/research/settings`);
  }
  /** Saves keys and the contact address. A field left out is unchanged; an empty string removes it. */
  async updateResearchSettings(input: UpdateResearchSettingsInput): Promise<unknown> {
    return this.fetch(`/api/research/settings`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  }
  async getAppsOverview(): Promise<AppsOverview> {
    const raw = await this.fetch<unknown>(`/api/apps`);
    return parseWithFallback(raw, AppsOverviewSchema, EMPTY_APPS_OVERVIEW, { endpoint: "GET /api/apps" });
  }
  /** The workspace's catalog; `view: "all"` (admins) lists every app with
   * whether the catalog allows it. */
  async listAppCatalog(view?: "all"): Promise<AppCatalogResponse> {
    const raw = await this.fetch<unknown>(`/api/apps/catalog${view ? `?view=${view}` : ""}`);
    return parseWithFallback(raw, AppCatalogResponseSchema, EMPTY_APP_CATALOG, { endpoint: "GET /api/apps/catalog" });
  }
  /** Starts connecting an account; the browser is sent to the returned URL
   * and comes back to `returnTo` (an app path) when given. */
  async connectApp(scope: AppAccountScope, toolkitSlug: string, returnTo?: string): Promise<ComposioConnectInitResponse> {
    const raw = await this.fetch<unknown>(`/api/apps/${scope}/connect`, {
      method: "POST",
      body: JSON.stringify({ toolkit_slug: toolkitSlug, ...(returnTo ? { return_to: returnTo } : {}) }),
    });
    const endpoint = `POST /api/apps/${scope}/connect`;
    const parsed = parseWithFallback<ComposioConnectInitResponse | null>(raw, AppConnectInitSchema, null, { endpoint });
    if (!parsed) throw new Error(`Malformed response from ${endpoint}`);
    return parsed;
  }
  async disconnectTeamApp(connectionId: string): Promise<void> {
    await this.fetch(`/api/apps/team/connections/${connectionId}`, { method: "DELETE" });
  }
  async getTeamAppImpact(toolkitSlug: string): Promise<TeamAppImpact> {
    const raw = await this.fetch<unknown>(`/api/apps/team/impact?toolkit_slug=${encodeURIComponent(toolkitSlug)}`);
    return parseWithFallback(raw, TeamAppImpactSchema, EMPTY_TEAM_APP_IMPACT, { endpoint: "GET /api/apps/team/impact" });
  }
  /** Asks the workspace's admins to connect a team account for an app. */
  async requestTeamApp(toolkitSlug: string, agentId?: string): Promise<void> {
    await this.fetch(`/api/apps/team/requests`, {
      method: "POST",
      body: JSON.stringify({ toolkit_slug: toolkitSlug, ...(agentId ? { agent_id: agentId } : {}) }),
    });
  }
  /** An admin declines every open team-account request for an app; a member
   * withdraws their own. */
  async dismissTeamAppRequests(toolkitSlug: string): Promise<void> {
    await this.fetch(`/api/apps/team/requests?toolkit_slug=${encodeURIComponent(toolkitSlug)}`, { method: "DELETE" });
  }
  /** What disconnecting one of the caller's personal accounts would stop in
   * its workspace. */
  async getPersonalAppImpact(connectionId: string): Promise<PersonalAppImpact> {
    const raw = await this.fetch<unknown>(`/api/integrations/composio/connections/${connectionId}/impact`);
    return parseWithFallback(raw, PersonalAppImpactSchema, { agents: [] }, {
      endpoint: "GET /api/integrations/composio/connections/:id/impact",
    });
  }
  // Write responses parse to null when malformed; callers refetch instead of
  // caching a guess.

  async updateAppSettings(body: {
    catalog_mode: AppCatalogMode;
    allowed_toolkits?: string[];
  }): Promise<{ catalog_mode: AppCatalogMode; allowed_toolkits: string[] } | null> {
    const raw = await this.fetch<unknown>(`/api/apps/settings`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
    return parseWithFallback(raw, AppSettingsResponseSchema, null, { endpoint: "PUT /api/apps/settings" });
  }
  async listIssueAppCalls(issueId: string): Promise<{ calls: IssueAppCall[] }> {
    const raw = await this.fetch<unknown>(`/api/app-calls?issue_id=${encodeURIComponent(issueId)}`);
    return parseWithFallback(raw, IssueAppCallsResponseSchema, { calls: [] }, { endpoint: "GET /api/app-calls" });
  }
  async updateAppRules(
    toolkitSlug: string,
    scope: AppAccountScope,
    rules: { action_class: AppActionClass; policy: AppActionPolicy }[],
  ): Promise<{ rules: AppRule[] } | null> {
    const raw = await this.fetch<unknown>(`/api/apps/rules?toolkit_slug=${encodeURIComponent(toolkitSlug)}&scope=${scope}`, {
      method: "PUT",
      body: JSON.stringify({ rules }),
    });
    return parseWithFallback(raw, AppRulesResponseSchema, null, { endpoint: "PUT /api/apps/rules" });
  }
  async listAppActivity(toolkitSlug: string, scope: AppAccountScope): Promise<{ calls: AppToolCall[] }> {
    const raw = await this.fetch<unknown>(`/api/apps/activity?toolkit_slug=${encodeURIComponent(toolkitSlug)}&scope=${scope}`);
    return parseWithFallback(raw, AppActivityResponseSchema, { calls: [] }, { endpoint: "GET /api/apps/activity" });
  }
  async listAgentApps(agentId: string): Promise<{ apps: AgentApp[] }> {
    const raw = await this.fetch<unknown>(`/api/agents/${agentId}/apps`);
    return parseWithFallback(raw, AgentAppsResponseSchema, { apps: [] }, { endpoint: "GET /api/agents/:id/apps" });
  }
  async updateAgentApps(agentId: string, body: UpdateAgentAppsRequest): Promise<{ apps: AgentApp[] } | null> {
    const raw = await this.fetch<unknown>(`/api/agents/${agentId}/apps`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
    return parseWithFallback(raw, AgentAppsResponseSchema, null, { endpoint: "PUT /api/agents/:id/apps" });
  }
  /** Requests of an issue, of a chat, or (neither given) the caller's
   * pending ones. The first page holds every pending request and the newest
   * decided ones; `before` (a `next_before`) pages back through history. */
  async listAppRequests(
    params: { issueId?: string; chatSessionId?: string; before?: string } = {},
  ): Promise<AppRequestList> {
    const qs = new URLSearchParams();
    if (params.issueId) qs.set("issue_id", params.issueId);
    if (params.chatSessionId) qs.set("chat_session_id", params.chatSessionId);
    if (params.before) qs.set("before", params.before);
    const query = qs.toString();
    const raw = await this.fetch<unknown>(`/api/app-requests${query ? `?${query}` : ""}`);
    return parseWithFallback(raw, AppRequestListSchema, { requests: [], next_before: null }, {
      endpoint: "GET /api/app-requests",
    });
  }
  async approveAppRequest(requestId: string, args?: Record<string, unknown>): Promise<AppRequest | null> {
    const raw = await this.fetch<unknown>(`/api/app-requests/${requestId}/approve`, {
      method: "POST",
      body: JSON.stringify(args ? { arguments: args } : {}),
    });
    return parseWithFallback(raw, AppRequestSchema, null, { endpoint: "POST /api/app-requests/:id/approve" });
  }
  async rejectAppRequest(requestId: string, reason?: string): Promise<AppRequest | null> {
    const raw = await this.fetch<unknown>(`/api/app-requests/${requestId}/reject`, {
      method: "POST",
      body: JSON.stringify(reason ? { reason } : {}),
    });
    return parseWithFallback(raw, AppRequestSchema, null, { endpoint: "POST /api/app-requests/:id/reject" });
  }
  // Slack integration (MUL-3666)
  async listSlackInstallations(workspaceId: string): Promise<ListSlackInstallationsResponse> {
    return this.fetch(`/api/workspaces/${workspaceId}/slack/installations`);
  }
  // registerSlackBYO performs a bring-your-own-app install: the admin pastes the
  // bot token (xoxb-) + app-level token (xapp-) of the Slack app they created,
  // and the backend validates + persists it, returning the new installation.
  async registerSlackBYO(
    workspaceId: string,
    agentId: string,
    body: RegisterSlackBYORequest,
  ): Promise<SlackInstallation> {
    const search = new URLSearchParams({ agent_id: agentId });
    return this.fetch(`/api/workspaces/${workspaceId}/slack/install/byo?${search.toString()}`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }
  async deleteSlackInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/slack/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  async redeemSlackBindingToken(token: string): Promise<RedeemSlackBindingTokenResponse> {
    return this.fetch(`/api/slack/binding/redeem`, {
      method: "POST",
      body: JSON.stringify({ token }),
    });
  }
  // DingTalk integration
  async listDingTalkInstallations(
    workspaceId: string,
  ): Promise<ListDingTalkInstallationsResponse> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/dingtalk/installations`);
    return parseWithFallback(
      raw,
      ListDingTalkInstallationsResponseSchema,
      EMPTY_LIST_DINGTALK_INSTALLATIONS_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/dingtalk/installations" },
    );
  }
  async listDingTalkGroups(
    workspaceId: string,
    params: ListDingTalkGroupsParams = {},
  ): Promise<ListDingTalkGroupsResponse> {
    let raw: unknown;
    try {
      const search = dingTalkGroupSearch(params);
      raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/dingtalk/groups${search}`);
    } catch (error) {
      // Installed clients can be newer than the server they reconnect to. A
      // backend predating group discovery 404s this additive endpoint. The
      // first admin-only version returns 403 to members; both mean this client
      // cannot use the workspace inventory, while real failures stay visible.
      if (
        error instanceof ApiError &&
        (error.status === 404 || error.status === 403)
      ) {
        return EMPTY_LIST_DINGTALK_GROUPS_RESPONSE;
      }
      throw error;
    }
    return parseWithFallback(
      raw,
      ListDingTalkGroupsResponseSchema,
      EMPTY_LIST_DINGTALK_GROUPS_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/dingtalk/groups" },
    );
  }
  async listAgentDingTalkGroups(
    agentId: string,
    params: ListDingTalkGroupsParams = {},
  ): Promise<ListDingTalkGroupsResponse> {
    let raw: unknown;
    try {
      const search = dingTalkGroupSearch(params);
      raw = await this.fetch<unknown>(`/api/agents/${agentId}/dingtalk/groups${search}`);
    } catch (error) {
      // Keep installed clients compatible with servers that predate agent-
      // scoped group discovery. Authorization failures must remain visible.
      if (error instanceof ApiError && error.status === 404) {
        return EMPTY_LIST_DINGTALK_GROUPS_RESPONSE;
      }
      throw error;
    }
    return parseWithFallback(
      raw,
      ListDingTalkGroupsResponseSchema,
      EMPTY_LIST_DINGTALK_GROUPS_RESPONSE,
      { endpoint: "GET /api/agents/:id/dingtalk/groups" },
    );
  }
  async forgetDingTalkGroup(
    workspaceId: string,
    installationId: string,
    conversationId: string,
  ): Promise<void> {
    await this.fetch(
      `/api/workspaces/${workspaceId}/dingtalk/installations/${installationId}/groups/${encodeURIComponent(conversationId)}`,
      { method: "DELETE" },
    );
  }
  // registerDingTalkBYO performs a bring-your-own-app install: the agent owner
  // or a workspace owner/admin pastes the AppKey (client id) + AppSecret
  // (client secret) of the DingTalk Stream-mode robot they created, and the
  // backend validates + persists it, returning the new installation.
  async registerDingTalkBYO(
    workspaceId: string,
    agentId: string,
    body: RegisterDingTalkBYORequest,
  ): Promise<DingTalkInstallation> {
    const search = new URLSearchParams({ agent_id: agentId });
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/dingtalk/install/byo?${search.toString()}`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );
    return parseWithFallback(raw, DingTalkInstallationSchema, EMPTY_DINGTALK_INSTALLATION, {
      endpoint: "POST /api/workspaces/:id/dingtalk/install/byo",
    });
  }
  async deleteDingTalkInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/dingtalk/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  async redeemDingTalkBindingToken(
    token: string,
  ): Promise<RedeemDingTalkBindingTokenResponse> {
    const raw = await this.fetch<unknown>(`/api/dingtalk/binding/redeem`, {
      method: "POST",
      body: JSON.stringify({ token }),
    });
    return parseWithFallback(
      raw,
      RedeemDingTalkBindingTokenResponseSchema,
      EMPTY_REDEEM_DINGTALK_BINDING_TOKEN_RESPONSE,
      { endpoint: "POST /api/dingtalk/binding/redeem" },
    );
  }
  // WeCom smart-bot ("智能机器人" / aibot) integration. The bot dials a
  // WebSocket long connection to wss://openws.work.weixin.qq.com and stays
  // authenticated with (bot_id, secret); no public callback URL is required.
  // These three methods drive the Settings-page BYO Connect dialog + list +
  // disconnect only — the inbound WebSocket loop runs entirely server-side.
  async listWecomInstallations(workspaceId: string): Promise<ListWecomInstallationsResponse> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/wecom/installations`);
    return parseWithFallback(
      raw,
      ListWecomInstallationsResponseSchema,
      EMPTY_LIST_WECOM_INSTALLATIONS_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/wecom/installations" },
    );
  }
  // registerWecomBYO performs a bring-your-own-app install: the admin pastes
  // the bot id and long-connection secret from the WeCom admin console,
  // and the backend seals the secret with MULTICA_WECOM_SECRET_KEY before
  // persisting, returning the new installation.
  async registerWecomBYO(
    workspaceId: string,
    agentId: string,
    body: RegisterWecomBYORequest,
  ): Promise<WecomInstallation> {
    const search = new URLSearchParams({ agent_id: agentId });
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/wecom/install/byo?${search.toString()}`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );
    return parseWithFallback(raw, WecomInstallationSchema, EMPTY_WECOM_INSTALLATION, {
      endpoint: "POST /api/workspaces/:id/wecom/install/byo",
    });
  }
  // beginWecomInstall starts (or resumes) a scan-code install: the backend
  // hands back a session id, and the install worker fetches the QR from WeCom
  // out of band. Callers poll getWecomInstallStatus for the QR itself.
  //
  // idempotencyKey must be stable for one user gesture — a retried or
  // double-submitted request with the same key reuses the existing session
  // rather than burning a WeCom generate quota slot on a second QR.
  async beginWecomInstall(
    workspaceId: string,
    agentId: string,
    idempotencyKey: string,
  ): Promise<BeginWecomInstallResponse> {
    const search = new URLSearchParams({ agent_id: agentId });
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/wecom/install/begin?${search.toString()}`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
      },
    );
    return parseWithFallback(
      raw,
      BeginWecomInstallResponseSchema,
      MALFORMED_BEGIN_WECOM_INSTALL_RESPONSE,
      { endpoint: "POST /api/workspaces/:id/wecom/install/begin" },
    );
  }
  // getWecomInstallStatus is one poll of a scan-install session. The response
  // carries the QR while pending, and the installation id once bound.
  async getWecomInstallStatus(
    workspaceId: string,
    sessionId: string,
  ): Promise<WecomInstallStatusResponse> {
    const raw = await this.fetch<unknown>(
      `/api/workspaces/${workspaceId}/wecom/install/${sessionId}`,
    );
    return parseWithFallback(
      raw,
      WecomInstallStatusResponseSchema,
      MALFORMED_WECOM_INSTALL_STATUS_RESPONSE,
      { endpoint: "GET /api/workspaces/:id/wecom/install/:sessionId" },
    );
  }
  async deleteWecomInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/wecom/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  async beginWeixinLogin(workspaceId: string, agentId: string): Promise<WeixinLogin> {
    const search = new URLSearchParams({ agent_id: agentId });
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/weixin/logins?${search}`, { method: "POST" });
    return parseWithFallback(raw, WeixinLoginSchema, INVALID_WEIXIN_LOGIN, { endpoint: "POST /api/workspaces/:id/weixin/logins" });
  }
  async pollWeixinLogin(workspaceId: string, loginId: string, verifyCode = "", signal?: AbortSignal): Promise<WeixinLogin> {
    const raw = await this.fetch<unknown>(`/api/workspaces/${workspaceId}/weixin/logins/${loginId}/poll`, { method: "POST", body: JSON.stringify({ verify_code: verifyCode }), signal });
    return parseWithFallback(raw, WeixinLoginSchema, INVALID_WEIXIN_LOGIN, { endpoint: "POST /api/workspaces/:id/weixin/logins/:loginId/poll" });
  }
  async cancelWeixinLogin(workspaceId: string, loginId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/weixin/logins/${loginId}`, { method: "DELETE" });
  }
  async deleteWeixinInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/weixin/installations/${installationId}`, { method: "DELETE" });
  }
  async deleteTelegramInstallation(workspaceId: string, installationId: string): Promise<void> {
    await this.fetch(`/api/workspaces/${workspaceId}/telegram/installations/${installationId}`, {
      method: "DELETE",
    });
  }
  // The Secretary's digest and briefs
  async getCompanyDigest(params?: DigestParams): Promise<Digest> {
    const search = new URLSearchParams();
    if (params?.since) search.set("since", params.since);
    const qs = search.toString();
    const raw = await this.fetch<unknown>(`/api/company/digest${qs ? `?${qs}` : ""}`);
    return parseWithFallback(raw, DigestSchema, FALLBACK_DIGEST, {
      endpoint: "GET /api/company/digest",
    });
  }
  // The Secretary setup and its readiness checklist (owner or admin).
  async getCompanySetup(): Promise<CompanySetupReadiness> {
    const raw = await this.fetch<unknown>("/api/company/setup");
    return parseWithFallback(raw, CompanySetupReadinessSchema, FALLBACK_COMPANY_SETUP_READINESS, {
      endpoint: "GET /api/company/setup",
    });
  }
  // Idempotent: a second call changes nothing. Without an online Hermes runtime
  // it rejects with an ApiError whose body parseCompanySetupRefusal reads.
  async setupCompany(data: CompanySetupRequest): Promise<CompanySetupReadiness> {
    const raw = await this.fetch<unknown>("/api/company/setup", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CompanySetupReadinessSchema, FALLBACK_COMPANY_SETUP_READINESS, {
      endpoint: "POST /api/company/setup",
    });
  }
  // The starter company (owner or admin). Idempotent; a step that failed is reported in the result.
  async applyCompanyTemplate(data: CompanyTemplateApplyRequest): Promise<CompanyTemplateApplyResult> {
    const raw = await this.fetch<unknown>("/api/company/company-template/apply", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, CompanyTemplateApplyResultSchema, FALLBACK_COMPANY_TEMPLATE_RESULT, {
      endpoint: "POST /api/company/company-template/apply",
    });
  }
  async listBriefs(params?: ListBriefsParams): Promise<Brief[]> {
    const search = new URLSearchParams();
    if (params?.kind) search.set("kind", params.kind);
    if (params?.limit) search.set("limit", String(params.limit));
    const qs = search.toString();
    const raw = await this.fetch<unknown>(`/api/briefs${qs ? `?${qs}` : ""}`);
    return parseWithFallback<Brief[]>(raw, BriefListSchema, [], {
      endpoint: "GET /api/briefs",
    });
  }
  async getBrief(id: string): Promise<Brief> {
    const raw = await this.fetch<unknown>(`/api/briefs/${encodeURIComponent(id)}`);
    return parseWithFallback(raw, BriefSchema, FALLBACK_BRIEF, {
      endpoint: "GET /api/briefs/:id",
    });
  }
  async publishBrief(data: PublishBriefRequest): Promise<Brief> {
    const raw = await this.fetch<unknown>("/api/briefs", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, BriefSchema, FALLBACK_BRIEF, {
      endpoint: "POST /api/briefs",
    });
  }
  // Company knowledge. Status and enabling are for owners and admins; reads and
  // searches are open to members; writes land as drafts until an owner or
  // admin publishes them.
  async getKnowledgeStatus(): Promise<KnowledgeStatus> {
    const raw = await this.fetch<unknown>("/api/knowledge");
    return parseWithFallback(raw, KnowledgeStatusSchema, FALLBACK_KNOWLEDGE_STATUS, {
      endpoint: "GET /api/knowledge",
    });
  }
  async enableKnowledge(): Promise<KnowledgeStatus> {
    const raw = await this.fetch<unknown>("/api/knowledge/enable", { method: "POST" });
    return parseWithFallback(raw, KnowledgeStatusSchema, FALLBACK_KNOWLEDGE_STATUS, {
      endpoint: "POST /api/knowledge/enable",
    });
  }
  async listKnowledgeBases(): Promise<KnowledgeBases> {
    const raw = await this.fetch<unknown>("/api/knowledge/bases");
    return parseWithFallback(raw, KnowledgeBasesSchema, FALLBACK_KNOWLEDGE_BASES, {
      endpoint: "GET /api/knowledge/bases",
    });
  }
  async listKnowledgeDocuments(params?: ListKnowledgeDocumentsParams): Promise<KnowledgeDocumentList> {
    const search = new URLSearchParams();
    if (params?.kb) search.set("kb", params.kb);
    if (params?.state) search.set("state", params.state);
    const qs = search.toString();
    const raw = await this.fetch<unknown>(`/api/knowledge/documents${qs ? `?${qs}` : ""}`);
    return parseWithFallback(raw, KnowledgeDocumentListSchema, FALLBACK_KNOWLEDGE_DOCUMENT_LIST, {
      endpoint: "GET /api/knowledge/documents",
    });
  }
  async searchKnowledge(params: SearchKnowledgeParams): Promise<KnowledgeSearchResult> {
    const search = new URLSearchParams({ q: params.q });
    if (params.kb) search.set("kb", params.kb);
    if (params.kind) search.set("kind", params.kind);
    if (params.limit) search.set("limit", String(params.limit));
    const raw = await this.fetch<unknown>(`/api/knowledge/search?${search.toString()}`);
    return parseWithFallback(raw, KnowledgeSearchResultSchema, FALLBACK_KNOWLEDGE_SEARCH, {
      endpoint: "GET /api/knowledge/search",
    });
  }
  /** Changes some typed properties of a document; a null value clears its key. Content and index are untouched. */
  async updateKnowledgeProperties(
    id: string,
    properties: Record<string, unknown>,
  ): Promise<KnowledgeDocument> {
    const raw = await this.fetch<unknown>(`/api/knowledge/documents/${encodeURIComponent(id)}/properties`, {
      method: "PATCH",
      body: JSON.stringify({ properties }),
    });
    return parseWithFallback(raw, KnowledgeDocumentSchema, FALLBACK_KNOWLEDGE_DOCUMENT, {
      endpoint: "PATCH /api/knowledge/documents/:id/properties",
    });
  }
  /** The Collections view of a kind: its fields and the matching published documents. */
  async listKnowledgeCollection(params: {
    kind: string;
    kb?: string;
    match?: Record<string, string>;
    dueKey?: string;
    dueOn?: string;
    limit?: number;
  }): Promise<KnowledgeCollection> {
    const qs = new URLSearchParams({ kind: params.kind });
    if (params.kb) qs.set("kb", params.kb);
    for (const [key, value] of Object.entries(params.match ?? {})) qs.set(`field.${key}`, value);
    if (params.dueKey && params.dueOn) {
      qs.set("due_key", params.dueKey);
      qs.set("due_on", params.dueOn);
    }
    if (params.limit) qs.set("limit", String(params.limit));
    const raw = await this.fetch<unknown>(`/api/knowledge/collections?${qs.toString()}`);
    return parseWithFallback(raw, KnowledgeCollectionSchema, FALLBACK_KNOWLEDGE_COLLECTION, {
      endpoint: "GET /api/knowledge/collections",
    });
  }
  /** Adds a source to a Sales Loop squad: a knowledge document and a request issue for the squad. */
  async createSquadSource(squadId: string, body: { input: string; lane?: string }): Promise<SalesIntake> {
    return this.postSalesIntake(`/api/squads/${encodeURIComponent(squadId)}/sources`, body);
  }
  /** Adds a shared post to a Sales Loop squad. */
  async createSquadShare(
    squadId: string,
    body: { input: string; text?: string; lane?: string },
  ): Promise<SalesIntake> {
    return this.postSalesIntake(`/api/squads/${encodeURIComponent(squadId)}/shares`, body);
  }
  private async postSalesIntake(path: string, body: unknown): Promise<SalesIntake> {
    const raw = await this.fetch<unknown>(path, { method: "POST", body: JSON.stringify(body) });
    return parseWithFallback(raw, SalesIntakeSchema, FALLBACK_SALES_INTAKE, { endpoint: `POST ${path}` });
  }
  async createKnowledgeDocument(data: CreateKnowledgeDocumentRequest): Promise<KnowledgeDocument> {
    const raw = await this.fetch<unknown>("/api/knowledge/documents", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return parseWithFallback(raw, KnowledgeDocumentSchema, FALLBACK_KNOWLEDGE_DOCUMENT, {
      endpoint: "POST /api/knowledge/documents",
    });
  }
  async publishKnowledgeDocument(id: string): Promise<KnowledgeDocument> {
    const raw = await this.fetch<unknown>(`/api/knowledge/documents/${id}/publish`, { method: "POST" });
    return parseWithFallback(raw, KnowledgeDocumentSchema, FALLBACK_KNOWLEDGE_DOCUMENT, {
      endpoint: "POST /api/knowledge/documents/:id/publish",
    });
  }
  async discardKnowledgeDocument(id: string): Promise<void> {
    await this.fetch(`/api/knowledge/documents/${id}/discard`, { method: "POST" });
  }
  async deleteKnowledgeDocument(id: string): Promise<void> {
    await this.fetch(`/api/knowledge/documents/${id}`, { method: "DELETE" });
  }
  // Company knowledge, platform side (plan 10.11 to 10.13). Upload is multipart,
  // so the browser sets the boundary itself, as uploadFile and
  // publishPluginPackage do.
  async uploadKnowledgeDocument(params: UploadKnowledgeDocumentParams): Promise<KnowledgeDocument> {
    const formData = new FormData();
    formData.append("file", params.file);
    formData.append("kb", params.kb);
    formData.append("kind", params.kind);
    if (params.idempotencyKey) formData.append("idempotency_key", params.idempotencyKey);

    const credentialUsed = this.getToken();
    const res = await fetch(`${this.baseUrl}/api/knowledge/documents/upload`, {
      method: "POST",
      headers: this.authHeaders(),
      body: formData,
      credentials: "include",
    });
    if (!res.ok) {
      if (res.status === 401) this.handleUnauthorized(credentialUsed);
      throw new Error(await this.parseErrorMessage(res, `Upload failed: ${res.status}`));
    }
    const raw = (await res.json()) as unknown;
    return parseWithFallback(raw, KnowledgeDocumentSchema, FALLBACK_KNOWLEDGE_DOCUMENT, {
      endpoint: "POST /api/knowledge/documents/upload",
    });
  }
  async listKnowledgeSources(): Promise<KnowledgeSources> {
    const raw = await this.fetch<unknown>("/api/knowledge/sources");
    return parseWithFallback(raw, KnowledgeSourcesSchema, FALLBACK_KNOWLEDGE_SOURCES, {
      endpoint: "GET /api/knowledge/sources",
    });
  }
  async getKnowledgeRepoSync(): Promise<KnowledgeRepoSync> {
    const raw = await this.fetch<unknown>("/api/knowledge/repo-sync");
    return parseWithFallback(raw, KnowledgeRepoSyncSchema, FALLBACK_KNOWLEDGE_REPO_SYNC, {
      endpoint: "GET /api/knowledge/repo-sync",
    });
  }
  async updateKnowledgeRepoSync(req: KnowledgeRepoSyncRequest): Promise<KnowledgeRepoSync> {
    const raw = await this.fetch<unknown>("/api/knowledge/repo-sync", {
      method: "PUT",
      body: JSON.stringify(req),
    });
    return parseWithFallback(raw, KnowledgeRepoSyncSchema, FALLBACK_KNOWLEDGE_REPO_SYNC, {
      endpoint: "PUT /api/knowledge/repo-sync",
    });
  }
  async runKnowledgeRepoSync(): Promise<KnowledgeRepoSync> {
    const raw = await this.fetch<unknown>("/api/knowledge/repo-sync/run", { method: "POST" });
    return parseWithFallback(raw, KnowledgeRepoSyncSchema, FALLBACK_KNOWLEDGE_REPO_SYNC, {
      endpoint: "POST /api/knowledge/repo-sync/run",
    });
  }
  async getKnowledgeBaseAccess(key: string): Promise<KnowledgeAccess> {
    const raw = await this.fetch<unknown>(`/api/knowledge/bases/${encodeURIComponent(key)}/access`);
    return parseWithFallback(raw, KnowledgeAccessSchema, FALLBACK_KNOWLEDGE_ACCESS, {
      endpoint: "GET /api/knowledge/bases/:key/access",
    });
  }
  async setKnowledgeVisibility(key: string, visibility: string): Promise<KnowledgeAccess> {
    const raw = await this.fetch<unknown>(`/api/knowledge/bases/${encodeURIComponent(key)}/visibility`, {
      method: "PUT",
      body: JSON.stringify({ visibility }),
    });
    return parseWithFallback(raw, KnowledgeAccessSchema, FALLBACK_KNOWLEDGE_ACCESS, {
      endpoint: "PUT /api/knowledge/bases/:key/visibility",
    });
  }
  async getKnowledgeHealth(): Promise<KnowledgeHealth> {
    const raw = await this.fetch<unknown>("/api/knowledge/health");
    return parseWithFallback(raw, KnowledgeHealthSchema, FALLBACK_KNOWLEDGE_HEALTH, {
      endpoint: "GET /api/knowledge/health",
    });
  }
  async listKnowledgePolicies(): Promise<KnowledgePublishPolicies> {
    const raw = await this.fetch<unknown>("/api/knowledge/policies");
    return parseWithFallback(raw, KnowledgePublishPoliciesSchema, FALLBACK_KNOWLEDGE_POLICIES, {
      endpoint: "GET /api/knowledge/policies",
    });
  }
  async setKnowledgePolicy(req: SetKnowledgePolicyRequest): Promise<KnowledgePublishPolicy> {
    const raw = await this.fetch<unknown>("/api/knowledge/policies", {
      method: "PUT",
      body: JSON.stringify(req),
    });
    return parseWithFallback(raw, KnowledgePublishPolicySchema, FALLBACK_KNOWLEDGE_POLICY, {
      endpoint: "PUT /api/knowledge/policies",
    });
  }
  async deleteKnowledgePolicy(scope: "base" | "kind", key: string): Promise<void> {
    const qs = new URLSearchParams({ scope, key }).toString();
    await this.fetch(`/api/knowledge/policies?${qs}`, { method: "DELETE" });
  }
  async listKnowledgeEvalRuns(): Promise<KnowledgeEvalRuns> {
    const raw = await this.fetch<unknown>("/api/knowledge/eval/runs");
    return parseWithFallback(raw, KnowledgeEvalRunsSchema, FALLBACK_KNOWLEDGE_EVAL_RUNS, {
      endpoint: "GET /api/knowledge/eval/runs",
    });
  }
  async runKnowledgeEval(): Promise<KnowledgeEvalRun> {
    const raw = await this.fetch<unknown>("/api/knowledge/eval/runs", { method: "POST" });
    return parseWithFallback(raw, KnowledgeEvalRunSchema, FALLBACK_KNOWLEDGE_EVAL_RUN, {
      endpoint: "POST /api/knowledge/eval/runs",
    });
  }
  // Proposals (squad requests for a decision by the boss)
  async listProposals(params?: ListProposalsParams): Promise<Proposal[]> {
    const search = new URLSearchParams();
    if (params?.state) search.set("state", params.state);
    if (params?.squad_id) search.set("squad_id", params.squad_id);
    const qs = search.toString();
    const raw = await this.fetch<unknown>(`/api/proposals${qs ? `?${qs}` : ""}`);
    return parseWithFallback<Proposal[]>(raw, ProposalListSchema, [], {
      endpoint: "GET /api/proposals",
    });
  }
  async getProposal(id: string): Promise<Proposal> {
    const raw = await this.fetch<unknown>(`/api/proposals/${id}`);
    return parseWithFallback(raw, ProposalSchema, FALLBACK_PROPOSAL, {
      endpoint: "GET /api/proposals/:id",
    });
  }
  async listProposalRevisions(id: string): Promise<ProposalRevision[]> {
    const raw = await this.fetch<unknown>(`/api/proposals/${id}/revisions`);
    return parseWithFallback<ProposalRevision[]>(raw, ProposalRevisionListSchema, [], {
      endpoint: "GET /api/proposals/:id/revisions",
    });
  }
  // Governance (phase 4): the company state, its quiet hours and the log. The
  // two writes return the whole company state, so the caller reflects what the
  // server kept rather than what it sent.
  async getCompanyState(): Promise<CompanyStateInfo> {
    const raw = await this.fetch<unknown>("/api/company/company-state");
    return parseWithFallback<CompanyStateInfo>(raw, CompanyStateSchema, FALLBACK_COMPANY_STATE, {
      endpoint: "GET /api/company/company-state",
    });
  }
  /** A squad's handoff rule: where it hands its finished requests, and how. */
  async getSquadHandoffRule(squadId: string): Promise<SquadHandoffRule> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/handoff-rule`);
    return parseSquadHandoffRule(raw, "GET /api/squads/:id/handoff-rule");
  }
  /** Saves a squad's handoff rule, or removes it with no target. Owners and admins only. */
  async setSquadHandoffRule(squadId: string, data: SetSquadHandoffRuleRequest): Promise<SquadHandoffRule> {
    const raw = await this.fetch<unknown>(`/api/squads/${squadId}/handoff-rule`, {
      method: "PUT",
      body: JSON.stringify(data.target_squad_id ? data : {}),
    });
    return parseSquadHandoffRule(raw, "PUT /api/squads/:id/handoff-rule");
  }
  /** Where a request stands in its squad's handoff. */
  async getIssueHandoff(issueId: string): Promise<IssueHandoff> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/handoff`);
    return parseIssueHandoff(raw, "GET /api/issues/:id/handoff");
  }
  /** Hands a request to the squad its rule names, now. Owners and admins only. */
  async handOffIssue(issueId: string): Promise<IssueHandoff> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/handoff`, { method: "POST", body: JSON.stringify({}) });
    return parseIssueHandoff(raw, "POST /api/issues/:id/handoff");
  }
  /** Declines a pending handoff. Owners and admins only. */
  async declineIssueHandoff(issueId: string): Promise<IssueHandoff> {
    const raw = await this.fetch<unknown>(`/api/issues/${issueId}/handoff/decline`, { method: "POST", body: JSON.stringify({}) });
    return parseIssueHandoff(raw, "POST /api/issues/:id/handoff/decline");
  }
  /** The workspace's standing approvals, with this week's counts and flags (spec 4.4). */
  async listApprovalPolicies(): Promise<ApprovalPolicy[]> {
    const raw = await this.fetch<unknown>("/api/approval-policies");
    return parseApprovalPolicies(raw, "GET /api/approval-policies");
  }
  /** Revokes a standing approval in one step. Owners and admins only. */
  async revokeApprovalPolicy(policyId: string): Promise<ApprovalPolicy> {
    const raw = await this.fetch<unknown>(`/api/approval-policies/${policyId}/revoke`, { method: "POST" });
    return parseApprovalPolicy(raw, "POST /api/approval-policies/:id/revoke");
  }
  /** A policy's recent auto-approvals, newest first, with the reversal flags. */
  async listPolicyAutoApprovals(policyId: string): Promise<PolicyAutoApprovalHistory> {
    const raw = await this.fetch<unknown>(`/api/approval-policies/${policyId}/auto-approvals`);
    return parsePolicyAutoApprovals(raw, "GET /api/approval-policies/:id/auto-approvals");
  }
  /** The company pulse strip's read. The server caches it for a minute. */
  async getCompanyPulse(): Promise<CompanyPulse> {
    const raw = await this.fetch<unknown>("/api/company/pulse");
    return parseCompanyPulse(raw);
  }
  /** System health for the Controls tab (spec 4.9). */
  async getGovernanceHealth(): Promise<HealthReport> {
    const raw = await this.fetch<unknown>("/api/governance/health");
    return parseHealthReport(raw);
  }
  /** Budgets of the workspace with their current period (spec 4.2). */
  async listBudgets(): Promise<BudgetList> {
    const raw = await this.fetch<unknown>("/api/budgets");
    return parseWithFallback<BudgetList>(raw, BudgetListSchema, FALLBACK_BUDGET_LIST, {
      endpoint: "GET /api/budgets",
    });
  }
  async createBudget(data: CreateBudgetRequest): Promise<Budget> {
    const raw = await this.fetch<unknown>("/api/budgets", { method: "POST", body: JSON.stringify(data) });
    return parseBudget(raw, "POST /api/budgets");
  }
  async activateGoal(id: string): Promise<Goal> {
    const raw = await this.fetch<unknown>(`/api/goals/${id}/activate`, { method: "POST" });
    return parseWithFallback<Goal>(raw, GoalResponseSchema, FALLBACK_GOAL, { endpoint: "POST /api/goals/:id/activate" });
  }
  /** A 409 with `in_use` rejects as HubSourceInUseError. */
  async deleteHubSource(workspaceId: string, id: string, items: HubSourceDeleteItems): Promise<void> {
    try {
      await this.fetch<void>(`/api/hub/sources/${encodeURIComponent(id)}?items=${items}`, {
        method: "DELETE",
        headers: workspaceHeader({ workspaceId }),
      });
    } catch (error) {
      throw mapHubSourceDeleteError(error);
    }
  }
  /** A 409 with code `quota_exceeded` rejects as HubQuotaExceededError. Other errors pass through. */
  async syncHubSource(workspaceId: string, id: string): Promise<HubSyncResult> {
    let raw: unknown;
    try {
      raw = await this.fetch<unknown>(`/api/hub/sources/${encodeURIComponent(id)}/sync`, {
        method: "POST",
        headers: workspaceHeader({ workspaceId }),
      });
    } catch (error) {
      throw mapHubSyncError(error);
    }
    return parseWithFallback<HubSyncResult>(raw, HubSyncResultSchema, EMPTY_HUB_SYNC_RESULT, {
      endpoint: "POST /api/hub/sources/{id}/sync",
    });
  }
  /** One item, in the same shape as a row of listHubItems. Rejects with the server's 404 when the item does not exist. */
  async getHubItem(workspaceId: string, kind: HubItemKind, refId: string): Promise<HubItemMeta> {
    const raw = await this.fetch<unknown>(this.hubItemPath(kind, refId), {
      headers: workspaceHeader({ workspaceId }),
    });
    const item = parseWithFallback<HubItemMeta>(raw, HubItemMetaSchema, EMPTY_HUB_ITEM_META, {
      endpoint: "GET /api/hub/items/{kind}/{id}",
    });
    if (!item.id) throw new Error("The item response has no id");
    return item;
  }
  async getHubUpdateCheck(workspaceId: string, kind: HubItemKind, refId: string): Promise<HubUpdateCheck> {
    const raw = await this.fetch<unknown>(`${this.hubItemPath(kind, refId)}/update`, {
      headers: workspaceHeader({ workspaceId }),
    });
    return parseWithFallback<HubUpdateCheck>(raw, HubUpdateCheckSchema, EMPTY_HUB_UPDATE_CHECK, {
      endpoint: "GET /api/hub/items/{kind}/{id}/update",
    });
  }
  /** One page of activity, newest first. Pass the previous page's `next_cursor` as `before`. */
  async listHubActivity(
    workspaceId: string,
    params: HubActivityFilter & { limit?: number; before?: string | null } = {},
  ): Promise<HubActivityPage> {
    const search = new URLSearchParams();
    if (params.kind) search.set("kind", params.kind);
    if (params.refId) search.set("ref_id", params.refId);
    if (params.sourceId) search.set("source_id", params.sourceId);
    if (params.limit !== undefined) search.set("limit", String(params.limit));
    if (params.before) search.set("before", params.before);
    const suffix = search.toString();
    const raw = await this.fetch<unknown>(`/api/hub/activity${suffix ? `?${suffix}` : ""}`, {
      headers: workspaceHeader({ workspaceId }),
    });
    return parseWithFallback<HubActivityPage>(raw, HubActivityPageSchema, EMPTY_HUB_ACTIVITY_PAGE, {
      endpoint: "GET /api/hub/activity",
    });
  }
  /**
   * The export zip, fetched with the standard auth headers. A bearer token
   * (desktop) cannot be sent by a plain link, so the caller saves these bytes.
   * Admin and owner only; the server answers 403 otherwise.
   */
  async getHubExportBlob(workspaceId: string): Promise<Blob> {
    const res = await this.fetchRaw("/api/hub/export", {
      headers: workspaceHeader({ workspaceId }),
    });
    return res.blob();
  }
  /**
   * Fetches a same-origin hub bundle download route with the client's auth
   * headers. Used when the store cannot presign and the bundle url is a
   * relative API path that a plain link could not authenticate.
   */
  async getHubBundleBlob(workspaceId: string, path: string): Promise<Blob> {
    const res = await this.fetchRaw(path, {
      headers: workspaceHeader({ workspaceId }),
    });
    return res.blob();
  }
  /** Questions queued for the caller when their squad's weekly attention share is spent (durable spec 7.5). */
  async getQueuedAttention(): Promise<QueuedAttentionList> {
    return parseQueuedAttention(await this.fetch<unknown>("/api/company/attention/queued"));
  }
}
