# Catalog

The content that changes often and grows over time: skills, squads, agent instruction templates,
workflows, the starter company, agent packs. The product code reads it; it does not define it.
Design and rules: [docs/specs/template-registry/spec.md](../docs/specs/template-registry/spec.md).

Do not confuse it with `deploy/helm/multica/templates/` or `.github/ISSUE_TEMPLATE/`.

## What is here

| Folder | Holds |
| --- | --- |
| `collections/<id>/` | The groups of the squad quickstart (core, kitviet, vertical). Title in `i18n/` |
| `skillsets/<id>/` | Named sets of skills (`sdlc`, `planning`, ...). Members are the skills whose `sets` name it |
| `skills/<group>/<id>/` | A skill: `SKILL.md`, its reference files, and `template.json` (`sets`, `source`). The group folder only sorts them |
| `squads/<id>/` | A quickstart squad: `template.json`, `instructions.md`, `i18n/`, and `roles/<role>.md` for a role that writes its own agent instructions |
| `workflows/<id>/` | Workflow templates (SDLC Auto, planning). The web client and the Go server read the same file. `designer_<id>` holds a starting point of the workflow designer (read by `server/internal/issueworkflow`); a squad cannot run it |
| `agents/<id>/` | Agent instruction templates. `agents/_blocks/*.md` are shared text, used as `{{block:name}}` |
| `companies/starter/` | The starter company: floors, squad cards, stage owners |
| `packs/<id>/` | Agent packs (`pack.json`, instructions, skills), installable from the UI or `make agent-pack` |
| `bundles/quickstart/` | What `multica quickstart` installs on a machine |
| `_sources/kitviet/` | Inputs of the Kit Viet generator (vendored upstream, patches, squad configs). Not templates |

## Everyday work

```
pnpm catalog:new squad my_squad --collection core   # a squad that is valid from the start
pnpm catalog:new skill my-skill --set sdlc          # a skill under skills/custom/
pnpm catalog:check                                  # validate; also runs in CI
pnpm catalog:build                                  # write the generated code
pnpm catalog:list
```

Adding a squad, skill, skillset or collection edits nothing outside `catalog/`.

## Rules

- **Generated code is not committed.** `pnpm catalog:build` writes `packages/core/catalog/generated/` and
  `server/internal/catalog/generated/`. `pnpm typecheck`, `test`, `lint`, `build`, `dev` (through turbo), `make build`,
  `make test`, `make server` and the Docker builds run it for you. Run it yourself before calling `vitest`,
  `tsc` or `go test` directly.
- **Ids never change.** They are stored in workspaces (`squad.template_key`, skill, agent and workflow names).
  To rename, add a new template and deprecate the old one (`"status": "deprecated"`). To remove one for good,
  list `kind:id` under `removed` in `registry.config.json`.
- **Every language is required.** A template with a title (collection, squad, pack) needs `i18n/<locale>.json` for
  every locale in `packages/core/i18n/types.ts`. There is no fallback to English.
- **Skill folders are installed as they are.** `template.json` and `i18n/` are never part of an installed skill.
- A vendored skill names its `source` (`repo`, `ref`, `license`).
- Bump `version` when you change a template's content.
- A template the code finds by name or id declares `contracts` (e.g. `squad.sdlc`). The server lists what it needs in
  `server/internal/catalog/contracts.go` and the client in `packages/core/catalog/contracts.ts`; the tests fail when
  such a template is removed, renamed or no longer `stable`.
