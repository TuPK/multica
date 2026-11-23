# Fork branding: display text is Delegora, identifiers stay Multica

This fork ships under the **Delegora** name. The decision is deliberately narrow:
only text people read is renamed. Everything that identifies the code, the
protocol, or the upstream project keeps the `multica` name, so the fork can keep
merging from upstream `multica-ai/multica`.

## The rule

Change to **Delegora**:

- UI copy and i18n bundles (`packages/views/locales/**`, `apps/web/features/landing/i18n/**`, mobile copy)
- Page titles, metadata, emails, notifications, and error or CLI messages a user sees
- Docs prose and READMEs written for users (`README*.md`, `SELF_HOSTING*.md`, `apps/docs/content/**` prose)
- Capitalized product name in prose: `Multica` → `Delegora`

Keep as `multica` (do not rename):

- Go module path, npm scopes (`@multica/*`), and import paths
- Env vars (`MULTICA_*`), CLI binary and commands (`multica login`), config dirs (`.multica/`)
- HTTP headers and git trailers (`X-Multica-*`, `Multica-Workspace`, `Multica-Agent`)
- Database names, cookies, Docker image names, storage keys
- Domains and URLs (`multica.ai`, `github.com/multica-ai/multica`)
- Dev app names `Multica Canary` (dev builds, worktree envs, e2e fixtures) and the environment detection tests that match them.
- `LICENSE`, `NOTICE`, and upstream attribution lines, which name the upstream project on purpose. `NOTICE` and the `LICENSE` producer name stay as-is.
- Copyright holder lines ("© Multica", `Copyright 2026 Multica`).

## Desktop app identity (changed 2026-11-23)

The desktop app now uses `productName`/`package.json` `name` = **Delegora**, `app.setName("Delegora")`, and `StartupWMClass: Delegora` in `apps/desktop/electron-builder.yml`. Consequences:

- The installer file name and the Electron `userData` folder change (`…/Delegora` instead of `…/Multica`). Existing desktop users start with a fresh local data folder unless it is migrated. Check this before shipping a release.
- On upstream merges, keep these values as Delegora. Do not revert them to `Multica` to match upstream's `electron-builder.yml`.
- Historical migrations (`server/migrations/**`) and internal engineering notes (`docs/engineering/**`, `docs/specs/**`, `AGENTS.md`)

## Rules for future syncs and merges from upstream

1. When upstream changes a display string that is now `Multica`, keep the
   Delegora text. Resolve the conflict in favor of this fork's display text and
   keep upstream's logic, identifiers, and protocol changes.
2. Do not rename identifiers while resolving a merge to "match" the display text.
   A rename of `@multica/*`, `MULTICA_*`, or the Go module path is a separate,
   explicitly approved decision.
3. Any new user-facing copy added in this fork says **Delegora**, not `Multica`.
4. Tests that assert on display text use the Delegora text too.

## Why

The owner chose "display text only" on 2026-11-23. The alternative, renaming
identifiers, touches thousands of imports and breaks installed daemons and CLIs
that expect `MULTICA_*` variables and the `multica` binary. Keeping identifiers
is what makes upstream merges cheap.

Related: [apps/docs conventions](../apps/docs/content/docs/developers/conventions.mdx) for naming rules.
