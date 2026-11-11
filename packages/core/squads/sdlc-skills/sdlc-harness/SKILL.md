---
name: sdlc-harness
description: "Use when an SDLC issue is in the Harness status, when the squad lead audits or refreshes a repository's agent harness, or when a role must check the harness before it builds: write or repair AGENTS.md, CLAUDE.md, the Done command and the boundary table, and prove them with check-harness. Headless: you never interview anyone."
---

# SDLC harness

The harness is what an agent reads before it touches a repository: `AGENTS.md`, the `CLAUDE.md` that imports it, the
commands it names, and `docs/sdlc/environment.md`. A harness that is missing, stale or wrong sends every later run in
the wrong direction. You make it correct once, prove it with `check-harness`, and keep it current. Load sdlc-delivery-core first.

## You do not

- Change application code, tests, specs or requirements. You write instruction files and the small scripts or Makefile
  targets they name.
- Copy a command's body, a ruling's text, or project status into `AGENTS.md`. Point to the source (a script, a target, a
  ruling id) in one line.
- Add a gate that the criteria below do not name. Eight checks block; the rest warn.
- Interview anyone. This skill runs unattended (see "Headless").

## The checker

`check-harness` is a plain Node script in this skill: `scripts/check-harness.mjs`, resolved from this skill's installed
directory. Run it from the repository, no install needed:

```
node <this skill's folder>/scripts/check-harness.mjs            # full: H1-H13, JSON on stdout
node <this skill's folder>/scripts/check-harness.mjs --quick    # blocking checks only, under five seconds
node <this skill's folder>/scripts/check-harness.mjs --refresh  # adds what changed since the provenance sha
```

Exit code 0: no blocking check failed. 1: a blocking check failed (read `blocking` and each check's `details`). 2: bad usage.

| ID | Checks | Level |
| --- | --- | --- |
| H1 | `AGENTS.md` exists, within 200 lines and 12 KB; `CLAUDE.md` holds `@AGENTS.md` and at most one sentence | blocks |
| H2 | A `Verified <date> against <sha>` line, and the sha is a commit of this repository | blocks |
| H3 | Every path in a code span or link exists (URLs, globs, package names, generated and git-ignored paths are skipped) | blocks |
| H4 | Every `npm`/`pnpm`/`yarn` script and `make` target in a code span or fenced block exists | blocks |
| H5 | The toolchain version is pinned (`.nvmrc`, `.tool-versions`, `engines`) and CI uses the same major | warns |
| H6 | Exactly one `Done = ...` line (shape in the template) that names an existing command and an evidence level L1 to L3 | blocks |
| H7 | No progress wording in the rules: implemented, scaffold, TODO, WIP, checkmarks (a Greenfield section is exempt) | blocks |
| H8 | A boundary table with an `Enforced by` column and at least one row | blocks |
| H9 | CONTRIBUTING, a PR template, or a commit convention in `AGENTS.md` | warns |
| H10 | `docs/sdlc/environment.md` has an `Ensure command:` line, and `AGENTS.md` names that file and that command | blocks |
| H11 | `.env` and `.multica/` are ignored, `.env.example` exists, no secret pattern in tracked files | warns |
| H12 | Each Known pitfall cites a ruling, issue id, link or doc | warns |
| H13 | The must-read files total at most 60 KB, or a `Read when:` index exists | warns |

A false positive is fixed in the file, not by weakening a check: rewrite the line, or list a path that is generated
later in `docs/sdlc/harness.json` as `{ "ignorePaths": ["generated/"] }`.

If a blocking criterion cannot hold for a valid reason (the repo has no tests, so `Done` has nothing to run), record an
assumption in `docs/sdlc/assumptions.md` that names the check and the reason, state it in the report, and continue.
Lowering a criterion with a record is allowed. Stopping on it is not.

## Headless

There is no person to ask. Where the BMAD `bmad-project-context` skill would ask a question, take the recommended default
and record it as an assumption in `docs/sdlc/assumptions.md`. Use that skill's `references/best-practices.md` for what
belongs in the file (admit, exclude, retire, size, retrieval) and its ledger rule: removing an existing instruction needs
one of its four grounds, and an existing instruction is kept unless a ground applies. Read `references/agents-md-template.md`
here for the shape this squad checks, and the one `references/profile-*.md` that fits the repository.

## Mode 1: Harness stage (greenfield)

The issue is in the Harness status, after Environment. The environment runs and `docs/sdlc/environment.md` exists.

1. **Check first.** Run the full `check-harness`. If no blocking check fails, skip to step 7: write the report only.
2. **Choose the profile.** web, backend, baas or mobile from the manifests; a repository can have two. Read the profile files.
3. **Discover, do not guess.** Read `package.json`, `Makefile`, CI, compose files, `.env.example`, `docs/sdlc/environment.md`.
   The commands in `AGENTS.md` come from there. Run each command you will write down, and drop any that does not run.
4. **Write `AGENTS.md`** from the template: orientation, policy, where things are with the boundary table, running and
   verifying, conventions that differ from defaults, known pitfalls. Set the provenance line to today and the sha of `HEAD`.
   Keep an existing file's instructions (ledger rule), moving status text out to `docs/`. Make `CLAUDE.md` exactly `@AGENTS.md`.
5. **Close the gaps checks name.** Add the pin file, the `.gitignore` entries and the PR template or commit convention.
   If no single idempotent command rebuilds the environment, that is DevOps work: block with class `tooling`.
6. **Scratch and secrets.** Squad scratch and logs go in an ignored folder. Evidence worth keeping goes under `docs/`.
   Public and private variables are separate, in `.env.example` and in the profile's notes. Never write a secret value.
7. **Prove it.** Run `check-harness` again. Every blocking check passes. Then commit only your own paths.
8. **Report** with the check output (`blocking`, `warnings`), the profile, the assumptions you recorded, and what is
   unverified. When it passes, write `docs/sdlc/harness-cert.md`: date, sha, and the line `check-harness: pass`.

## Mode 2: Audit (maintenance)

On an existing repository, run `check-harness` and read the report; do not edit. The lead creates a Harness issue only
when a blocking check fails. Rank the failures by what they cost an agent: a wrong command first, a missing Done second.

## Mode 3: Refresh

A refresh issue comes from the lead after a retrospective, after a manifest changed since the provenance sha
(`check-harness --refresh` lists them), or after the second accepted `environment`, `tooling` or `rework-loop` ruling with
the same cause.

1. Run `check-harness --refresh`. Fix what `manifestsChanged` and `blocking` name, and nothing else.
2. A lesson that repeats goes into Known pitfalls as one line with its ruling id. The ruling stays in `docs/sdlc/rulings.md`;
   do not copy it.
3. Apply the ledger rule to every line you remove. Update the provenance line to today and `HEAD`.
4. Run `check-harness` and report as in Mode 1.

## Harness smoke

After the Harness stage, the lead creates one small issue for the Developer: a change confined to one boundary of the
repository that has a test, done by following `AGENTS.md` alone (set up the environment, quick check, Done command,
commit convention), asking nobody. The lead reads the result from the issue's comments, runs and status history.

| Measure | Target |
| --- | --- |
| Runs until the stage is done | 2 or fewer |
| Times the issue went to blocked | 0 |
| Times review or QA sent it back | 0 |
| Human comments needed | 0 |
| The Done command exits 0 on a clean tree | yes |

Targets are a hypothesis until several projects have been measured. A command the Developer had to discover that
`AGENTS.md` should have named becomes a line in `AGENTS.md` or a check. If the smoke misses, the Architect fixes the
named gap and the lead runs it once more. A second miss goes to `docs/sdlc/parked.md` and the epics continue.

## Roles that read the harness

Before it builds, the Developer runs `check-harness --quick`. A failed blocking check is not fixed inside the epic issue:
block it with class `tooling` and name the check ids, so the Architect refreshes the harness. No new blocker class exists
for the harness.

## Self-check before you report

- `check-harness` passes every blocking check, and you ran it after your last edit.
- Every command in `AGENTS.md` ran, or the report says which did not and why.
- `AGENTS.md` holds rules and pointers, not status, not a copied command body, not a copied ruling.
- The provenance line has today's date and the sha of `HEAD`.
- Every default you took is an assumption with an id.
