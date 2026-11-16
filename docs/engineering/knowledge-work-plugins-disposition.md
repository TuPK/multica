# Knowledge Work Plugins: phương án cho từng skill (đánh giá 2026-11-16)

Bảng này phủ toàn bộ 252 file `SKILL.md` của [anthropics/knowledge-work-plugins](https://github.com/anthropics/knowledge-work-plugins) tại commit `e605d8ddbfb688da57dfaaceebd93f12a64ef1d3`. Tóm tắt và lập luận ở [knowledge-work-plugins-squads.md](knowledge-work-plugins-squads.md) mục 0 và 4. Các quyết định đã chốt ở spec mục 9.2 (không còn điểm chờ duyệt). Đổi quyết định thì sửa bảng này và cập nhật tổng.

## Ký hiệu

| Ký hiệu | Nghĩa | Cách đưa vào catalog |
| --- | --- | --- |
| **A** | Dùng nguyên trạng: vendor nguyên văn, chỉ liệt kê thay đổi cục bộ | `catalog/skills/kwp/<tên gốc>/`, `source.type: vendored`; thay đổi ghi trong `SOURCES.md` |
| **B** | Bổ trợ: viết lại thành skill dẫn xuất, gắn cho vai đã có | `catalog/skills/kwp/kwp-<tên>/`, `source.type: authored`, ghi nguồn trong frontmatter |
| **C** | Không đưa vào | Lý do ở cột cuối |

## Tổng

| Nhóm | Số | Ghi chú |
| --- | ---: | --- |
| A | 11 | 7 ở `data`, 1 mỗi ở `engineering`, `operations`, `finance`, `design` |
| B | 13 | 7 plugin: `customer-support`, `engineering`, `operations`, `enterprise-search`, `product-management`, `design`, `data` |
| C | 228 | 157 ở 16 plugin chính + 71 trong `partner-built` (kể cả 27 file lồng trong skill của zoom) |
| **Tổng** | **252** | 181 + 71 |

Nguyên tắc dùng chung (`untrusted-content`, `absent-is-not-zero`) lấy từ `small-business/shared/*.md`, không nằm trong 252 file này. Chúng được viết thành skill authored riêng (xem spec mục 4.3).

## Chi tiết theo plugin

### bio-research (6)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `instrument-data-to-allotrope` | C | — | Life-sciences tools and databases; not a Multica squad domain |
| `nextflow-development` | C | — | Life-sciences tools and databases; not a Multica squad domain |
| `scientific-problem-selection` | C | — | Life-sciences tools and databases; not a Multica squad domain |
| `scvi-tools` | C | — | Life-sciences tools and databases; not a Multica squad domain |
| `single-cell-rna-qc` | C | — | Life-sciences tools and databases; not a Multica squad domain |
| `start` | C | — | Life-sciences tools and databases; not a Multica squad domain |

### cowork-plugin-management (2)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `cowork-plugin-customizer` | C | — | Plugin-authoring for Cowork; reference only for the Skill Hub |
| `create-cowork-plugin` | C | — | Plugin-authoring for Cowork; reference only for the Skill Hub |

### customer-support (5)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `customer-escalation` | B | company support_lead | Escalation brief to build squad; asks replaced by assumptions |
| `kb-article` | B | company support_agent | KB article draft; search before filing |
| `ticket-triage` | B | company support_agent | Triage P1–P4; connector refs removed |
| `customer-research` | C | — | Support triage and escalation are covered by Kit Việt CSKH (phan-hoi-khieu-nai-cskh) in Vietnamese; English-only drafts |
| `draft-response` | C | — | Support triage and escalation are covered by Kit Việt CSKH (phan-hoi-khieu-nai-cskh) in Vietnamese; English-only drafts |

### data (10)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `analyze` | A | quickstart kwp, data_lead / data_analyst | Ask-the-user step rewritten (local change) |
| `build-dashboard` | A | quickstart kwp, data_viz | Self-contained HTML; large (925 lines) |
| `create-viz` | A | quickstart kwp, data_viz | Python charts; runtime needs Python |
| `explore-data` | A | quickstart kwp, data_profiler | Ask-the-user step rewritten (local change) |
| `sql-queries` | A | quickstart kwp, data_analyst | Read-only SELECT only (local change) |
| `statistical-analysis` | A | quickstart kwp, data_analyst | Method only; no connector |
| `validate-data` | A | quickstart kwp; company critic roles; data_validator | Method only; QA gate |
| `data-context-extractor` | B | data_profiler (bootstrap mode only) | Interview step rewritten; connector refs removed |
| `data-visualization` | C | — | No data-analysis skill exists in the catalog; general method is reusable |
| `write-query` | C | — | No data-analysis skill exists in the catalog; general method is reusable |

### design (7)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `user-research` | A | company product_discovery, validation_analyst | Plan only; no connector |
| `design-critique` | B | quickstart ux (sdlc), plan_ux (bmad) | Critique framework; no edits |
| `design-handoff` | B | quickstart ux (sdlc), plan_ux (bmad) | Dev spec; fills a gap in the SDLC UX skill |
| `ux-copy` | B | quickstart ux (sdlc), plan_ux (bmad) | Microcopy; follows UI-descriptions rule |
| `accessibility-review` | C | — | Covered by sdlc-ux-design, frontend-design and accessibility-* (WCAG 2.2) skills |
| `design-system` | C | — | Covered by sdlc-ux-design, frontend-design and accessibility-* (WCAG 2.2) skills |
| `research-synthesis` | C | — | Covered by sdlc-ux-design, frontend-design and accessibility-* (WCAG 2.2) skills |

### engineering (10)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `tech-debt` | A | sdlc_maintenance, lead | Prioritisation formula; no connector |
| `incident-response` | B | company release_engineer | SEV1–4 and blameless postmortem; no paging or chat |
| `architecture` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `code-review` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `debug` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `deploy-checklist` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `documentation` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `standup` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `system-design` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |
| `testing-strategy` | C | — | SDLC squads already cover this (sdlc-code-review, sdlc-bug-fix, sdlc-architecture, sdlc-qa-verification, sdlc-devops-release) |

### enterprise-search (5)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `knowledge-synthesis` | B | company librarian | Dedup and confidence scoring; 26 connector refs rewritten to multica knowledge |
| `search-strategy` | B | company librarian | Query decomposition over the knowledge base |
| `digest` | C | — | Connector-only (search across Slack, Drive, Gmail) or duplicates the secretary and knowledge flows |
| `search` | C | — | Connector-only (search across Slack, Drive, Gmail) or duplicates the secretary and knowledge flows |
| `source-management` | C | — | Connector-only (search across Slack, Drive, Gmail) or duplicates the secretary and knowledge flows |

### finance (8)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `variance-analysis` | A | data_analyst | Decomposition method; USD examples kept as examples |
| `audit-support` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `close-management` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `financial-statements` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `journal-entry` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `journal-entry-prep` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `reconciliation` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |
| `sox-testing` | C | — | Covered by kv_accounting (Vietnamese circulars); US GAAP / SOX / audit framing does not apply |

### human-resources (9)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `comp-analysis` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `draft-offer` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `interview-prep` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `onboarding` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `org-planning` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `people-report` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `performance-review` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `policy-lookup` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |
| `recruiting-pipeline` | C | — | Individual-level HR; conflicts with People & Performance charter; Kit Việt covers recruiting |

### legal (9)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `brief` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `compliance-check` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `legal-response` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `legal-risk-assessment` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `meeting-briefing` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `review-contract` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `signature-request` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `triage-nda` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |
| `vendor-check` | C | — | Covered by kv_legal (Vietnamese law); US/EU statutes and e-signature actions do not apply |

### marketing (8)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `brand-review` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `campaign-plan` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `competitive-brief` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `content-creation` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `draft-content` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `email-sequence` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `performance-report` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |
| `seo-audit` | C | — | Duplicates the vendored marketing domain (51 skills) and growth_lab / kv_marketing |

### operations (9)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `risk-assessment` | A | company finance_lead | Risk-register method; no connector |
| `change-request` | B | company release_engineer | Change record draft with rollback |
| `runbook` | B | company release_engineer | Runbook draft; commands are text, never run |
| `capacity-plan` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |
| `compliance-tracking` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |
| `process-doc` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |
| `process-optimization` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |
| `status-report` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |
| `vendor-review` | C | — | Covered by kv_officeops, kv_admin or secretary-brief; no separate need |

### pdf-viewer (1)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `view-pdf` | C | — | Needs a PDF viewer UI; not a squad skill |

### product-management (8)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `synthesize-research` | B | company customer_lead | Theme extraction with quotes and sources |
| `competitive-brief` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `metrics-review` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `product-brainstorming` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `roadmap-update` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `sprint-planning` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `stakeholder-update` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |
| `write-spec` | C | — | Covered by bmad-* planning skills, growth_lab measurement and competitor-profiling |

### productivity (4)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `memory-management` | C | — | Personal task list and memory tiers compete with issues and the knowledge base |
| `start` | C | — | Personal task list and memory tiers compete with issues and the knowledge base |
| `task-management` | C | — | Personal task list and memory tiers compete with issues and the knowledge base |
| `update` | C | — | Personal task list and memory tiers compete with issues and the knowledge base |

### sales (36)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `account-context` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `account-plan` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `account-research` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `account-tiering` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `call-prep` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `call-summary` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `close-plan` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `competitive-intelligence` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `create-an-asset` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `crm-hygiene-check` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `customer-health` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `customer-voice` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `daily-briefing` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `deal-advance-gap` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `deal-review` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `deal-signals` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `deal-slip-scenario` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `draft-outreach` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `end-of-day` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `expansion-whitespace` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `forecast` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `handle-objection` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `inbox-sweep` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `lead-triage` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `log-activity` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `pipeline-review` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `renewal-radar` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `rep-context` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `route-lead` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `schedule-meeting` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `setup` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `stakeholder-map` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `team-pipeline` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `update-opportunity` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `weekly-wrap` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |
| `win-loss-review` | C | — | CRM-bound (writes records, sends mail, books meetings); sales_loop skills cover the lane |

### small-business (44)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `ad-manager` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `ap-processor` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `brand-style` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `build-agent` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `build-connector` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `business-pulse` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `call-list` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `canva-creator` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `cash-flow-snapshot` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `close-month` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `content-strategy` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `contract-review` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `crm-autopilot` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `grant-rfp-writer` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `grow-pipeline` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `growth-pulse` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `hiring-screener` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `inbox-manager` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `inventory-planner` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `invoice-chase` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `job-post-builder` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `lead-finder` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `lead-triage` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `marketing-monday` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `monday-brief` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `month-end-prep` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `outreach-composer` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `pay-the-bills` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `payroll-prep` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `plan-payroll` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `proposal-builder` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `reactivate` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `report-builder` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `report-pack` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `restock` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `review-reputation` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `seo-ai-visibility` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `smb-onboard` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `smb-router` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `social-content-engine` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `speed-to-lead` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `tax-prep` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `tax-season-organizer` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |
| `ticket-deflector` | C | — | QuickBooks, PayPal, Shopify, Gusto, Canva connectors; US tax assumptions |

### partner-built/apollo (3)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `skills/enrich-lead` | C | — | Third-party service skill; not vendored in this round |
| `skills/prospect` | C | — | Third-party service skill; not vendored in this round |
| `skills/sequence-load` | C | — | Third-party service skill; not vendored in this round |

### partner-built/brand-voice (3)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `skills/brand-voice-enforcement` | C | — | Third-party service skill; not vendored in this round |
| `skills/discover-brand` | C | — | Third-party service skill; not vendored in this round |
| `skills/guideline-generation` | C | — | Third-party service skill; not vendored in this round |

### partner-built/common-room (6)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `skills/account-research` | C | — | Third-party service skill; not vendored in this round |
| `skills/call-prep` | C | — | Third-party service skill; not vendored in this round |
| `skills/compose-outreach` | C | — | Third-party service skill; not vendored in this round |
| `skills/contact-research` | C | — | Third-party service skill; not vendored in this round |
| `skills/prospect` | C | — | Third-party service skill; not vendored in this round |
| `skills/weekly-prep-brief` | C | — | Third-party service skill; not vendored in this round |

### partner-built/slack (2)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `skills/slack-messaging` | C | — | Third-party service skill; not vendored in this round |
| `skills/slack-search` | C | — | Third-party service skill; not vendored in this round |

### partner-built/zoom-plugin (57)

| Skill | Quyết định | Đích | Lý do |
| --- | :---: | --- | --- |
| `skills/build-zoom-bot` | C | — | Third-party service skill; not vendored in this round |
| `skills/build-zoom-meeting-app` | C | — | Third-party service skill; not vendored in this round |
| `skills/choose-zoom-approach` | C | — | Third-party service skill; not vendored in this round |
| `skills/cobrowse-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/contact-center` | C | — | Third-party service skill; not vendored in this round |
| `skills/contact-center/android` | C | — | Third-party service skill; not vendored in this round |
| `skills/contact-center/ios` | C | — | Third-party service skill; not vendored in this round |
| `skills/contact-center/web` | C | — | Third-party service skill; not vendored in this round |
| `skills/debug-zoom` | C | — | Third-party service skill; not vendored in this round |
| `skills/debug-zoom-integration` | C | — | Third-party service skill; not vendored in this round |
| `skills/design-mcp-workflow` | C | — | Third-party service skill; not vendored in this round |
| `skills/general` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/android` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/electron` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/ios` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/linux` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/macos` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/react-native` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/unreal` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/web` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/web/client-view` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/web/component-view` | C | — | Third-party service skill; not vendored in this round |
| `skills/meeting-sdk/windows` | C | — | Third-party service skill; not vendored in this round |
| `skills/oauth` | C | — | Third-party service skill; not vendored in this round |
| `skills/phone` | C | — | Third-party service skill; not vendored in this round |
| `skills/plan-zoom-integration` | C | — | Third-party service skill; not vendored in this round |
| `skills/plan-zoom-product` | C | — | Third-party service skill; not vendored in this round |
| `skills/probe-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/rest-api` | C | — | Third-party service skill; not vendored in this round |
| `skills/rivet-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/rtms` | C | — | Third-party service skill; not vendored in this round |
| `skills/scribe` | C | — | Third-party service skill; not vendored in this round |
| `skills/setup-zoom-mcp` | C | — | Third-party service skill; not vendored in this round |
| `skills/setup-zoom-oauth` | C | — | Third-party service skill; not vendored in this round |
| `skills/start` | C | — | Third-party service skill; not vendored in this round |
| `skills/team-chat` | C | — | Third-party service skill; not vendored in this round |
| `skills/ui-toolkit` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/android` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/flutter` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/ios` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/linux` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/macos` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/react-native` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/unity` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/web` | C | — | Third-party service skill; not vendored in this round |
| `skills/video-sdk/windows` | C | — | Third-party service skill; not vendored in this round |
| `skills/virtual-agent` | C | — | Third-party service skill; not vendored in this round |
| `skills/virtual-agent/android` | C | — | Third-party service skill; not vendored in this round |
| `skills/virtual-agent/ios` | C | — | Third-party service skill; not vendored in this round |
| `skills/virtual-agent/web` | C | — | Third-party service skill; not vendored in this round |
| `skills/webhooks` | C | — | Third-party service skill; not vendored in this round |
| `skills/websockets` | C | — | Third-party service skill; not vendored in this round |
| `skills/zoom-apps-sdk` | C | — | Third-party service skill; not vendored in this round |
| `skills/zoom-mcp` | C | — | Third-party service skill; not vendored in this round |
| `skills/zoom-mcp/whiteboard` | C | — | Third-party service skill; not vendored in this round |

