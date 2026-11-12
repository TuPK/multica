# Kit Việt upstream (vendored)

Source: https://github.com/mochistudio-vn/kitviet, licence MIT (see `LICENSE` in this folder).
Pinned commit: `505a472685c85ecefd4eb127ff016b988226ad05` (v1.5.0), recorded in `UPSTREAM.json`.
Fetched: 2026-11-12. Written by `scripts/kitviet-update.mjs`; do not edit by hand.

What is here: the `claude-code/kv/skills` and `claude-code/kv/agents` folders of the 20 trades listed in
`UPSTREAM.json`. Items byte-identical in all trades are stored once under `common/` (30 paths); the rest
sit under their trade folder (520 files in total). Nothing else is copied (no `codex/`, `claude-web/`,
`brand/`, zip archives or plugin metadata). Text files are stored with LF line endings.

Changes: none to the vendored files. Every change Multica applies lives in the generator
(`scripts/generate-kitviet-pack.mjs`), in `packages/core/squads/kitviet-rewrites.json`, in
`packages/core/squads/kitviet-patches/` and in the squad configs.

Verification: every vendored file equals its file at the pinned commit after LF normalisation (byte comparison of the
written file against the upstream file). Mismatches: 0.
