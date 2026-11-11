# AGENTS.md shape checked by check-harness

Fill every section from the repository. Delete a section with nothing in it, except those `check-harness` needs (marked).
At most 150 lines. Rules and pointers only: no progress, no copied command bodies, no copied rulings.

````markdown
<!-- harness:v1 -->
<!-- Verified YYYY-MM-DD against <sha of HEAD>. Refresh: sdlc-harness skill, Mode 3. -->

## <project name>

<Three or four sentences: what it is, the stack, where the deeper docs live.>

## Policy

- <A rule the organisation requires, with the alternative: "Never X; do Y".>

## Where things are

<!-- needed by H8: a table with an "Enforced by" column and at least one row -->
| Path | Responsibility | Forbidden | Enforced by |
| --- | --- | --- | --- |
| `<dir>/` | <what lives here> | <what must not happen here> | lint rule / test / prose |

- Environment and squad rulings: `docs/sdlc/environment.md`, `docs/sdlc/rulings.md`. Read when: you run the app or hit an environment error.

## Running and verifying

<!-- needed by H10: names docs/sdlc/environment.md (above) and the Ensure command that file holds -->
- Environment: `<ensure command>` before you open a browser or run an integration test.
- Quick check while editing: `<command>`
<!-- needed by H6: exactly one Done line, with the evidence level (sdlc-delivery-core, "Evidence ladder") -->
- Done = `<verify command>` exits 0 (evidence level L<1-3>)
- Run it at: <URL and the context that matters, for example HTTP versus HTTPS>
- Versions: read from `.nvmrc` / `engines`.

## Conventions that differ from defaults

- <Convention> (`<path that shows it>`)

## Known pitfalls

- <What goes wrong and what to do instead> (<ruling or issue id>)
````

## `docs/sdlc/environment.md` must hold one line, written by DevOps in the Environment stage

```
- Ensure command: `<the one idempotent command that checks health and brings up what is missing>`
```

## Wording that fails H7

`implemented`, `not yet implemented`, `scaffold`, `TODO`, `WIP`, `coming soon`, checkmarks, and status table cells such as
`| done |`. Say what to do, not how far the project has come. Progress belongs in the issue tracker and `docs/`.
A section titled Greenfield is exempt, for a repository that really is empty.

## Splitting

Add a nested `AGENTS.md` only when its rules are exclusive to that directory, large enough to matter, and the tool in use
loads it (check this once). Otherwise keep one file.
