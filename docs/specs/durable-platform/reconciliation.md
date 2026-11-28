# Đối chiếu plan.md với code, test và commit

Ngày: 2026-11-28. Nhánh `durable-platform`, HEAD `33a0ce26ea`, bằng `origin/durable-platform` (0 commit lệch).

Đối chiếu 126 checkbox chưa tick và bảng "Trạng thái thực tế" của [plan.md](plan.md) với cây hiện tại. Báo cáo này không sửa plan.md; người điều phối dùng mục 4 để cập nhật.

## 0. Điều kiện đối chiếu

- Plan ghi tip `1811940ade`, đã cũ. Tip hiện tại là `33a0ce26ea`.
- Cây làm việc có thay đổi chưa commit ngoài phạm vi báo cáo: `docs/specs/durable-platform/plan.md` (dòng 63, E2, của phiên E2) và nhiều file `server/internal/handler/*_test.go`. Báo cáo không đụng tới các file này.
- Không chạy `go test`, `vitest`, Playwright, `make check`, `pnpm sync:*`, `pnpm test:scripts`. Không dùng DB nào.
- Cách xác nhận: tên test tìm bằng `func TestX(` trong `server/` và tên trong `packages/`, `e2e/`; file kiểm bằng `find`; commit kiểm bằng `git log`.
- Ý nghĩa mức **Có**: code, test hoặc commit tồn tại ở HEAD. Không có nghĩa là đang xanh. Mọi test Go có DB, vitest và Playwright đều chưa chạy lại trong phiên này.
- Mức **Thiếu**: không tìm thấy. Mức **Cần người xác nhận**: GIF, trải nghiệm thật, bật cờ trên máy chủ dev.

## 1. Tóm tắt

| Mức | Số checkbox |
|---|---|
| Có | 110 |
| Thiếu | 9 |
| Cần người xác nhận | 7 |
| **Tổng** | **126** |

Đã tick sẵn 5 checkbox (B5 ×2, D5 ×3), đúng với code.

Điểm chính:

- Thiếu thật: C6 (e2e F5/F6), D7 (`e2e/durable/shadow.spec.ts` không có), B3 (test khối khai báo trong briefing), B2 (hai nguồn hệ quả chưa có hook), F1 (không có bằng chứng chạy, không có `rollout.md`), F2 (chưa làm).
- Bảng trạng thái có nhiều dòng lỗi thời, chi tiết ở mục 3. Đáng chú ý: B5 cite sai commit và nói chưa push; C1 nói thiếu FE; E1, E3, E5 nói thiếu test hoặc UI đã có; E4 nói thiếu route đã có.
- C2 có test `TestDeleteProfileRemovesItsRows` (tên khác với plan `TestDeleteProfile`), nên không thiếu.

## 2. Đối chiếu từng checkbox

Cột "Dòng" là số dòng trong plan.md. Mức: Có / Thiếu / Cần người xác nhận.

### P0 (14 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 78 | Chạy 5 lệnh nền, lưu kết quả | Có | `docs/specs/durable-platform/p0-findings.md` mục "Lệnh và kết quả": 5 lệnh đều xanh. Lệnh Go chạy trên DB `multica_p0_test` |
| 79 | Ghi mục đỏ sẵn | Có | p0-findings mục "Đỏ sẵn ở HEAD": không có lệnh đỏ sau khi dùng DB đúng |
| 85 | U1: hàm ghi `issue.status` | Có | p0-findings U1 (24 điểm ghi, không có hàm chung) |
| 86 | U2: activity_log và backfill | Có | p0-findings U2 |
| 87 | U3: gateway connectedapps | Có | p0-findings U3 |
| 88 | U4, U5, U6: daemon claim, thư mục tạm, payload | Có | p0-findings U4–U6 |
| 89 | U7: manifest flow | Có | p0-findings U7 |
| 90 | U8: sự kiện revert | Có | p0-findings U8 |
| 91 | U9: runtime tuỳ chỉnh | Có | p0-findings U9 |
| 92 | U10: kho bí mật | Có | p0-findings U10 |
| 93 | Sửa spec nếu giả định sai | Có | spec mục 0 có D26–D31 (`docs/specs/durable-platform/spec.md:61-66`); mục 15 có "Giả định đã kiểm ở P0" (spec.md:1247) |
| 105 | Test đỏ: cờ mặc định tắt và phụ thuộc | Có | `TestDurableFlagsDefaultOff`, `TestDurableFlagDependencies` (`server/internal/featureflags/keys_fork_test.go`); `packages/core/durable/flags.test.ts` |
| 106 | Cài đặt cờ; go test featureflags xanh | Có | `server/internal/featureflags/keys_fork.go:16-41`: 5 khoá, mặc định tắt, bảng phụ thuộc khớp spec 14.1. Chưa chạy lại |
| 107 | Commit `feat(durable): feature flags for the durable platform program` | Có | `55c13fcb3a` (đúng tiêu đề) |

### A1–A9 (42 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 123 | Test đỏ: TestEveryStatusWriterRecordsOneTransition, TestStatusTriggerCatchesRawSQL, TestEveryWriterSetsActorSession | Có | `server/internal/service/durable_transition_fork_test.go` |
| 124 | TestTransitionNotRecordedWhenFlagOff | Có | cùng file |
| 125 | TestReopenDetected | Có | cùng file |
| 126 | Migration, query, sqlc, hook | Có | migration `1041`–`1044` (1044 là trigger ghi transition), `server/pkg/db/queries/durable_transition.sql`, `server/internal/service/durable_transition_fork.go`, đăng ký ở `server/cmd/migrate/main.go` |
| 127 | `go test ./cmd/migrate ./internal/migrations` xanh | Có | `server/internal/migrations/durable_append_only_test.go`. Chưa chạy lại |
| 128 | Commit `feat(durable): record issue status transitions` | Có | `7e6afb8166` (đúng tiêu đề) |
| 141 | TestWorkClassPriority | Có | `server/internal/service/durable_work_class_fork_test.go` |
| 142 | TestDecideVerdictTable | Có | `server/internal/service/durable_outcome_fork_test.go` |
| 143 | TestOutcomeLifecycle | Có | `server/internal/service/durable_outcome_lifecycle_fork_test.go` |
| 144 | TestSettleJobSettlesOnlyDue, TestOutcomeFlagOffStopsWrites | Có | cùng file lifecycle |
| 145 | TestOutcomeVerificationAppendOnly | Có | `server/internal/migrations/durable_append_only_test.go` |
| 146 | Commit `feat(durable): work outcomes and their lifecycle` | Có | `d01a66379c` (đúng tiêu đề) |
| 159 | TestVerifierNeverDoer, TestNoEligibleVerifierYieldsUnable | Có | `server/internal/service/durable_verification_fork_test.go` |
| 160 | TestImplicitCriterionWhenNone | Có | cùng file |
| 161 | TestArtifactCriterionPassesOnPublish, TestMetricCriterionFailsAfterWindow | Có | cùng file |
| 162 | TestVerificationRunClassHeldByBudget | Có | cùng file; `heldRunClasses()` gọi ở `server/internal/service/budget_gate.go:82` |
| 163 | Test handler: quyền, outcome_settled, override | Có | `server/internal/handler/durable_outcome_fork_test.go` |
| 164 | Test CLI: exit code và thân | Có | `server/cmd/multica/cmd_outcome_fork_test.go` |
| 165 | Trường outcomeCriteria, test catalog, TestSquadTemplateCriteriaApplied | Có | `scripts/catalog/lib/validate.mjs`, `scripts/catalog/catalog.test.mjs` có `outcomeCriteria`; `TestSquadTemplateCriteriaApplied` trong `durable_verification_fork_test.go`. `pnpm catalog:check` chưa chạy |
| 166 | Commit `feat(durable): acceptance criteria, verification tasks and outcome API` | Có | `97506a07ba` (đúng tiêu đề) |
| 178 | TestDoerBriefingListsCriteria, TestVerifierBriefingHidesOtherVerdicts, TestBriefingEmptyWhenFlagOff | Có | `server/internal/service/durable_briefing_fork_test.go` |
| 179 | TestSettledFailPostsSystemComment | Có | `durable_verification_fork_test.go` |
| 180 | Commit `feat(durable): outcome briefing blocks` | Có | `cdea3270db` (đúng tiêu đề). Ghi chú: plan yêu cầu hook ở `daemon*.go`; lời gọi `DurableBriefingBlocks` chỉ thấy ở `server/internal/handler/durable_outcome_fork.go`. Cần xác nhận vị trí hook |
| 191 | TestScorecardUsesDoneTransition, TestReworkRateFromTransitions, TestScorecardUnchangedWhenFlagOff | Có | `server/internal/service/scorecard_metrics_fork_test.go`; hook `durableScorecardMetrics` trong `scorecard_metrics.go` |
| 192 | Commit `feat(durable): real cycle time and rework in scorecards` | Có | `49a5ab2903` (đúng tiêu đề) |
| 204 | TestWilsonLowerKnownValues | Có | `server/internal/service/durable_stats_fork_test.go` |
| 205 | TestModeTransitions | Có | cùng file |
| 206 | TestSampledDeterministic | Có | cùng file; `Sampled` được gọi ở `server/internal/service/durable_outcome_store_fork.go:355` |
| 207 | TestHighFailRecomputesImmediately, TestOutcomeKeepsModelAtCreation | Có | `server/internal/service/durable_track_record_fork_test.go` |
| 208 | Nối ModeFor và Sampled vào OnEnteredDone | Có | `ModeFor` và `Sampled` gọi ở `durable_outcome_store_fork.go:338, 355` |
| 209 | Route pin: agent 403; event verification_mode_changed | Có | route `PUT /api/track-records/pin` (`server/cmd/server/router_fork.go:336`); `GovernanceKindVerificationMode = "verification_mode_changed"` (`server/internal/service/governance_log.go:44`). Test 403 chưa đọc |
| 210 | Commit `feat(durable): track records and verification modes` | Có | `cc88a87195` (đúng tiêu đề) |
| 223 | TestNextSquadLevelTable | Có | `server/internal/service/durable_envelope_fork_test.go` |
| 224 | TestNoEnvelopeNoAutoChange, TestHighFailDropsSquadNow, TestEnvelopeAgentForbidden, TestAutoChangeLogsOneEvent | Có | cùng file (service và handler) |
| 225 | Commit `feat(durable): evidence-driven autonomy within an envelope` | Có | `19164b8e8b` (đúng tiêu đề). Ghi chú: hook một dòng ở `server/internal/service/autonomy.go` không thấy; `governanceActorTypes` → `durableActorTypes()` thì có (`server/internal/handler/governance.go:26`) |
| 238 | Test core: schema, verdict lạ, phản hồi thiếu | Có | `packages/core/durable/outcomes/schemas.test.ts`, `track-records/schemas.test.ts`; fallback `unverified` trong `outcomes/schemas.ts` |
| 239 | Test view: khối Nghiệm thu, tab, dialog envelope | Có | `packages/views/durable/outcomes/issue-acceptance-panel.test.tsx`, `governance-outcomes-tab.test.tsx`, `envelope-dialog.test.tsx`, `agent-track-record-tab.test.tsx`. Nội dung assertion chưa đọc |
| 240 | typecheck, lint, vitest | Có | Không chạy lại |
| 241 | GIF và `scripts/motion/capture.mjs` cho khối Nghiệm thu | Cần người xác nhận | Không có GIF trong repo. Có kịch bản `scripts/motion/acceptance-block.mjs` (commit `7d89ad79cf`) |
| 242 | Commit `feat(durable): outcome and track record UI` | Có | `2e013afc79` (đúng tiêu đề) |
| 248 | Kịch bản 17.2 mục 1 qua TestApiClient và route test | Có | `e2e/durable/outcomes.spec.ts:98`; gọi `POST /api/test/durable/jobs` |
| 249 | Playwright xanh; commit `test(durable): outcome week e2e` | Có | `708c734647` (đúng tiêu đề). Chưa chạy lại |

### B1–B7 (20 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 265 | TestDecideEffectFullTable | Có | `server/internal/service/effect_policy_fork_test.go` |
| 266 | TestOverrideRules | Có | cùng file |
| 267 | TestStricterWithRehearse | Có | `server/internal/connectedapps/effect_fork_test.go`; `strictnessFork` trong `policy.go` |
| 268 | Commit `feat(durable): effect classes, reversibility and policy` | Có | `9e4ff82f5b` (đúng tiêu đề) |
| 281 | TestEachSourceLogsOneEvent (sáu nguồn) | Thiếu | Test có (`server/internal/service/durable_effect_log_fork_test.go`). Hook một dòng thấy ở bốn nguồn: gateway (`server/internal/connectedapps/effect_log_fork.go`, `service.go`), publish (`server/internal/publish/effect_fork.go`), kênh chat (`server/internal/integrations/channel/engine/effect_fork.go`). Không thấy hook ở Action API (`server/internal/handler/plugin_action.go`). Không có file `webhook_delivery_worker`. CLI middleware chỉ có `durable_shadow_fork.go` (403 shadow) |
| 282 | TestShadowTaskGatewayWriteBlocked | Có | `durable_effect_log_fork_test.go` |
| 283 | TestEffectEventNoDelete | Có | cùng file |
| 284 | BenchmarkGate; đếm truy vấn | Có | `BenchmarkGate` (`durable_effect_log_fork_test.go:380`); `TestGateReadBudget`; commit `222cc89215`, `6c4f86d5ab` |
| 285 | Commit `feat(durable): effect log across platform sources` | Có | Tiêu đề khác. Gộp trong `23d1a16920` và `f64b18d642` |
| 296 | TestDeclareExitCodes, TestUndoCallsInverseTool, TestUndoAfterWindow409, TestUndoInternalRevisionKinds | Có | `server/cmd/multica/cmd_effect_fork_test.go`, `server/internal/handler/durable_effect_fork_test.go` |
| 297 | Test briefing: khối khai báo có khi cờ bật | Thiếu | Không tìm thấy test về khối khai báo trong `durable_briefing_fork_test.go`. Có commit `78e2ae6a11` (declare outside actions in agent briefings) |
| 298 | Commit `feat(durable): declared effects and undo` | Có | Gộp trong `23d1a16920` |
| 311 | Bảy test ý định | Có | `server/internal/handler/connected_apps_intent_fork_test.go`, `server/internal/service/durable_intent_fork_test.go`: TestRehearseDoesNotCallGateway, TestApproveExecutesExactCall, TestApproveIntentConcurrentlyExecutesOnce, TestEditedApproveRehashesAndComments, TestExpireWakesAgentOnce, TestBatchApproveFive, TestIntentFlagOffStopsWrites |
| 312 | Test preview | Có | `server/internal/service/durable_preview_fork_test.go` (tên case chưa đọc) |
| 313 | Commit | Có | Gộp trong `f64b18d642`, `23d1a16920` |
| 331 | Test view: thẻ ý định, sửa rồi duyệt, toast sau unmount, lưới, tab | Có | `packages/views/durable/effects/intent-card.test.tsx`, `effect-policy-grid.test.tsx`, `governance-effects-tab.test.tsx`, `intent-batch-card.test.tsx` |
| 332 | Wire web và desktop; vitest; GIF thẻ ý định | Cần người xác nhận | Không có GIF trong repo. Vitest chưa chạy |
| 333 | Commit `feat(durable): effects and intents UI` | Có | `df4db55403` (đúng tiêu đề) |
| 339 | Phần F3, F4 kịch bản 17.2 mục 2 | Có | `e2e/durable/effects.spec.ts` dòng 174–538 |
| 340 | Commit `test(durable): rehearsed email e2e` | Có | `95f1a17212` (đúng tiêu đề) |

### C1–C6 (15 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 356 | TestEstimateMinutesTable | Có | `server/internal/service/durable_attention_fork_test.go` |
| 357 | TestSquadShareExhaustedQueues, TestUrgentNotQueued, TestDelegatedCostsNothing, TestAttentionFlagOffNoLedger | Có | cùng file |
| 358 | TestAgentAskRequiresReason | Có | cùng file |
| 359 | Commit `feat(durable): attention budget and ask ledger` | Có | `0e5eb7f868` (đúng tiêu đề) |
| 371 | TraitRequiresOwnEvidence, EditedTraitNotRetiredByJob, ProfileReadAccess, DeleteProfile, ReviewJobsNeverReadProfile | Có | `server/internal/service/durable_principal_fork_test.go`. `DeleteProfile` có tên thật `TestDeleteProfileRemovesItsRows` (dòng 338) |
| 372 | Test phỏng vấn 8 câu | Có | `TestInterviewAnswerWritesItsMessage`, `TestInterviewAnswerWithoutProfileWritesNothing`, `TestInterviewAnswerNeedsASecretary` (cùng file, dòng 427+). Số 8 câu chưa đếm |
| 373 | Commit `feat(durable): principal profile with consent` | Có | `dd1e710ff1` (đúng tiêu đề) |
| 385 | TestNoProfileNoPrediction, TestPredictionResolvedOnDecision, TestAccuracyLast50, TestNoPredictionAboutSelf, TestProfileUpdateJobBatches | Có | `durable_prediction_fork_test.go` (service và handler) |
| 386 | Commit `feat(durable): decision predictions and profile learning` | Có | `064ee650c6` (đúng tiêu đề) |
| 398 | TestEnableRequires20And85, TestForbiddenKindsNeverDelegated, TestDelegatedDecisionLogged, TestAutoDisableOnTwoReverts, TestAutoDisableBelow80, TestOnlyProfileOwnerEnables | Có | `server/internal/service/durable_delegation_fork_test.go` |
| 399 | Commit `feat(durable): delegation by prediction` | Có | `6e6550905e` (đúng tiêu đề) |
| 408 | Test view: consent, trait, bảng uỷ quyền, đồng hồ phút, dự đoán | Có | `packages/views/durable/principal/consent-dialog.test.tsx`, `trait-list.test.tsx`, `delegation-panel.test.tsx`, `prediction-line.test.tsx`; đồng hồ phút ở `packages/views/governance/attention-budget-section.tsx`. Đường dẫn `packages/views/durable/attention/` trong plan không tồn tại |
| 409 | Wire; typecheck; GIF bảng uỷ quyền | Cần người xác nhận | Không có GIF |
| 410 | Commit `feat(durable): principal profile and attention UI` | Có | Tiêu đề khác: `636a462f53` (principal, delegation và attention queue UI), `468359e820` |
| 416 | Phần F5, F6 của e2e (dự đoán, ledger 0.5 phút) | Thiếu | `e2e/durable/effects.spec.ts` chỉ có F3–F4, không có dự đoán hay attention. Không có spec khác |

### D1–D7 (16 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 432 | TestPickJurorsDiversity | Có | `server/internal/service/durable_jury_fork_test.go` |
| 433 | TestJuryUnanimous, TestJurySplitCreatesOneAsk, TestJurorUnableReplacedOnce, TestJuryTimeout24h, TestJuryWithOneAgentEscalates, TestJurorTrackRecordOnOverrule | Có | cùng file |
| 434 | Commit `feat(durable): cross-model jury` | Có | `32c70f0932` (đúng tiêu đề) |
| 446 | Hook retro: lesson task sau retrospective | Có | `server/internal/service/retro.go:392` gọi `afterFireDossier`; `TestLessonTaskAfterRetro` (`server/internal/service/retro_dossier_fork_test.go`) |
| 447 | TestSelectDossierLimitAndOrder, TestLessonTaskOnFailOnly, TestMax100Active, TestMemberEntryOnlyMemberRetires, TestModelChangeLogsEvent | Có | `server/internal/service/durable_dossier_fork_test.go` |
| 448 | Commit `feat(durable): seat dossier` | Có | `6792c5517d` (đúng tiêu đề) |
| 461 | Chín test shadow | Có | `server/internal/service/durable_shadow_fork_test.go`, `durable_shadow_claim_fork_test.go`, `durable_shadow_judge_fork_test.go`; `server/internal/middleware/durable_shadow_fork_test.go` (403); `connected_apps_intent_fork_test.go` (gateway) |
| 462 | Test daemon: TestShadowClaimRequiresCapability | Có | `server/internal/daemon/durable_shadow_fork_test.go`; `DaemonCapabilityShadowV1` trong `server/internal/daemon/client.go`. Tên TestShadowClaimRequiresCapability nằm ở service |
| 463 | Commit `feat(durable): shadow runs with isolation` | Có | `4b33d6cab1` (đúng tiêu đề) |
| 475 | TestPromotionReadyTable, TestCompletedCreatesProposal, TestAutoPromoteAppliesAndProbation, TestProbationWorseReverts, TestAutoPromoteOwnerOnly | Có | `server/internal/service/durable_runtime_change_fork_test.go` |
| 476 | Commit `feat(durable): runtime change after shadow trial` | Có | `fd9433b181` (đúng tiêu đề) |
| 499 | Test view: dialog shadow, báo cáo, thẻ split, hồ sơ | Có | `packages/views/durable/seats/shadow-trial-dialog.test.tsx`, `shadow-report.test.tsx`, `seat-dossier-tab.test.tsx`, `necessity-tests.test.tsx`; `packages/views/durable/outcomes/jury-split-card.test.tsx` |
| 500 | Wire; GIF báo cáo shadow | Cần người xác nhận | Không có GIF |
| 501 | Commit `feat(durable): seats, jury and shadow UI` | Có | Tiêu đề khác: `87cd169b47` (seat dossier and shadow trial UI), `66c2b4c8bb` |
| 507 | Kịch bản 17.2 mục 3 trong `e2e/durable/shadow.spec.ts` | Thiếu | File không tồn tại. `effects.spec.ts` có hai case shadow bị chặn (dòng 404, 472), nhưng không có kịch bản 20 cặp và `runtime_change` |
| 508 | Chạy; commit `test(durable): shadow trial e2e` | Thiếu | Không có commit với tiêu đề này |

Bảng D5 đã tick sẵn (3 checkbox): đối chiếu với `f951a1b1a4` (`feat(durable): necessity tests for scaffolding`) và tên test `TestNewcombeKnownValues`, `TestNecessityVerdictTable`, `TestShadowOmitsSkillUnderTest`. Đúng.

### E1–E5 (14 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 524 | TestReachStateTable, TestSnapshotIdempotentPerDay, TestCloseGapCreatesIssueWithMetricCriterion, TestCloseGapTwiceOpensExisting, TestReachRetention400Days | Có | `server/internal/service/durable_reach_fork_test.go`. TestCloseGapCreatesIssueWithMetricCriterion có sẵn, bảng cũ ghi là thiếu |
| 525 | Commit `feat(durable): reach map` | Có | Gộp trong `c9d56fdd90` (tiêu đề khác) |
| 537 | Bảy test export/import OCF | Có | `server/internal/service/durable_ocf_fork_test.go` (TestExportHistoryValidatesAgainstSchema … TestEmbeddedSchemaMatchesDocs) |
| 538 | Adapter `server/cmd/multica-ocf-adapter/` và test với worker giả | Có | `main.go`, `adapter_test.go`, `adapter_pi_test.go`; commit `8383b12a8d`. Test handler OCF đang đỏ do DB dùng chung (theo bảng cũ), chưa chạy lại |
| 539 | Commit `feat(durable): open company format v1` | Có | `03dd039236` (đúng tiêu đề), kèm `c2f4387b6f` (docs) |
| 550 | TestCanonicalizeRFC8785Vectors, TestSignVerifyRoundTrip, TestTamperFails, TestRotatedKeyStillVerifies, TestForeignKeyRejected | Có | `server/internal/service/durable_jcs_fork_test.go`, `durable_attestation_fork_test.go` |
| 551 | TestIssueRequires30, TestWorkClassCoarsenedToArea, TestWorkspaceHiddenByDefault, TestVerifyRateLimited | Có | `durable_attestation_fork_test.go`; `TestVerifyRateLimited` ở `server/cmd/server/durable_attestation_route_fork_test.go` |
| 552 | TestIssueDisabledWithoutKey | Có | `durable_attestation_fork_test.go`; mã `attestation_disabled` trong `server/internal/service/durable_attestation_fork.go` và `server/internal/handler/durable_attestation_fork.go` |
| 553 | Commit `feat(durable): signed track record attestations` | Có | Gộp trong `c9d56fdd90`; bổ sung `361add799b` (malformed docs) |
| 566 | TestHireFromOfferFilesHireProposalWithAttestation, TestHiredSeatStartsFull, TestInvalidAttestationShownAsUnverified, TestOfferScopedToHubOrg | Cần người xác nhận | Tên test có trong `server/internal/service/durable_seat_offer_fork_test.go`. Bảng cũ ghi chưa có envelope theo ghế, nên cần đọc assertion của `TestHiredSeatStartsFull`. Route `/api/hub/seat-offers` đã có (`server/cmd/server/router_fork.go:283`) |
| 567 | Commit `feat(durable): seat offers on the hub` | Có | Tiêu đề khác: `3acacaf3ed`, `10e0c1769b` |
| 576 | Test view: lưới Reach, Lấp khoảng trống, huy hiệu, dialog export | Có | `packages/views/durable/reach/governance-reach-tab.test.tsx`, `reach-surface-detail.test.tsx`; `packages/views/durable/open/attestation-badge.test.tsx`, `export-history-dialog.test.tsx` |
| 577 | Wire; GIF lưới Reach | Cần người xác nhận | Không có GIF |
| 578 | Commit `feat(durable): reach map, export and attestation UI` | Có | `9b8cbf0ba6` (đúng tiêu đề) |

### F1–F2 (5 checkbox)

| Dòng | Checkbox | Mức | Bằng chứng |
|---|---|---|---|
| 586 | make check; playwright e2e durable; sync:contract; sync:coverage; test:scripts | Thiếu | Không có bằng chứng chạy. e2e durable chưa đủ (thiếu shadow.spec.ts và C6) |
| 587 | /security-review; sửa trong commit riêng | Thiếu | Không có commit hay báo cáo security |
| 588 | `docs/specs/durable-platform/rollout.md` | Thiếu | File không tồn tại |
| 594 | Bật cờ trên máy chủ dev cho workspace `tupk` (cần đồng ý người dùng) | Cần người xác nhận | Thay đổi môi trường thật; cần người dùng đồng ý |
| 595 | Đo 7 ngày; PR bật mặc định | Thiếu | Chưa có dữ liệu; phụ thuộc 594 |

## 3. Đối chiếu bảng "Trạng thái thực tế"

| Task | Bảng ghi | Kết quả | Lý do |
|---|---|---|---|
| P0.1–P0.3 | Xong | Đồng ý một phần | Thiếu `packages/views/durable/README.md`. Chưa thấy đăng ký khoá trong `packages/core/feature-flags/*` |
| A1–A9 | Đã merge | Đồng ý | Các commit tồn tại; test đều có tên. Hook A4 và A7 lệch so với plan (xem mục 2) |
| B1 | Đã merge | Đồng ý | |
| B2 | Đã xong (`f64b18d642`, `6c4f86d5ab`) | Không đồng ý | Hai nguồn chưa có hook (Action API, webhook). Test có |
| B3 | Đã xong (`f64b18d642`) | Đồng ý một phần | Thiếu test khối khai báo trong briefing. Commit gộp là `23d1a16920` |
| B4 | Đã xong (`f64b18d642`) | Đồng ý | Test đủ. Commit gộp `23d1a16920` |
| B5 | Đã xong (commit `f951a1b1a4` trên nhánh, chưa push) | Không đồng ý | `f951a1b1a4` là commit D5 (`necessity tests for scaffolding`). Commit B5 là `0586e1ccf8` và `f8600ab0b1`. Đã push: `origin/durable-platform` trùng HEAD, `f8600ab0b1` là tổ tiên của HEAD |
| B6 | Đã merge | Đồng ý | Thiếu GIF |
| B7 | Đã merge | Đồng ý | |
| C1 | Backend xong; "còn thiếu FE chưa gửi header" | Không đồng ý | FE đã có ở `f5789be6f5` (send attention decision time from the web client). Test: `packages/core/api/attention-headers.client.test.ts`, `attention-opened.test.ts` |
| C2–C4 | Đã merge | Đồng ý | C2 có test `TestDeleteProfileRemovesItsRows` |
| C5 | Đã merge (`1811940ade`, từ `636a462f53`) | Đồng ý một phần | `attention-meter.tsx` và `queued-asks.tsx` theo plan không tồn tại. Có `packages/views/governance/attention-queue-section.tsx` và `attention-budget-section.tsx` |
| C6 | Chưa làm | Đồng ý | Đã kiểm `effects.spec.ts`: không có F5/F6 |
| D1 | Đã merge | Đồng ý | |
| D2 | Đã merge (`235d48f272`) | Đồng ý | |
| D3 | Đã xong (`0d6b48fe26`, `f64b18d642`) | Đồng ý | |
| D4 | Đã merge | Đồng ý | |
| D5 | Đã xong (`f951a1b1a4`, …) | Đồng ý | Commit đúng |
| D6 | Đã merge (`8bd67750c3`) | Đồng ý | Thiếu GIF |
| D7 | Chưa làm, thiếu `shadow.spec.ts` | Đồng ý | |
| E1 | Một phần; thiếu TestCloseGap… và UI reach | Không đồng ý | Test có. UI reach có (`packages/views/durable/reach/`, commit `9b8cbf0ba6`). Nên ghi "Đã xong, chờ chạy" |
| E2 | Code backend xong, chưa xác nhận (`08012ab28f`) | Đồng ý | Phiên E2 đang làm; dòng 63 chưa commit |
| E3 | Một phần; chưa đối chiếu test | Không đồng ý | Route verify, `AttestationKeysPath` (`/.well-known/multica-attestation-keys.json`), issue, CLI và test E3 đều có. Thiếu chạy lại |
| E4 | Một phần (`10e0c1769b`); thiếu TestHiredSeatStartsFull, route, UI | Không đồng ý một phần | Test và route `/api/hub/seat-offers` có. UI seat offer không có (không thấy `seat-offer` trong `packages/`). Envelope theo ghế, ngân sách và work class chưa thấy |
| E5 | Một phần; huy hiệu Hub hoãn | Không đồng ý | Huy hiệu đã có ở `33a0ce26ea`: `packages/views/durable/open/attestation-badge.tsx`, kind agent template `server/internal/pack/attestations_fork.go`. Lưới Reach đã có |
| F1 | Chưa làm | Đồng ý | |
| F2 | Chưa làm | Đồng ý | |

## 4. Bảng trạng thái đề xuất (để người điều phối dán vào plan.md)

Thay nội dung mục "Trạng thái thực tế" (plan.md dòng 36–68):

```markdown
## Trạng thái thực tế (cập nhật 2026-11-28)

Bảng này ghi trạng thái theo mã nhánh trên `durable-platform` (tip `33a0ce26ea`, bằng origin). Đối chiếu chi tiết: [reconciliation.md](reconciliation.md). Các ô `[ ]` trong từng task chưa tick lại; tick khi phiên làm task đó xác nhận từng bước.

| Task | Trạng thái | Bằng chứng / còn thiếu |
|---|---|---|
| P0.1–P0.3 | Xong | `p0-findings.md`, `packages/core/durable/flags.ts`, `55c13fcb3a`. Còn thiếu: `packages/views/durable/README.md` |
| A1–A9 | Đã merge | Migration 1041–1066, `durable_transition_fork.go`, `durable_outcome_fork.go`, `durable_track_record_fork.go`, `durable_envelope_fork.go`; e2e `outcomes.spec.ts`. Thiếu: hook A7 ở `autonomy.go` (không thấy), hook A4 đặt ở `durable_outcome_fork.go` |
| B1 | Đã merge | `effect_policy_fork.go`, `connectedapps/effect_fork.go` |
| B2 | Một phần | Test có. Hook có ở gateway, publish, kênh chat. Thiếu hook ở Action API; `webhook_delivery_worker` không tồn tại, cần xác định nguồn thật |
| B3 | Một phần | `cmd_effect_fork.go`, `durable_effect_fork.go`, test undo có. Thiếu test khối khai báo trong briefing |
| B4 | Đã xong | `f64b18d642`, `23d1a16920`; test ý định đủ |
| B5 | Đã xong | `f8600ab0b1`, `0586e1ccf8` (đã trên origin). `f951a1b1a4` là D5 |
| B6 | Đã merge | `df4db55403`. Thiếu GIF thẻ ý định |
| B7 | Đã merge | `e2e/durable/effects.spec.ts` (F3–F4), `95f1a17212` |
| C1 | Đã xong | Backend `0e5eb7f868`, `f5789be6f5` (FE gửi `X-Attention-Opened-At`) |
| C2 | Đã merge | `dd1e710ff1`; test xoá hồ sơ `TestDeleteProfileRemovesItsRows` |
| C3 | Đã merge | `064ee650c6` |
| C4 | Đã merge | `6e6550905e` |
| C5 | Đã merge | `packages/views/durable/principal/`, `packages/views/governance/attention-queue-section.tsx`. Thiếu GIF bảng uỷ quyền |
| C6 | Chưa làm | Thiếu F5/F6 trong `e2e/durable/` (chỉ có F3–F4 trong `effects.spec.ts`) |
| D1 | Đã merge | `durable_jury_fork.go`, `32c70f0932` |
| D2 | Đã merge | `235d48f272`; hook retro ở `retro.go:392` |
| D3 | Đã xong | `4b33d6cab1`, `0d6b48fe26`, `f64b18d642` |
| D4 | Đã merge | `fd9433b181` |
| D5 | Đã xong | `f951a1b1a4`, `ff44804c81`, `f64b18d642` |
| D6 | Đã merge | `8bd67750c3`. Thiếu GIF báo cáo shadow |
| D7 | Chưa làm | Thiếu `e2e/durable/shadow.spec.ts` |
| E1 | Đã xong | `durable_reach_fork.go`, `c9d56fdd90`; test có đủ; chờ chạy |
| E2 | Code backend xong, chưa xác nhận | `03dd039236`, `08012ab28f`, adapter `server/cmd/multica-ocf-adapter/`. Test handler OCF đỏ do DB dùng chung; chạy lại khi DB rảnh |
| E3 | Đã xong, chờ chạy | `durable_attestation_fork.go`, `durable_jcs_fork.go`, route verify và well-known; test có đủ |
| E4 | Một phần | Route `/api/hub/seat-offers`, `durable_seat_offer_fork.go`. Thiếu UI seat offer, envelope theo ghế, work class, lấy khoá host từ xa, kiểm owner/admin |
| E5 | Đã xong | `9b8cbf0ba6`; huy hiệu có ở `33a0ce26ea`. Thiếu GIF lưới Reach |
| F1 | Chưa làm | Chưa có bằng chứng chạy; chưa có `rollout.md` |
| F2 | Chưa làm | Cần người dùng đồng ý trước khi bật trên máy chủ dev |
```

Lưu ý khi dán: các bảng trạng thái "Đã xong" không thay checkbox. Checkbox của task chỉ tick khi chạy được lệnh trong task và đã xem kết quả.

## 5. Gap theo spec mục 14 và 17 không có task trong plan

- **14.1 phụ thuộc cờ:** kiểm phụ thuộc chỉ có lúc khởi động (`server/cmd/server/main.go:392`, `RequireDurableDependencies`). Chưa thấy kiểm ở đường bật cờ khi đang chạy, mà spec yêu cầu. Plan không có task.
- **14.2 bản dịch:** spec yêu cầu `packages/views/locales/<locale>/durable.ts` (mới). Thực tế khoá durable nằm trong `packages/views/locales/<locale>.ts`, đủ 9 locale. Lệch cấu trúc, cần quyết định chấp nhận hay không.
- **14.3 quyền 403:** có test cho envelope, profile owner, `auto_promote`, shadow. Chưa thấy test 403 cho chính sách hệ quả (`EffectPolicy`) và cho ghim chế độ (có route, chưa thấy test tên rõ).
- **14.4 mã lỗi:** `envelope_violation` và `effect_blocked` không có trong code và test. Plan không có task cho hai mã này.
- **14.4 enum:** `decision` lạ → `confirm` không thấy trong `packages/core/durable/intents/schemas.ts`. `verdict` lạ → `unverified` và `effect_class` lạ → `write` có.
- **14.4 phân trang và kích thước:** con trỏ tối đa 100, thân 16 KiB (32 KiB cho preview). Không có task, không có test tên tương ứng.
- **14.5 WebSocket:** không tìm thấy chuỗi `outcome:*`, `effect_intent:*`, `attention:*`, `track_record:*` ở server và packages (grep rỗng với tiền tố `"outcome:"` …). Cần xác nhận cách đặt tên sự kiện thực tế.
- **14.6 dữ liệu ngoài:** nội dung lấy từ bên ngoài trong briefing (hồ sơ ghế, trait) được đánh dấu là dữ liệu. Không có task và không có test.
- **14.7 job đêm:** phân trang theo workspace, không có test tên riêng.
- **17.1 tầng test:** đủ cho Go đơn vị, Go DB, core và views. E2E: kịch bản 1 có, kịch bản 2 một phần (F3–F4 có, F5–F6 thiếu), kịch bản 3 thiếu.
- **17.3 lệnh:** `node scripts/check-migration-numbers.mjs --head <PR head>` trước khi merge PR có migration, chưa chạy (chưa có PR).

## 6. Cờ và rollout (F1, F2)

**Cờ.** Năm khoá trong `server/internal/featureflags/keys_fork.go:16-20` khớp spec 14.1. Mặc định tắt (`IsEnabled(..., false)`). Bảng phụ thuộc (`keys_fork.go:33-41`) khớp bảng spec: `durable_seats` cần outcomes và effects; `durable_open` cần effects và outcomes; `durable_principal` không cần cờ nào. Hook một dòng ở `keys.go:108` (`durableFrontendFlags...`). Khớp với spec. Điểm còn thiếu là kiểm phụ thuộc ở đường bật cờ lúc chạy (mục 5).

**Lệnh F1 còn thiếu.** Không lệnh nào trong bảng dưới đây đã chạy trong phiên này. Chạy trên DB test riêng, không dùng DB `multica`.

```bash
make check
pnpm exec playwright test e2e/durable
pnpm sync:contract
pnpm sync:coverage
pnpm test:scripts
cd server && go test ./cmd/migrate ./internal/migrations
```

Cần thêm: `/security-review` trên nhánh, ghi kết quả vào `docs/specs/durable-platform/rollout.md` (chưa có). Trước khi merge PR có migration: `node scripts/check-migration-numbers.mjs --head <PR head>`.

Rủi ro ghi nhận: `server/internal/testutil/db_test.go:29-32` rơi về DB `multica` khi không đặt `DATABASE_URL` (đã ghi trong p0-findings). Mọi lệnh Go có DB phải đặt `DATABASE_URL` rõ ràng.

## 7. Việc còn lại theo thứ tự

Số phiên là ước lượng. Phiên chạy song song được nếu không đụng cùng file.

**Nhóm 1: không phụ thuộc, làm song song được**

1. **C6 e2e F5/F6**: dự đoán và ledger attention trong `e2e/durable/` (có thể thêm spec mới). Phụ thuộc: C1–C4 đã có. Khoảng 1 phiên.
2. **D7 `e2e/durable/shadow.spec.ts`**: kịch bản 17.2 mục 3, có runtime giả. Phụ thuộc: D3–D4 đã có. Khoảng 1 phiên.
3. **Vá test và hook nhỏ**: test khối khai báo của B3 (`durable_briefing_fork_test.go`); hook A7 ở `autonomy.go` (nếu cần); README `packages/views/durable/README.md` (P0.3); mã lỗi `envelope_violation` và `effect_blocked` (A7, B2); fallback `confirm` cho `decision`. Khoảng 1 phiên.
4. **B2 hai nguồn hệ quả**: cần người điều phối chọn đúng nguồn thật (Action API qua `plugin_action.go`; "webhook" không có file tên đó). Khoảng 0.5–1 phiên.

**Nhóm 2: giao diện và GIF**

5. **E4 backend và UI**: UI seat offer trong Hub (`packages/views/` mới), envelope theo ghế, ngân sách và work class, lấy khoá host từ xa, kiểm owner/admin. Phụ thuộc: route đã có. Khoảng 2 phiên (1 backend, 1 UI).
6. **GIF** cho A8, B6, C5, D6, E5 và E4 bằng `scripts/motion/capture.mjs`. Phụ thuộc: UI đã xong (mục 5 cho E4). Khoảng 1 phiên. Người xem và xác nhận GIF.

**Nhóm 3: xác nhận chạy**

7. **Chạy test**: Go có DB (E2 handler OCF khi DB rảnh), vitest, Playwright `e2e/durable`. Phụ thuộc: mục 1, 2 xong để e2e đủ. Khoảng 0.5 phiên.
8. **Phiên E2** hoàn tất và commit dòng 63 của plan.md (đang làm, ngoài báo cáo này).

**Nhóm 4: rollout**

9. **F1**: các lệnh ở mục 6, `/security-review`, `rollout.md`. Phụ thuộc: 1, 2, 5, 7. Khoảng 1–2 phiên.
10. **F2**: bật cờ trên máy chủ dev cho workspace `tupk`, chạy 7 ngày, đo thước đo. Phụ thuộc: 9 và **đồng ý người dùng**. Không tính là phiên làm việc.

Tổng ước lượng: khoảng 8–10 phiên làm việc, chưa kể F2 (7 ngày theo dõi).

## 8. Phạm vi không làm

- Không sửa plan.md, không sửa code, không commit, không push, không tạo worktree, không đổi nhánh.
- Không chạy test có DB, không chạy `make check` hay `pnpm sync:*`. Không tạo file ngoài `docs/specs/durable-platform/reconciliation.md`.
