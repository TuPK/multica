# Marketing domain: một nơi cho squad, skill và thị trường

Trạng thái: đã triển khai catalog và engine, đã commit lên main. Diễn tập trên máy chủ dev đã chạy đến cổng stage 3 và dừng do thiếu tài liệu của squad (xem mục 12). Người dùng giao toàn quyền chọn phương án cho mọi câu hỏi mở (2026-11-16); các quyết định D1 đến D16 ở mục 0 do kỹ sư thiết kế chọn, lý do ở mục 5 và mục 6. Kế hoạch: [plan.md](plan.md).
Bản sửa lần 2 (2026-11-16): bản đầu chỉ gom các squad vào một nhóm và hoãn "một lần tạo là dùng được cả bộ" (D13 cũ). Yêu cầu của người dùng là chỉ tạo **một** squad Marketing là dùng được toàn bộ phần đang có; bản này thêm squad cửa trước `marketing` và cài cả bộ bằng một lần chọn (D13, D15, D16, mục 5.11).
Ngày rà soát: 2026-11-16, trên HEAD `eac969224`. Cây làm việc có nhiều thay đổi chưa commit ở `platform-management/` và `server/`; không có thay đổi nào dưới `catalog/` hay `packages/core/` lúc rà soát.

Ngôn ngữ: tiếng Việt. Định danh, đường dẫn và lệnh giữ nguyên tiếng Anh.
Quy ước: đường dẫn kèm "(mới)" là đường dẫn đề xuất, chưa tồn tại. Đường dẫn còn lại đã được đọc hoặc `ls` trong lúc rà soát. Phần "Chưa kiểm chứng" (mục 11) nói rõ những gì chưa chạy.
Nền: spec này dùng cơ chế của [template-registry](../template-registry/spec.md) (thư mục `catalog/`, manifest `template.json`, id bất biến, đủ bản dịch). Không đọc lại spec đó thì vẫn đọc được mục này, nhưng mọi quy tắc "id không đổi", "đủ 7 ngôn ngữ", "mã sinh ra không commit" là của spec đó.

## 0. Quyết định đã chốt

| # | Quyết định | Mục |
| --- | --- | --- |
| D1 | "Domain" là khái niệm hạng nhất của catalog: kind mới `domain` (`catalog/domains/<id>/`) và trường `domain` trên các thành viên (squad, skill, skillset, pack). Không chuyển file vật lý | 5.1, 6 |
| D2 | Mỗi domain có một collection giao diện cùng id; squad có `domain: X` bắt buộc có `collection: X`. Domain Marketing hiện thành nhóm "Marketing" trong bộ chọn squad | 5.2 |
| D3 | Giữ cả `kv_marketing` và `content_studio` (và `kv_pr`), không gộp, không đổi id. Định vị lại bằng mô tả và bảng "khi nào dùng cái nào"; người dùng không phải chọn giữa chúng vì đã có squad cửa trước (D13) | 5.3 |
| D4 | Thêm một squad mới `growth_lab` cho năng lực tối ưu (SEO, CRO, ads, lifecycle, đo lường). Skill tiếng Anh của repo marketingskills gắn vào squad này, không gắn vào `kv_marketing` | 5.4 |
| D5 | Nhập 33 skill của marketingskills vào `catalog/skills/marketing/`, giữ tên upstream, ghim commit, bỏ `evals/`, script và `tools/` | 5.5 |
| D6 | Không sửa skill vendor để thích nghi với Multica. Thích nghi nằm ở ba skill tự viết (`mkt-context`, `mkt-market-vn`, `mkt-market-intl`) mà squad nạp cùng. Chỉ được gỡ quảng cáo thương mại và script, và phải ghi vào `SOURCES.md` | 5.5, 5.6 |
| D7 | Thị trường là trục thứ hai của domain: trường `markets`, danh sách khai báo ở manifest domain, Việt Nam (`vn`) là thị trường chính, quốc tế (`intl`) là thị trường mở rộng. Thêm thị trường mới là thêm một skill `mkt-market-<id>` | 5.6 |
| D8 | Pack `market-strategist` giữ id và hành vi, nhưng skill của nó nằm trong catalog (một bản duy nhất); pack tham chiếu skill theo tên | 5.8 |
| D9 | Bundle skill của skillset `marketing` là một mô-đun tải lười riêng, không nhập vào `SDLC_BUILTIN_SKILLS` | 5.7 |
| D10 | Phép thử "squad Kit Việt" đổi từ `category === "kitviet"` sang `adapter.type === "kitviet"`. Đây là thay đổi hành vi có chủ ý duy nhất: nút "làm mới squad Kit Việt" bao gồm cả các squad `vertical` | 5.7 |
| D11 | `growth_lab` bản đầu không có flow thực thi. Flow viết sau khi diễn tập, theo [flow-authoring-runbook](../../engineering/flow-authoring-runbook.md) | 10 |
| D12 | Không nhập: nhóm bán hàng (`cold-email`, `prospecting`, `sales-enablement`, `revops`), `conversion-tracking`, `reverse-engineer-marketing`, `marketing-council`, `sms`. Đợt 2 xét tiếp (mục 5.5) | 5.5 |
| D13 | Người dùng chỉ tạo **một** squad: `marketing` (cửa trước). Chọn nó trong bộ chọn squad thì quickstart cài luôn các squad chuyên môn nó điều phối (`kv_marketing`, `content_studio`, `growth_lab`, `kv_pr`) bằng một lần chạy; người dùng gửi mọi yêu cầu marketing cho squad `marketing`. Không thêm màn hình mới: giao diện thêm nhóm "Marketing" và một danh sách "cài kèm" trong bộ chọn squad | 5.9, 5.11 |
| D14 | Thẻ công ty mẫu (`growth_gtm`, `market_research`, `competitor_intelligence`, `customer_voice`) và `kv_ecom`, `sales_loop`, `market_research` (squad) không đổi. Liệt kê trong `related` của domain | 5.1 |
| D15 | Squad `marketing` điều phối bằng cơ chế có sẵn: lead tạo issue con gán cho squad chuyên môn, và được đánh thức khi issue con đóng (`child_done`). Không thêm cơ chế giao việc mới | 5.11 |
| D16 | Không gộp mọi vai vào một squad duy nhất. Squad chuyên môn Kit Việt giữ flow có cổng và skill tiếng Việt do bộ sinh quản lý; gộp phải viết lại cả hai (mục 6) | 5.11, 6 |

## 1. Tóm tắt

Marketing của Multica hôm nay nằm ở năm nơi dựng bằng ba cơ chế skill khác nhau, không có chỗ nào cho biết "marketing gồm những gì". Chiến lược của sản phẩm là Việt Nam trước, quốc tế sau, nhưng cấu trúc không có khái niệm thị trường. Repo marketingskills (53 skill tiếng Anh, MIT) lấp đúng chỗ ta thiếu (SEO, CRO, ads, lifecycle, đo lường) và ta đã dùng 4 skill của nó trong pack.

Giải pháp: một domain `marketing` trong catalog. Nó là một thư mục `catalog/domains/marketing/` có manifest và README (trang chủ của domain), một nhóm "Marketing" trong bộ chọn squad (5 squad: 3 squad có sẵn được đưa về cùng nhóm, 2 squad mới), trong đó squad `marketing` là cửa trước duy nhất: người dùng chỉ tạo squad này, quickstart cài kèm các squad chuyên môn, và mọi yêu cầu marketing gửi vào đây để lead điều phối xuống đúng squad; một cây skill `catalog/skills/marketing/` là nơi duy nhất chứa skill marketing có thể nạp, và hai lớp thị trường (`vn`, `intl`) làm trục mở rộng. Lệnh `node scripts/catalog/cli.mjs map marketing` in ra bản đồ đầy đủ của domain, nên "squad làm được gì, có thành phần nào" luôn đúng với dữ liệu, không phải một trang tài liệu viết tay có thể lệch.

Thay đổi lõi nhỏ và có ranh giới: kind `domain`, kiểm tra tham chiếu skill của squad, pack tham chiếu skill catalog, bundle skill tải lười theo skillset, và sửa phép thử Kit Việt (mục 5.7). Không có migration DB, không đổi id nào, không có thay đổi Go dự kiến.

## 2. Vấn đề

### 2.1 Kiểm kê: marketing hôm nay nằm ở đâu

| Thành phần | Nơi | Loại | Thị trường | Skill đến từ |
| --- | --- | --- | --- | --- |
| Marketing Studio `kv_marketing` | `catalog/squads/kv_marketing/`, collection `kitviet`; flow `catalog/workflows/flow_kv_marketing/` | squad Kit Việt, 9 vai (lead, scout, insight, content, ads, 3 cổng, handover) | VN | 25 skill tiếng Việt do bộ sinh Kit Việt tạo từ `catalog/_sources/kitviet/upstream/marketing/skills/`, cấu hình ở `catalog/_sources/kitviet/squads/marketing.json` (8 chung, 15 theo vai, 2 của lead); tên cài vào workspace có tiền tố `kv:` |
| PR & Comms `kv_pr` | `catalog/squads/kv_pr/`, collection `kitviet`; flow `flow_kv_pr` | squad Kit Việt, 9 vai | VN | skill của trade `bao-chi-truyen-thong` |
| Content Studio `content_studio` | `catalog/squads/content_studio/`, collection `core` | squad thường, 10 vai (có Localizer, Market Gate, Publisher qua Ayrshare), không flow | VN và quốc tế | **không có skill nào** (`skills: { sets: [] }`) |
| Market Strategist | `catalog/packs/market-strategist/` | agent pack, một agent, 19 skill | EN | 4 từ marketingskills (commit `820ad2c`), 8 từ phuryn/pm-skills, 3 từ wondelai, 2 từ MaxKmet, 2 tự viết; mỗi skill là một bản sao trong thư mục pack |
| Thẻ công ty mẫu | `catalog/companies/starter/template.json` | `growth_gtm` (1 vai `growth_lead`, tầng Launch), `market_research`, `competitor_intelligence`, `customer_voice`, `opportunity_lab` | không phân | mẫu agent `growth_lead`, `market_scout`, ... |
| Squad `market_research` | collection `core` | 4 vai, quét thị trường hằng tuần | không phân | không |
| Tài liệu | `docs/engineering/{kitviet-squads,content-studio-squad,sales-loop-squad}.md` | mỗi squad một tài liệu riêng | | |

Ngoài domain marketing nhưng gần nó: `kv_ecom` (đăng sản phẩm lên sàn, tầng Revenue), `kv_b2b` (chào giá), `sales_loop` (tìm khách, soạn thư mời).

### 2.2 Các vấn đề

1. **Ba cơ chế skill cho cùng một việc.** Skill Kit Việt đi qua bộ sinh và tiền tố `kv:`; skill catalog đi qua skillset vào `SDLC_BUILTIN_SKILLS`; skill pack là bản sao riêng trong thư mục pack, cài bằng `scripts/install-agent-pack.sh`. Nhập thêm marketingskills mà không sửa gì sẽ tạo hai bản của cùng một skill (một trong pack, một trong catalog).
2. **Hai squad nội dung chạy song song.** `kv_marketing` (VN, có flow, có 3 cổng claim, voice, số liệu, không đăng gì) và `content_studio` (VN và quốc tế, không flow, 3 cổng claim, market, voice, đăng qua Ayrshare sau khi người duyệt) soạn nội dung và soát bằng hai bộ cổng khác nhau. `content_studio` có vai nhưng không có skill nào; mỗi vai chỉ có một đoạn hướng dẫn khoảng 1 KB.
3. **Giao diện nhóm theo nguồn gốc, không theo nghiệp vụ.** Bộ chọn squad chia `core`, `kitviet`, `vertical`. Người muốn làm marketing tìm ở hai nhóm (Content Studio ở Core; Marketing Studio và PR ở Kit Viet), ở trang Agents (pack), và ở công ty mẫu (Growth & GTM).
4. **Thiếu năng lực tối ưu.** Không squad nào làm SEO, AI search, CRO, đánh giá ads, vòng đời email, churn, đo lường. Đó là phần mạnh nhất của marketingskills.
5. **Chiến lược VN trước, quốc tế sau không có chỗ trong cấu trúc.** Kiến thức VN nằm trong skill tiếng Việt của Kit Việt; kiến thức quốc tế chỉ có một vai (`cs_market_gate`, 916 byte). Không có khái niệm "thị trường" để một squad khai báo mình phục vụ thị trường nào.
6. **Bản vendor lệch và không có quy trình nâng cấp.** Upstream marketingskills nay ở `1f83744` (2026-11-11, v2.14.12); 4 skill trong pack ở `820ad2c` (mỗi skill lệch 1 file, ví dụ `customer-research` 2.0.5 so với 2.0.8).
7. **Một phép thử hiện đồng nhất "Kit Việt" với "collection kitviet".** `packages/core/squads/mutations.ts:54` (nút làm mới squad Kit Việt) và `packages/core/squads/upgrade.ts:398` (`hasKitvietSquad`) lọc theo `squadTemplateCategory(tpl) === "kitviet"`. Đưa `kv_marketing` và `kv_pr` sang nhóm Marketing mà không sửa chỗ này sẽ lặng lẽ loại hai squad đó khỏi nút làm mới. Cũng vì phép thử này, 11 squad `vertical` (cũng là Kit Việt) hiện không được làm mới.
8. **Không có chỗ nào trả lời "marketing gồm những gì".** Người phát triển phải `grep` qua `catalog/`, `docs/engineering/` và `packages/core/`.
9. **Người dùng phải biết trước có những squad nào rồi tạo từng cái.** Muốn dùng đủ marketing là phải tạo ít nhất ba squad ở hai nhóm, rồi tự quyết một yêu cầu thuộc squad nào. Bộ chọn squad chỉ tạo một squad mỗi lần (`packages/views/modals/squad-quickstart.tsx`).

## 3. Mục tiêu và không phải mục tiêu

Mục tiêu:

- G1. Một nơi duy nhất cho domain: `catalog/domains/marketing/` là trang chủ, và `catalog map marketing` in bản đồ luôn đúng.
- G2. Một nơi duy nhất cho skill marketing nạp được: `catalog/skills/marketing/`. Mỗi skill có đúng một bản.
- G3. Người dùng thấy các squad marketing ở một nhóm trong bộ chọn squad, mỗi squad có mô tả nói rõ nó làm gì và khi nào dùng.
- G4. Việt Nam trước, quốc tế sau là cấu trúc, không phải lời hứa: mỗi squad và skill khai báo thị trường; thêm thị trường là thêm dữ liệu, không sửa squad.
- G5. Dễ mở rộng: thêm skill, squad, thị trường hoặc cả domain mới không sửa mã trong `packages/` và `server/`.
- G6. Giữ nguyên hành vi đang chạy: id, tên squad và agent, flow `rev`, nội dung `SDLC_BUILTIN_SKILLS` không đổi.
- G7. Chỉ tạo **một** squad (`marketing`) là dùng được toàn bộ phần đang có của domain: soạn chiến dịch có soát luật, đăng đa thị trường, tối ưu tăng trưởng, PR. Người dùng không phải biết có squad chuyên môn nào, và một yêu cầu cần nhiều bước (soạn rồi đăng) chạy hết chuỗi mà không phải tạo issue thứ hai bằng tay.

Không phải mục tiêu: gộp `kv_marketing` với `content_studio`; viết flow cho squad mới; kết nối dữ liệu thật (GA4, Search Console, tài khoản ads); màn hình mới; sửa công ty mẫu; Hub (`docs/engineering/skill-plugin-hub.md`); đổi nội dung skill Kit Việt; domain khác (chỉ để cấu trúc dùng lại được).

## 4. Đánh giá marketingskills và độ phủ

Phần đánh giá ngắn (đã trình bày ở phiên trước): repo MIT, 53 skill, định dạng Agent Skills giống catalog, có evals, chất lượng nội dung tốt; rủi ro là tiếng Anh và thị trường Mỹ, giả định file `.agents/product-marketing.md`, một số skill có script hoặc gọi công cụ ngoài, một khối quảng cáo sản phẩm của tác giả trong `ad-creative`, và không có cổng pháp lý.

Độ phủ theo năng lực. "VN" là skill Kit Việt đã có; "EN" là skill marketingskills (hoặc pack). Cột cuối là quyết định.

| Năng lực | VN (Kit Việt, `kv:`) | EN | Quyết định |
| --- | --- | --- | --- |
| Chiến dịch, kế hoạch truyền thông, lịch nội dung, mùa vụ | `day-chuyen-chien-dich-marketing`, `ke-hoach-truyen-thong-chien-dich`, `lich-noi-dung-thang`, `lich-marketing-mua-vu` | `marketing-plan`, `launch` | VN là chuẩn cho VN. EN cho độ sâu phương pháp và cho thị trường quốc tế |
| Bài đăng, kênh xã hội, video, hình | `bai-dang-fanpage`, `kich-ban-video-ngan`, `viet-giong-gen-z`, `brief-thiet-ke-key-visual`, `toi-uu-tieu-de-thumbnail` | `social`, `video`, `image` | Đợt 2, gắn vào Content Studio bản quốc tế |
| Ad copy, landing | `ad-copy-nhieu-bien-the`, `noi-dung-landing-page` | `ad-creative`, `ads`, `copywriting`, `cro` | EN vào Growth Lab (đánh giá, tối ưu); VN giữ việc soạn trong Marketing Studio |
| SEO, từ khóa, blog | `nghien-cuu-tu-khoa-topic-cluster`, `bai-blog-chuan-seo` | `seo-audit`, `ai-seo`, `programmatic-seo`, `schema`, `site-architecture`, `content-strategy`, `link-building` | EN vào Growth Lab; đây là khoảng trống lớn |
| Email, vòng đời | `chuoi-email-cham-soc` | `emails`, `onboarding`, `churn-prevention`, `referrals`, `lead-magnets` | EN vào Growth Lab |
| Khách hàng, đối thủ, thị trường | `insight-tu-binh-luan-review`, `persona-tu-du-lieu-that`, `phan-tich-doi-thu-noi-dung`, `nghien-cuu-thi-truong-marketing` | `customer-research`, `competitor-profiling`, `competitors` | Cả hai tồn tại; EN dùng khi brief là quốc tế hoặc cần khung phương pháp |
| Đo lường, thử nghiệm | `bao-cao-hieu-qua-chien-dich` | `analytics`, `attribution`, `ab-testing` | EN vào Growth Lab |
| KOL, minigame, soát luật quảng cáo | `brief-kol-koc`, `the-le-minigame-khuyen-mai`, `soat-luat-quang-cao`, `cam-nang-giong-thuong-hieu` | `influencer-marketing` | Chỉ VN. Không có tương đương, và soát luật VN không thay được |
| Chiến lược, định vị, giá | | `product-marketing`, `marketing-ideas`, `marketing-psychology`, `marketing-loops`, `pricing`, `offers` và 15 skill của pack | EN, đã có trong pack; chuyển vào catalog |
| PR | skill của `kv_pr` | `public-relations` | Đợt 2 |

Danh sách đầy đủ 33 skill nhập đợt 1 ở mục 5.5.

## 5. Thiết kế

### 5.1 Mô hình

- **Domain** (mới): một mảng nghiệp vụ lớn có tên (marketing; sau này sales, legal...). Một domain gom squad, skill, skillset, pack của nó, khai báo các thị trường nó phục vụ và các template liên quan nằm ngoài nó. Manifest `catalog/domains/<id>/template.json`:

```json
{
  "schemaVersion": 1, "kind": "domain", "id": "marketing", "version": "1.0.0",
  "markets": [{ "id": "vn", "role": "primary" }, { "id": "intl", "role": "expansion" }],
  "related": [
    { "kind": "squad", "id": "market_research", "note": "Scans markets for the product pipeline; feeds Growth Lab with sources." },
    { "kind": "squad", "id": "kv_ecom", "note": "Marketplace listings and margin; commerce, not campaigns." },
    { "kind": "squad", "id": "sales_loop", "note": "Outbound and sales enablement; a separate domain." }
  ]
}
```

- **Thành viên**: squad, skill, skillset, pack khai báo `"domain": "marketing"` trong `template.json`. Quan hệ suy ra từ thành viên, như skillset suy ra từ `sets` của skill (spec template-registry 6.4): thêm một skill vào domain không sửa file nào khác.
- **Thị trường**: trường `markets: ["vn"]` hoặc `["vn", "intl"]` trên squad, pack và skill tự viết. Mỗi id phải nằm trong `markets` của domain. Thị trường không phải ngôn ngữ giao diện (`SUPPORTED_LOCALES`) mà là thị trường đầu ra của agent.
- **Overlay**: skill tự viết `mkt-*` mà squad nạp cùng skill vendor, mang phần Multica và phần thị trường. Skill vendor không bị sửa.
- **Collection giao diện**: `catalog/collections/marketing/`, cùng id với domain (D2).
- **Không có file `related` thừa**: `related` là danh sách duy nhất các template ngoài domain mà bản đồ nhắc đến, mỗi mục kiểm tra là có tồn tại.

### 5.2 Bố cục thư mục

```
catalog/
├── domains/marketing/                  (mới)
│   ├── template.json                   markets, related
│   └── README.md                       trang chủ: bản đồ, khi nào dùng cái nào, cách mở rộng, cách nâng cấp
├── collections/marketing/              (mới) template.json (order 15) + i18n/ x7: nhóm "Marketing"
├── squads/
│   ├── marketing/                      (mới) cửa trước: template.json (includes), instructions.md, roles/ x1, i18n/ x7
│   ├── growth_lab/                     (mới) template.json, instructions.md, roles/ x7, i18n/ x7
│   ├── kv_marketing/                   collection -> marketing, domain, markets, version 1.0.1
│   ├── kv_pr/                          như trên
│   └── content_studio/                 như trên
├── skillsets/marketing/                (mới) target quickstart, chunk true
├── skills/marketing/                   (mới) nơi duy nhất của skill marketing nạp được
│   ├── SOURCES.md                      nguồn, commit, giấy phép, "Local changes", cách nâng cấp
│   ├── licenses/                       MIT của marketingskills, pm-skills, wondelai, idea-validation-agents
│   ├── foundation/                     product-marketing, evidence-discipline, mkt-context, mkt-market-vn, mkt-market-intl
│   ├── strategy/                       marketing-plan, marketing-ideas, marketing-psychology, marketing-loops, launch, pricing, offers, gtm-motions, growth-loops, value-proposition, obviously-awesome
│   ├── research/                       customer-research, competitor-profiling, mom-test, jobs-to-be-done, market-sizing, user-segmentation, ideal-customer-profile, beachhead-segment, distribution-analysis, cac-modeler, prioritize-assumptions, outlet-finder
│   ├── search/                         seo-audit, ai-seo, programmatic-seo, schema, site-architecture, content-strategy, link-building
│   ├── conversion/                     cro, signup, popups, copywriting, copy-editing, competitors
│   ├── paid/                           ads, ad-creative
│   ├── lifecycle/                      emails, onboarding, churn-prevention, referrals, lead-magnets
│   └── measurement/                    analytics, attribution, ab-testing
├── packs/market-strategist/            pack.json tham chiếu skill; bỏ thư mục skills/ và licenses/
└── _sources/kitviet/                   không đổi; skill VN của kv_marketing và kv_pr vẫn sinh từ đây
```

Nhóm con trong `skills/marketing/` chỉ để sắp xếp (như `builtin`, `starter`; `skillFolders` đi qua nhóm lồng nhau). Tổng 51 skill: 33 nhập từ marketingskills (đợt 1), 15 chuyển từ pack, 3 tự viết (`mkt-*`). Nơi duy nhất của skill VN là nguồn Kit Việt; bản đồ của domain đọc chúng từ `catalog/_sources/kitviet/squads/*.json` chứ không sao chép.

### 5.3 Thành phần của Marketing v1 và khi nào dùng cái nào

Nhóm "Marketing" trong bộ chọn squad, theo thứ tự. Người dùng chỉ cần chọn dòng đầu: nó cài kèm bốn squad còn lại (5.11).

| Thứ tự | Squad | Làm gì | Thị trường | Khi nào dùng |
| --- | --- | --- | --- | --- |
| 5 | Marketing (`marketing`, mới) | Cửa trước duy nhất: nhận mọi yêu cầu marketing, nêu thị trường, điều phối xuống đúng squad chuyên môn (kể cả chuỗi nhiều squad), kiểm tra và tổng hợp kết quả, lập báo cáo hằng tuần. Tự làm chiến lược và nghiên cứu. Không viết sản phẩm cuối | vn, intl | Luôn: đây là squad người dùng tạo và gửi yêu cầu vào |
| 10 | Marketing Studio (`kv_marketing`) | Lập kế hoạch chiến dịch, soạn nội dung đa kênh và ad copy, soát claim theo luật quảng cáo VN, giọng thương hiệu và số liệu, bàn giao. Flow có cổng. Không đăng gì | vn | Có chiến dịch, bài, ad, landing cần soạn và cần soát luật |
| 20 | Content Studio (`content_studio`) | Đưa nội dung đã có brief tới kênh: soạn một thị trường, bản địa hóa sang thị trường thứ hai, soát, xin duyệt rồi đăng qua Ayrshare | vn, intl | Cần đăng đều đặn lên nhiều kênh, hoặc cần bản quốc tế |
| 30 | Growth Lab (`growth_lab`, mới) | Kiểm toán và tối ưu: SEO và AI search, CRO, đánh giá ads, vòng đời email và churn, đo lường và thử nghiệm. Đưa ra khuyến nghị có bằng chứng; không sửa gì đang chạy | vn, intl | Cần biết nên đổi gì để tăng truy cập, chuyển đổi, giữ khách |
| 40 | PR & Comms (`kv_pr`) | Thông cáo báo chí, Q&A, câu trả lời khủng hoảng, soát sự thật và đạo đức | vn | Cần làm việc với báo chí hoặc truyền thông khủng hoảng |

Ngoài nhóm: pack Market Strategist (trang Agents, một agent chiến lược và nghiên cứu: phân khúc, định vị, giá, kênh) cho người chỉ cần một cố vấn, không cần một squad.

Bốn squad chuyên môn là phần "bên trong" của squad `marketing`: người dùng vẫn có thể gửi yêu cầu thẳng cho một squad (nó vẫn là squad bình thường, có flow riêng), nhưng đường mặc định là gửi cho `marketing`.

Quy tắc phân vai giữa Marketing Studio và Content Studio (giải quyết vấn đề 2 mà không gộp):

- Marketing Studio quyết định **nói gì** (thông điệp, claim, chiến dịch) và là nơi duy nhất soát luật quảng cáo VN.
- Content Studio quyết định **đăng gì, ở đâu, khi nào** và là nơi duy nhất đăng.
- Nội dung VN cần cả hai: soạn và soát ở Marketing Studio, rồi đưa bản đã qua cổng vào Content Studio làm brief. Việc chuyển do lead của squad `marketing` làm (5.11); gửi thẳng cho từng squad thì người gửi tự chuyển.
- Growth Lab chỉ **khuyến nghị**. Chữ có thể đăng công khai nó viết ra là bản nháp, bắt buộc qua cổng claim của Marketing Studio (VN) hoặc Market Gate của Content Studio (quốc tế) trước khi người đăng.

### 5.4 Growth Lab

Squad thường (không Kit Việt): `catalog/squads/growth_lab/`, `collection: marketing`, `domain: marketing`, `markets: ["vn", "intl"]`, `floorName: "Launch"`, `workflow: null`, `order: 30`.

Tên agent có tiền tố "Growth Lab" vì quickstart tìm agent theo tên và dùng lại agent trùng tên: thẻ công ty mẫu `growth_gtm` đã có agent tên "Growth Lead" làm việc khác, nên tên "Growth Lead" sẽ bị dùng nhầm.

| Vai | Tên agent | Mẫu agent | Skill riêng của vai |
| --- | --- | --- | --- |
| `gl_lead` (lead) | Growth Lab Lead | `growth_lead` | `product-marketing`, `marketing-plan`, `marketing-loops`, `launch`, `marketing-ideas` |
| `gl_search` | Growth Lab Search | `market_scout` | `seo-audit`, `ai-seo`, `programmatic-seo`, `schema`, `site-architecture`, `content-strategy`, `link-building`, `competitor-profiling` |
| `gl_conversion` | Growth Lab Conversion | `prd_writer` | `cro`, `signup`, `popups`, `copywriting`, `copy-editing`, `competitors`, `marketing-psychology`, `customer-research` |
| `gl_paid` | Growth Lab Paid Media | `competitor_analyst` | `ads`, `ad-creative`, `ab-testing`, `offers` |
| `gl_lifecycle` | Growth Lab Lifecycle | `customer_lead` | `emails`, `onboarding`, `churn-prevention`, `referrals`, `lead-magnets`, `pricing` |
| `gl_measure` | Growth Lab Measurement | `validation_analyst` | `analytics`, `attribution`, `ab-testing` |
| `gl_checker` | Growth Lab Checker | `critic` | không có skill riêng |

Skill chung cho mọi vai (`skills.extra`): `automation-policy`, `mkt-context`, `mkt-market-vn`, `mkt-market-intl`, `evidence-discipline`. `skills.sets` để trống: gắn cả skillset `marketing` cho mọi vai sẽ nạp 51 mô tả vào ngữ cảnh của mỗi agent. Cả 33 skill đợt 1 được gắn cho ít nhất một vai.

`gl_checker` không viết sản phẩm, chỉ soát độc lập (theo khuôn của `sales_checker`, `cs_*_gate`): mỗi con số phải tính lại; mỗi khẳng định phải có nguồn và ngày hoặc nhãn `MISSING`; thị trường của đầu ra phải khớp thị trường của brief; chữ có thể đăng phải có dấu hiệu "cần soát claim". Kết thúc bằng một dòng `VERDICT: PASS` hoặc `VERDICT: FAIL` và danh sách sửa, như `cs_market_gate`. Hai vòng FAIL thì dừng và trả cho người yêu cầu.

Quy tắc thị trường trong `instructions.md` của lead (nội dung cụ thể viết khi triển khai): thị trường mặc định là `vn` nếu brief không nói; mọi sản phẩm nêu thị trường ở dòng đầu; sản phẩm `vn` viết bằng tiếng Việt; sản phẩm `intl` viết bằng ngôn ngữ của brief; khi skill phương pháp (tiếng Anh) và skill thị trường (`mkt-market-*`) mâu thuẫn, skill thị trường thắng và mâu thuẫn được ghi thành giả định.

Charter:

- Mission: tìm và xếp thứ tự những thay đổi tăng truy cập có chất lượng, chuyển đổi, giữ khách và doanh thu, có bằng chứng, cho thị trường Việt Nam trước rồi quốc tế. Không có gì rời workspace.
- Responsibilities: `triage` (xếp yêu cầu vào kiểm toán, kế hoạch, thử nghiệm hay báo cáo; xác định thị trường), `audit` (kiểm toán và khuyến nghị theo vai), `experiments` (thiết kế thử nghiệm: giả thuyết, chỉ số, cỡ mẫu, điều kiện dừng), `review` (soát độc lập mọi sản phẩm).
- Outputs: `growth_report` (report, hằng tuần), `audit` (kb_document, theo yêu cầu), `experiment_plan` (kb_document, theo yêu cầu).
- Boundaries: không sửa trang, tài khoản ads, email hay theo dõi đang chạy; không gửi, đăng, chi tiền; không bịa dữ liệu: đầu vào thiếu thì ghi `MISSING` và nói bằng chứng nào mở khóa kết luận; chữ có thể đăng là bản nháp phải qua cổng claim.
- Escalation: kết luận cần quyết định (chi tiền, đổi giá, đổi tracking) thành một proposal cho người; yêu cầu bị chặn trả lại người yêu cầu.
- Intake: yêu cầu là issue gán cho squad; routine hằng tuần mở issue xem lại.

Routine: `Weekly growth review`, `0 9 * * 1`, bật khi tạo. Một lần chạy "đã kiểm tra, không có gì mới" là kết quả hợp lệ (đúng cấu trúc của `marketing-loops`: tách "check" khỏi "act", có điều kiện dừng). Không có kết nối dữ liệu trong v1, nên bản xem lại chỉ đọc dữ liệu có sẵn trong knowledge base và khuyến nghị cũ.

### 5.5 Skill: nhập, giữ, bỏ

Quy tắc vendor (áp cho mọi skill nhập từ ngoài vào `skills/marketing/`):

1. Mỗi skill có `template.json`: `sets: ["marketing"]`, `domain: "marketing"`, `source: { type: "vendored", repo, ref (commit 40 ký tự), license }`. Pin `ref` là bắt buộc cho domain này (kiểm tra bằng test của domain, mục 7).
2. Chỉ lấy `SKILL.md` và các thư mục văn bản mà skill liên kết tới (`references/`, `assets/` dạng văn bản). **Bỏ** `evals/` (fixture kiểm thử của upstream, 400 KB trên 33 skill, không có ích cho agent), mọi script (`*.py`, `*.mjs`, `*.js`, `*.sh`) và `tools/`.
3. Không sửa nội dung để thích nghi. Hai ngoại lệ, cả hai phải ghi ở "Local changes" của `SOURCES.md` kèm tên file và dòng: (a) gỡ khối quảng cáo sản phẩm của chính tác giả (hiện biết một khối: `ad-creative/SKILL.md`, đoạn "Skip the plumbing" nhắc Magister); khi nhập sẽ `grep` các từ `magister`, `conversionfactory`, `swipefiles`, `?ref=marketingskills` ở cả 33 skill và gỡ chỉ những khối quảng cáo, giữ bảng liệt kê công cụ trung lập; (b) gỡ liên kết trỏ vào `tools/` (không còn đích).
4. Giấy phép: MIT của từng nguồn nằm ở `skills/marketing/licenses/`; `SOURCES.md` liệt kê skill theo nguồn như `packs/market-strategist/SOURCES.md` hiện làm.
5. Nâng cấp: clone upstream ở commit mới, diff từng skill với bản trong catalog, áp lại "Local changes", chạy kiểm tra bí mật và mẫu tiêm lệnh (như `SOURCES.md` của pack), đổi `ref`, tăng `version`.

33 skill đợt 1, theo nhóm: `strategy` (marketing-plan, marketing-ideas, marketing-psychology, marketing-loops, launch, pricing, offers), `foundation` (product-marketing), `research` (customer-research, competitor-profiling), `search` (7), `conversion` (competitors, cro, signup, popups, copywriting, copy-editing), `paid` (ads, ad-creative), `lifecycle` (5), `measurement` (analytics, attribution, ab-testing). Kích thước đo trên upstream, không tính `evals/`: khoảng 1,9 MB; bundle quickstart hiện tại 2,2 MB (`skills-builtin.ts`).

Không nhập đợt 1:

| Skill | Lý do |
| --- | --- |
| `conversion-tracking` | 28 script và 588 KB tài liệu, đòi đặt snippet vào site và đụng tài khoản ads; cần chính sách thực thi script trước |
| `reverse-engineer-marketing` | có `teardown.mjs` tự gọi web và Wayback |
| `marketing-council` | mô phỏng ý kiến người thật; giá trị thấp so với rủi ro bịa trích dẫn |
| `cold-email`, `prospecting`, `sales-enablement`, `revops` | thuộc domain bán hàng, đã có `sales_loop`; xét khi làm domain Sales |
| `sms` | quy định gửi tin chủ động ở VN khác Mỹ; chưa có người sở hữu để soát |
| `aso`, `directory-submissions`, `free-tools`, `paywalls` | ít liên quan tới squad v1 |
| `social`, `video`, `image`, `public-relations`, `influencer-marketing`, `community-marketing`, `co-marketing`, `events` | đợt 2: chồng lấn với skill VN đã có, đáng nhập khi gắn vào Content Studio bản quốc tế và PR (mục 10) |

### 5.6 Lớp thị trường và overlay

Ba skill tự viết trong `skills/marketing/foundation/`, `source.type: "authored"`:

- `mkt-context`: thay chỗ `.agents/product-marketing.md` mà các skill vendor đọc đầu tiên. Nói: ngữ cảnh marketing của workspace là tài liệu knowledge base `Hồ sơ marketing` (kind `reference`), cùng khuôn với `Hồ sơ đơn vị` của `kv-company-profile`; đọc cả hai nếu có; chưa có thì lập bản nháp theo skill `product-marketing`, đánh `[cần bổ sung: ...]` cho trường chưa biết và tiếp tục, không dừng. Nói rõ công cụ có thật trong runtime (web và knowledge base; không MCP Ahrefs, GA4...), nên bước nào skill vendor đòi công cụ vắng thì dùng nhánh "nếu không có" của skill, và dữ liệu vắng ghi `MISSING`. Nói nơi đặt sản phẩm (comment và tài liệu knowledge base, không gửi ra ngoài).
- `mkt-market-vn` (`markets: ["vn"]`): thị trường Việt Nam. Dàn ý nội dung: ngôn ngữ và xưng hô mặc định; kênh và nền tảng chính (Facebook, Zalo, TikTok, YouTube, Google, sàn thương mại điện tử) và cách chúng đổi lời khuyên của skill phương pháp; đặc thù tìm kiếm và AI search tiếng Việt; thanh toán và niềm tin (COD, ví, chuyển khoản); mùa vụ (trỏ tới `lich-marketing-mua-vu` khi có); ràng buộc đo lường; danh sách "điều chỉnh skill phương pháp" (ví dụ: tạo khan hiếm và bằng chứng xã hội phải qua cổng claim). Mọi quy định pháp lý ghi kèm nguồn và ngày đọc, không nêu từ trí nhớ; thiếu nguồn thì `[cần bổ sung: nguồn]`; kết luận pháp lý cuối thuộc người phụ trách pháp chế, như `soat-luat-quang-cao`.
- `mkt-market-intl` (`markets: ["intl"]`): cùng dàn ý cho thị trường quốc tế, tách theo khu vực khi cần (mỗi khu vực có thể thành skill riêng sau). Quy định (quyền riêng tư, gửi email chủ động, quảng cáo trả phí, nhãn nội dung AI, chứng thực) và chính sách nền tảng đều theo quy tắc "nguồn chính thức, có ngày đọc, không đoán" của `cs_market_gate`.

Thêm thị trường mới (ví dụ `sea`): thêm `{ "id": "sea" }` vào `markets` của domain, viết `mkt-market-sea`, thêm `sea` vào `markets` của squad nào phục vụ nó, thêm skill đó vào `skills.extra` của squad. Không sửa skill vendor và không sửa mã.

### 5.7 Thay đổi engine

Tất cả nhỏ, có ranh giới, có test. Không đụng Go.

| # | Thay đổi | File | Lý do |
| --- | --- | --- | --- |
| E1 | Kind `domain`: thêm vào `KIND_DIRS` (`domains`), bộ kiểm tra, scaffold (`catalog new domain <id>` tạo `template.json` và `README.md`), `baseIdProblems`. Trường `domain` tùy chọn trên squad, skill, skillset, pack. Quy tắc: `domain` trỏ tới domain có thật; squad có `domain: X` phải có `collection: X` và collection `X` phải tồn tại; `related` trỏ tới template có thật; mỗi domain có `README.md`. Domain không có chuỗi hiển thị nên không cần bản dịch | `scripts/catalog/lib/discover.mjs`, `validate.mjs`, `scaffold.mjs`, `cli.mjs` | G1, G5 |
| E2 | Trường `markets` tùy chọn trên squad, pack, skill; mỗi id phải thuộc `markets` của domain của template đó | `validate.mjs` | G4 |
| E3 | Lệnh `catalog map <domain> [--json]`: squad (collection, thị trường, số vai, số skill, có flow hay không), skill theo nhóm, skill Kit Việt của squad Kit Việt (đọc `_sources/kitviet/squads/*.json`), pack, `related`, và cảnh báo skill dưới `skills/marketing/` thiếu `domain` | `scripts/catalog/cli.mjs`, `scripts/catalog/lib/map.mjs` (mới), `package.json` (script `catalog:map`) | G1 |
| E4 | Kiểm tra tham chiếu (L2) còn thiếu: tên trong `squad.skills.extra`, `leader`, `byRole` (và vai trong `byRole` thuộc squad) phải là skill có thật thuộc một skillset `quickstart`. Chạy trên mọi squad không phải Kit Việt; nếu squad có sẵn vi phạm thì sửa dữ liệu của nó trong cùng bước | `validate.mjs` | squad mới dựa nhiều vào `byRole`; hiện không có kiểm tra nào, tên sai chỉ lộ ra lúc chạy quickstart |
| E5 | Pack tham chiếu skill catalog: tên trong `pack.json` `skills` được tìm trước ở `packs/<id>/skills/<tên>/`, không có thì ở skill catalog cùng id; có cả hai là lỗi. `buildPackBundle` và `validatePack` đổi theo; `install-agent-pack.sh` tìm skill catalog bằng `find catalog/skills -type d -name <folder>` khi thư mục pack không có | `scripts/catalog/lib/build-packs.mjs`, `scripts/install-agent-pack.sh` | G2: một bản duy nhất của mỗi skill |
| E6 | Skillset có `"chunk": true` được sinh thành mô-đun riêng (`skills-builtin-<set>.ts`, xuất `<SET>_BUILTIN_SKILLS`) và **không** nằm trong `SDLC_BUILTIN_SKILLS`. `builtinSkillNamed` (`packages/core/squads/quickstart.ts:526`) tra skillset của tên qua `SKILLSET_MEMBERS` rồi nạp đúng chunk. Test đang giả định bundle = thư mục `builtin` (`sdlc-skills.test.ts:61`) và test liệt kê tên bundle (`squad-invariants.test.ts:13`, `templates.test.ts:276`, `kitviet-skills.test.ts:368`) được cập nhật để chấp nhận chunk | `scripts/catalog/lib/build-skills.mjs`, `build.mjs`, `packages/core/squads/quickstart.ts`, các test kể trên | G6: không để cài một squad SDLC tải thêm 1,9 MB marketing |
| E7 | Thêm `isKitvietTemplate(tpl)` (`tpl.adapter?.type === "kitviet"`; `SquadTemplate` hiện chưa mang `adapter`, thêm vào lúc lắp ráp ở `packages/core/catalog/squads.ts`) và thay hai chỗ lọc theo collection | `packages/core/squads/mutations.ts:54`, `packages/core/squads/upgrade.ts:398`, `packages/core/squads/templates.ts`, `packages/core/catalog/squads.ts` | Điều kiện để chuyển `kv_marketing` và `kv_pr` khỏi collection `kitviet` mà không mất làm mới (vấn đề 7). Hệ quả có chủ ý D10: squad `vertical` vào danh sách làm mới. Thêm test cho cả hai hướng |
| E9 | Trường `includes` tùy chọn ở mức manifest của squad: danh sách id squad được cài kèm. Quy tắc: mỗi id là squad có thật, cùng `domain`, không phải chính nó, và squad được cài kèm không có `includes` (một tầng). `SquadTemplate` mang `includes` | `scripts/catalog/lib/validate.mjs`, `scripts/catalog/lib/build-squads.mjs`, `packages/core/catalog/types.ts`, `packages/core/catalog/squads.ts`, `packages/core/squads/templates.ts` | D13 |
| E10 | Quickstart cài một bộ: một hàm chạy tuần tự các squad được cài kèm (theo thứ tự `includes`) rồi squad chính, mỗi squad bằng `runSquadQuickstart` có sẵn (đã idempotent), trả kết quả gộp theo squad; một squad lỗi không dừng các squad sau. Bộ chọn squad hiện danh sách "cài kèm" có hộp chọn (mặc định chọn hết), danh sách agent gộp, một lựa chọn runtime dùng chung, và kết quả theo từng squad | `packages/core/squads/quickstart.ts`, `packages/core/squads/mutations.ts`, `packages/views/modals/squad-quickstart.tsx`, `packages/views/locales/*/modals.json` x7 | D13 |
| E8 | Bản dịch: collection `marketing` (title), squads `marketing` và `growth_lab` (title, description) đủ 7 ngôn ngữ, cùng các khóa mới của E10. `kv_marketing`, `kv_pr`, `content_studio` giữ bản dịch hiện có (cập nhật mô tả nếu bảng ở 5.3 làm câu cũ sai) | `catalog/collections/marketing/i18n/`, `catalog/squads/growth_lab/i18n/` | Q3 của template-registry |

Cái E7 sửa là một lỗi tiềm ẩn sẵn có (squad `vertical` không được làm mới), không phải thứ do spec này gây ra. Bản này sửa vì không thể đổi phép thử mà giữ nguyên sai sót; nếu người sở hữu muốn giữ đúng hành vi cũ, thay bằng `adapter kitviet && collection !== "vertical"` (một dòng), nhưng điều đó mã hóa một quy tắc không có lý do rõ.

### 5.8 Pack Market Strategist

Giữ id `market-strategist`, tên hiển thị, hướng dẫn và danh sách 19 skill. Thay đổi: 19 thư mục skill trong pack chuyển vào `catalog/skills/marketing/` (E5 cho phép pack tham chiếu); 4 skill của marketingskills (`product-marketing`, `customer-research`, `competitor-profiling`, `marketing-ideas`) lên bản mới nhất theo quy tắc 5.5; 15 skill còn lại chuyển nguyên văn kèm `source` lấy từ `SOURCES.md` của pack. `SOURCES.md` và `licenses/` của pack được gộp vào `skills/marketing/` và pack chỉ giữ `README.md` ngắn trỏ tới domain. Kiểm tra: bundle `market-strategist.json` có cùng 19 tên skill; nội dung 15 skill chuyển giống hệt byte.

### 5.9 Trải nghiệm theo vai

- **Người phát triển**: mở `catalog/domains/marketing/README.md`, chạy `node scripts/catalog/cli.mjs map marketing` (hoặc `pnpm catalog:map marketing`). Thêm skill: tạo thư mục dưới `skills/marketing/<nhóm>/` có `template.json` với `domain`. Thêm squad: `pnpm catalog:new squad <id> --collection marketing` rồi điền `domain` và `markets`. Mọi kiểm tra chạy trong `pnpm catalog:check`.
- **Người quản trị workspace**: mở bộ chọn squad, thấy nhóm "Marketing" ngay sau "Core"; squad đầu tiên là **Marketing**. Chọn nó thì thấy squad sẽ tạo kèm (bốn squad chuyên môn, mỗi squad một dòng mô tả), danh sách agent gộp (khoảng 36), một ô chọn runtime. Một lần bấm tạo là xong. Trang Agents vẫn có pack Market Strategist.
- **Người dùng cuối**: tạo issue gán cho squad **Marketing**, nói việc cần làm bằng lời thường ("làm chiến dịch Tết cho sản phẩm X và đăng lên fanpage", "kiểm toán SEO cho trang này"). Lead của squad nêu thị trường, chọn squad chuyên môn, theo dõi và trả kết quả. Không cần biết squad nào làm gì; README có bảng "khi nào dùng cái nào" (5.3) cho ai muốn biết.

Giao diện thêm hai thứ trong bộ chọn squad hiện có: nhóm mới (bộ chọn đã nhóm theo collection, `packages/views/modals/squad-quickstart.tsx:47` đến `51`) và danh sách "cài kèm" (E10).

### 5.10 Mở rộng

| Muốn | Làm | Không phải sửa |
| --- | --- | --- |
| Thêm skill marketing | thư mục + `template.json` (`sets`, `domain`, `source`) | mã, manifest domain |
| Thêm vai hoặc đổi skill của vai | `squads/growth_lab/template.json` và `roles/<vai>.md` | mã |
| Thêm squad marketing | `catalog new squad`, `collection: marketing`, `domain`, `markets`; thêm id vào `includes` của squad `marketing` và một dòng vào bảng điều phối trong `instructions.md` của nó | mã |
| Thêm thị trường | mục 5.6 | skill vendor, mã |
| Thêm domain khác (sales, legal...) | `catalog new domain <id>`, collection cùng id, gán `domain` cho thành viên | mã |
| Nâng cấp skill vendor | quy tắc 5.5 mục 5 | |

### 5.11 Squad cửa trước `marketing` và cài cả bộ

Yêu cầu (G7): tạo **một** squad là dùng được toàn bộ. Thiết kế: một squad `marketing` chỉ có một vai (lead) làm cửa trước, cộng với cài kèm bốn squad chuyên môn. Squad có một vai là dạng đã có (thẻ công ty mẫu `growth_gtm`, `customer_voice` chỉ có một vai).

**Squad.** `catalog/squads/marketing/`, `collection: marketing`, `domain: marketing`, `markets: ["vn","intl"]`, `floorName: "Launch"`, `workflow: null`, `order: 5`, `includes: ["kv_marketing", "content_studio", "growth_lab", "kv_pr"]`. Vai duy nhất `mktg_chief`, tên agent "Marketing Chief" (không trùng "Growth Lead" của thẻ công ty mẫu, xem 5.4), mẫu `growth_lead`. Skill (`skills.extra`, vì squad chỉ có một vai): `automation-policy`, `mkt-context`, `mkt-market-vn`, `mkt-market-intl`, `evidence-discipline`, `product-marketing`, `marketing-plan`, `marketing-loops`, `launch`, `marketing-ideas`.

**Việc của lead** (`instructions.md`, nội dung cụ thể viết khi triển khai):
1. Nhận yêu cầu, xác định thị trường (mặc định `vn`) và loại việc, ghi vào comment đầu.
2. Chọn đường theo bảng điều phối. Một yêu cầu có thể cần nhiều squad nối tiếp.
3. Với mỗi squad cần dùng, tạo issue con của yêu cầu, gán cho squad đó (`multica issue create --parent <yêu cầu> --assignee-squad "<tên squad>"`), nội dung gồm mục tiêu, thị trường, đầu vào đã có, và điều kiện hoàn thành. Việc tuân thủ nằm ở squad chuyên môn: lead không viết sản phẩm cuối và không thay một cổng soát.
4. Khi issue con đóng, đọc kết quả (comment bàn giao và tệp đính kèm), kiểm tra với yêu cầu ban đầu, rồi hoặc tạo issue con kế tiếp với kết quả làm đầu vào, hoặc tổng hợp và trả lời. Kết quả công khai tiếng Việt phải đi qua Marketing Studio trước khi sang Content Studio.
5. Chiến lược, định vị, nghiên cứu thị trường và kế hoạch: lead tự làm bằng skill của mình (và tạo nghiên cứu cho Growth Lab nếu cần dữ liệu).
6. Không đăng, không gửi, không chi tiền; mọi lệnh duyệt của người giữ nguyên như ở từng squad (Content Studio đăng sau khi người duyệt proposal).

**Bảng điều phối** (trong `instructions.md`):

| Yêu cầu | Squad nhận | Chuỗi thường gặp |
| --- | --- | --- |
| Chiến dịch, bài đăng, kịch bản, ad copy, landing, minigame, soát claim | Marketing Studio | rồi Content Studio nếu cần đăng |
| Đăng, lên lịch, bản thị trường thứ hai, nội dung quốc tế | Content Studio | |
| Kiểm toán và tối ưu SEO, AI search, CRO, ads, email, churn, đo lường, thử nghiệm | Growth Lab | khuyến nghị có chữ công khai thì qua Marketing Studio |
| Thông cáo, Q&A báo chí, khủng hoảng truyền thông | PR & Comms | |
| Chiến lược, định vị, giá, kế hoạch, nghiên cứu | chính lead | |

**Cơ chế điều phối đã có (đã đọc mã, chưa chạy):**
- Một agent được gán issue cho một squad nếu nó được phép gọi lead của squad đó: `validateAssigneePair`, nhánh `squad` (`server/internal/handler/issue.go:4551`). Cổng `squadLeaderAssigneeGate` (`:4631`) chỉ hạn chế việc gán cho **agent**, không áp cho squad, nên lead `marketing` gán được cho squad khác. CLI có `--assignee-squad` (`server/cmd/multica/cmd_issue.go:1681`).
- Issue gán cho squad có workflow chạy trên flow thì máy chủ tự đưa nó vào flow (`server/cmd/multica/cmd_issue_flow.go:25`), nên `kv_marketing` và `kv_pr` chạy đúng flow có cổng của chúng.
- Quy tắc hệ thống `child_done` đánh thức assignee của issue cha khi issue con đóng, gồm lead nếu cha gán cho squad (`server/internal/service/issue_wakeup_system.go:24`).
- Lead `marketing` không phải request của flow nên được tạo issue con (cấm tạo issue con chỉ áp cho request do flow quản lý, E8 của flow-engine-generalization-spec).

**Cài kèm (E9, E10).** `includes` làm bộ chọn squad cài bốn squad chuyên môn trước, squad `marketing` sau cùng. Mỗi squad dùng `runSquadQuickstart` hiện có nên đã idempotent: chạy lại không nhân đôi, squad có sẵn được giữ. Hộp chọn cho phép bỏ một squad chuyên môn (ví dụ PR & Comms); lead biết squad nào có trong workspace vì đọc danh sách squad lúc điều phối, và nếu squad cần không có thì nói với người yêu cầu thay vì làm thay. Số agent tạo ra: 1 + 9 + 10 + 7 + 9 = 36. Đó là cái giá của việc giữ nguyên các squad có flow; mục 6 giải thích vì sao không gộp thành một squad nhỏ hơn.

**Charter.** Mission: nhận mọi yêu cầu marketing và đưa chúng tới kết quả đúng thị trường, qua đúng squad chuyên môn, với đủ cổng soát, Việt Nam trước rồi quốc tế. Responsibilities: `intake` (nêu thị trường, loại việc), `routing` (chọn squad, nối chuỗi), `verification` (đối chiếu kết quả với yêu cầu), `reporting` (báo cáo hằng tuần). Outputs: `marketing_report` (report, hằng tuần), `marketing_context` (kb_document `Hồ sơ marketing`, một lần và khi đổi). Boundaries: không viết sản phẩm cuối, không thay cổng soát, không gửi, đăng, chi tiền. Escalation: một yêu cầu bị chặn ở squad chuyên môn thì trả cho người yêu cầu kèm lý do của squad đó. Routine: `Weekly marketing report`, `0 8 * * 1`: các yêu cầu đang mở, kết quả tuần qua theo squad, việc kẹt; "không có gì mới" là kết quả hợp lệ.

**Nếu cơ chế điều phối không chạy như đã đọc** (xem mục 11, plan T0.4): thu hẹp thành "cài cả bộ một lần bấm" cộng bảng "khi nào dùng cái nào" trong README, và lead `marketing` chỉ làm chiến lược, kế hoạch và báo cáo, nói người yêu cầu gửi việc tới squad nào. Đó vẫn tốt hơn hiện trạng nhưng chưa đạt G7, và phải ghi rõ như vậy trong báo cáo.

## 6. Các phương án đã loại

| Phương án | Vì sao loại |
| --- | --- |
| Chuyển mọi thứ của marketing vào một thư mục vật lý `catalog/domains/marketing/{squads,skills,...}` | Phá quy ước bố cục theo kind của template-registry (6.1); `scripts/kitviet-update.mjs` và nhiều test hard-code `catalog/squads/...`; id và tên thư mục phải bằng nhau nên di chuyển có thể gây lỗi khó thấy. Domain logic (trường `domain` + `catalog map`) cho cùng khả năng tìm kiếm với rủi ro thấp hơn nhiều |
| Dùng `tags` có sẵn làm domain | Không kiểm tra được, không có nơi khai báo thị trường và `related`, không có chỗ đặt README |
| Mở rộng `collection` thành domain | Collection chỉ nhóm squad và chỉ có `title`; gắn skill và pack vào collection làm lẫn nghĩa. Giữ collection là nhóm giao diện |
| Gộp `kv_marketing` và `content_studio` | Hai squad có cơ chế khác nhau (flow có cổng so với đăng qua Ayrshare), đã có người dùng workspace theo id; gộp phải viết lại flow và diễn tập lại. Định vị rõ và nối bằng bàn giao rẻ hơn |
| **Một squad thật duy nhất chứa mọi vai** (khoảng 14 vai: nghiên cứu, soạn, ads, PR, SEO, CRO, vòng đời, đo lường, bản địa hóa, ba cổng, bàn giao). Đây là cách hiểu sát nhất của "một squad", nên đã được cân nhắc kỹ | Bị chặn bởi hai ràng buộc có thật. (1) Skill tiếng Việt `kv:` của `kv_marketing` do bộ sinh Kit Việt quản lý: một squad không phải Kit Việt chỉ nạp được skill trong gói `common` hoặc gói extension của nó, và bộ sinh từ chối một skill được hai cấu hình cùng nhận (`scripts/generate-kitviet-pack.mjs`, hàm `claim`); đưa chúng cho squad mới phải sửa bộ sinh hoặc tách đôi skill. Gói `common` thì mọi squad Kit Việt đều gắn (`core = [...sharedKeys(), ...]`). (2) Flow có cổng của `kv_marketing` và `kv_pr` (đã diễn tập) chỉ ở squad Kit Việt; một squad mới không có flow nên cổng claim chỉ còn là lời nhắc trong prompt, mất đúng phần có giá trị nhất của VN. Viết lại hai thứ đó nằm ngoài phạm vi và rủi ro hơn nhiều so với điều phối. Có thể làm sau (mục 10) |
| Chỉ cài một bộ nhưng không có squad cửa trước | Người dùng vẫn phải tự quyết yêu cầu thuộc squad nào và tự nối chuỗi soạn rồi đăng; không đạt G7 |
| Gắn skill tiếng Anh vào các vai của `kv_marketing` | Phải nới `adapter kitviet` (hiện cấm `squad.skills`) và đổi hành vi một flow đã diễn tập mà không diễn tập lại; thêm nguy cơ mâu thuẫn "tạo khan hiếm" của skill EN với luật quảng cáo VN ngay trong squad có cổng. Growth Lab là chỗ tự nhiên cho skill EN |
| Chỉ cập nhật 4 skill trong pack, không làm gì thêm | Không giải quyết phân mảnh; không có squad nào dùng được phần còn lại của repo |
| Nhập cả 53 skill | Thêm 1,5 MB mà không có squad nào dùng; nhóm bán hàng thuộc domain khác; skill có script cần chính sách riêng |
| Tiền tố tên cho skill vendor (`marketingskills:cro`) | Đổi tên skill đã cài trong workspace qua pack (tên là định danh, quickstart cập nhật theo tên). Quickstart đã xử lý trùng tên: skill của người dùng khác nội dung thì không bị ghi đè (`importBuiltinSkills`, `packages/core/squads/quickstart.ts:550`) |
| Sửa skill vendor cho hợp (đường dẫn file, tiếng Việt) | Mỗi lần nâng cấp phải làm lại và dễ lệch; overlay rẻ hơn và nâng cấp được |
| Thêm kết nối dữ liệu (GA4, ads) ngay | Cần credential và chính sách chi tiêu; không có trong mục tiêu |

## 7. Kiểm tra (validation)

Lệnh chuẩn: `pnpm catalog:check`, `pnpm catalog:build`, `node --test scripts/catalog/*.test.mjs`, `pnpm --filter @multica/core exec vitest run catalog squads agent-packs`, `pnpm typecheck`, `pnpm lint`.

| Lớp | Nội dung | Mức |
| --- | --- | --- |
| V1 Domain | E1, E2: tham chiếu `domain`, `collection` khớp, `related` tồn tại, README tồn tại, `markets` hợp lệ | Lỗi |
| V2 Skill của squad | E4: mọi tên `extra`, `leader`, `byRole` là skill quickstart có thật | Lỗi |
| V3 Pack | E5: tên skill phân giải được, không mơ hồ; bundle sinh ra | Lỗi |
| V4 Skill vendor của domain | test `packages/core/catalog/marketing-domain.test.ts` (mới): `ref` là commit 40 ký tự; không có file `evals/`, script, hay `tools/`; không còn chuỗi `magister`, `?ref=marketingskills`; có `LICENSE` tương ứng; số skill khớp mục 5.2 | Lỗi |
| V5 Bất biến hành vi | `SDLC_BUILTIN_SKILLS` có đúng tên cũ và nội dung cũ (băm trước và sau); `rev` của mọi flow không đổi (`flows.test.ts`); `hasKitvietSquad` và nút làm mới gồm `kv_marketing`, `kv_pr`; test đếm cứng được cập nhật có chủ ý | Lỗi |
| V6 Quickstart | test với client giả: cài `growth_lab` tạo 7 agent, squad, gắn đúng skill chung cho mọi vai và đúng skill theo vai; cài lại không nhân đôi | Lỗi |
| V9 Cài cả bộ | E9, E10: `includes` hợp lệ (id có thật, cùng domain, một tầng); cài `marketing` tạo 5 squad và 36 agent với client giả; bỏ chọn một squad chuyên môn thì không tạo nó; một squad lỗi không dừng squad sau; chạy lại không nhân đôi | Lỗi |
| V10 Điều phối | test trong `marketing-domain.test.ts`: `instructions.md` của `marketing` nêu đủ năm dòng của bảng điều phối, nêu thị trường mặc định `vn`, dùng `--assignee-squad`, cấm tự viết sản phẩm cuối, đăng, gửi và chi tiền; tên squad trong bảng khớp tên squad của catalog | Lỗi |
| V7 Bản đồ | `catalog map marketing` liệt kê đúng 5 squad (và `includes` của squad `marketing`), 1 pack, 51 skill trong 8 nhóm, skill VN của `kv_marketing` và `kv_pr`; không có cảnh báo thiếu `domain` | Lỗi |
| V8 Nội dung squad | test trong `marketing-domain.test.ts`: lead của `growth_lab` nêu thị trường mặc định `vn`; mỗi vai của squad có file `roles/<vai>.md`; mỗi file hướng dẫn nhắc nạp skill thị trường; không vai nào gửi, đăng, chi tiền | Lỗi |

Tiêu chí nghiệm thu:

- A1 `pnpm catalog:check` và `pnpm catalog:build` xanh.
- A2 Bộ chọn squad có nhóm "Marketing" với 5 squad, `marketing` đứng đầu; "Core" không còn `content_studio`, "Kit Viet" không còn `kv_marketing`, `kv_pr`.
- A7 (G7) Chọn squad `marketing` một lần thì cả bộ được tạo; một yêu cầu gán cho `marketing` đi tới đúng squad chuyên môn và trở về lead khi issue con đóng, kể cả chuỗi Marketing Studio rồi Content Studio. Mục này cần diễn tập thật (plan T6.2); chưa có diễn tập thì không được ghi là đạt.
- A3 `catalog map marketing` in bản đồ đúng (V7).
- A4 V5 xanh: không đổi hành vi hiện có, trừ D10.
- A5 `make agent-pack PACK=market-strategist ARGS=--dry-run` liệt kê cùng 19 skill.
- A6 Thêm một thị trường thử `zz` (skill + khai báo) chỉ sửa file dưới `catalog/`; sau đó xóa (kiểm chứng G5, như plan T3.7 của template-registry).

## 8. Bảo mật, giấy phép và tuân thủ

- Skill là chuỗi cung ứng prompt: coi thay đổi `skills/marketing/**` như thay đổi code khi duyệt. Mọi skill nhập phải qua kiểm tra bí mật của L3 (hiện bốn mẫu) và kiểm tra mẫu tiêm lệnh/mạng của `SOURCES.md` (script, `curl`, lệnh tải).
- MIT: giữ nguyên thông báo bản quyền trong `licenses/`. Repo upstream có hệ thống đối tác tài trợ nhưng nó nằm ở `tools/` và README, không trong lõi skill; ta bỏ `tools/`. Bảng công cụ trung lập trong skill được giữ.
- Skill vendor và nội dung từ trang web (đánh giá, bài đăng, HTML) là dữ liệu không tin cậy: `mkt-context` nhắc lại điều upstream đã nói trong `customer-research`.
- Không có script nào chạy: bỏ script khỏi tất cả skill nhập. Không có thông tin đăng nhập: v1 không có kết nối dữ liệu.
- Quy định pháp lý trong skill thị trường ghi nguồn và ngày; kết luận pháp lý của nội dung VN thuộc người phụ trách pháp chế (không đổi so với `soat-luat-quang-cao`).
- Tuân thủ "Automation First" của `AGENTS.md`: squad không thêm điểm chờ người ngoài những điểm spec cho phép (chi tiền, gửi, đăng, đổi giá hay tracking trên hệ thống thật, phê duyệt cuối). Thiếu đầu vào thì ghi giả định và `MISSING`, không dừng.

## 9. Tương thích và rủi ro

Không có migration DB. Không id nào đổi (squad, skill, pack, collection mới chỉ thêm). `kv_marketing`, `kv_pr`, `content_studio` đổi collection và tăng `version` (patch); collection không lưu trong dữ liệu workspace (`squad.template_key` lưu id squad). `rev` của flow tính từ khối `flow` nên không đổi.

| # | Rủi ro | Giảm thiểu |
| --- | --- | --- |
| R1 | Đổi phép thử Kit Việt làm lộ hành vi mới ở squad `vertical` | Là D10, có test; ghi rõ trong báo cáo; một dòng để quay lại nếu người sở hữu không muốn |
| R2 | Bundle phình làm chậm quickstart | E6 tải lười theo skillset; đo và ghi kích thước trước và sau |
| R3 | Skill vendor đòi công cụ hoặc file không có (`.agents/product-marketing.md`, MCP) | `mkt-context`; nhánh "nếu không có" của skill; `MISSING` |
| R4 | Quá nhiều skill làm loãng ngữ cảnh agent | Gắn theo vai, chung chỉ 5; mô tả skill vendor dài (đến 500 ký tự) nên đo khi diễn tập |
| R5 | Mâu thuẫn giữa lời khuyên tiếng Anh và luật VN | Skill thị trường thắng; checker; chữ công khai qua cổng claim |
| R6 | Agent trùng tên bị dùng nhầm | Tiền tố "Growth Lab"; test kiểm tên agent của mọi squad là duy nhất trong catalog |
| R7 | Upstream trôi hoặc đổi quảng cáo | Ghim commit, `SOURCES.md`, quy trình nâng cấp, test V4 |
| R8 | `install-agent-pack.sh` hỏng sau E5 | Chạy `--dry-run` trước và sau, so danh sách |
| R9 | Khối lượng bản dịch (7 ngôn ngữ cho collection và squad mới) | Thuộc Q3; làm trong cùng task |
| R10 | Growth Lab chạy không có dữ liệu thật và tạo "khuyến nghị rỗng" | `MISSING`, routine cho phép "không có gì mới", checker chặn khuyến nghị không bằng chứng; diễn tập (plan T6.2) |
| R11 | Nội dung `mkt-market-*` sai về pháp lý | Nguồn và ngày bắt buộc, `[cần bổ sung: nguồn]`; người pháp chế duyệt trước khi coi là ổn định (mục 10) |
| R12 | Cài cả bộ tạo 36 agent, nhiều hơn người dùng cần | Hộp chọn bỏ squad chuyên môn; agent chỉ chạy khi có việc; nếu đo cho thấy phiền, thu bớt `includes` mặc định (ví dụ PR) |
| R13 | Lead `marketing` giao việc nhưng squad nhận không chạy, hoặc lead không được đánh thức khi issue con đóng | Spike T0.4 trước khi xây E9, E10; diễn tập T6.2; nếu hỏng thì dùng phương án thu hẹp ở 5.11 và ghi rõ chưa đạt G7 |
| R14 | Lead vòng lặp: giao đi giao lại một yêu cầu, hoặc tự viết sản phẩm thay squad | `instructions.md` giới hạn số lần giao lại (hai vòng như các squad khác), kiểm tra V10, checker của squad chuyên môn giữ nguyên |
| R15 | Yêu cầu nhiều bước dừng chờ người duyệt ở squad con (ví dụ cổng thông điệp của Marketing Studio) làm lead tưởng bị kẹt | Lead đọc trạng thái issue con, ghi nhận "đang chờ người" và báo cho người yêu cầu, không giao lại; `instructions.md` nói rõ |

## 10. Câu hỏi còn mở và việc chưa làm

Đã quyết (không hỏi lại): các D ở mục 0. Việc để lại, mỗi việc có lý do:

- **Đợt 2 skill**: `social`, `video`, `image`, `public-relations`, `influencer-marketing`, `community-marketing`, `co-marketing`, `events` gắn vào Content Studio bản quốc tế và PR; trước đó cần quyết định skill VN tương ứng đứng ở đâu so với skill EN. Content Studio hiện không có skill, nên đây là cải thiện lớn nhất sau v1.
- **Flow cho `growth_lab`** (D11): viết theo flow-authoring-runbook sau khi diễn tập v1 cho thấy chỗ nào cần cưỡng chế; cần test DB, theo quy định test DB của repo.
- **Một squad thật duy nhất** (thay cho cửa trước và bốn squad chuyên môn): cần bộ sinh Kit Việt cho phép squad khác dùng skill `kv:` của `kv_marketing` mà không nhân đôi, và flow có cổng cho squad mới; làm sau khi cả hai có, rồi chuyển bốn squad chuyên môn sang `deprecated` (mục 6).
- **Flow cho chính squad `marketing`** (cổng cho mọi chuỗi): lead hiện điều phối bằng lời; flow cưỡng chế chuỗi soạn, soát, đăng sẽ chắc hơn nhưng phải viết và diễn tập.
- **Gộp hoặc hội tụ `kv_marketing` và `content_studio`**: xét sau khi `content_studio` có flow và skill.
- **Thị trường mới ngoài `intl`** và tách `intl` theo khu vực.
- **`conversion-tracking`**: cần chính sách thực thi script và quyền ghi vào site khách.
- **Domain Sales**: xét `cold-email`, `prospecting`, `sales-enablement`, `revops` cùng `sales_loop`.
- **Thẻ công ty mẫu `growth_gtm`**: gắn kỹ năng Growth Lab hoặc bàn giao sang squad khi domain ổn định.
- **Kết nối dữ liệu** (GA4, Search Console, ads): cần credential và chính sách chi tiêu.

## 11. Chưa kiểm chứng

- Chưa chạy `catalog check`, `catalog build`, hay test nào trong lúc rà soát; mọi khẳng định về "hiện đang xanh" chưa được xác nhận. Plan T0.1 ghi nền trước khi sửa.
- Chưa đọc từng dòng 33 skill sẽ nhập (chỉ đọc `ads`, `cro`, `copywriting`, `marketing-loops`, `marketing-council` và đo kích thước, script, khối quảng cáo bằng lệnh). Danh sách khối quảng cáo cần gỡ ngoài `ad-creative` sẽ có khi `grep` lúc nhập (plan T2.3).
- Con số "không đụng Go" suy ra từ việc `server/internal/catalog` chỉ nhúng dữ liệu công ty, workflow và bundle quickstart; chưa chạy `go build` hay `go test` để xác nhận. Plan T6.1 chạy.
- Chưa kiểm tra e2e và giao diện thật của nhóm "Marketing"; đã đọc mã bộ chọn (`squad-quickstart.tsx`) và thấy nó nhóm theo collection.
- `agentTemplate` của từng vai `growth_lab` (mục 5.4) chọn theo mẫu mà `content_studio` đã dùng cho vai tương tự; chưa đọc từng mẫu để xác nhận khớp.
- Số liệu kích thước đo từ clone upstream ở `1f83744` (không tính `evals/`); bản nhập có thể nhỏ hơn sau khi bỏ script và khối quảng cáo.
- Điều kiện của E4 (squad có sẵn có thể đang vi phạm) chưa được thử; plan T1.4 xử lý nếu có.
- **Điều phối giữa squad (5.11) mới được đọc, chưa chạy.** Đã đọc: quyền gán issue cho squad, cờ `--assignee-squad`, tự vào flow khi gán cho squad có flow, quy tắc `child_done`. Chưa xác nhận: (a) agent do quickstart tạo qua được `canInvokeAgent` của lead squad đích; (b) `child_done` đánh thức đúng lead `marketing` khi issue con của nó (gán cho squad Kit Việt chạy flow) đóng, kể cả khi flow đóng issue ở trạng thái đặc biệt; (c) issue con gán cho squad flow không bị coi là sub-issue do flow quản lý; (d) lead lập kế hoạch và đọc kết quả tốt trên thực tế. Đây là điều kiện của G7: plan T0.4 kiểm trước, T6.2 diễn tập sau.
- Chưa thiết kế chi tiết giao diện danh sách "cài kèm" (bố cục, trạng thái lỗi từng squad); chỉ nêu hành vi ở E10. Quy tắc UI Copy của `AGENTS.md` áp dụng: không viết mô tả thừa.

## 12. Hiện trạng triển khai

Ngày: 2026-11-16. Thực hiện theo [plan.md](plan.md). Đã commit lên main theo yêu cầu. Cây làm việc còn các thay đổi của người khác ở `server/`, `platform-management/` và `packages/views/squads/flow-designer/`; không đụng vào.

### 12.1 Đã làm

- **Engine** (mục 5.7): E1/E2 kind `domain`, `markets`, `catalog map` (E3), kiểm tên skill của squad (E4), pack tham chiếu skill catalog (E5, `scripts/install-agent-pack.sh` cũng vậy), skillset `chunk` (E6), `adapter.type` cho phép thử Kit Việt (E7), `includes` (E9), cài cả bộ trong quickstart và bộ chọn squad (E10). Test: `scripts/catalog/catalog.test.mjs`, `packages/core/squads/upgrade.test.ts`, `packages/core/squads/quickstart.test.ts`, `packages/views/modals/squad-quickstart.test.tsx`.
- **Nội dung**: domain `marketing` (manifest và README); collection `marketing` (7 bản dịch); 51 skill trong `catalog/skills/marketing/` (33 nhập ghim `1f837447`, 15 chuyển từ pack giữ nguyên byte, 3 tự viết); `SOURCES.md`; skillset `marketing` (chunk); squad `marketing` (cửa trước, `includes` 4 squad), `growth_lab` (mới, 7 vai); `kv_marketing`, `kv_pr`, `content_studio` chuyển sang collection `marketing`.
- **Pack** `market-strategist`: giữ 19 tên và hành vi; skill nằm trong catalog; xóa `skills/` và `licenses/` của pack; `SOURCES.md` và `README.md` trỏ về domain.
- **Tài liệu**: README domain, `catalog/README.md`, `AGENTS.md` (một mệnh đề), mục 0 của template-registry spec.

### 12.2 Khác thiết kế

1. **Giao việc theo id, không theo tên.** `multica issue create --assignee-squad` nhận id squad (`server/cmd/multica/cmd_issue.go:698`), không nhận tên như mục 5.11 viết. Lead tìm id bằng `multica squad list` rồi giao; `instructions.md` của `marketing` ghi đúng cách này.
2. **D10 có hệ quả đã thấy**: nút làm mới squad Kit Việt nay gồm 11 squad `vertical` (`kv_health`, `kv_construction`, ...). Đây là hành vi có chủ ý; để quay lại, đổi `isKitvietTemplate` theo mục 5.7 E7.
3. **Mô tả của `kv_marketing`, `kv_pr`, `content_studio` không đổi.** Mục E8 và 5.11 yêu cầu thêm câu nói về squad `marketing`; plan T3.2 chỉ cho sửa khi câu cũ sai, và câu cũ không sai.
4. **Thứ tự**: `marketing` 5, `kv_marketing` 10, `content_studio` 20, `growth_lab` 30, `kv_pr` 40 (trước đây `kv_pr` là 70).
5. **Ngôn ngữ**: chỉ dẫn agent và `SOURCES.md` viết tiếng Anh (như `content_studio` và pack); ba skill `mkt-*` và phần giải thích trong README viết tiếng Việt.
6. **Mở rộng phần gỡ quảng cáo** (mục 5.5 cho phép hai loại). Ngoài sản phẩm của tác giả và liên kết tới `tools/`, còn gỡ quảng cáo đối tác đã đánh dấu (◆), liên kết tương đối tới file ngoài thư mục skill (không resolve khi cài), và một lệnh chạy script trong `tools/`. Mỗi thay đổi được ghi trong `catalog/skills/marketing/SOURCES.md` với mã R1 đến R5.
7. **Bundle chunk không chứa `template.json` và `i18n/` của skill catalog** (cũng là quy tắc của skill cài đặt). Skill ở pack tham chiếu catalog được lọc như vậy.

### 12.3 Spike điều phối (T0.4)

- **(a) Agent do quickstart tạo có được lead squad đích gọi không.** Đọc mã: `validateAssigneePair` (nhánh squad) gọi `canInvokeAgent` với leader; người được phép là chủ agent hoặc người khởi chuỗi có quyền trên agent `public_to`. Quickstart không đặt `permission_mode`, nên agent là private: chủ là người chạy quickstart nên được phép; thành viên khác của workspace sẽ nhận 403 khi gọi lead của squad không do họ tạo. (`server/internal/handler/issue.go:4551-4600`, `agent_access.go:49-89`.)
- **(b) `child_done` đánh thức lead khi squad cha bị giao con.** Test có sẵn chạy trên `multica_engine_test`: `TestChildDoneWakesSquadLeader`, các test `TestChildDone*`, `TestChildrenDoneStageCondition`, `TestStageProgress*`, `TestIssueWakeupSquadLeaderAttribution`: đều đạt. Chưa có test chuỗi thật "lead tạo con gán squad khác, con đóng, lead được đánh thức".
- **(c) Issue con gán squad có flow.** Agent tạo con dưới cha không chạy flow không bị từ chối (`checkFlowChildCreate`, `issue.go:3573`; chỉ chặn khi cha do flow quản lý). Giới hạn độ sâu (`CheckSecretaryRequestCaps`) chỉ áp khi cây nằm dưới bàn thư ký. Chưa chạy chuỗi thật.

Kết luận: (a) (b) (c) đạt về đọc mã và test có sẵn; chưa có bằng chứng chạy thật. **G7 chưa được xác nhận.** Phương án thu hẹp (5.11) không cần dùng, vì không có điểm nào hỏng.

### 12.4 Chưa làm

- **Diễn tập trên máy chủ dev** (T0.4 phần 2 và T6.2): tạo 5 squad, 36 agent và các issue thật trên workspace `tupk`. Plan và mục 5.11 yêu cầu người dùng đồng ý trước; chưa chạy.
- **Pháp chế** chưa duyệt `mkt-market-vn` và `mkt-market-intl` (đang beta, mục R11).
- **Flow** cho `growth_lab` và squad `marketing` (mục 10, D11).
- **Test chuỗi UI**: nhóm "Marketing" trong bộ chọn chưa có test giao diện riêng; nhóm này do collection `marketing` sinh ra và đã kiểm qua dữ liệu thật trong test quickstart.

### 12.5 Lệnh đã chạy và kết quả

| Lệnh | Kết quả |
| --- | --- |
| `pnpm catalog:check` | ok, 262 template (263 trong lúc thử mở rộng) |
| `pnpm catalog:check --base HEAD` | ok: không id nào biến mất |
| `pnpm catalog:build` | ok. Băm nền (V5): `skills-builtin.ts` vẫn 2 248 538 byte, cùng băm nội dung; 21 flow giữ nguyên `rev`; danh sách squad khác có chủ ý (2 squad mới, 3 đổi collection) |
| `node --test scripts/catalog/*.test.mjs` | 60/60 (trước: 41/41) |
| `pnpm --filter @multica/core exec vitest run catalog squads agent-packs` | đạt; trong toàn bộ core: 5446 đạt, 1 hết giờ 5 s (`sdlc-skills.test.ts`, đạt khi chạy riêng) |
| `pnpm --filter @multica/views exec vitest run modals/squad-quickstart` | 18/18 |
| `pnpm --filter @multica/views exec vitest run modals/agent-pack-install` | 4/4 |
| `pnpm typecheck` | 11/11 task thành công |
| `pnpm lint` | 0 lỗi; cảnh báo có sẵn, không ở file đã sửa |
| `go build ./...` và `go test ./internal/catalog/...` | ok |
| `go test -run` các test child_done và squad (CSDL `multica_engine_test`) | đạt |
| `make agent-pack PACK=market-strategist ARGS=--dry-run` (với CLI `multica` giả, không đăng nhập) | 19 skill, cùng danh sách trước và sau |
| thử mở rộng thị trường `zz` | `catalog:check` ok; `catalog map` liệt kê `zz`; thị trường `yy` không khai báo bị từ chối; đã gỡ |
| `pnpm test` (toàn repo) | nhiều lỗi hết giờ dưới tải song song; các file lỗi đều đạt khi chạy riêng (mục 12.6) |
| `pnpm --filter @multica/views test` (toàn bộ) | 712 file đạt, 11 file lỗi do hết giờ 30 s dưới tải; chạy riêng 11 file đó: 375/375 đạt |

Không chạy: `make test` (Go đầy đủ; có thay đổi WIP của người khác ở `server/`), diễn tập trên máy chủ dev (mục 12.4).

### 12.6 Ghi chú về kiểm tra

- Các test có giới hạn 5 s chạy chậm khi tất cả workspace chạy song song. Những file lỗi trong `pnpm test` (harness SDLC, cron grammar, linkify, icon-button, tooltip, text-contrast, platform-console Overview, sdlc-skills bundle) đều đạt khi chạy riêng; không có file nào trong danh sách liên quan đến thay đổi này ngoài `sdlc-skills.test.ts` (đọc bundle nên chậm).
- Các sửa test có chủ ý: `squad-invariants.test.ts` (tên skill chunk), `research-lab.test.ts` (thứ tự nhóm core: squad sau `bmad_planning`).

### 12.7 Diễn tập trên máy chủ dev (tupk), 2026-11-16

Người dùng đồng ý chạy (2026-11-16). Đối tượng tạo ra và kết quả ghi trong nhật ký riêng của lượt diễn tập (không đưa vào repo). Máy chủ: workspace dev `tupk` (địa chỉ máy chủ không ghi trong tài liệu).

**Cài bộ bằng `runSquadSetQuickstart` (cùng hàm mà giao diện gọi):**
- Hai squad tạo mới: Marketing (`0e497ccf`) và Growth Lab (`6c11021b`). Cả hai báo "no free seat on the Launch floor": squad có mặt nhưng chưa được đặt trên tầng.
- Tám agent tạo mới (Marketing Chief, bảy vai Growth Lab), 28 agent dùng lại vì tên đã có. Vì vậy con số 36 agent của mục 5.11 chỉ đúng trên workspace trống; trên tupk là 8 mới và 28 tái dùng.
- Ba squad đã có và được dùng lại: Marketing Studio, Content Studio, PR & Comms.
- Hai autopilot hằng tuần được tạo và đã tạm dừng sau diễn tập theo yêu cầu.
- Bốn skill không ghi đè vì tên đã có skill riêng của workspace (`product-marketing`, `marketing-ideas`, `competitor-profiling`, `customer-research`). Growth Lab và Marketing chạy với bản cũ của các skill này.
- Phụ: hai issue workflow mới cho Marketing Studio và PR & Comms (hai squad này trước chưa có).

**Chuỗi mặt tiền (TUPK-1134):**
- Lần chạy đầu của Marketing Chief thất bại lúc 09:24 vì CLI Claude báo "còn tải cài đặt tổ chức". Tôi giao lại (thành viên rồi squad) để kích hoạt lần chạy mới; lần này chạy được.
- Lead đặt thị trường `vn` và tạo một việc con TUPK-1138 cho Marketing Studio. Định tuyến đúng dòng bảng điều phối.
- Marketing Studio dừng ở cổng thông điệp (stop 1) vì chưa có thông điệp đã duyệt. Đây là điểm dừng đúng như thiết kế; người duyệt cần quyết định thông điệp. Chuỗi chưa đi tới Content Studio.
- **child_done:** tôi hủy TUPK-1138 lúc 11:08:10. Lead Marketing Chief phản hồi lúc 11:08:39 (khoảng 30 giây): báo việc con đã hủy, không tự tạo lại, nêu thông điệp đề xuất và câu hỏi cho người duyệt. Đây là bằng chứng chạy thật cho điểm (b) của T0.4: lead được đánh thức khi việc con đóng.

**Yêu cầu Growth Lab (trực tiếp):**
- TUPK-1135 (SEO audit, vn): đã làm xong, đang chờ duyệt (in review).
- TUPK-1136 (landing review, intl): bị chặn sau hai lần Checker trả FAIL (trích dẫn skill không kiểm chứng được, và các claim chưa có nguồn). Lead ghi rõ MISSING và hỏi người yêu cầu.
- TUPK-1137 (kế hoạch thử email, vn): vẫn đang chạy ở lần kiểm tra cuối. Một phần bình luận không đăng được vì token của lần chạy hết hạn; nội dung còn nguyên trong thư mục làm việc.

**Phát hiện cần xử lý (không sửa trong lượt này):**
1. Checker không thấy skill của vai (`cro`, `copywriting`): vai Checker chỉ có skill chung, nên không kiểm được trích dẫn skill mà vai Conversion đưa ra. Cần xem lại `byRole` hoặc quyền đọc cho Checker.
2. Tìm kiếm knowledge base trả về tài liệu của dự án khác (Mùa Ngon). Lead đã bỏ qua, nhưng việc cô lập dữ liệu giữa các dự án cần kiểm tra.
3. Token của lần chạy hết hạn trước khi bình luận được đăng (TUPK-1137).
4. Quickstart để squad mới không có chỗ trên tầng Launch; cần người quản trị đặt chỗ thủ công hoặc đổi tầng.
5. Bốn skill trùng tên với bản cũ của workspace: quickstart giữ bản của người dùng, nên bản mới của catalog không có hiệu lực ở đây.

**Trạng thái cuối (11:10):** TUPK-1134 blocked (chờ người duyệt thông điệp), TUPK-1135 in review, TUPK-1136 blocked (chờ trả lời), TUPK-1137 in progress, TUPK-1138 cancelled. Hai autopilot đã tạm dừng.

### 12.8 Tiếp tục diễn tập (phê duyệt giả lập), 2026-11-16

Người vận hành (chủ các agent) theo yêu cầu của người dùng đã đóng vai người phê duyệt. Chỉ phê duyệt cổng thông điệp; không duyệt đề xuất đăng.

- **Cổng thông điệp (TUPK-1134):** duyệt đúng đề xuất của lead, không thêm claim. Lead đã mở TUPK-1139 với thông điệp đã duyệt.
- **TUPK-1136 (landing review):** trả lời giả lập của người yêu cầu (sản phẩm giả định; không dùng dùng-thử, không-cần-thẻ, 10 giờ, 5.000 đội). Conversion nộp bản sửa 2; Checker PASS bốn điểm sau một lần sửa câu. Issue vẫn in review, các mục MISSING còn nguyên.
- **TUPK-1139 (Marketing Studio, stage 1):** lead không tạo được sub-issue (lỗi quyền, 8 lần) và mention lần đầu bị từ chối ("permission not granted"). Lần mention của người vận hành viết thuần văn bản nên không kích hoạt; lần sau dùng liên kết mention đúng, Mkt Scout chạy và nộp kết luận lúc 11:32:51 (6 văn bản, nguồn chính thức chưa mở được).
- **Kiểm bàn giao stage 1 (lead, 11:33:36):** không đạt chuẩn để sang stage 2: đầu ra chỉ nằm trong bình luận, không có file, không có bản nháp trong knowledge base; knowledge service báo offline.
- **Chẩn đoán knowledge:** container knowledge (app, docreader, postgres) và Ollama (bge-m3) đều chạy và khỏe. Khả năng cao là kết nối giữa máy chủ Multica và dịch vụ knowledge; cần người có quyền cấu hình máy chủ kiểm tra.

**Chưa hoàn tất:** chuỗi chưa đi tới soạn, soát claim, bàn giao và đề xuất đăng ở Content Studio. Không có gì được đăng, gửi hay chi tiền.

**Lỗi nền tảng cần sửa (được quan sát, không sửa trong lượt này):**
1. Lead của squad không tạo được sub-issue (lỗi quyền) và mention tới agent cùng squad bị từ chối "permission not granted", dù agent cùng chủ.
2. Knowledge service báo offline từ phía máy chủ Multica, trong khi các thành phần knowledge đều khỏe.
3. Token của lần chạy có thể hết hạn trước khi bình luận được đăng (đã thấy ở TUPK-1137).
4. Vai Checker không đọc được skill của vai Conversion, nên không kiểm được trích dẫn skill.
5. Quickstart không đặt được squad mới lên tầng Launch khi tầng đã đầy.

### 12.9 Đổi cấu hình agent và chuỗi stage 2–3, 2026-11-16

- **Đổi agent:** 466 agent đang hoạt động của tupk nay dùng runtime kiro (`397b125c`) và model `claude-haiku-5.5`; 220 agent được cập nhật trong lượt chạy, không có lỗi; kiểm tra lại: 466/466 khớp. Không có agent lưu trữ. Các agent đều thuộc tài khoản vận hành và có `runtime_bound` = true.
- **Stage 2 thành công (11:42):** lead giao được TUPK-1141 (Mkt Content) và TUPK-1140 (Mkt Ads) khi lượt chạy có người gốc (lượt được kích hoạt bằng bình luận của người vận hành). TUPK-1140 đã xong; TUPK-1141 đang chạy.
- **Stage 3 bị chặn (11:45):** lead không tạo được sub-issue cho ba cổng (Mkt Claim Gate, Mkt Voice Gate, Mkt Numbers Gate). Log máy chủ: `agent invoke denied: delegation chain carries no human originator`, cho lượt chạy của lead. Lượt này được kích hoạt bởi việc con hoàn tất (child_done), không phải bởi người.
- **Kết luận chẩn đoán:** agent `private` chỉ được gọi khi chuỗi có người gốc. Lượt wakeup do việc con hoàn tất không mang người gốc, nên lead không gọi được agent private nào; đổi sang `public_to` không đủ vì việc gọi cũng cần người gốc. Đây là lỗi của thiết kế nền tảng (chuỗi wakeup không kế thừa người gốc từ yêu cầu), không phải lỗi cấu hình.
- **Chưa làm:** cổng claim, cổng giọng thương hiệu, cổng số liệu, bàn giao và đề xuất đăng ở Content Studio. Không có gì được đăng, gửi hay chi tiền.

### 12.10 Cổng stage 3 (thủ công), 2026-11-16

- Lead không tạo được sub-issue cho ba cổng (chuỗi child_done không có người gốc, xem 12.9). Người vận hành đã tạo thủ công TUPK-1142 (Mkt Claim Gate), TUPK-1143 (Mkt Voice Gate) và TUPK-1144 (Mkt Numbers Gate) dưới TUPK-1139.
- Lần đầu, cổng claim chặn vì không thấy bản nháp (bản nháp nằm trong bình luận). Người vận hành chỉ đường đọc bình luận của TUPK-1140 và TUPK-1141 cho cả ba cổng.
- **Mkt Claim Gate: CHẶN.** Thiếu "Sổ quy định quảng cáo" của squad và giấy công bố sản phẩm.
- **Mkt Voice Gate: CHẶN.** Thiếu cẩm nang giọng thương hiệu trong knowledge base.
- **Mkt Numbers Gate: QUA.** Không có con số thị trường cần đối chiếu nguồn; các số còn lại tính lại khớp.
- Chuỗi dừng đúng quy tắc: không có nội dung nào được chuyển sang handover hay Content Studio, nên không có đề xuất đăng nào được tạo. Không có gì được đăng, gửi hay chi tiền.
- Để đi tiếp cần người dùng cung cấp hai tài liệu thật của squad (sổ quy định quảng cáo và cẩm nang giọng thương hiệu). Không tự tạo các tài liệu này.
