# Kit Việt upstream (vendored)

Source: https://github.com/mochistudio-vn/kitviet, licence MIT (see `LICENSE` in this folder).
Pinned commit: `faee4f14cd21672325a7f442b19c7239a5d7f152` (v1.4.0), recorded in `UPSTREAM.json`.
Fetched: 2026-11-10.

What is here: the `claude-code/kv/skills` and `claude-code/kv/agents` folders of the five P0 trades
(`phap-ly`, `ke-toan`, `kinh-doanh`, `marketing`, `hanh-chinh-van-phong`). Items byte-identical in all five
trades are stored once under `common/`; the rest sit under their trade folder. Nothing else is copied
(no `codex/`, `claude-web/`, `brand/`, zip archives or plugin metadata).

Changes: none to the vendored files. Every change Multica applies lives in the generator
(`scripts/generate-kitviet-pack.mjs`) and in `packages/core/squads/kitviet-rewrites.json` and
`packages/core/squads/kitviet-patches/`. To update, change `UPSTREAM.json`, re-vendor from the new commit,
and run `pnpm generate:kitviet`.
