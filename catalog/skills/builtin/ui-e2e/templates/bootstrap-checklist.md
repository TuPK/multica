# Bootstrap checklist (phase 0): run the full stack for testing

Work through each item. Record the answer and the evidence (command output, file path). Items marked (ask) need the user only when no fake or generated value can replace them.

## 1. Project instructions
- [ ] README, CONTRIBUTING, AGENTS.md / CLAUDE.md, docs/ read; rules about environments, secrets, ports, worktrees noted
- [ ] Makefile, package.json scripts, task runners, CI workflows read: note every documented start, build, migrate and seed command

## 2. Stack map
| Part | Language and version | Source of truth | Start command found | Needs |
| --- | --- | --- | --- | --- |
| backend | | | | database, cache, env |
| frontend | | | | backend URL |
| worker or daemon | | | | queue, database |
| database | engine and version | migration tool and folder | | |
| cache or queue | | | | |
| external providers | | | | fake or real (prefer fake) |

## 3. Present or missing
- [ ] Toolchain versions match the project's declared versions (`.nvmrc`, `.tool-versions`, `go.mod`, `engines`)
- [ ] Dependencies installed (`node_modules`, module cache) or the install command is known
- [ ] Env file: `.env.example` (or equivalent) read; a test env file is generated with random secrets, never copied from another checkout
- [ ] Database reachable; migrations applied; seed data for at least one user per role
- [ ] Docker responds (`docker version`); required images exist or can be pulled
- [ ] Ports free for the test run; no stale server process from an earlier run

## 4. Chosen path
- [ ] A. The project has a start command: it is used; only test data and the test database are added
- [ ] B. Pieces exist, nothing wires them: `e2e-up` and `e2e-down` written (see `e2e-up.sh`)
- [ ] C. Nothing exists: toolchain installed as declared; compose file or Dockerfile added only where the project has none

## 5. Fakes for external services
- [ ] Each external provider listed with its fake or disabled mode and the config flag that selects it
- [ ] No real provider is called by the test run

## 6. Verification (must all pass before tests are written)
- [ ] Backend health endpoint answers 200
- [ ] Frontend first page loads in a browser (read the title or a heading)
- [ ] A seeded user signs in through the UI
- [ ] Worker or daemon, if any, reports ready
- [ ] Startup logs have no errors

## 7. Record
- [ ] Commands, env variable names (no values), ports, seed data and known issues written to the setup doc
- [ ] `e2e-down` stops everything and leaves the development data untouched
