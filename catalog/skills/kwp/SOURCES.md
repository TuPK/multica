# Sources: Knowledge Work Plugins (KWP) skills

This folder holds skills adapted from Anthropic's [knowledge-work-plugins](https://github.com/anthropics/knowledge-work-plugins) repository. It includes 11 vendored skills (Group A) copied verbatim with only two localized automated-runtime adjustments, alongside 15 authored derivative skills (13 Group B and 2 Group D) housed in `kwp-*` folders.

Each skill's `template.json` declares its source. This file records the upstream source, pinned commit, licence, local changes applied to vendored skills, reasons for excluded skills (summarised from the disposition table), and instructions for upgrading.

## Sources

| Source | Commit | Licence | Skills |
|---|---|---|---|
| [anthropics/knowledge-work-plugins](https://github.com/anthropics/knowledge-work-plugins) | `e605d8ddbfb688da57dfaaceebd93f12a64ef1d3` (2026-11-09) | Apache-2.0, `licenses/kwp.LICENSE` | 11: `analyze`, `explore-data`, `sql-queries`, `statistical-analysis`, `validate-data`, `create-viz`, `build-dashboard`, `tech-debt`, `risk-assessment`, `variance-analysis`, `user-research` |
| This repository (authored derivatives) | n/a | Apache-2.0 / Multica licence | 15: `kwp-ticket-triage`, `kwp-customer-escalation`, `kwp-kb-article`, `kwp-incident-response`, `kwp-runbook`, `kwp-change-request`, `kwp-knowledge-synthesis`, `kwp-search-strategy`, `kwp-synthesize-research`, `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique`, `kwp-data-context-extractor`, `kwp-untrusted-content`, `kwp-absent-is-not-zero` |

## Group A: Vendored skills

| Skill | Upstream plugin | Destination | Description |
|---|---|---|---|
| `analyze` | `data/skills/analyze` | quickstart `data_lead` | Core data analysis workflow and problem decomposition |
| `explore-data` | `data/skills/explore-data` | quickstart `data_profiler` | Schema inspection, structural profiling, anomaly scanning |
| `sql-queries` | `data/skills/sql-queries` | quickstart `data_analyst` | SQL query authoring across dialects, query tuning |
| `statistical-analysis` | `data/skills/statistical-analysis` | quickstart `data_analyst` | Descriptive stats, hypothesis testing, correlations |
| `validate-data` | `data/skills/validate-data` | quickstart `data_validator`, company `critic` | Data sanity checks, constraint checking, reconciliation |
| `create-viz` | `data/skills/create-viz` | quickstart `data_viz` | Chart and plot generation via Python libraries |
| `build-dashboard` | `data/skills/build-dashboard` | quickstart `data_viz` | Self-contained HTML interactive dashboard creation |
| `tech-debt` | `engineering/skills/tech-debt` | quickstart `sdlc_maintenance` (`lead`) | Technical debt identification, cataloging, remediation prioritization |
| `risk-assessment` | `operations/skills/risk-assessment` | company `finance_lead` | Risk matrix scoring, likelihood/impact grading, mitigation planning |
| `variance-analysis` | `finance/skills/variance-analysis` | quickstart `data_analyst` | Budget vs. actual variance analysis, driver breakdown |
| `user-research` | `design/skills/user-research` | company `validation_analyst` | User interview synthesis, user journey insight extraction |

## Local changes

In accordance with Multica's unattended automation principles (`AGENTS.md` Automation First), vendored skills must not stop unattended execution to ask a human when data is not connected. Only two lines across all 11 vendored skills are modified:

### `analyze`

- `SKILL.md` (line 44): Replaced `"1. Ask the user to provide data in one of these ways:"` with `"1. Use the data the request attaches or pastes; if none, state the assumption and continue:"`.

### `explore-data`

- `SKILL.md` (line 36): Replaced `"1. Ask the user to provide a table name (with their warehouse connected) or upload a file"` with `"1. Use the table or file the request names; if none, record the gap as MISSING and continue"`.

All other lines and files in the 11 Group A skills are byte-for-byte identical to upstream commit `e605d8ddbfb688da57dfaaceebd93f12a64ef1d3`.

## Left out of the upstream repository

The upstream repository contains 16 plugins with 252 skills, 174 MCP server declarations, and 5 slash commands. The bulk of upstream content is excluded for the following reasons (see `docs/engineering/knowledge-work-plugins-disposition.md` for full per-file analysis):

1. **Human-in-the-loop and interactive commands**: Skills that assume an interactive user sitting next to the agent, asking questions, prompting for file uploads interactively, or using slash commands (`/lead-score`, `/pipeline-review`, etc.).
2. **External connectors and SaaS integrations**: Upstream heavily depends on live connectors for CRM (Salesforce, HubSpot), email (Gmail, Outlook), calendar, Slack, and external ticketing tools. Multica executes inside the workspace without external API credentials or writeouts.
3. **Existing Multica coverage**: Multica already has specialized squads and skills for SDLC (`sdlc-*`, `superpowers:*`, `bmad-*`), marketing (51 vendored skills), sales loops, and Kit Viet domains. Upstream skills in these areas are redundant or lower fidelity.
4. **Outbound actions / external mutations**: Skills sending emails, signing documents (DocuSign), scheduling meetings, or creating accounts violate Multica's boundary rules.
5. **Upstream test data and evaluation files**: Upstream `evals/`, sample fixtures, and plugin setup scripts are omitted.

## Upgrading a vendored skill

1. Check out `anthropics/knowledge-work-plugins` at the target commit.
2. Diff each of the 11 Group A skills against this directory:
   ```bash
   diff -u /path/to/upstream/data/skills/analyze/SKILL.md catalog/skills/kwp/analyze/SKILL.md
   ```
3. Re-apply the two local changes above to `analyze/SKILL.md` and `explore-data/SKILL.md`.
4. Run `pnpm catalog:check` to verify schema, source pins, and absence of secrets.
5. Update `source.ref` in all `catalog/skills/kwp/*/template.json` to the new 40-character commit hash and update this file.
