# Kit Việt upstream (vendored)

Source: https://github.com/mochistudio-vn/kitviet, licence MIT (see `LICENSE` in this folder).
Pinned commit: `faee4f14cd21672325a7f442b19c7239a5d7f152` (v1.4.0), recorded in `UPSTREAM.json`.
Fetched: 2026-11-10. The five trades below were added from the same commit on 2026-11-11.

What is here: the `claude-code/kv/skills` and `claude-code/kv/agents` folders of ten trades:
the five P0 trades (`phap-ly`, `ke-toan`, `kinh-doanh`, `marketing`, `hanh-chinh-van-phong`) and the five P1 trades
(`nhan-su`, `bao-chi-truyen-thong`, `ban-hang-online`, `hoc-thuat-nghien-cuu`, `cong-nghe-thong-tin`).
Items byte-identical in all ten trades are stored once under `common/` (30 paths: seven agents, the base skills,
the three dropped skills and the export scripts); the rest sit under their trade folder. Nothing else is copied
(no `codex/`, `claude-web/`, `brand/`, zip archives or plugin metadata). P2 trades are not vendored.

Changes: none to the vendored files. Every change Multica applies lives in the generator
(`scripts/generate-kitviet-pack.mjs`) and in `packages/core/squads/kitviet-rewrites.json` and
`packages/core/squads/kitviet-patches/`. To update, change `UPSTREAM.json`, re-vendor from the new commit,
and run `pnpm generate:kitviet`.

Verification: every vendored file equals its file at the pinned commit after LF normalisation (sha256), and the
vendored set is the commit's skills and agents of each trade minus the `common/` paths.
