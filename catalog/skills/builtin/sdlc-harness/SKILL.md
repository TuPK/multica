---
name: sdlc-harness
description: "Use when an SDLC issue is in the Harness status, when the squad lead audits or refreshes a repository's agent harness, or when a role must check the harness before it builds: prove the Ensure and Done commands run, write a minimal AGENTS.md (rules, pitfalls, conditional pointers) and CLAUDE.md, and certify them with check-harness. Headless: you never interview anyone."
---

# SDLC harness

The harness is what an agent reads and runs before it touches a repository: `AGENTS.md`, the `CLAUDE.md` that imports it, and two
commands that work: Ensure (bring the environment up and check it) and Done (the project's own verification). A harness that is
missing, stale or wrong sends every later run in the wrong direction. You make it correct once, prove it by running it, and keep it
current. Load sdlc-delivery-core first.

The file is small on purpose. A context file does not reliably raise an agent's success rate and it raises cost, because the agent
follows it. What pays is a command that proves the work, and a fact the agent cannot read from the code. Everything else stays out.

## You do not

- Change application code, tests, specs or requirements. You write instruction files and the small scripts or Makefile targets
  they name. Add no new tool: the Done command gathers the checks the repository already has.
- Describe the project. No overview, no stack list, no directory tree: the agent reads those from the code and the README.
- Copy a command's body, a ruling's text, or project status into `AGENTS.md`. Name the command; point to the source.
- Add a gate that the levels below do not name.
- Interview anyone. This skill runs unattended (see "Headless").

## What goes where

| Kind | Example | Lives in | Loaded |
| --- | --- | --- | --- |
| Contract | Ensure, Quick, Done | `AGENTS.md` section Verify, one line each | always |
| Rule | "Never X; do Y", with what enforces it | `AGENTS.md` section Rules | always |
| Pitfall | A fact the agent cannot read from the code and that cost a run, with evidence | `AGENTS.md` section Known pitfalls | always |
| Knowledge | Architecture, domain, history of decisions | `docs/` (for example `docs/planning-artifacts/architecture.md`) | on a condition, from a pointer in Read when |
| Procedure | A multi-step how-to | a skill or a `docs/` runbook | when called |
| State | Progress, rulings, assumptions | issues and `docs/sdlc/` | never in `AGENTS.md` |

Ask in this order. Can the agent learn it by reading code, a manifest or the README? Then leave it out. Would breaking it be an
error some tool or reviewer rejects? Then it is a Rule, and it says what enforces it. Did an agent trip on it, or did you while
building the harness? Then it is a Pitfall, with the evidence. Anything else stays out.

## Content rules for `AGENTS.md`

1. Only four sections: Verify, Rules, Known pitfalls, Read when. Delete a section with nothing in it, except Verify.
2. No product status: not implemented, scaffold, TODO, WIP, checkmarks, status table cells. A section titled Greenfield is exempt.
3. Name a command once per purpose. No flag lists, no multi-line scripts, no copied bodies.
4. A pointer says when to read: "Changing the API client: read `docs/...`". No bare paths. Every path exists.
5. Delete test: if you removed this line, would an agent make a mistake? If not, delete it.
6. A Pitfall cites its evidence: a ruling id, an issue id, or the command and its exit code. Say what goes wrong and what to do instead.
7. Do not restate what a linter, formatter or CI job already enforces. At most one Rule line that names the tool.
8. Every Rule ends with `Enforced by: <lint rule | test | CI job | hook | prose>`. A `prose` rule is counted; when it is broken twice, move it into code.
9. Target 80 lines. Above 120 lines or 8 KiB is a warning; above 200 lines or 24 KiB the stage is not certified.
10. No secrets, no absolute personal paths, no variable values. `.env.example` holds names only.
11. Concrete and checkable: "Run `<command>` before committing", not "test your changes". English, one idea per line.
12. Exception for an empty repository with no README and no architecture document: at most three lines of orientation that point to the document that will exist.

`references/agents-md-template.md` has the shape. Section names `Known pitfalls` and `Gotchas` are both read.

## The checker

`check-harness` is a plain Node script in this skill: `scripts/check-harness.mjs`, resolved from this skill's installed
directory. Run it from the repository, no install needed:

```
node <this skill's folder>/scripts/check-harness.mjs                  # static checks, JSON on stdout
node <this skill's folder>/scripts/check-harness.mjs --quick          # gate checks only, under one second
node <this skill's folder>/scripts/check-harness.mjs --cert           # gate and cert failures both block
node <this skill's folder>/scripts/check-harness.mjs --cert --run --write-cert   # Architect only: also run Ensure and Done, then write the certificate
node <this skill's folder>/scripts/check-harness.mjs --refresh        # adds what changed since the certificate
```

Exit code 0: nothing blocks in this mode. 1: a check blocks (read `blocking` and each check's `details`). 2: bad usage.
`--run` executes commands from the repository: only the Architect uses it, only on the squad's own repository, and only on a clean git tree.

Three levels. `gate` stops a Developer: the harness is missing or points at things that do not exist. `cert` is what the Harness stage
must satisfy before it is done. `warning` is reported, never blocks.

| ID | Checks | Level |
| --- | --- | --- |
| H1 | `AGENTS.md` exists; `CLAUDE.md` holds `@AGENTS.md` and at most one sentence | gate |
| H3 | Every path in a code span or link exists (URLs, globs, package names, generated and git-ignored paths are skipped) | gate |
| H4 | Every `npm`/`pnpm`/`yarn` script and `make` target in a code span or fenced block exists (other tools are not resolved; E1 and E2 cover them) | gate |
| H6 | Exactly one `Done = ...` line that names a command and an evidence level L1 to L3 | cert |
| H7 | No progress wording in the rules (a Greenfield section is exempt) | cert |
| H10 | `docs/sdlc/environment.md` has an `Ensure command:` line, and `AGENTS.md` names that file and that command | cert |
| H14 | `AGENTS.md` within 200 lines and 24 KiB (a warning above 120 lines or 8 KiB) | cert |
| E1 | `--run`: the Ensure command exits 0 | cert |
| E2 | `--run`: the Done command exits 0 on a clean git tree | cert |
| H2 | `docs/sdlc/harness-cert.json` is valid and its sha is a commit | warns |
| H5 | The toolchain version is pinned and CI uses the same major | warns |
| H8 | Each Rule ends with `Enforced by:`, and a named path or command exists | warns |
| H9 | CONTRIBUTING, a PR template, or a commit convention | warns |
| H11 | `.env` and `.multica/` are ignored, `.env.example` exists, no secret pattern in tracked files | warns |
| H12 | Each Known pitfall cites its evidence | warns |
| H13 | The must-read files total at most 32 KiB, or a `Read when:` index exists | warns |
| H15 | Every pointer to a document says when to read it | warns |
| H16 | No overview signs: directory tree, Overview or Project structure heading, narrative paragraph | warns |

A false positive is fixed in the file, not by weakening a check: rewrite the line, or list a path that is generated later in
`docs/sdlc/harness.json` as `{ "ignorePaths": ["generated/"] }`. The report also holds `footprint` (lines, bytes, list items, prose rules).

A `gate` failure cannot be lowered: fix the file. If a `cert` criterion cannot hold for a valid reason (the repository has no tests, so
Done has nothing to prove), record an assumption in `docs/sdlc/assumptions.md` that names the check and the reason, run with
`--accept <ID>=<assumption id>`, and continue. Lowering with a record is allowed. Stopping on it is not.

## Headless

There is no person to ask. Where the BMAD `bmad-project-context` skill would ask a question, take the recommended default and record
it as an assumption in `docs/sdlc/assumptions.md`. Use that skill's `references/best-practices.md` for what belongs in the file and its
ledger rule: removing an existing instruction needs one of its four grounds, and an existing instruction is kept unless a ground applies.
Read `references/agents-md-template.md` for the shape and the one `references/profile-*.md` that fits the repository.

## Mode 1: Harness stage (greenfield)

The issue is in the Harness status, after Environment. The environment runs and `docs/sdlc/environment.md` exists.

1. **Check first.** Run `check-harness --cert`. If it is certified and `--refresh` does not ask for a refresh, write the report only.
2. **Choose the profile.** web, backend, baas or mobile from the manifests; a repository can have two. Read the profile files: they list what to probe.
3. **Discover and run.** Read `package.json`, `Makefile`, CI, compose files, `.env.example`, `docs/sdlc/environment.md`. Run the Ensure command and every
   command you mean to write down; drop any that does not run. Probe what the profile lists.
4. **Make Done real.** If there is no single verification command, add one (a `verify` script or target) that gathers the typecheck, lint and tests the
   repository already has, plus a faster variant. It must exit non-zero when any of them fails. If no single idempotent command rebuilds the environment,
   that is DevOps work: block with class `tooling`.
5. **Keep a list of every trip.** Each environment error, wrong order of commands, missing variable or slow command you met in steps 3 and 4 is a candidate
   Pitfall: record the command, the exit code and what fixed it. These are the facts an agent cannot read from the code.
6. **Collect the Rules.** From the architecture document sections "Non-default conventions" and "Verification strategy" (if present), the policy the
   organisation requires, and the tools the repository already configures. Each Rule names what enforces it, or says `prose`.
7. **Write `AGENTS.md`** from the template. Keep an existing file's instructions (ledger rule), moving status and overview text out to `docs/` with a pointer.
   Make `CLAUDE.md` exactly `@AGENTS.md`. Apply the delete test to every line.
8. **Scratch and secrets.** Squad scratch and logs go in an ignored folder. Evidence worth keeping goes under `docs/`. Never write a secret value.
9. **Prove it.** Commit only your own paths. Run `check-harness --cert --run --write-cert` until it certifies, then commit `docs/sdlc/harness-cert.json`.
10. **Report** with `blocking`, `cert`, `warnings`, E1 and E2 (command, exit code, seconds), `footprint`, the number of `prose` Rules, the assumptions you recorded,
    and what is unverified. On an empty repository E2 may pass because there is nothing to check yet; the smoke below proves it can fail.

## Mode 2: Audit (maintenance)

On an existing repository, run `check-harness` and read the report; do not edit. The lead creates a Harness issue first only when a `gate` check
fails. A `cert` or `warning` failure gets a Harness issue queued behind the work, and never stops the Developer. Rank by what a failure costs an
agent: a wrong command or path first, a missing Done second. Never rewrite a person's `AGENTS.md`; fix wrong commands and paths, and add the Verify lines.

## Mode 3: Refresh

A refresh issue comes from the lead when `check-harness --refresh` says `needsRefresh` (the certificate is missing, broken or not an ancestor of HEAD,
or a manifest changed since its sha), after an epic retrospective is accepted, after the second accepted `environment`, `tooling` or `rework-loop`
ruling with the same cause, or when a Developer blocked on a `gate` failure.

1. Run `check-harness --refresh`. Fix what `manifestsChanged` and `blocking` name.
2. Growth rule: add a line only if an agent was corrected or blocked twice by the same thing, or you tripped on it now, and cite the evidence.
   Budget rule: above 80 lines, adding a line means removing one or moving a rule into code.
3. Pruning rule: apply the delete test to every line outside Verify. Remove a Pitfall whose cause was fixed in the code (cite the commit). The ledger rule
   governs every removal.
4. A Rule marked `prose` that was broken twice becomes a lint rule, test, CI job or hook. The lead queues that as a Developer issue; it is not part of this stage.
5. Run `check-harness --cert --run --write-cert`, commit the certificate, and report as in Mode 1 with lines before and after.

## Harness smoke

After the Harness stage, the lead creates one small issue for the Developer, who follows `AGENTS.md` alone and asks nobody:

1. Run the Ensure command; run it a second time.
2. Make one change with a test, inside one part of the repository (a fixed task for the profile).
3. Run the Quick check, then Done; record the exit codes.
4. **Negative step:** break an assertion or a type on purpose, run Done, confirm it exits non-zero, then undo the break.
5. Commit with the repository's convention.
6. Report every command you had to discover that `AGENTS.md` should have named.

| Measure | Target |
| --- | --- |
| Runs until the stage is done | 2 or fewer |
| Times the issue went to blocked | 0 |
| Times review or QA sent it back | 0 |
| Human comments needed | 0 |
| Done exits 0 on a clean tree, and non-zero in the negative step | yes |

Targets are a hypothesis until several projects have been measured. A command the Developer had to discover becomes a line in `AGENTS.md` or a check.
If the smoke misses, the Architect fixes the named gap and the lead runs it once more. A second miss goes to `docs/sdlc/parked.md` and the epics continue.
The smoke is a pass or fail test of the harness, not a statistic: one run cannot tell noise from effect.

## Measures

`scripts/harness-metrics.mjs` turns the platform's own run, usage, timeline and comment data into first-pass rate, runs and tokens per delivered
issue, blocks by class and harness-attributable blocks. The lead pastes the table into the epic report. Read it as a description with `n` shown, not as proof.

## Roles that read the harness

Before it builds, the Developer runs `check-harness --quick`. A failed `gate` check is not fixed inside the epic issue: block it with class `tooling`
and name the check ids, so the Architect refreshes the harness. A `cert` or `warning` failure does not stop the work. No new blocker class exists for the harness.

## Self-check before you report

- `check-harness --cert --run --write-cert` certified, and you ran it after your last edit.
- Every command in `AGENTS.md` ran, or the report says which did not and why.
- `AGENTS.md` holds Verify, Rules, Known pitfalls and Read when: no overview, no status, no copied command body, no copied ruling.
- Every Pitfall has evidence and every Rule says what enforces it.
- Every default you took is an assumption with an id.
