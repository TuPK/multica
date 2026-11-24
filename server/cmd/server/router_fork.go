package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// registerForkWorkspaceRoutes mounts the routes only this fork has, inside the
// workspace-scoped group of NewRouterWithOptions (router.go): auth, workspace rate limit and
// membership are already applied to r. Keeping them here means upstream edits to router.go
// do not conflict with them (docs/specs/upstream-sync/spec.md section 3.9).
// TestRouteTableMatchesGolden pins what is registered.
func registerForkWorkspaceRoutes(r chi.Router, h *handler.Handler, queries *db.Queries) {
	// Company knowledge base. Status and one-time provisioning are for
	// human owners and admins. Reading, searching and writing drafts
	// are open to members and agents. Publish, discard and delete are
	// for human owners and admins: a person curates what becomes the
	// company's record.
	r.Route("/api/knowledge", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(handler.RequireHumanActor)
			r.Use(middleware.RequireWorkspaceRole(queries, "owner", "admin"))
			r.Get("/", h.GetKnowledge)
			r.Post("/enable", h.EnableKnowledge)
			r.Post("/documents/{id}/publish", h.PublishKnowledgeDocument)
			r.Post("/documents/{id}/discard", h.DiscardKnowledgeDocument)
			r.Delete("/documents/{id}", h.DeleteKnowledgeDocument)
			// Taxonomy and eval administration: owners and admins.
			r.Patch("/kinds/{key}", h.UpdateKnowledgeKind)
			r.Put("/policies", h.SetKnowledgePolicy)
			r.Delete("/policies", h.DeleteKnowledgePolicy)
			r.Post("/eval/runs", h.RunKnowledgeEval)
			r.Delete("/eval-cases/{id}", h.DeleteKnowledgeEvalCase)
			// Sources, restricted-base access, per-base LLM switches and
			// the model connection test (plan 10.11 to 10.13).
			r.Get("/sources", h.ListKnowledgeSources)
			r.Get("/bases/{key}/access", h.GetKnowledgeBaseAccess)
			r.Put("/bases/{key}/visibility", h.SetKnowledgeBaseVisibility)
			r.Put("/bases/{key}/access/{subject_type}/{subject_id}", h.PutKnowledgeGrant)
			r.Delete("/bases/{key}/access/{subject_type}/{subject_id}", h.DeleteKnowledgeGrant)
			r.Patch("/bases/{key}/settings", h.PatchKnowledgeBaseSettings)
			r.Post("/bases/{key}/wiki/generate", h.GenerateKnowledgeWiki)
			r.Post("/models/test", h.TestKnowledgeModels)
			r.Post("/owner-account/reveal", h.RevealKnowledgeOwnerPassword)
			// The repo-docs sync setting and a run now (plan 10.12).
			r.Get("/repo-sync", h.GetKnowledgeRepoSync)
			r.Put("/repo-sync", h.UpdateKnowledgeRepoSync)
			r.Post("/repo-sync/run", h.RunKnowledgeRepoSync)
		})
		// A person uploads a file and it is published at once: the
		// person is the curator. Agents are refused by RequireHumanActor.
		r.With(handler.RequireHumanActor).Post("/documents/upload", h.UploadKnowledgeDocument)
		r.Get("/bases", h.ListKnowledgeBases)
		r.Get("/bases/{key}/wiki", h.GetKnowledgeWiki)
		r.Get("/bases/{key}/wiki/pages", h.ListKnowledgeWikiPages)
		r.Get("/bases/{key}/wiki/pages/*", h.GetKnowledgeWikiPage)
		r.Get("/documents", h.ListKnowledgeDocuments)
		r.Post("/documents", h.CreateKnowledgeDocument)
		r.Patch("/documents/{id}/properties", h.UpdateKnowledgeDocumentProperties)
		r.Get("/collections", h.ListKnowledgeCollection)
		r.Post("/documents/{id}/review", h.ReviewKnowledgeDocument)
		r.Get("/search", h.SearchKnowledge)
		r.Get("/kinds", h.ListKnowledgeKinds)
		r.Get("/health", h.GetKnowledgeHealth)
		r.Get("/policies", h.ListKnowledgePolicies)
		r.Get("/eval-cases", h.ListKnowledgeEvalCases)
		r.Post("/eval-cases", h.CreateKnowledgeEvalCase)
		r.Get("/eval/runs", h.ListKnowledgeEvalRuns)
	})

	// Pipeline and initiatives (spec 3.1). Decisions on a stage gate go through
	// /api/proposals; the routes here move an initiative directly.
	r.Get("/api/pipelines", h.ListPipelines)
	r.Route("/api/initiatives", func(r chi.Router) {
		r.Get("/", h.ListInitiatives)
		r.Post("/", h.CreateInitiative)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetInitiative)
			r.Patch("/", h.UpdateInitiative)
			r.Post("/advance", h.AdvanceInitiative)
			r.Post("/attest", h.AttestInitiative)
			r.Post("/send-back", h.SendBackInitiative)
			r.Post("/shelve", h.ShelveInitiative)
			r.Post("/resume", h.ResumeInitiative)
			r.Post("/close", h.CloseInitiative)
			r.Post("/force", h.ForceInitiative)
			r.Post("/artifacts/{kind}/waive", h.WaiveInitiativeArtifact)
		})
	})

	// Metrics and goals (spec 4.1). Reads are for every member. A reading is
	// reported by a member or an agent, or by a member's token from outside
	// (source "webhook"); the report is rate-limited per workspace and metric.
	// Goals are written by the boss and admins; a squad's goal comes from a
	// goal proposal.
	r.Get("/api/metrics", h.ListMetrics)
	r.Get("/api/metrics/{key}/readings", h.ListMetricReadings)
	r.Post("/api/metrics/{key}/readings", h.ReportMetricReading)
	// Workspace metric tokens for outside feeds (spec 4.1). Minting and
	// revoking are for people, and for owners and admins.
	r.Route("/api/metric-tokens", func(r chi.Router) {
		r.Use(handler.RequireHumanActor)
		r.Get("/", h.ListMetricWebhookTokens)
		r.Post("/", h.CreateMetricWebhookToken)
		r.Delete("/{id}", h.RevokeMetricWebhookToken)
	})
	r.Route("/api/goals", func(r chi.Router) {
		r.Get("/", h.ListGoals)
		r.Post("/", h.CreateGoal)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetGoal)
			r.Patch("/", h.UpdateGoal)
			r.Delete("/", h.DeleteGoal)
			r.Post("/activate", h.ActivateGoal)
			r.Post("/status", h.SetGoalStatus)
			r.Put("/confidence", h.SetGoalConfidence)
		})
	})

	// Proposals: a squad's request for a decision by the boss. Decisions
	// are refused to agent actors inside the handler.
	r.Route("/api/proposals", func(r chi.Router) {
		r.Get("/", h.ListProposals)
		r.Post("/", h.CreateProposal)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetProposal)
			r.Get("/revisions", h.ListProposalRevisions)
			r.Post("/decision", h.DecideProposal)
			r.Post("/revision", h.ReviseProposal)
		})
	})
	// Publishing channels (Content Studio): the accounts an approved post goes out on.
	r.Route("/api/publish-channels", func(r chi.Router) {
		r.Get("/", h.ListPublishChannels)
		r.Post("/", h.CreatePublishChannel)
		// Workspace publishing keys. Static, so they win over the {id} routes below.
		r.Get("/settings", h.GetPublishSettings)
		r.Put("/settings", h.PutPublishSettings)
		r.Route("/{id}", func(r chi.Router) {
			r.Delete("/", h.DeletePublishChannel)
			r.Post("/connect", h.ConnectPublishChannel)
			r.Post("/check", h.CheckPublishChannel)
			r.Post("/dry-run", h.DryRunPublishChannel)
		})
	})
	// Research Lab source settings: the OpenAlex and Semantic Scholar keys. Members read what is
	// configured (never a key); owners and admins change it; members and agents fetch the keys to run
	// `multica research`.
	r.Route("/api/research", func(r chi.Router) {
		r.Get("/settings", h.GetResearchSettings)
		r.Put("/settings", h.UpdateResearchSettings)
		r.Get("/credentials", h.GetResearchCredentials)
	})
	// Standing approvals (spec 4.4). Reads for members; creating and revoking
	// are for owners and admins, decided in the handler. Flat registrations,
	// as the company endpoints below are.
	r.Get("/api/approval-policies", h.ListApprovalPolicies)
	r.Post("/api/approval-policies", h.CreateApprovalPolicy)
	r.Post("/api/approval-policies/{id}/revoke", h.RevokeApprovalPolicy)
	r.Get("/api/approval-policies/{id}/auto-approvals", h.ListPolicyAutoApprovals)
	// Digital company: floors (owner/admin) and squad seats.
	// Flat registrations on purpose: other company endpoints (digest,
	// briefs) live under the same prefix, and a second r.Route on it
	// would panic at startup.
	r.Get("/api/company/floors", h.ListCompanyFloors)
	r.Post("/api/company/floors", h.CreateCompanyFloor)
	r.Put("/api/company/floors/{id}", h.UpdateCompanyFloor)
	r.Delete("/api/company/floors/{id}", h.DeleteCompanyFloor)
	r.Put("/api/company/seats", h.PutCompanySeats)
	// The Secretary's digest: facts about the company, any member.
	r.Get("/api/company/digest", h.GetCompanyDigest)
	// The org map: floors, chiefs, squads, leaders, members and open work. Any member.
	r.Get("/api/company/org-map", h.GetCompanyOrgMap)
	r.Get("/api/company/requests/{id}/usage", h.GetCompanyRequestUsage)
	// Floor chiefs (spec 4.8): one agent per floor publishes its brief. Reads
	// for members; changes for owners and admins, decided in the handler.
	r.Get("/api/company/floors/{id}/chief", h.GetFloorChief)
	r.Put("/api/company/floors/{id}/chief", h.SetFloorChief)
	r.Delete("/api/company/floors/{id}/chief", h.ClearFloorChief)
	// The digest source (auto, squads or floors) and the company pulse strip.
	r.Get("/api/company/digest-mode", h.GetDigestMode)
	r.Put("/api/company/digest-mode", h.SetDigestMode)
	r.Get("/api/company/pulse", h.GetCompanyPulse)
	// Company as code (spec 4.9): the structure as a document, and its import.
	r.Get("/api/company/export", h.ExportCompany)
	r.Post("/api/company/import", h.ImportCompany)
	// The Secretary setup and its readiness checklist (owner or admin).
	// Setup is idempotent: a second POST changes nothing.
	r.Get("/api/company/setup", h.GetCompanySetup)
	r.Post("/api/company/setup", h.SetupCompany)
	r.Post("/api/company/company-template/apply", h.ApplyCompanyTemplate)
	// Company controls (spec 4.5): the state and quiet hours. Reads for
	// members; changes for owners and admins, decided in the handler.
	r.Get("/api/company/company-state", h.GetCompanyState)
	r.Put("/api/company/company-state", h.SetCompanyState)
	r.Put("/api/company/quiet-hours", h.SetQuietHours)
	// The governance log (spec 4.5). Read-only: no route writes an entry.
	r.Get("/api/governance/events", h.ListGovernanceEvents)
	r.Get("/api/governance/events.csv", h.ExportGovernanceEvents)
	// Budgets (spec 4.2). Reads for members; writes are decided in the handler.
	r.Get("/api/budgets", h.ListBudgets)
	r.Post("/api/budgets", h.CreateBudget)
	r.Patch("/api/budgets/{id}", h.UpdateBudget)
	r.Post("/api/budgets/{id}/extend", h.ExtendBudget)
	r.Post("/api/budgets/{id}/release-holds", h.ReleaseBudgetHolds)
	r.Get("/api/budgets/{id}/periods", h.ListBudgetPeriods)
	r.Get("/api/budget-holds", h.ListBudgetHolds)
	// Scorecards and reviews (spec 4.6). Reads are limited to the owners,
	// admins and the subject's leader; POST /api/reviews is the reviewer's.
	r.Get("/api/scorecards", h.ListScorecards)
	r.Route("/api/reviews", func(r chi.Router) {
		r.Get("/", h.ListReviews)
		r.Post("/", h.CreateReview)
		r.Get("/{id}", h.GetReview)
	})
	// System health (spec 4.9): job lag, deliveries, WeKnora and the budget gate.
	r.Get("/api/governance/health", h.GetGovernanceHealth)
	// The caller's brief push to a chat app (plan 9.10). Member scope only:
	// the handler reads and writes the caller's own row and refuses agents.
	r.Get("/api/company/brief-push", h.GetBriefPushPreference)
	r.Put("/api/company/brief-push", h.UpdateBriefPushPreference)
	// The chats the caller can pick for that preference, with labels.
	r.Get("/api/company/brief-push/bindings", h.ListBriefPushBindings)
	// Briefs: the Secretary's reports to the boss. Who may publish is
	// decided in the handler (secretary agent or admin member).
	r.Route("/api/briefs", func(r chi.Router) {
		r.Get("/", h.ListBriefs)
		r.Post("/", h.CreateBrief)
		// The Secretary's accounted questions to squad leaders (daily cap).
		r.Post("/inquiries", h.AskBriefInquiry)
		// The boss's ratings of briefs (plan 9.12). The reads are the Secretary's
		// and an admin's; the rating itself is the caller's own.
		r.Get("/feedback/recent", h.ListBriefFeedbackRecent)
		r.Get("/feedback/summary", h.GetBriefFeedbackSummary)
		r.Get("/{id}", h.GetBrief)
		r.Get("/{id}/feedback", h.GetMyBriefFeedback)
		r.Post("/{id}/feedback", h.SubmitBriefFeedback)
	})

	// Skill / plugin hub sources and the import preview/commit flow.
	r.Route("/api/hub", func(r chi.Router) {
		r.Get("/sources", h.ListHubSources)
		r.Get("/items", h.ListHubItems)
		r.Route("/items/{kind}/{id}", func(r chi.Router) {
			r.Get("/", h.GetHubItem)
			r.Put("/files", h.SaveHubItemFile)
			r.Get("/bundle", h.GetHubItemBundle)
			r.Get("/bundle/download", h.DownloadHubItemBundle)
			r.Get("/update", h.GetHubItemUpdate)
			r.Get("/update/diff", h.GetHubItemUpdateDiff)
			r.Post("/update/apply", h.ApplyHubItemUpdate)
			r.Post("/fork", h.ForkHubItem)
		})
		r.Get("/activity", h.GetHubActivity)
		r.Get("/quota", h.GetHubQuota)
		r.Get("/export", h.ExportHubItems)
		r.Post("/import/preview", h.PreviewHubImport)
		r.Post("/import/commit", h.CommitHubImport)
		r.Post("/sources", h.CreateHubSource)
		r.Route("/sources/{id}", func(r chi.Router) {
			r.Patch("/", h.UpdateHubSource)
			r.Delete("/", h.DeleteHubSource)
			r.Post("/sync", h.SyncHubSource)
		})
	})
}
