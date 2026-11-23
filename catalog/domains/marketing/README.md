# Marketing

Marketing in Delegora: one front door, the squads that do the work, and the skills they load. Vietnam comes first; international markets are an expansion, and each squad and skill says which market it serves.

Design and decisions: [docs/specs/marketing-domain/spec.md](../../../docs/specs/marketing-domain/spec.md). Work plan: [plan.md](../../../docs/specs/marketing-domain/plan.md).

## Start here

1. Open the squad picker and choose **Marketing**. Creating it also installs the four squads it works with: Marketing Studio, Content Studio, Growth Lab and PR & Comms. You do this once.
2. Create an issue, assign it to **Marketing**, and describe the request in plain words: "chiến dịch Tết cho sản phẩm X và đăng lên fanpage", "kiểm toán SEO cho trang này", "thông cáo về việc ra mắt". The lead names the market, sends each part to the squad that does it, and reports back.
3. A request that needs a person's decision (publish, spend, a claim to approve) waits in that squad's proposal queue. The lead tells you which decision is waiting.

## Which squad does what

| Order | Squad | Does | Markets | Use it when |
| --- | --- | --- | --- | --- |
| 5 | Marketing | The front door. Sets the market, routes the work, checks the results, reports weekly. Does strategy and planning itself. Writes no finished piece | vn, intl | Always: this is the squad you create and send requests to |
| 10 | Marketing Studio | Plans campaigns, drafts multi-channel content and ad copy, checks claims against Vietnamese advertising rules, brand voice and figures. Flow with gates. Publishes nothing | vn | A campaign, post, script, ad or landing page needs writing and checking |
| 20 | Content Studio | Takes checked content to its channels: drafts one market, localises the second, checks, files a publish proposal. Publishes through Ayrshare after a person approves | vn, intl | Content must go out on several channels, or a second-market version is needed |
| 30 | Growth Lab | Audits and improves SEO and AI search, conversion, paid media, lifecycle email and measurement. Recommends with evidence; changes nothing that runs | vn, intl | You need to know what to change to get more visits, conversions or retention |
| 40 | PR & Comms | Press releases, press questions and answers, crisis statements. Checks facts, quotes and legal lines. Sends nothing | vn | Press or crisis communication is involved |

Who writes and who checks. Marketing Studio decides **what is said** and is the only place Vietnamese advertising claims are checked. Content Studio decides **what goes out, where and when**, and is the only place that publishes. Growth Lab recommends; anything it writes for the public is a draft that goes through a claim check first. A Vietnamese piece is checked in Marketing Studio, then passed to Content Studio as a brief.

A single squad can be created on its own, but then the requests have to go to the right squad by hand. Use **Marketing** unless you have a reason not to.

Outside the domain, but related: `market_research` (scans markets for the product pipeline and feeds Growth Lab with sources), `kv_ecom` (marketplace listings and margin, which is commerce, not campaigns), `sales_loop` (outbound sales, a separate domain). The company template's growth card (`growth_gtm`) is unchanged.

## Skills

The domain holds 51 skills in `catalog/skills/marketing/`, in eight groups: `foundation`, `strategy`, `research`, `search`, `conversion`, `paid`, `lifecycle`, `measurement`. The list below is not copied here, so it cannot drift: run

```bash
pnpm catalog:map marketing
```

Three of them are written for this workspace rather than vendored: `mkt-context` (where the marketing context lives and what the runtime can do), `mkt-market-vn` and `mkt-market-intl` (the market layer; both are beta until the legal owner reviews them). The rest come from the sources listed in [catalog/skills/marketing/SOURCES.md](../../skills/marketing/SOURCES.md), pinned to a commit. Their text is not changed to fit Delegora; the few local changes are listed there.

Skills do not carry the market. A squad loads `mkt-context` and the market skill for the brief's market, then the method skills of its role.

## Extend it

| To | Do | Edit nothing else |
| --- | --- | --- |
| Add a marketing skill | A folder under `catalog/skills/marketing/<group>/<id>/` with `template.json` (`sets: ["marketing"]`, `domain: "marketing"`, `source`) | code, the domain manifest |
| Change a role of Growth Lab | `catalog/squads/growth_lab/template.json` and `roles/<role>.md` | code |
| Add a squad to the domain | `pnpm catalog:new squad <id> --collection marketing`; set `domain` and `markets`; add its id to `includes` of `marketing` and a row to its routing table in `instructions.md` | code |
| Add a market | Add it to `markets` here, write `mkt-market-<id>`, and list it in the `markets` of each squad or skill that serves it (see the spec, section 5.6) | vendored skills, code |
| Add another domain (sales, legal) | `pnpm catalog:new domain <id>`, a collection with the same id, `domain` on its members | code |
| Update a vendored skill | The procedure in `catalog/skills/marketing/SOURCES.md` | |

Run `pnpm catalog:check` after any change; it checks the domain's references, the markets, the squads' skill names and the translations.

## Not done yet

Each item has its reason in the spec, section 10.

- Second-round skills for the international Content Studio and PR (`social`, `video`, `image`, `public-relations`, and others).
- A flow for Growth Lab and for the `marketing` squad itself.
- One real squad that replaces the front door and the four specialists. That needs the Kit Việt generator to share its `kv:` skills and a flow for a new squad.
- Merging `kv_marketing` and `content_studio`.
- Sales as a domain (`cold-email`, `prospecting`, `sales-enablement`, `revops`).
- `conversion-tracking`: needs a policy for scripts and for changes to a customer's site.
- Data connections (GA4, Search Console, ads). They need credentials and a spend policy.
