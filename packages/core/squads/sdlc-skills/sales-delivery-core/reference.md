# Sales kinds, headings and Record fields

All kinds live in the knowledge base `sales`. The publish policy decides whether a draft from an agent is published at
once (`auto`), waits for a person (`manual`), or is published after another squad's critic attests it
(`auto_after_review`). A kind not listed here is not yours to write.

| Kind | Policy | Review days | Required headings | Record fields |
| --- | --- | --- | --- | --- |
| `sales_lane` | auto | 90 | Record, Goal, Segment, Offer, Signals, Sources | `goal`, `status` (draft/active/paused), `regions`, `offer`, `families`, `weekly_cap`, `min_score` |
| `sales_source` | auto | 30 | Record, What it gives, How to read it, Recipe, Terms and limits, Probe log | `platform`, `family`, `adapter`, `rung`, `url`, `regions`, `sectors`, `auth`, `terms`, `status`, `added_by`, `last_run`, `last_http`, `hits_7d`, `hits_30d`, `qualified_30d`, `cost_30d`, `recipe_version` |
| `sales_account` | auto | 90 | Record, Why now, Evidence, Contact path, Fit | `entity_key`, `lane`, `country`, `province`, `sector`, `size_band`, `signal_family`, `signal_url`, `signal_date`, `fit_score`, `stage`, `stage_set_by`, `do_not_contact`, `contact_channel`, `contact_value`, `contact_source`, `owner`, `next_action_date`, `last_touch` |
| `sales_opportunity` | auto | 30 | Record, Need, Decision, Next step | `account`, `lane`, `stage` (discovery/proposal/negotiation/won/lost), `value`, `currency`, `timeline`, `budget_signal` (none/hinted/confirmed), `close_reason`, `next_action_date` |
| `sales_outreach` | auto | 0 | Record, Message | `account`, `opportunity`, `channel`, `state` (draft/checked/approved/sent/replied/rejected), `verdict` (pass/concerns/fail), `compliance` (ok/missing), `sent_date` |
| `sales_lesson` | auto_after_review | 180 | Record, Pattern, Evidence, Applies to | `lane`, `evidence_count`, `family`, `effect` (raise_weight/lower_weight/new_rule/template_change), `confidence` |
| `sales_share` | auto | 90 | Record, Shared content, What it says, Outcome | `platform`, `url`, `state` (new/processed/rejected/expired), `shared_by`, `shared_at`, `intent` (seeking/complaint/announcement/hiring/irrelevant), `account`, `expires_at` |
| `sales_icp` | manual | 180 | Record, Segments, Buyers, Triggers, Disqualifiers, Regions | `lane`, `valid_until` |
| `sales_offer` | manual | 90 | Record, Offerings, Price book, Terms | `lane`, `valid_until` |
| `sales_proof` | manual | 180 | Record, Cases, Capabilities, Numbers | `lane` |
| `sales_voice` | manual | 365 | Record, Tone, Say, Avoid | `language` |
| `sales_templates` | manual | 365 | Record, Approved templates | `lane`, `channel`, `goal` |
| `sales_report` | auto | 0 | Record, Window, Stages, Needs you, Due, Source health | `window_start`, `window_end`, `lanes` |

## Record format

```
## Record

entity_key: domain:example.vn
lane: fnb-hcm
stage: qualified
stage_set_by: agent
fit_score: 72
signal_family: expansion
signal_url: https://example.vn/news/khai-truong-chi-nhanh-3
signal_date: 2026-11-08
```

One `key: value` per line, no nesting, no quotes unless the value contains a colon. Empty values are written as
`key:` with nothing after it. Dates are `YYYY-MM-DD`.

## Idempotency keys

| Entity | Key |
| --- | --- |
| lane | `sales:lane:<lane>` |
| source | `sales:source:<source id>` (a starter source uses its catalogue id) |
| account | `sales:account:<lane>:<entity_key>` |
| opportunity | `sales:opportunity:<lane>:<account key>:<n>` |
| outreach | `sales:outreach:<lane>:<account key>:<channel>:<n>` |
| lesson | `sales:lesson:<lane>:<pattern slug>` |
| share | `sales:share:<platform>:<hash of url>` |
| report | `sales:report:<window start>` |

## Evidence grades

| Grade | Meaning | Use |
| --- | --- | --- |
| A | Primary source, dated, read by the squad in this run | Any claim, any stage |
| B | Secondary source (news, directory), dated | Signals, Why now |
| C | Undated, or a third-party summary | Context only; never a reason to qualify |
| U | Unverified (no source) | Must not appear in any Record |

## Assumptions

Assumptions live in the `## Assumptions` section of the lane's `sales_lane` document. One line each: `id`, `decision`,
`why`, `who can reverse it`. Lead adds the calibration assumptions; any role adds its own.
