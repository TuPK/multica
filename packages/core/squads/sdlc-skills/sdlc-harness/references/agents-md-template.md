# AGENTS.md shape checked by check-harness

Fill every section from the repository. Delete a section with nothing in it, except Verify. Target 80 lines.
Rules and pointers only: no overview, no progress, no copied command bodies, no copied rulings.

````markdown
# <project name>

## Verify
<!-- needed by H10: names docs/sdlc/environment.md and the Ensure command that file holds -->
- Ensure: `<ensure command>`. Read `docs/sdlc/environment.md` when the app or a service will not start.
- Quick check: `<fast command>`
<!-- needed by H6: exactly one Done line, with the evidence level (sdlc-delivery-core, "Evidence ladder") -->
- Done = `<verify command>` exits 0 (evidence level L<1-3>)

## Rules
- Never <X>; do <Y>. Enforced by: <lint rule | test | CI job | hook | prose>

## Known pitfalls
- <What goes wrong, and what to do instead> (<ruling id, issue id, or command and exit code>)

## Read when
- Changing <area>: read `<path>`
````

Write nothing about the project itself. The agent reads the stack from the manifests and the structure from the code.

## What belongs in each section

| Section | Holds | Does not hold |
| --- | --- | --- |
| Verify | The three commands. Where it runs (URL, context) only if behaviour changes with it | Flags, script bodies, setup prose |
| Rules | A prohibition with its alternative, and what enforces it | Style a formatter or linter already enforces |
| Known pitfalls | A fact the agent cannot read from the code, that cost a run, with its evidence | Guesses, history, status |
| Read when | A condition and the document to read | A bare path, or a document the agent finds by itself |

## `docs/sdlc/environment.md` must hold one line, written by DevOps in the Environment stage

```
- Ensure command: `<the one idempotent command that checks health and brings up what is missing>`
```

## Wording that fails H7

`is implemented`, `not yet implemented`, `scaffold`, `TODO`, `WIP`, `coming soon`, checkmarks, and status table cells such as `| done |`.
"Must be implemented in Go" is a requirement and passes. Say what to do, not how far the project has come. Progress belongs in the issue
tracker and `docs/`. A section titled Greenfield is exempt, for a repository that really is empty.

## Empty repository

With no README and no architecture document, up to three lines of orientation are allowed, in a section titled Greenfield, pointing to the
document that will exist. Nothing else describes the project.

## Splitting

Add a nested `AGENTS.md` only when its rules are exclusive to that directory, large enough to matter, and the tool in use loads it
(check this once). Otherwise keep one file. Pointers that say when to read work in every tool; conditional loading by configuration does not.
