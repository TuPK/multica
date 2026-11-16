# Sources: marketing skills

This folder holds every skill of the marketing domain that Multica can load (51 skills in `catalog/skills/marketing/`). Each skill's `template.json` names its source. This file records the sources, each local change made to a vendored skill, what was left out, and how to update.

Rule (`docs/specs/marketing-domain/spec.md`, section 5.5): a vendored skill is never edited to fit Multica. The only changes allowed are the ones under *Local changes*: removing advertising for the maintainer's own product or for a listed partner, and removing links into the upstream `tools/` folder, which is not copied here. Each change is listed with its file.

## Sources

| Source | Commit | Licence | Skills |
|---|---|---|---|
| [coreyhaines31/marketingskills](https://github.com/coreyhaines31/marketingskills) `skills/<name>` | `1f837447b20bb6f0e72d87b80ece1c221c362c7d` (v2.14.12, 2026-11-11) | MIT, `licenses/marketingskills.LICENSE` | 33: `ab-testing`, `ad-creative`, `ads`, `ai-seo`, `analytics`, `attribution`, `churn-prevention`, `competitor-profiling`, `competitors`, `content-strategy`, `copy-editing`, `copywriting`, `cro`, `customer-research`, `emails`, `launch`, `lead-magnets`, `link-building`, `marketing-ideas`, `marketing-loops`, `marketing-plan`, `marketing-psychology`, `offers`, `onboarding`, `popups`, `pricing`, `product-marketing`, `programmatic-seo`, `referrals`, `schema`, `seo-audit`, `signup`, `site-architecture` |
| [phuryn/pm-skills](https://github.com/phuryn/pm-skills) | `2a12295a7717569baa7be181b4cfebe3ca38a6bb` | MIT, `licenses/pm-skills.LICENSE` | `market-sizing`, `user-segmentation`, `ideal-customer-profile`, `beachhead-segment`, `growth-loops`, `gtm-motions`, `value-proposition`, `prioritize-assumptions` |
| [wondelai/skills](https://github.com/wondelai/skills) | `c172996495bed0fcd26896a9416b2093fd7073f0` | MIT, `licenses/wondelai-skills.LICENSE` | `mom-test`, `jobs-to-be-done`, `obviously-awesome` |
| [MaxKmet/idea-validation-agents](https://github.com/MaxKmet/idea-validation-agents) | `3a4c800a3022e73e9997533b603c79b60ee7597a` | MIT, `licenses/idea-validation-agents.LICENSE` | `distribution-analysis`, `cac-modeler` |
| This repository (authored) | n/a | same as this repository | `outlet-finder`, `evidence-discipline`, `mkt-context`, `mkt-market-vn` (beta), `mkt-market-intl` (beta) |

## Skills by group

The group folder only sorts the skills; the installed name is the skill id. Skills with `markets` (the two `mkt-market-*` skills) apply to one market; the others apply to every market.

| Group | Skills |
|---|---|
| `foundation` | `evidence-discipline`, `product-marketing` |
| `strategy` | `growth-loops`, `gtm-motions`, `launch`, `marketing-ideas`, `marketing-loops`, `marketing-plan`, `marketing-psychology`, `obviously-awesome`, `offers`, `pricing`, `value-proposition` |
| `research` | `beachhead-segment`, `cac-modeler`, `competitor-profiling`, `customer-research`, `distribution-analysis`, `ideal-customer-profile`, `jobs-to-be-done`, `market-sizing`, `mom-test`, `outlet-finder`, `prioritize-assumptions`, `user-segmentation` |
| `search` | `ai-seo`, `content-strategy`, `link-building`, `programmatic-seo`, `schema`, `seo-audit`, `site-architecture` |
| `conversion` | `competitors`, `copy-editing`, `copywriting`, `cro`, `popups`, `signup` |
| `paid` | `ad-creative`, `ads` |
| `lifecycle` | `churn-prevention`, `emails`, `lead-magnets`, `onboarding`, `referrals` |
| `measurement` | `ab-testing`, `analytics`, `attribution` |
| `foundation` (authored) | `mkt-context`, `mkt-market-vn`, `mkt-market-intl` |

## Local changes

Each entry gives the file and the change. The number in brackets is the count of replacements.

- **R1**: advertising for the maintainer's own product removed, with its disclosure line.
- **R2**: advertising for a listed partner removed or generalised (upstream marks partners with ◆).
- **R3**: links into the upstream `tools/` folder reduced to their label. A file-name label in a table cell became a dash.
- **R3b**: absolute links to the upstream repository rewritten to a sibling skill path, or to their label when the skill is not imported.
- **R4**: a command that ran a script in `tools/` replaced by a plain step.
- **R5**: a relative link to a file outside its own skill folder, or to a skill not imported, reduced to its label. An installed skill carries only its own folder, so these links could not resolve.

### `ad-creative`

- `SKILL.md`: R4: tools/ CLI command replaced by a plain step (1).
- `SKILL.md`: R1: promotional block for the maintainer's product (Magister) and its disclosure removed; tools/SETUP link removed (1).
- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (5).

### `ads`

- `SKILL.md`: R3: link into tools/ removed (1).
- `SKILL.md`: R3: links into tools/ removed (1).
- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (4).
- `references/abm-playbook.md`: R2: partner example generalised (1).
- `references/reading-google-ads-data.md`: R3: link into tools/ replaced by the platform's docs (1).
- `paid/ads/references/creative-research-automation.md`: R5: relative link to a file outside this skill (or to an unimported skill) reduced to its label (9).
- `paid/ads/references/google-ads-audit-checklist.md`: R5: relative link to a file outside this skill (or to an unimported skill) reduced to its label (1).

### `ai-seo`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (1).
- `references/agent-readiness.md`: R2: vendor example and partner marker generalised (1).

### `analytics`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (6).

### `attribution`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (9).

### `churn-prevention`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (2).

### `content-strategy`

- `references/headless-cms.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (3).

### `cro`

- `conversion/cro/SKILL.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (1).

### `customer-research`

- `references/source-guides.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (1).

### `emails`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (7).

### `launch`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (1).
- `references/site-launch-qa.md`: R2: vendor example removed (1).
- `references/site-launch-qa.md`: R2: Verified Partner marker paragraph removed (1).

### `lead-magnets`

- `lifecycle/lead-magnets/references/content-to-skill.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (1).

### `link-building`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (6).

### `marketing-loops`

- `strategy/marketing-loops/references/loop-catalog.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (1).
- `strategy/marketing-loops/references/seo-operator.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (1).
- `strategy/marketing-loops/references/loop-catalog.md`: R5: relative link to a file outside this skill (or to an unimported skill) reduced to its label (1).
- `strategy/marketing-loops/references/seo-operator.md`: R5: relative link to a file outside this skill (or to an unimported skill) reduced to its label (1).

### `programmatic-seo`

- `references/implementation-platforms.md`: R2: partner name removed from the category (1).
- `references/implementation-platforms.md`: R2: vendor specifics paragraph removed (1).
- `references/implementation-platforms.md`: R2: Verified Partner marker paragraph removed (1).

### `referrals`

- `SKILL.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (8).
- `references/review-programs.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (3).
- `references/affiliate-programs.md`: R3: links into tools/ reduced to their label (file-name labels became a dash) (1).
- `lifecycle/referrals/references/review-programs.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (2).

### `signup`

- `conversion/signup/SKILL.md`: R3b: absolute link to the upstream repo rewritten to a sibling-skill path, or reduced to its label when the skill is not imported (1).

### `site-architecture`

- `references/platforms-and-migration.md`: R2: partner name removed from the category (1).
- `references/platforms-and-migration.md`: R2: Verified Partner marker paragraph removed (1).
- `references/platforms-and-migration.md`: R2: vendor-specific example generalised (1).

## Left out of the upstream copy

- `evals/`: 33 `evals.json` files, upstream's own test data. Agents do not read them.
- Scripts (`*.py`, `*.mjs`, `*.js`, `*.sh`) and the `tools/` folder. None of the 33 imported skills ships a script. `tools/` holds the upstream partner registry and integration guides.
- Skills not imported in this round (D12, spec 5.5): `conversion-tracking` (scripts, and a reference set that asks for code on the site), `reverse-engineer-marketing` (a script that calls the web), `marketing-council` (simulated opinions of real people), the sales group `cold-email`, `prospecting`, `sales-enablement`, `revops` (domain Sales, next to `sales_loop`), `sms` (messaging rules not yet owned). Second round: `aso`, `directory-submissions`, `free-tools`, `paywalls`, `social`, `video`, `image`, `public-relations`, `influencer-marketing`, `community-marketing`, `co-marketing`, `events`.

Some skills name a skill that is not imported, in plain text. The names are information only; an agent that needs one reports it as missing.

## Review of executable patterns

Nothing in this folder runs. Some references show commands for a person or an agent that has its own access. The review of every imported file found these, all kept as documentation:

- `paid/ad-creative/references/generative-tools.md`: `curl` requests to public generation APIs, and `npx` commands for video rendering tools.
- `search/ai-seo/references/linkedin-ai-citations.md`: `curl` requests that read a page's headers and robots meta tags.
- `search/ai-seo/references/agent-readiness.md`: one `npx` command for a public site checker.

The secret-pattern scan (AWS keys, private keys, Anthropic and GitHub tokens) runs on every skill in `pnpm catalog:check`.

## Upgrading a vendored skill

1. Clone the upstream repository at the new commit and diff each skill against this folder.
2. Re-apply the *Local changes* to the new text, and list any change that no longer applies.
3. Run `pnpm catalog:check`, the domain tests, and grep for `magister`, `◆`, `Verified Partner` and `tools/`. Read any new executable pattern.
4. Update `source.ref` and `version` in the skill's `template.json`, and the commit in the table above.
