# Kit Việt upstream (vendored)

Source: https://github.com/mochistudio-vn/kitviet, licence MIT (see `LICENSE` in this folder).
Pinned commit: `faee4f14cd21672325a7f442b19c7239a5d7f152` (v1.4.0), recorded in `UPSTREAM.json`.
Fetched: 2026-11-10. The five P1 trades were added from the same commit on 2026-11-11; the ten P2 trades on 2026-11-11 (same commit).

What is here: the `claude-code/kv/skills` and `claude-code/kv/agents` folders of ten trades:
the five P0 trades (`phap-ly`, `ke-toan`, `kinh-doanh`, `marketing`, `hanh-chinh-van-phong`), the five P1 trades
(`nhan-su`, `bao-chi-truyen-thong`, `ban-hang-online`, `hoc-thuat-nghien-cuu`, `cong-nghe-thong-tin`) and the ten P2 trades
(`y-te`, `xay-dung`, `nha-hang-fnb`, `bao-hiem`, `tai-chinh-ngan-hang`, `bat-dong-san`, `du-lich-khach-san`, `xuat-nhap-khau`,
`san-xuat-nha-may`, `giao-duc`).
Items byte-identical in all twenty trades are stored once under `common/` (30 paths: seven agents, the base skills,
the three dropped skills and the export scripts); the rest sit under their trade folder. Nothing else is copied
(no `codex/`, `claude-web/`, `brand/`, zip archives or plugin metadata).

Changes: none to the vendored files. Every change Multica applies lives in the generator
(`scripts/generate-kitviet-pack.mjs`) and in `packages/core/squads/kitviet-rewrites.json` and
`packages/core/squads/kitviet-patches/`. To update, change `UPSTREAM.json`, re-vendor from the new commit,
and run `pnpm generate:kitviet`.

Verification: every vendored file equals its file at the pinned commit after LF normalisation (sha256), and the
vendored set is the commit's skills and agents of each trade minus the `common/` paths. For the P2 trades: 219 trade
files copied and checked (sha256 after LF normalisation), and the 30 `common/` paths checked present and identical in
each of the ten P2 trades (300 checks, 0 mismatches).
