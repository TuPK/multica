# Platform management: licence record (G-LIC, spec U10)

Scope: every module the R1 to R5 platform slices added to `server/go.mod` and `platform-management/go.mod`, and every direct npm dependency the staff console added to `platform-management/console/package.json`.

Method:
- Go: versions are the pins now in `go.mod`. Each licence was read from the module's own `LICENSE` file in the local module cache (`go mod download -json`, which did not change `go.mod` or `go.sum`). The licence type was then matched by its text.
- npm: versions are the lockfile pins. Each licence was read from `node_modules/<pkg>/package.json` in the existing install. No `pnpm install` was run.
- Allowed set: MIT, Apache-2.0, BSD (2- or 3-Clause) and ISC. Anything else is an open issue.

This record checks licence types only. It is not the commercial licence sign-off that G-LIC asks for in the plan.

## Go modules

| Module | Version | Licence | Proof (source file) | Added by | Used in |
| --- | --- | --- | --- | --- | --- |
| github.com/coreos/go-oidc/v3 | v3.21.0 | Apache-2.0 | `~/go/pkg/mod/github.com/coreos/go-oidc/v3@v3.21.0/LICENSE` | a5b5c6c78 (R3, OIDC) | server |
| golang.org/x/oauth2 | v0.38.0 | BSD-3-Clause | `~/go/pkg/mod/golang.org/x/oauth2@v0.38.0/LICENSE` | a5b5c6c78 (R3, OIDC) | server |
| github.com/go-jose/go-jose/v4 | v4.1.4 | Apache-2.0 | `~/go/pkg/mod/github.com/go-jose/go-jose/v4@v4.1.4/LICENSE` | a5b5c6c78 (R3, OIDC, indirect) | server |
| github.com/pquerna/otp | v1.5.0 | Apache-2.0 | `~/go/pkg/mod/github.com/pquerna/otp@v1.5.0/LICENSE` | 6f7399032 (R1/R2, TOTP) | server |
| github.com/boombuler/barcode | v1.0.1-0.20190219062509-6c824513bacc | MIT | `~/go/pkg/mod/github.com/boombuler/barcode@v1.0.1-0.20190219062509-6c824513bacc/LICENSE` | 6f7399032 (TOTP, indirect) | server |
| github.com/jackc/pgx/v5 | v5.11.0 | MIT | `~/go/pkg/mod/github.com/jackc/pgx/v5@v5.11.0/LICENSE` | 0ae5d5c62 (platform skeleton); already in server | server, platform-management |
| github.com/redis/go-redis/v9 | v9.22.0 | BSD-2-Clause | `~/go/pkg/mod/github.com/redis/go-redis/v9@v9.22.0/LICENSE` | 0ae5d5c62 (platform skeleton); already in server | server, platform-management |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT | `~/go/pkg/mod/github.com/cespare/xxhash/v2@v2.3.0/LICENSE.txt` | 0ae5d5c62 (indirect) | server, platform-management |
| github.com/jackc/pgpassfile | v1.0.0 | MIT | `~/go/pkg/mod/github.com/jackc/pgpassfile@v1.0.0/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | MIT | `~/go/pkg/mod/github.com/jackc/pgservicefile@v0.0.0-20240606120523-5a60cdf6a761/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT | `~/go/pkg/mod/github.com/jackc/puddle/v2@v2.2.2/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |
| go.uber.org/atomic | v1.11.0 | MIT | `~/go/pkg/mod/go.uber.org/atomic@v1.11.0/LICENSE.txt` | 0ae5d5c62 (indirect) | server, platform-management |
| golang.org/x/sync | v0.24.0 | BSD-3-Clause | `~/go/pkg/mod/golang.org/x/sync@v0.24.0/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause | `~/go/pkg/mod/golang.org/x/sys@v0.49.0/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |
| golang.org/x/text | v0.43.0 | BSD-3-Clause | `~/go/pkg/mod/golang.org/x/text@v0.43.0/LICENSE` | 0ae5d5c62 (indirect) | server, platform-management |

`~` is the Go module cache root, `$(go env GOMODCACHE)`, which is `/home/av-server/go/pkg/mod` on this machine.

The `Added by` column gives the commit that first put the module in a `go.mod`. Rows marked "already in server" were in `server/go.mod` before the platform skeleton; they are listed again because `platform-management/go.mod` now pins them too.

## npm packages (platform-management/console)

| Package | Version | Licence | Proof (source file) | Added by |
| --- | --- | --- | --- | --- |
| shadcn | 4.1.0 | MIT | `platform-management/console/node_modules/shadcn/package.json` | ac0ce0a17 (console) |
| tw-animate-css | 1.4.0 | MIT | `platform-management/console/node_modules/tw-animate-css/package.json` | ac0ce0a17 (console) |
| vite | 8.0.1 | MIT | `platform-management/console/node_modules/vite/package.json` | ac0ce0a17 (console, devDependency) |

The other console entries in its `package.json` (`@multica/*`, `react`, `zod`, `@tanstack/react-query`, testing and tooling packages) use `catalog:` or `workspace:` and were already in the repo before this slice. They are not recorded here.

## Open issues

- No direct dependency in this record has a licence outside MIT, Apache-2.0, BSD or ISC. No owner decision is needed for these rows.
- **Not covered: transitive npm packages.** This record lists direct npm dependencies only. A workspace-wide `pnpm licenses list` (run without a filter, so it is not limited to the console tree) reports some non-permissive licences in the lockfile: MPL-2.0 (`lightningcss`, `lightningcss-linux-x64-gnu`), LGPL-3.0-or-later (`@img/sharp-libvips-linux-x64`), OFL-1.1 (`@fontsource-variable/*`), BlueOak-1.0.0 (14 packages), CC-BY-4.0 (`caniuse-lite`), and a few with no licence string (`khroma` reports `Unknown`). It is not yet known which of these are in the console's shipped tree. A follow-up should scope them to the console and the Go binaries. If any ships, it needs an owner decision.
- The licence tool named in the plan was not installed (`go-licenses` and `licensee` are not on the path), so this record was done by hand from the files above.
