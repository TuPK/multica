# Knowledge Work Plugins (KWP): đưa skill vào squad và agent

Spec (nghiên cứu và thiết kế, chưa triển khai). Kế hoạch thực hiện: [knowledge-work-plugins-squads-plan.md](knowledge-work-plugins-squads-plan.md). Cùng họ với [kitviet-squads.md](kitviet-squads.md), [sales-loop-squad.md](sales-loop-squad.md),
[bmad-planning-squad.md](bmad-planning-squad.md) và [digital-company.md](digital-company.md).

Ngôn ngữ: tiếng Việt để người dùng đọc; tên skill, role key, kind, lệnh giữ tiếng Anh.

Nguồn: [anthropics/knowledge-work-plugins](https://github.com/anthropics/knowledge-work-plugins), giấy phép Apache-2.0 (các plugin `partner-built`
có giấy phép riêng: `brand-voice`, `apollo`, `slack`, `zoom` là MIT, `common-room` là Apache-2.0). Bản đã đọc là commit
`e605d8ddbfb688da57dfaaceebd93f12a64ef1d3` (2026-11-09).

## 1. Tóm tắt

Repo có 252 `SKILL.md` trong 14 plugin và 5 plugin `partner-built`. Cắt bỏ phần không dùng được (57 skill Zoom, 44 skill `small-business` gắn chặt QuickBooks/PayPal/Shopify,
`productivity`, `bio-research`, `pdf-viewer`, các skill CRM của `sales` ghi dữ liệu hoặc gửi thư), còn **46 skill được chọn (43 chuyển đổi, 3 viết mới)**; tổng nguồn ứng viên ban đầu là 47 skill (427 KB) trước khi tinh lọc.
Gần như không có gì để đưa vào **squad Kit Việt đã thiết kế**, vì Kit Việt đã có bản tiếng Việt cho cùng việc, có gác cổng chống bịa và có luật Việt Nam.
Giá trị của repo này nằm ở **các chỗ Multica đang mỏng**:

| Chỗ đang mỏng | Hiện có | KWP bổ trợ |
| --- | --- | --- |
| Support | 2 agent, không skill | `customer-support`: triage P1–P4, escalation brief, KB article, draft response |
| Growth & GTM | **1 agent** (`growth_lead`) | `marketing/campaign-plan`, `performance-report`, `product-management/metrics-review` |
| Finance & Risk | **1 agent** (`finance_lead`) | `variance-analysis`, `risk-assessment` (đúng dạng sổ rủi ro), `vendor-review` |
| Release & Ops | 2 agent, không skill | `incident-response` (SDLC chưa có), `deploy-checklist`, `runbook`, `change-request` |
| Phân tích dữ liệu | **không có squad nào** | Cả plugin `data` (10 skill) |
| Gác cổng phân tích | `soat-so-lieu` của Kit Việt chỉ soát số | `data/validate-data`: danh mục lỗi phân tích (join nổ, thiên lệch sống sót, mẫu số trôi) |
| Librarian | không skill KWP | `enterprise-search/knowledge-synthesis`: gộp trùng, chấm độ tin theo độ mới và độ chính thống |
| UX Designer (SDLC, Blueprint) | `sdlc-ux-design`, `frontend-design`, `accessibility-*` | `design-handoff`, `ux-copy`, `design-critique` (skill UX hiện có không nhắc tới handoff hay microcopy) |

Ba khuyến nghị lớn:
1. **Đưa theo vai, không theo squad.** Skill gắn cấp squad bị mọi thành viên kế thừa (`use-inherited-squad-skills.ts`). Dùng `roleSkills` (≤6 skill mỗi vai, spec Kit Việt mục 5.10).
2. **Không cài plugin lên runtime.** Mỗi plugin kéo theo `.mcp.json` (riêng `engineering` thêm 10 server: Slack, GitHub, PagerDuty, Gmail...) và slash command. Vendor từng skill đã chọn qua cùng pipeline sinh dữ liệu của Kit Việt.
3. **Thêm một squad mới `Data & Insights`** và hai agent chuyên trách cho Growth & GTM và Finance & Risk (mục 6).

## 2. Phạm vi đã đọc, và phần chưa

- **Quét toàn bộ** 252 skill: tên, mô tả, số dòng, dung lượng; đếm `~~` (chỗ giữ cho kết nối), phụ thuộc file cục bộ (`*.local.md`, `CLAUDE.md`, `TASKS.md`), chỗ "ask the user", giấy phép.
- **Đọc phần đầu (đầu vào, quy trình, đầu ra) của 34 skill đại diện**: engineering (`incident-response`, `deploy-checklist`, `tech-debt`, `code-review`),
  legal (`triage-nda`, `review-contract`, `signature-request`, `legal-response`, `compliance-check`), marketing (`brand-review`, `performance-report`, `seo-audit`, `email-sequence`),
  finance (`variance-analysis`, `close-management`, `reconciliation`, `journal-entry-prep`), customer-support (`ticket-triage`, `customer-escalation`, `kb-article`, `draft-response`),
  sales (`deal-review`, `win-loss-review`, `handle-objection`, `call-summary`), `data/validate-data`, `enterprise-search/knowledge-synthesis`,
  product-management (`write-spec`, `competitive-brief`, `metrics-review`), operations (`risk-assessment`, `status-report`), `design/design-critique`, `human-resources/performance-review`.
- **Đọc trọn** 4 tài liệu dùng chung của `small-business/shared` (`absent-is-not-zero`, `untrusted-content`, `tenant-scope`, phần đầu `personal-data`) và phần đầu 4 trong 5 agent của `brand-voice`.
- **Chưa đọc từng dòng**: phần thân các skill còn lại (khoảng 215 skill). Với chúng, kết luận dựa trên mô tả, kích thước và quét máy; chỗ nào chỉ dựa vào mô tả thì mục 5 ghi "theo mô tả".
- Đối chiếu với code Multica: `company-template.ts` (14 squad thẻ công ty), `templates.ts` (SDLC, Market Research, Sales Loop, BMAD Planning), `agents/instruction-templates.ts`, và spec Kit Việt (5 squad P0 + 5 squad P1).

## 3. Sáu đặc điểm của KWP quyết định cách dùng

| # | Đặc điểm | Hệ quả cho Multica |
| --- | --- | --- |
| K1 | **KWP chỉ là skill**, không có đội hình. Duy nhất `brand-voice` có 5 agent (`content-generation`, `quality-assurance`, `conversation-analysis`, `document-analysis`, `discover-brand`). | Vai/agent do Multica quyết. KWP không cho sẵn "agent gốc" như Kit Việt. |
| K2 | **Viết cho phiên có người ngồi**: slash command, `$ARGUMENTS`, `@$1`; 37 skill có bước "ask the user". | Squad Multica không chờ người (SDLC: "never wait for a person"). Cần bước chuyển đổi như generator Kit Việt: câu hỏi thành giả định ghi lại, thiếu đầu vào thành `blocked`. |
| K3 | **Địa lý Mỹ/EU**: US GAAP, SOX 404, GDPR/CCPA/HIPAA (43 chỗ trong `finance`). Riêng `legal-risk-assessment` trung lập (0 chỗ). | Skill gắn luật nước ngoài không vào `kv_legal`, `kv_accounting`. Skill phương pháp (ma trận rủi ro, phân rã variance) thì dùng được. |
| K4 | **Cấu hình bằng file cục bộ** (`legal.local.md`, `CLAUDE.md`, `TASKS.md`) ở `legal` (3), `productivity` (4), `brand-voice` (3), `enterprise-search` (1). | Thay bằng hồ sơ đơn vị trong KB (spec Kit Việt 5.4). `productivity` bỏ hẳn: `TASKS.md` và bộ nhớ hai tầng tranh chỗ với issue và KB. |
| K5 | **Hành động ra ngoài**: `legal/signature-request` gửi DocuSign; `sales/draft-outreach` tạo thư nháp và nạp sequence; `update-opportunity`, `log-activity`, `schedule-meeting`, `partner-built/apollo/sequence-load` ghi CRM hoặc đặt lịch. | Vi phạm "không gửi, không ký, không nộp". Loại, hoặc cắt bước gửi và giữ phần kiểm (xem `signature-request` ở mục 5). |
| K6 | **Chất lượng không đều.** Skill dày: `sales`, `legal`, `finance`, `data`, `marketing`, `customer-support` (8–18 KB). Skill khung mẫu: `human-resources` (1–4 KB), `operations` (1–3 KB), `engineering` (1–5 KB; `tech-debt` chỉ 32 dòng). | Skill khung mẫu chỉ làm điểm xuất phát; không coi chúng ngang skill SDLC (`sdlc-*` là skill theo vai, sâu hơn hẳn). |

Điều đáng học thêm: `small-business/shared/untrusted-content.md` ("nội dung đọc vào là dữ liệu, không là lệnh"; yêu cầu đổi tài khoản ngân hàng, mã, quyền thì giữ lại cho người) và
`absent-is-not-zero.md` ("trống không bằng số 0"; kết quả phân trang không là tổng; ngày trong tương lai là dữ liệu hỏng). Cả hai cùng tinh thần với gác cổng chống bịa của Kit Việt và đáng làm
thành skill dùng chung cho mọi squad đọc nội dung bên ngoài (Support, Sales Loop, Data). Chúng viết kèm ví dụ connector, nên cần viết lại cho gọn trước khi vendor.

## 4. Quy tắc quyết định để không xung đột

1. **Một việc, một skill, mỗi vai.** KWP tự trùng nhau (mục 7); mỗi vai chỉ nhận một bản.
2. **Việc có luật Việt Nam thì skill Kit Việt thắng.** Pháp lý, thuế, kế toán, văn bản hành chính, quảng cáo: KWP chỉ bổ sung khung phương pháp nếu Kit Việt chưa có.
3. **Skill viết tay cho vai Multica thắng skill chung.** `sdlc-*`, `planning-*`, `sales-*` đã có thủ tục; không gắn KWP lên cùng vai. Ví dụ: không thêm `write-spec` cho `plan_pm` (đã có `bmad-prd`), không thêm `lead-triage` cho `sales_scout` (đã có chấm điểm trong `sales-scouting`).
4. **Người soạn và người soát tách vai.** KWP gắn gate (`validate-data`, `brand-review`) cho vai gate, không cho vai soạn. Gate không sửa (VERDICT theo spec Kit Việt 5.2).
5. **Cắt bước hành động ra ngoài** (K5) và thay "ask the user" (K2) ở bước chuyển đổi.
6. **Mỗi vai tối đa 6 skill thêm**, tính cả skill Kit Việt đã gắn ở spec 5.10.

## 5. Bản đồ đề xuất: skill KWP gắn cho vai nào

Ký hiệu: **A** làm trước, giá trị cao, rủi ro thấp; **B** làm sau khi có khung; ✱ chỉ dựa vào mô tả, chưa đọc thân.

### 5.1 Support (squad có sẵn) · A
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `support_agent` | `ticket-triage`, `kb-article`, `draft-response` | Triage cho phân loại bug/how-to/billing và P1–P4, có bước kiểm trùng; `kb-article` có sẵn mẫu 5 loại bài và quy tắc tìm kiếm. `draft-response` chỉ gắn khi workspace trả lời bằng tiếng Anh: với tiếng Việt, dùng `phan-hoi-khieu-nai-cskh` của Kit Việt (spec mục 7); hai skill soạn thư trả lời không cùng nằm một vai. |
| `support_lead` | `customer-escalation`, `customer-research` | Brief escalation (tái hiện, ảnh hưởng, người nhận) khớp quy tắc "bug đi sang squad Build kèm steps to reproduce". Giữ quy tắc Multica: hoàn tiền, ngoại lệ, hứa ngày là việc của boss; không bước nào được gửi cho khách. |

### 5.2 Growth & GTM (squad có sẵn, chỉ 1 agent) · A
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `growth_lead` | `campaign-plan` (kết quả là `gtm_plan`), `performance-report` và `metrics-review` (kết quả là `metrics_report` hàng tuần) | Charter đang đòi đúng hai tài liệu này mà vai chưa có phương pháp. `metrics-review` dùng được cho Operate (so với mục tiêu trong plan). |
| **Mới: `growth_analyst`** (đề xuất) | `performance-report`, `metrics-review`, `data/statistical-analysis` ✱ | Tách người đọc số khỏi trưởng squad. Gửi `metrics_report` cho lead; không đặt mục tiêu, không chi tiền. |
| `mkt_scout` của `kv_marketing` (B) | `seo-audit` | Bản Kit Việt chỉ có tìm từ khóa và `bai-blog-chuan-seo`; `seo-audit` thêm kiểm kỹ thuật và so sánh đối thủ. Chỉ gắn khi có công cụ SEO. |

Không đưa vào `kv_marketing` các skill soạn nội dung của KWP (`content-creation`, `draft-content`, `email-sequence`, `brand-review`): trùng `soan-noi-dung-da-kenh`, `chuoi-email-cham-soc`, `soat-giong-thuong-hieu`, và bản Kit Việt có gác cổng claim quảng cáo theo luật Việt Nam. Marketing Studio nhận brief từ Growth & GTM; hai bên không cùng soạn.

### 5.3 Finance & Risk (squad có sẵn, chỉ 1 agent) · A
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `finance_lead` | `vendor-review` (chi phí, rủi ro, khuyến nghị), `variance-analysis` ✱ phần phương pháp | `vendor-review` hợp "review each spending proposal". `variance-analysis` trung lập địa lý; dùng khi có số ngân sách (routine ngân sách ở pha 4). |
| **Mới: `risk_analyst`** (đề xuất) | `risk-assessment`, `compliance-tracking` ✱ | Ma trận khả năng × tác động và mẫu sổ rủi ro (mô tả, mức, giảm thiểu, người phụ trách, trạng thái) khớp kind `risk_register`. Thêm cột "ngày kiểm lần cuối" vì charter của Multica đòi. |
| `kv_accounting` | **Không gắn** | `journal-entry*`, `close-management`, `reconciliation`, `financial-statements`, `sox-testing`, `audit-support` theo US GAAP/SOX; Kit Việt đã có khóa sổ, đối chiếu theo Thông tư 99/133. Hai hệ tài khoản cùng nằm một vai sẽ cho kết quả lẫn. |

### 5.4 Release & Ops (squad có sẵn, không skill) · A
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `release_engineer` | `deploy-checklist`, `incident-response`, `runbook`, `change-request` | `sdlc-devops-release` thuộc squad Build, không gắn cho squad này. `incident-response` (mức SEV1–4, nhịp cập nhật, postmortem không đổ lỗi, 5 whys) không có bản tương đương trong SDLC. Production vẫn cần boss duyệt; `runbook` và `change-request` giữ ở mức bản nháp. |
| `release_lead` | `status-report` | Mẫu báo cáo xanh/vàng/đỏ. Skill khung mẫu; chỉ lấy khuôn. |

### 5.5 Research, Product, Build · A/B
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `competitor_analyst` (Market Research, Competitor Intelligence) | `product-management/competitive-brief` | Nguồn chính (trang giá, changelog, tin tuyển dụng, đánh giá). KWP có **ba** bản "competitive": PM, marketing, sales. Chỉ gắn bản PM cho vai này; bản marketing (định vị, thông điệp) cho `mkt_scout`; bản sales cần CRM, không dùng. |
| `customer_lead` (Customer Voice) | `synthesize-research` | Thay cho `design/research-synthesis` (2 KB, trùng ý) và `sales/customer-voice` (cần bản ghi cuộc gọi). |
| `validation_analyst` (Product Discovery) | `design/user-research`, `data/statistical-analysis` ✱ | Kế hoạch phỏng vấn/khảo sát và kiểm định giả thuyết cho bước Validate. |
| `critic` (Research Critic, Opportunity Critic), `validation_analyst` | `data/validate-data` | Danh mục lỗi phân tích và checklist QA trước khi chia sẻ. Bổ sung cho `soat-so-lieu` (soát số) của Kit Việt, không thay. Chỉ soát, không viết phát hiện riêng. |
| `prd_writer` (squad Planning thẻ công ty) | `write-spec`, `roadmap-update` | Squad này không nạp skill BMAD nên không trùng. **Không** gắn `write-spec` cho `plan_pm` hoặc SDLC (đã có `bmad-prd`, `bmad-spec`); không gắn `product-brainstorming` (tương tác, trùng `superpowers:brainstorming`). |
| `ux` (SDLC) và `plan_ux` (BMAD Planning) | `design-handoff`, `ux-copy`, `design-critique` | Lấp đúng chỗ thiếu: spec cho dev (token, state, breakpoint), microcopy (khớp quy ước "UI descriptions" của repo), phê bình có khung. Không gắn `accessibility-review` (WCAG 2.1, trùng và kém hơn bộ `accessibility-*` WCAG 2.2 đã có). |
| SDLC Maintenance, vai `lead` | `tech-debt` | Công thức (Impact + Risk) × (6 − Effort) hợp mục "upkeep". 32 dòng, rủi ro thấp. Không gắn cho SDLC Greenfield. |
| Các skill `engineering` còn lại | **Không gắn** | `code-review`, `debug`, `architecture`, `system-design`, `testing-strategy`, `documentation`, `standup` trùng và mỏng hơn `sdlc-*`, `superpowers:*`, `bmad-*`. |

### 5.6 Executive Office · A
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `librarian` | `enterprise-search/knowledge-synthesis`, `search-strategy` | Gộp trùng nhiều nguồn, ưu tiên bản đầy đủ/chính thống/mới nhất, trích nguồn. Hợp với kiểm tra truy xuất và rà độ mới hằng tuần. Thay `~~` bằng `multica knowledge search`; bỏ `search`, `digest`, `source-management` (phụ thuộc kết nối). |
| `secretary` | **Không gắn** | Đã có `secretary-*`. `productivity/*` tranh chỗ với issue và KB. |

### 5.7 Sales Loop và B2B Proposal Desk · B
Sales Loop có thủ tục riêng (nguồn, `sales_account`, `sales_opportunity`, bài học theo độ tin) và quy tắc **không gửi gì**. 36 skill `sales` của KWP viết cho từng rep làm việc trên CRM, nên phần lớn không dùng được.
| Vai | Skill KWP | Lý do và xử lý trùng |
| --- | --- | --- |
| `sales_analyst` | `win-loss-review`, `deal-review`, `handle-objection` | Kiểm "đã xác nhận / giả định / chưa biết, kèm bằng chứng" khớp luật "bài học ghi ở độ tin thấp kèm số bằng chứng". Bỏ các bước đọc CRM ghi và gửi; dùng KB. |
| `b2b_quote_drafter` (`kv_b2b`) | `close-plan` | Kế hoạch hành động chung có ngày; Kit Việt chưa có. Phải qua `b2b_commit_gate` vì chứa cam kết. `handle-objection` trùng `xu-ly-tu-choi-b2b`: giữ bản Kit Việt. |
| `b2b_company_lookup` | `stakeholder-map` | Bản đồ người ra quyết định; bản Kit Việt chỉ có hồ sơ doanh nghiệp công khai. |
| Không gắn | `lead-triage`, `draft-outreach`, `call-summary`, `update-opportunity`, `log-activity`, `schedule-meeting`, `crm-hygiene-check`, `route-lead`, `forecast`, `pipeline-review`, `daily-briefing`, `end-of-day`, `inbox-sweep`, `rep-context`, `team-pipeline`, `weekly-wrap`, `deal-slip-scenario` | Ghi CRM/lịch/thư, hoặc làm việc của quota và quản lý rep, hoặc trùng `sales-scouting`/`sales-writing`. `customer-health`, `renewal-radar`, `expansion-whitespace` (khách đang dùng) để dành cho squad Customer Success nếu sau này cần. |

### 5.8 Legal và Accounting (Kit Việt) · hầu như không bổ sung
| Skill KWP | Quyết định |
| --- | --- |
| `legal-risk-assessment` | Gắn cho `legal_lead`: ma trận mức nghiêm trọng × khả năng và tiêu chí chuyển cấp. Trung lập luật. |
| `signature-request` | Không gắn (gửi DocuSign). Lấy phần **checklist trước khi ký** (bản cuối đã chốt, phụ lục đủ, pháp nhân đúng, người ký đúng thẩm quyền, có duyệt nội bộ) đưa vào `legal_handover`; bỏ bước gửi. |
| `triage-nda`, `review-contract` | Không gắn: trùng `ra-soat-thoa-thuan-bao-mat`, `ra-soat-hop-dong-theo-vi-the`. Học ý tưởng playbook và phân GREEN/YELLOW/RED vào hồ sơ đơn vị. |
| `legal-response`, `compliance-check`, `meeting-briefing`, `brief`, `vendor-check` | Không gắn: GDPR/CCPA thay cho Nghị định 13/2023 sẽ làm agent trích sai luật; phần còn lại cần CLM/CRM hoặc trùng `so-quan-ly-hop-dong`, `lich-thoi-han-phap-ly`. |

### 5.9 Squad P1 của Kit Việt · B
| Squad | Skill KWP có thể góp | Lưu ý |
| --- | --- | --- |
| Talent Acquisition | `interview-prep`, `draft-offer`, `onboarding` (đều 1–3 KB, khung mẫu) | Thư mời nhận việc chứa lương: quyết định của boss. Gate `soat-thien-kien-tuyen-dung` và `soat-du-lieu-nhan-su` của Kit Việt vẫn bắt buộc. Cách `small-business/hiring-screener` chấm ứng viên chỉ theo rubric đã nêu là ý hay, nhưng skill dài 35 KB và nối Gusto/Gmail. |
| Office Ops | `process-doc` (SOP, RACI), `vendor-review`, `change-request` | Bổ sung lớp quản lý cho 13 skill vận hành văn phòng; `vendor-review` gắn cùng `so-sanh-bao-gia-mua-sam` cần chọn một. |
| Academic Evidence Desk | Không | `bio-research` cần công cụ chuyên ngành (PubMed, Benchling, scverse). |

## 6. Squad và agent nên thêm

**Squad mới: `Data & Insights`** (floor `product`, hiện 3/6). Dùng khuôn Kit Việt (lead giao sub-issue, gate QUA/CHẶN, bàn giao). Hiện không squad nào đọc dữ liệu hay trả lời câu hỏi số liệu.
| Vai | Loại | Skill KWP |
| --- | --- | --- |
| `data_lead` | lead | `analyze` |
| `data_profiler` | collector | `explore-data`, `data-context-extractor` ✱ |
| `data_analyst` | drafter | `sql-queries` (+ `write-query` trùng nên bỏ), `statistical-analysis` |
| `data_viz` | drafter | `create-viz` (bỏ `data-visualization` trùng), `build-dashboard` (25 KB, xuất HTML tự đủ) |
| `data_validator` | gate | `validate-data` |
| (hoãn) `data_handover` | handover | `kiem-dinh-ban-giao` dùng chung; thêm khi `kv-delivery-core` có. Bản đầu: lead tự đối chiếu với câu hỏi (mục 10.7) |

Điều kiện: runtime có quyền đọc cơ sở dữ liệu (chỉ đọc) và Python. Quy tắc cứng đưa vào charter: truy vấn chỉ đọc, kết quả phân trang không là tổng, số 0 và rỗng là hai thứ khác nhau (theo `absent-is-not-zero`).

**Agent mới trong squad có sẵn**: `growth_analyst` (Growth & GTM) và `risk_analyst` (Finance & Risk), như mục 5.2 và 5.3. Cả hai cần instruction template mới trong `instruction-templates.ts`
và thẻ cập nhật trong `company-template.ts`. Thẻ công ty là bộ 14 squad cố định mà apply service khớp theo key; thêm vai vào một squad có sẵn thay đổi cả lần chạy lại (rerun) nên cần kiểm test `company-template.test.ts`.

## 7. Trùng lặp bên trong KWP (chọn một)

| Việc | Các bản trong KWP | Chọn |
| --- | --- | --- |
| Phân tích đối thủ | `product-management/competitive-brief`, `marketing/competitive-brief`, `sales/competitive-intelligence` | PM cho nghiên cứu; marketing cho định vị; sales bỏ |
| Soạn nội dung | `marketing/content-creation`, `marketing/draft-content` | Không dùng cả hai (đã có Kit Việt) |
| Bút toán | `finance/journal-entry`, `finance/journal-entry-prep` | Không dùng (US GAAP) |
| SOX | `finance/sox-testing`, `finance/audit-support` | Không dùng |
| Tổng hợp nghiên cứu người dùng | `design/research-synthesis`, `product-management/synthesize-research`, `sales/customer-voice` | PM |
| Nghiên cứu tài khoản, chuẩn bị cuộc gọi | `sales/*` và `partner-built/common-room/*` | Không dùng (cần Common Room/CRM) |
| Phân loại lead | `sales/lead-triage`, `small-business/lead-triage` | Không dùng (đã có `sales-scouting`) |
| Viết truy vấn | `data/sql-queries`, `data/write-query` | `sql-queries` |
| Biểu đồ | `data/create-viz`, `data/data-visualization` | `create-viz` |
| Quét a11y | `design/accessibility-review` | Không dùng (đã có `accessibility-*`) |

## 8. Cái không nên đưa vào, và lý do

| Nhóm | Lý do |
| --- | --- |
| `productivity` (4) | `TASKS.md` và bộ nhớ hai tầng tranh chỗ với issue và KB. |
| `small-business` (44, phần skill) | 6–55 KB mỗi skill, gắn QuickBooks, PayPal, Shopify, Gusto, Canva, HubSpot, TikTok Ads, giả định luật thuế Mỹ. Chỉ đọc các tài liệu `shared/`. |
| `partner-built/{apollo,common-room,slack,zoom-plugin}` | Cần dịch vụ bên thứ ba. `brand-voice` hữu ích khi chưa có cẩm nang thương hiệu (`guideline-generation` rút giọng văn từ tài liệu và cuộc gọi), nhưng `kv_marketing` đã có `cam-nang-giong-thuong-hieu`; xếp sau. |
| `bio-research` (6) | Ngách khoa học sự sống; mọi skill cần công cụ chuyên ngành. |
| `pdf-viewer`, `cowork-plugin-management` | Cần trình xem PDF; hoặc để làm plugin chứ không làm việc. `create-cowork-plugin` có thể tham khảo cho Skill/Plugin Hub (nhánh `skill-plugin-hub`) khi viết trình tạo gói. |
| `human-resources` còn lại | `performance-review`, `comp-analysis`, `people-report` bàn về từng cá nhân, trái hướng của People & Performance (mục 9). |

## 9. Xung đột tìm thấy ở thiết kế hiện có

**People & Performance và skill nhân sự.** Charter của squad ghi "Reports on squads, not on individuals… Sets no targets". Spec Kit Việt mục 7 lại đề nghị gắn vào squad này
`soat-hop-dong-lao-dong`, `soat-bang-cham-cong`, `onboarding-30-60-90`, `quy-trinh-ky-luat-lao-dong`, `danh-gia-thu-viec`: các skill về hợp đồng lao động, chấm công và kỷ luật cá nhân.
Hai thứ mâu thuẫn ngay trong một squad. Đề xuất: chuyển các skill này sang squad Talent Acquisition hoặc một squad People Ops riêng, để People & Performance giữ đúng việc đọc hồ sơ squad. Cần bạn quyết (mục 11).
KWP `human-resources` không sửa được chỗ này, nó cũng thuộc nhóm cá nhân.

## 10. Thiết kế

### 10.1 Hai đường gắn skill, và vì sao cần cả hai

Mã hiện có hai đường khác nhau; KWP phải đi đúng đường của từng squad.

| Đường | Dùng cho | Cơ chế hiện có | Việc phải thêm |
| --- | --- | --- | --- |
| **Thẻ công ty** | 14 squad thẻ (Support, Growth & GTM, Finance & Risk, Release & Ops, Executive Office, Market Research, Opportunity Lab, Competitor Intelligence, Customer Voice, Product Discovery, Planning) | Máy chủ Go (`server/internal/service/company_starter_skills.go`): `starterSkillsForRole` chỉ cho **leader** hai skill `squad-reporting`, `directive-execution`; thành viên không có skill. Skill starter lấy từ `catalog/skills/starter/` qua `company_template_data.json` (sinh bởi `pnpm generate:company-template`). | Thêm trường `skills` vào vai của thẻ (`CompanyRoleSchema`), đi qua JSON sang Go; `starterSkillsForRole` trả thêm `role.Skills`; generator đưa các skill KWP được thẻ nhắc đến vào `starter_skills`. |
| **Template Squads quickstart** | SDLC Greenfield/Maintenance, Market Research, Sales Loop, BMAD Planning, `data_insights` (mới), các `kv_*` (spec Kit Việt) | `attachBuiltinSkills` trong `packages/core/squads/quickstart.ts` gắn **cấp squad** bằng `setSquadSkills`; mọi thành viên nhận. | `SquadTemplate.roleSkills` (gắn từng vai bằng `addAgentSkills`), đúng như spec Kit Việt 5.10 và 10.2 dòng a. Nếu khung Kit Việt chưa có thì KWP làm phần này theo đúng thiết kế đó (một bản, không hai). |

Quyết định chung:
- Tên skill KWP có tiền tố `kwp-` và dùng gạch ngang, **không dùng dấu hai chấm** (`kwp-ticket-triage`). Tránh rủi ro tên có `:` trên đĩa và ở bước kiểm của Go; thư mục lưu trùng tên skill.
- `isStarterSkillConfig` chỉ cho phép gắn skill do starter tạo; skill KWP cùng tên của boss được giữ nguyên, không ghi đè (hành vi hiện có).
- Rerun an toàn: apply gắn thêm skill còn thiếu cho agent đã có (`attachStarterSkills` bỏ qua cái đã có), không xóa gì.

### 10.2 Nguồn, thư mục và sinh dữ liệu

```
packages/core/squads/
  kwp-upstream/            # bản chép nguyên văn, ghim commit e605d8d, không sửa tay
    LICENSE  NOTICE.md  UPSTREAM.json
    <plugin>/skills/<skill>/...        # chỉ 43 skill được chọn (mục 10.5)
  kwp-rewrites.json        # bảng chuyển đổi từng skill (mục 10.3)
  kwp-skills/              # đầu ra của generator; mỗi thư mục là một skill hoàn chỉnh
    kwp-ticket-triage/SKILL.md ...
    kwp-untrusted-content/SKILL.md     # 3 skill viết mới nằm thẳng ở đây, không có upstream
  kwp-skills-data.ts       # sinh; nạp bằng import() động, không vào gói của trang không dùng
scripts/generate-kwp-skills.mjs   # pnpm generate:kwp-skills (và --check)
```

- `generate-kwp-skills.mjs` đọc `kwp-upstream` + `kwp-rewrites.json`, ghi `kwp-skills/` và `kwp-skills-data.ts`. Dùng lại `scripts/lib/skill-bundle.mjs` (spec Kit Việt, PR 1 K1). Nếu lib chưa có, tạo nó theo mô tả ở đó và để `generate-sdlc-skills.mjs` gọi lại.
- Dữ liệu sinh không sửa tay; test `--check` fail khi tệp cũ. Tên skill trùng nhau giữa hai plugin thì generator báo lỗi (KWP có `competitive-brief` ở ba plugin; chỉ bản `product-management` được vendor).
- `UPSTREAM.json`: `{ "repo": "anthropics/knowledge-work-plugins", "ref": "e605d8ddbfb688da57dfaaceebd93f12a64ef1d3", "license": "Apache-2.0", "fetchedAt": "2026-11-09" }`. `NOTICE.md` nêu nguồn, giấy phép, và "mọi thay đổi do `kwp-rewrites.json` và generator thực hiện" (Apache-2.0 yêu cầu giữ thông báo và nêu tệp đã đổi). Mỗi `SKILL.md` đầu ra có dòng chú thích nguồn sau frontmatter.
- Đường thẻ công ty: `generate-company-template.mjs` bổ sung vào `starter_skills` những skill `kwp-*` mà vai trong `COMPANY_SQUAD_CARDS` nhắc đến (đọc từ `kwp-skills/`), cùng các skill `skills/` hiện có. Kích thước `company_template_data.json` tăng bao nhiêu phải được báo trong PR (ngân sách: dưới 400 KB thêm).

### 10.3 Quy tắc chuyển đổi (`kwp-rewrites.json`)

Mỗi skill có một mục `{ "skill": "<plugin>/<tên>", "drop": [...], "replace": [...], "append": [...], "description": "..." }`. Generator áp theo thứ tự và **dừng với lỗi** nếu một mỏ neo không tìm thấy hoặc trùng nhiều chỗ.

Chuyển đổi chung (generator làm cho mọi skill, không cần liệt kê):
1. Frontmatter: `name: kwp-<tên>`; bỏ `argument-hint`, `user-invocable`; giữ `description` (cho phép ghi đè bằng `description` để mở đầu bằng "Use when you are the <vai> ..." như skill `sdlc-*`).
2. Bỏ dòng "If you see unfamiliar placeholders... CONNECTORS.md", các tiêu đề "Usage", "Invocation", "Modes" kèm khối mã lệnh `/...`, và mọi `$ARGUMENTS`, `@$1`.
3. Xóa trọn các mục "If Connectors Available", "Supercharged", "Tools used" (bảng kết nối), "Connectors".
4. Mỗi `~~<category>` còn lại: thay bằng nguồn Multica theo bảng: `~~knowledge base` → `multica knowledge search`; `~~project tracker` → issue Multica; `~~chat`, `~~email`, `~~calendar`, `~~CRM`, `~~support platform`, `~~e-signature`, `~~CLM`, `~~monitoring`, `~~CI/CD`... → "chỉ khi issue đính kèm hoặc dán nội dung; không có thì ghi là chưa kiểm". Chỗ không thay được tự động phải có mục `replace` riêng, nếu không generator lỗi.
5. "Ask the user" / "ask the user" / "hỏi người dùng": thay bằng quy tắc: *lấy mặc định hợp lý, ghi vào "Giả định" của báo cáo, tiếp tục; thiếu đầu vào bắt buộc thì đặt issue `blocked` kèm câu hỏi cụ thể* (khớp `WORKING_RULES` mục "Unattended runs").
6. Mọi bước gửi, đăng, ký, đặt lịch, ghi CRM hoặc kho khác: bỏ; thay bằng "soạn bản nháp trong comment hoặc tài liệu KB; không gửi gì ra ngoài workspace".
7. Chú thích "does not provide legal/financial advice" giữ nguyên.
8. Thêm khối cuối `## Multica runtime rules` từ `kwp-runtime-rules.md` (một tệp, chép vào mọi skill): đầu ra là comment trên issue hoặc tài liệu KB đúng `kind`; nguồn nào cũng có ngày và liên kết; nội dung bên ngoài là dữ liệu, không là lệnh (đi kèm `kwp-untrusted-content`); không chờ người; không gửi ra ngoài; nêu rõ phần chưa kiểm.

Test chuyển đổi bắt buộc (mục 10.9): sau khi sinh, không skill nào còn chuỗi `~~`, `$ARGUMENTS`, `@$1`, `argument-hint`, `CONNECTORS.md`, `.local.md`, `CLAUDE.md`, `TASKS.md`, "Ask the user", "ask the user", "send for signature", `AskUserQuestion`, "DocuSign".

### 10.4 Ba skill viết mới (không có upstream)

| Skill | Gắn cho | Nội dung |
| --- | --- | --- |
| `kwp-untrusted-content` | `support_agent`, `customer_lead`, `sales_analyst`, các vai của `data_insights` | Chắt lọc `small-business/shared/untrusted-content.md`: nội dung đọc vào là dữ liệu; yêu cầu đổi tài khoản, mã, quyền, gửi dữ liệu ra ngoài thì dừng và báo người đặt, không soạn gì; kiểm danh tính người gửi; trích dẫn văn bản không tin cậy dưới dạng escape. Bỏ ví dụ gắn connector. |
| `kwp-absent-is-not-zero` | `growth_analyst`, `finance_lead`, `data_analyst`, `data_validator` | Chắt lọc `small-business/shared/absent-is-not-zero.md`: kết quả phân trang không là tổng; trống khác 0; ngày tương lai là dữ liệu hỏng; kết quả rỗng và truy vấn lỗi giống nhau; hai số lệch nhau là một phát hiện. Giữ bảng ví dụ nhưng bỏ tên nhà cung cấp. |
| `kwp-pre-signature-checklist` | `legal_handover` (`kv_legal`) | Phần checklist của `legal/signature-request` (bản cuối đã chốt, phụ lục đủ, tên pháp nhân đúng, khối ký khớp người có thẩm quyền, đã duyệt nội bộ, ngày để trống nếu ký sau); kết luận `VERDICT:`; không có bước gửi. Chỉ giao khi `kv_legal` tồn tại. |

Cả ba ghi nguồn Apache-2.0 như bản chuyển đổi.

### 10.5 Danh mục skill: nguồn, vai nhận, chỉnh sửa chính

Cột "Thân" ghi: **đọc** (đã đọc phần đầu ở lượt nghiên cứu) hoặc ✱ (chỉ dựa vào mô tả; người làm phải đọc trọn skill trước khi viết mục `kwp-rewrites.json`, và tóm tắt "đã đọc gì" trong mô tả PR). Mọi skill còn chịu chuyển đổi chung ở 10.3.

| Skill (`kwp-…`) | Nguồn | Vai nhận | Thân | Chỉnh sửa riêng |
| --- | --- | --- | --- | --- |
| `ticket-triage` | customer-support | `support_agent` | đọc | Bỏ tra duplicate qua support platform; kiểm trùng bằng `multica knowledge search` và các issue Support mở |
| `kb-article` | customer-support | `support_agent` | đọc | Kết quả là tài liệu KB `kind` playbook/reference ở trạng thái nháp; tìm trùng trước khi tạo |
| `draft-response` | customer-support | `support_agent` | đọc | Description mở đầu "Use when the customer writes in English"; câu tiếng Việt xem skill Kit Việt `phan-hoi-khieu-nai-cskh`. Không hứa ngày, giá, hoàn tiền: dẫn tới proposal |
| `customer-escalation` | customer-support | `support_lead` | đọc | Người nhận là squad Build qua `multica issue create --assignee-squad`; giữ phần tái hiện, ảnh hưởng, ưu tiên |
| `customer-research` | customer-support | `support_lead` | ✱ | Nguồn là KB và issue; bỏ nguồn kết nối |
| `campaign-plan` | marketing | `growth_lead` | ✱ | Kết quả là tài liệu KB kind `gtm_plan`; chi tiêu, giá, công bố công khai chỉ là đề xuất cho boss |
| `performance-report` | marketing | `growth_lead`, `growth_analyst` | đọc | Kết quả là `metrics_report`; số liệu do người đặt dán hoặc đính kèm; bảng trạng thái so với mục tiêu trong gtm_plan |
| `metrics-review` | product-management | `growth_lead`, `growth_analyst` | đọc | Như trên; thêm bước "mỗi số liên kết nguồn dữ liệu" (yêu cầu của weekly growth review) |
| `statistical-analysis` | data | `growth_analyst`, `validation_analyst`, `data_analyst` | ✱ | Giữ nguyên phương pháp; ghi cỡ mẫu và giả định |
| `vendor-review` | operations | `finance_lead` | ✱ | Kết quả là nhận xét trên proposal chi tiêu; **không duyệt gì** (charter Finance & Risk) |
| `variance-analysis` | finance | `finance_lead` | đọc | Giữ phần phân rã giá/khối lượng/cơ cấu; bỏ ví dụ USD thành đơn vị tiền tệ do người đặt nêu; không nhắc GAAP/SOX |
| `risk-assessment` | operations | `risk_analyst` | đọc | Mẫu sổ rủi ro thêm cột "ngày kiểm lần cuối"; kết quả là tài liệu KB kind `risk_register` |
| `compliance-tracking` | operations | `risk_analyst` | ✱ | Theo dõi nghĩa vụ và hạn; không nhận định pháp lý |
| `deploy-checklist` | engineering | `release_engineer` | đọc | Không gọi lệnh deploy; checklist kèm tiêu chí rollback; production cần boss duyệt (giữ luật sẵn có) |
| `incident-response` | engineering | `release_engineer` | đọc | Giữ SEV1–4, nhịp cập nhật, postmortem không đổ lỗi, 5 whys; bỏ gọi PagerDuty/Slack; thông báo khách chỉ là bản nháp |
| `runbook` | operations | `release_engineer` | ✱ | Các lệnh viết trong runbook là văn bản, agent không chạy chúng trên production |
| `change-request` | operations | `release_engineer` | ✱ | Là bản nháp để boss duyệt; có kế hoạch rollback |
| `status-report` | operations | `release_lead` | đọc | Giữ mẫu xanh/vàng/đỏ; skill ngắn, dùng làm khuôn |
| `competitive-brief` | product-management | `competitor_analyst`, `competitor` | đọc | Nguồn chính có ngày và liên kết (khớp `market_scout`); phần web search giữ |
| `synthesize-research` | product-management | `customer_lead` | ✱ | Nguồn là ghi chú, phiếu hỗ trợ, KB; trích dẫn nguyên văn có nguồn |
| `user-research` | design | `validation_analyst` | ✱ | Kế hoạch phỏng vấn và khảo sát là tài liệu nháp; không liên hệ ai |
| `validate-data` | data | `critic` (Market Research, Opportunity Lab), `validation_analyst`, `data_validator` | đọc | Chuyển thành vai gate: kết luận dòng `VERDICT: QUA` hoặc `VERDICT: CHẶN`, không sửa bài |
| `write-spec` | product-management | `prd_writer` | đọc | Chỉ thẻ Planning; **không** gắn cho SDLC hay BMAD |
| `roadmap-update` | product-management | `prd_writer` | ✱ | Kết quả là tài liệu KB |
| `knowledge-synthesis` | enterprise-search | `librarian` | đọc | `~~` thành `multica knowledge search`; giữ gộp trùng và chấm độ tin theo độ mới và độ chính thống |
| `search-strategy` | enterprise-search | `librarian` | ✱ | Phân rã câu hỏi theo nguồn KB và loại tài liệu |
| `design-handoff` | design | `ux`, `plan_ux` | ✱ | Kết quả là tệp spec cho developer trong repo; token theo `packages/ui/styles` của dự án khi có |
| `ux-copy` | design | `ux`, `plan_ux` | ✱ | Thêm tham chiếu quy ước "UI descriptions" của dự án khi squad làm việc trên repo Multica |
| `design-critique` | design | `ux`, `plan_ux` | đọc | Gate mềm; kết luận có mức nghiêm trọng, không sửa |
| `tech-debt` | engineering | `lead` của `sdlc_maintenance` | đọc | Công thức ưu tiên giữ nguyên; kết quả là issue backlog, không tự sửa |
| `win-loss-review` | sales | `sales_analyst` | đọc | Nguồn là KB Sales Loop (`sales_account`, `sales_opportunity`); bỏ CRM; bài học ghi ở độ tin thấp kèm số bằng chứng |
| `deal-review` | sales | `sales_analyst` | đọc | Giữ kiểm xác nhận/giả định/chưa biết kèm bằng chứng; bỏ xác suất CRM |
| `handle-objection` | sales | `sales_analyst` | đọc | Chỉ phân loại và chuẩn bị luận điểm; không soạn thư gửi |
| `close-plan` | sales | `b2b_quote_drafter` (`kv_b2b`) | ✱ | Phải qua `b2b_commit_gate` trước khi ra; không gửi khách |
| `stakeholder-map` | sales | `b2b_company_lookup` (`kv_b2b`) | ✱ | Dữ liệu cá nhân tối thiểu; theo `an-thong-tin-nhay-cam` của Kit Việt |
| `legal-risk-assessment` | legal | `legal_lead` (`kv_legal`) | ✱ | Trung lập luật; gắn nhãn "không thay ý kiến luật sư" |
| `seo-audit` | marketing | `mkt_scout` (`kv_marketing`) | đọc | Không có công cụ SEO thì dựa vào web search và nói rõ mức chính xác; kiểm từ khóa tiếng Việt bằng skill Kit Việt |
| `analyze` | data | `data_lead` | đọc | Mở đầu thành thủ tục lead: nhận câu hỏi, giao sub-issue, kiểm kết quả |
| `explore-data` | data | `data_profiler` | đọc | Chỉ đọc; báo tỷ lệ null, trùng, giá trị lạ |
| `data-context-extractor` | data | `data_profiler` | ✱ | Chỉ chế độ BOOTSTRAP: ghi bối cảnh dữ liệu của công ty vào tài liệu KB; chế độ cần phỏng vấn nhà phân tích thì bỏ |
| `sql-queries` | data | `data_analyst` | đọc | **Chỉ `SELECT`**; bỏ `write-query` (trùng) |
| `create-viz` | data | `data_viz` | ✱ | Bỏ `data-visualization` (trùng) |
| `build-dashboard` | data | `data_viz` | ✱ | 25 KB, dài nhất danh sách; giữ đầu ra HTML tự đủ, bỏ gọi ngoài |

Tổng: 43 skill chuyển đổi cộng 3 skill viết mới = 46. **Hoãn** (chưa làm ở lượt này): `process-doc`, `interview-prep`, `draft-offer`, `onboarding` (đợi Office Ops và Talent Acquisition P1 của Kit Việt), `financial-statements`, `brand-voice/*`.

Giới hạn mỗi vai: tối đa 6 skill KWP, cộng skill Kit Việt cùng vai không quá 6 mỗi nhóm theo spec Kit Việt 5.10. Test kiểm số này và kiểm không vai nào nhận hai skill trong cùng một dòng của bảng ở mục 7.

### 10.6 Thay đổi thẻ công ty

1. `CompanyRoleSchema` thêm `skills: z.array(z.string().regex(KEY_PATTERN_SKILL)).default([])` (tên `kwp-…` hoặc skill starter hiện có); `superRefine` kiểm mỗi tên tồn tại trong `kwp-skills/` hoặc `skills/`.
2. Hai agent mới (đồng thời là hai `AgentInstructionTemplate.id`): `growth_analyst`, `risk_analyst` trong `agents/instruction-templates.ts`, dùng `SHARED_BLOCKS` và `WORKING_RULES`. Nội dung chính:
   - `growth_analyst`: đọc số, soạn `metrics_report` theo yêu cầu của `growth_lead`; không đặt mục tiêu, không chi tiền, không đăng; kết quả có "chưa kiểm"; số có nguồn.
   - `risk_analyst`: giữ sổ rủi ro `risk_register` (khả năng, tác động, người phụ trách, ngày kiểm lần cuối); rủi ro không có người phụ trách thì gắn cờ và đề xuất; mọi quyết định cho boss đi qua `multica proposal create --squad "Finance & Risk"`; không duyệt, không cam kết.
3. Thẻ `growth_gtm` thêm vai `growth_analyst`; thẻ `finance_risk` thêm vai `risk_analyst` (mô tả ≤200 ký tự, theo "UI descriptions"). Charter hai squad thêm một dòng `responsibilities` nêu việc mới; `boundaries` giữ nguyên.
4. Bản dịch tên và mô tả hai agent mới trong `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/agents.json` (khóa theo `id`, cạnh `growth_lead`), và kiểu trong `packages/views/i18n/resources-types.ts` nếu cần.
5. Go: `companyRole` thêm trường `Skills []string` với thẻ JSON `skills`; `starterSkillsForRole` trả `append(leaderSkills, role.Skills...)` không trùng; `COMPANY_SQUAD_CARDS` vẫn 14 squad (test `company-template.test.ts` giữ `toHaveLength(14)`).
6. Hậu quả với workspace có sẵn: rerun apply tạo thêm hai agent mới vào squad đã có và gắn skill; kiểm bằng test có DB (mục 10.9).

Bảng vai và skill của thẻ công ty (đầu ra của bước 1, chép vào `company-template.ts`):

| Squad | Vai | `skills` |
| --- | --- | --- |
| `executive_office` | `librarian` | `kwp-knowledge-synthesis`, `kwp-search-strategy` |
| `market_research` | `critic` | `kwp-validate-data` |
| `opportunity_lab` | `critic` | `kwp-validate-data` |
| `competitor_intelligence` | `competitor_analyst` | `kwp-competitive-brief` |
| `customer_voice` | `customer_lead` | `kwp-synthesize-research`, `kwp-untrusted-content` |
| `product_discovery` | `validation_analyst` | `kwp-user-research`, `kwp-statistical-analysis`, `kwp-validate-data` |
| `planning` | `prd_writer` | `kwp-write-spec`, `kwp-roadmap-update` |
| `release_ops` | `release_lead` | `kwp-status-report` |
| `release_ops` | `release_engineer` | `kwp-deploy-checklist`, `kwp-incident-response`, `kwp-runbook`, `kwp-change-request` |
| `growth_gtm` | `growth_lead` | `kwp-campaign-plan`, `kwp-performance-report`, `kwp-metrics-review` |
| `growth_gtm` | `growth_analyst` (mới) | `kwp-performance-report`, `kwp-metrics-review`, `kwp-statistical-analysis`, `kwp-absent-is-not-zero` |
| `support` | `support_lead` | `kwp-customer-escalation`, `kwp-customer-research` |
| `support` | `support_agent` | `kwp-ticket-triage`, `kwp-kb-article`, `kwp-draft-response`, `kwp-untrusted-content` |
| `finance_risk` | `finance_lead` | `kwp-vendor-review`, `kwp-variance-analysis`, `kwp-absent-is-not-zero` |
| `finance_risk` | `risk_analyst` (mới) | `kwp-risk-assessment`, `kwp-compliance-tracking` |

### 10.7 Thay đổi template Squads quickstart

`SquadTemplate` thêm `roleSkills?: Partial<Record<SquadRoleKey, readonly string[]>>`. `attachBuiltinSkills` tra tên `kwp-*` bằng `import("./kwp-skills-data")`; các tên khác tra như hiện nay. Sau khi tạo agent, `addAgentSkills(agentId, skillIds)` cho từng vai. `SquadQuickstartClient` thêm `addAgentSkills` nếu chưa có (kiểm `packages/core/api/client.ts`).

| Template | Vai | `roleSkills` |
| --- | --- | --- |
| `sdlc_greenfield`, `sdlc_maintenance` | `ux` | `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique` |
| `sdlc_maintenance` | `lead` | `kwp-tech-debt` |
| `bmad_planning` | `plan_ux` | `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique` |
| `market_research` | `competitor` | `kwp-competitive-brief` |
| `market_research` | `critic` | `kwp-validate-data` |
| `sales_loop` | `sales_analyst` | `kwp-win-loss-review`, `kwp-deal-review`, `kwp-handle-objection`, `kwp-untrusted-content` |

Đã có `builtinSkills` cấp squad cho SDLC và BMAD; `roleSkills` cộng thêm, không thay.

**Template mới `data_insights` · Data & Insights**: `floorName: "Product"`; không routine; năm vai (khác báo cáo gốc, thêm vai bàn giao khi `kv-delivery-core` có).

| Role key | Agent template | Tên | Loại | `roleSkills` |
| --- | --- | --- | --- | --- |
| `data_lead` | `data_lead` (mới) | Data Lead | lead | `kwp-analyze`, `kwp-untrusted-content` |
| `data_profiler` | `data_profiler` (mới) | Data Profiler | collector, chỉ đọc | `kwp-explore-data`, `kwp-data-context-extractor`, `kwp-untrusted-content` |
| `data_analyst` | `data_analyst` (mới) | Data Analyst | drafter | `kwp-sql-queries`, `kwp-statistical-analysis`, `kwp-absent-is-not-zero`, `kwp-untrusted-content` |
| `data_viz` | `data_viz` (mới) | Data Visualizer | drafter | `kwp-create-viz`, `kwp-build-dashboard` |
| `data_validator` | `data_validator` (mới) | Data Validator | gate, không sửa | `kwp-validate-data`, `kwp-absent-is-not-zero` |

- Luồng: lead nhận câu hỏi → profiler mô tả dữ liệu → analyst viết truy vấn và phân tích → viz dựng biểu đồ/bảng → validator ra `VERDICT: QUA|CHẶN` (tối đa 2 vòng sửa, theo giao thức Kit Việt 5.2) → lead đối chiếu với câu hỏi ban đầu rồi báo cáo.
- Charter: mission "Trả lời câu hỏi bằng số liệu có nguồn, kèm giới hạn của dữ liệu"; outputs `data_report` (kb_document), `data_findings` (report); boundaries "chỉ đọc dữ liệu; không ghi, không xóa, không chia sẻ ra ngoài workspace; số liệu không có nguồn không được đưa vào báo cáo"; escalation "thiếu quyền hoặc nguồn dữ liệu: `blocked` kèm câu hỏi cho người đặt".
- Điều kiện bật: runtime có quyền đọc cơ sở dữ liệu (chỉ đọc) hoặc tệp CSV đính kèm, và Python cho biểu đồ. Không có thì lead đặt `blocked`, không phân tích trên dữ liệu bịa.
- `SquadRoleKey` thêm năm khóa trên; `SquadTemplateId` thêm `data_insights`; bản dịch `modals.json` bảy ngôn ngữ (khóa `templates.data_insights`), `agents.json` cho năm agent template; test `templates.test.ts` cập nhật số lượng.

### 10.8 Sửa spec Kit Việt đã có

- Mục 7 của [kitviet-squads.md](kitviet-squads.md): bỏ `People & Performance ← HR vận hành`; ghi các skill `soat-hop-dong-lao-dong`, `soat-bang-cham-cong`, `onboarding-30-60-90`, `quy-trinh-ky-luat-lao-dong`, `danh-gia-thu-viec` thuộc Talent Acquisition hoặc `People Ops` (P1), vì charter People & Performance là "reports on squads, not on individuals".
- Mục 5.10 và 9: bảng vai Kit Việt nhận thêm cột "skill KWP" theo mục 10.5 (b2b, legal, marketing) và danh sách hoãn của KWP (`process-doc`, `interview-prep`, `draft-offer`, `onboarding` cho Office Ops / Talent Acquisition).
- Mục 7, dòng Support: ghi quy tắc ngôn ngữ giữa `phan-hoi-khieu-nai-cskh` (tiếng Việt) và `kwp-draft-response` (tiếng Anh).

### 10.9 Kiểm thử và tiêu chí xong

Test mặc định không gọi mạng, không chạy agent CLI thật.

| Test | Vị trí | Kiểm |
| --- | --- | --- |
| Sinh dữ liệu không cũ | `packages/core/squads/kwp-skills.test.ts` | `pnpm generate:kwp-skills --check` không đổi; số skill = 46; mỗi thư mục có `SKILL.md` với `name` bằng tên thư mục |
| Chuyển đổi sạch | như trên | Không còn chuỗi cấm (mục 10.3) trong `kwp-skills/` |
| Bảng gắn skill | như trên | Mọi tên trong `skills`/`roleSkills` tồn tại; tối đa 6 skill KWP mỗi vai; không hai skill cùng nhóm trùng (mục 7) trên một vai |
| Thẻ công ty | `company-template.test.ts`, `company-template-data.test.ts` | 14 squad; `skills` hợp lệ; JSON sinh khớp hằng số |
| Starter skills Go | `server/internal/service/company_starter_skills_test.go` | `starterSkillsForRole` trả leader skills + `Skills`, không trùng, thứ tự ổn định |
| Apply có DB | `server/internal/service/phase4_company_db_test.go` hoặc test mới | Chạy apply hai lần: lần hai không nhân đôi agent/skill; skill `kwp-*` được gắn cho đúng agent; skill cùng tên của boss không bị ghi đè |
| Template | `templates.test.ts` | `roleSkills` chỉ nêu vai thuộc template; `data_insights` đủ vai, leader, charter, `floorName` |
| Quickstart | `quickstart.test.ts` | `addAgentSkills` gọi đúng vai; `kwp-*` nạp động; gói chính không chứa `kwp-skills-data` |
| Khung kích thước | PR mô tả | Chunk của trang không dùng KWP không tăng; `company_template_data.json` tăng dưới 400 KB |

Chạy thật có người cho phép (theo spec Kit Việt 8.2), mỗi nhóm một lần: đủ đầu vào, thiếu đầu vào (kỳ vọng `blocked`), và bản nháp cố tình sai (kỳ vọng gate CHẶN): Support (`ticket-triage`), Release & Ops (`incident-response`), Data & Insights (`validate-data` với một truy vấn cố ý có join nổ).

Tiêu chí xong của một PR: `pnpm typecheck`, `pnpm lint`, `pnpm test` xanh; `pnpm generate:kwp-skills --check` và `pnpm generate:company-template` không đổi tệp; test Go liên quan xanh (`make test` hoặc test hẹp; nhớ `go-test-db-leftovers`: kiểm dòng sót trước khi gọi một lỗi là hồi quy); mô tả PR nêu phạm vi đã đọc của skill ✱; không mở PR vào `multica-ai/multica`.

### 10.10 Rủi ro

| Rủi ro | Giảm thiểu |
| --- | --- |
| Skill chuyển đổi còn sót bước gửi hoặc chờ người | Test chuỗi cấm; đọc trọn skill ✱ khi viết mục `kwp-rewrites.json`; chạy thật thiếu đầu vào |
| `company_template_data.json` phình, nhúng vào binary Go | Chỉ đưa skill được thẻ nhắc; đo và báo; ngân sách 400 KB |
| Vai công ty đã có nhận thêm agent làm thay đổi squad đang chạy | Rerun chỉ cộng thêm; test apply hai lần; ghi vào ghi chú phát hành |
| Hai skill cùng việc trên một vai (KWP và Kit Việt) | Bảng trùng (mục 7) thành test; quy tắc luật Việt thắng |
| Skill dài (25 KB `build-dashboard`) làm nặng ngữ cảnh | Gắn theo vai, không theo squad; chỉ `data_viz` mang |
| Nâng commit upstream sau này | `UPSTREAM.json` + `generate` + đọc diff; ghi vào `kitviet-runbook.md` hoặc runbook KWP |

## 11. Quyết định đã chốt (2026-11-11)

Chủ sản phẩm đồng ý toàn bộ đề xuất của bản nghiên cứu:
1. Thêm squad `Data & Insights` (mục 10.7).
2. Thêm agent `growth_analyst` và `risk_analyst` vào thẻ công ty (mục 10.6).
3. Skill nhân sự cá nhân chuyển sang Talent Acquisition hoặc People Ops; sửa spec Kit Việt mục 7 (mục 10.8).
4. Support: `kwp-draft-response` chỉ cho tiếng Anh; tiếng Việt dùng `phan-hoi-khieu-nai-cskh` của Kit Việt.
5. Chấp nhận Apache-2.0 kèm NOTICE cho phần vendor.

Ngoài phạm vi: cài plugin KWP lên runtime; MCP server của KWP; skill `sales` ghi CRM; `small-business`; `productivity`; `bio-research`; `partner-built/*`; mọi skill chạy bằng slash command; thay đổi schema cơ sở dữ liệu. Không có migration.
