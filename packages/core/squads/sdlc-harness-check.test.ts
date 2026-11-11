// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const script = fileURLToPath(new URL("./sdlc-skills/sdlc-harness/scripts/check-harness.mjs", import.meta.url));

interface Check {
  id: string;
  level: "blocking" | "warning";
  status: "pass" | "fail" | "skipped";
  details: string[];
}
interface Report {
  ok: boolean;
  mode: string;
  blocking: string[];
  warnings: string[];
  checks: Check[];
  refresh?: { provenanceSha: string | null; manifestsChanged: string[]; needsRefresh: boolean; reasons: string[] };
}

const BLOCKING = ["H1", "H2", "H3", "H4", "H6", "H7", "H8", "H10"];

const dirs: string[] = [];
afterEach(() => {
  for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

function git(dir: string, ...args: string[]) {
  return execFileSync("git", ["-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false", ...args], {
    encoding: "utf8",
  }).trim();
}

function put(dir: string, files: Record<string, string>) {
  for (const [rel, text] of Object.entries(files)) {
    mkdirSync(dirname(join(dir, rel)), { recursive: true });
    writeFileSync(join(dir, rel), text);
  }
}

const AGENTS = (sha: string) => `# Demo

<!-- Verified 2026-11-11 against ${sha}. -->

Demo is a small web app. Deep docs live in \`docs/\`.

## Policy
- Use the API client in \`src/lib/api.ts\`; never call fetch from a component.

## Where things are
| Path | Responsibility | Forbidden | Enforced by |
| --- | --- | --- | --- |
| \`src/lib/\` | API client | UI imports | lint rule |
| \`src/pages/\` | Screens | direct fetch | prose |

Environment and squad rulings: \`docs/sdlc/environment.md\` (read when you run the app).

## Running and verifying
- Environment: \`pnpm env:ensure\` before you open a browser or run integration tests.
- Quick check: \`pnpm test\`
- Done = \`pnpm verify\` exits 0 (evidence level L3)
- Build: \`make build\`; use \`pnpm install\` once, and make sure the lockfile is current.

## Known pitfalls
- A stale session returns 401 (TUPK-182).
`;

const BASE_FILES: Record<string, string> = {
  "CLAUDE.md": "@AGENTS.md\n",
  "package.json": JSON.stringify({ name: "demo", scripts: { test: "vitest run", verify: "pnpm test", "env:ensure": "sh scripts/env.sh" }, engines: { node: ">=22" } }),
  Makefile: "build:\n\techo build\n\n.PHONY: build\n",
  "src/lib/api.ts": "export {};\n",
  "src/pages/home.ts": "export {};\n",
  "docs/sdlc/environment.md": "# Environment\n\n- Ensure command: `pnpm env:ensure`\n",
  "CONTRIBUTING.md": "Conventional commits.\n",
  ".gitignore": ".env\n.multica/\nnode_modules/\n",
  ".env.example": "API_URL=\n",
};

/** A repository that passes every check. `agents` and `extra` override files. */
function fixture(options: { agents?: (sha: string) => string; extra?: Record<string, string> } = {}) {
  const dir = mkdtempSync(join(tmpdir(), "harness-"));
  dirs.push(dir);
  git(dir, "init", "-q");
  put(dir, BASE_FILES);
  put(dir, { "AGENTS.md": "# placeholder\n" });
  git(dir, "add", "-A");
  git(dir, "commit", "-q", "-m", "init");
  const sha = git(dir, "rev-parse", "--short", "HEAD");
  put(dir, { "AGENTS.md": (options.agents ?? AGENTS)(sha), ...options.extra });
  return { dir, sha };
}

function run(dir: string, ...flags: string[]) {
  const res = spawnSync("node", [script, "--root", dir, ...flags], { encoding: "utf8" });
  return { code: res.status, report: JSON.parse(res.stdout) as Report, stderr: res.stderr };
}

const check = (r: Report, id: string) => r.checks.find((c) => c.id === id)!;

describe("check-harness", () => {
  it("passes a complete harness, prints JSON, and exits 0", () => {
    const { dir } = fixture();
    const { code, report } = run(dir);
    expect(report.checks.map((c) => c.id)).toEqual(["H1", "H2", "H3", "H4", "H5", "H6", "H7", "H8", "H9", "H10", "H11", "H12", "H13"]);
    expect(report.checks.filter((c) => c.status === "fail").map((c) => `${c.id}: ${c.details.join("; ")}`)).toEqual([]);
    expect(report.ok).toBe(true);
    expect(code).toBe(0);
  });

  it("blocks on exactly the eight criteria of the proposal", () => {
    const { dir } = fixture();
    const { report } = run(dir);
    expect(report.checks.filter((c) => c.level === "blocking").map((c) => c.id)).toEqual(BLOCKING);
  });

  it("fails every blocking check and exits 1 on an empty repository", () => {
    const dir = mkdtempSync(join(tmpdir(), "harness-"));
    dirs.push(dir);
    const { code, report } = run(dir);
    expect(report.ok).toBe(false);
    expect(report.blocking).toEqual(BLOCKING);
    expect(code).toBe(1);
  });

  it("--quick runs only the blocking checks", () => {
    const { dir } = fixture({ extra: { ".gitignore": "" } });
    const { report } = run(dir, "--quick");
    expect(report.mode).toBe("quick");
    for (const c of report.checks) expect(c.status === "skipped", c.id).toBe(c.level === "warning");
    expect(report.warnings).toEqual([]);
  });

  describe("H1", () => {
    it("fails when CLAUDE.md does not import AGENTS.md", () => {
      const { dir } = fixture({ extra: { "CLAUDE.md": "Read AGENTS.md\n" } });
      expect(check(run(dir).report, "H1").status).toBe("fail");
    });
    it("accepts @AGENTS.md plus one sentence, rejects more", () => {
      const ok = fixture({ extra: { "CLAUDE.md": "@AGENTS.md\nThe rules live there.\n" } });
      expect(check(run(ok.dir).report, "H1").status).toBe("pass");
      const bad = fixture({ extra: { "CLAUDE.md": "@AGENTS.md\nOne.\nTwo.\n" } });
      expect(check(run(bad.dir).report, "H1").status).toBe("fail");
    });
    it("fails above 200 lines", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}${"- filler\n".repeat(200)}` });
      expect(check(run(dir).report, "H1").details.join()).toContain("lines");
    });
  });

  describe("H2", () => {
    it("fails without a provenance line", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace(/<!--.*-->\n/, "") });
      expect(check(run(dir).report, "H2").status).toBe("fail");
    });
    it("fails when the sha is not in git", () => {
      const { dir } = fixture({ agents: () => AGENTS("deadbeefcafe") });
      expect(check(run(dir).report, "H2").details.join()).toContain("not a commit");
    });
  });

  describe("H3", () => {
    it("fails on a path that does not exist and names it", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\nSee \`src/missing/thing.ts\`.\n` });
      expect(check(run(dir).report, "H3").details).toEqual(["path does not exist: src/missing/thing.ts"]);
    });
    it("fails on a missing link target but ignores URLs, anchors, globs, package specifiers, generated and git-ignored paths", () => {
      const noise = [
        "[site](https://example.com/a/b.md) [top](#policy) `src/**/*.ts` `react/jsx-runtime` `dist/app.js`",
        "`node_modules/x/y.js` `.multica/runs/a.md` `http://localhost:3000/x` `GET /api/items` `@scope/pkg`",
      ].join("\n");
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n${noise}\n[gone](docs/gone.md)\n` });
      expect(check(run(dir).report, "H3").details).toEqual(["path does not exist: docs/gone.md"]);
    });
    it("skips paths in fenced blocks and honours docs/sdlc/harness.json ignorePaths", () => {
      const { dir } = fixture({
        agents: (sha) => `${AGENTS(sha)}\n\`\`\`\ntree\n  \`src/in-fence.ts\`\n\`\`\`\n\`generated/out.json\`\n`,
        extra: { "docs/sdlc/harness.json": JSON.stringify({ ignorePaths: ["generated/"] }) },
      });
      expect(check(run(dir).report, "H3").status).toBe("pass");
    });
  });

  describe("H4", () => {
    it("fails on a missing script and a missing make target", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\nRun \`pnpm nope\`, \`npm run ghost\` and \`make deploy\`.\n` });
      expect(check(run(dir).report, "H4").details).toEqual([
        "`pnpm nope`: no such script",
        "`npm run ghost`: no such script",
        "`make deploy`: no such target",
      ]);
    });
    it("does not judge package-manager built-ins, filtered or placeholder commands, or the word make in prose", () => {
      const lines = "`pnpm install` `pnpm --filter @demo/web test` `pnpm exec vitest` `npm run <script>` `make -n` Please make sure it works.";
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n${lines}\n` });
      expect(check(run(dir).report, "H4").status).toBe("pass");
    });
    it("reads commands in fenced blocks and follows cd to the nested package.json", () => {
      const fence = "```bash\ncd apps/web && pnpm dev\npnpm verify\n```\n";
      const { dir } = fixture({
        agents: (sha) => `${AGENTS(sha)}\n${fence}`,
        extra: { "apps/web/package.json": JSON.stringify({ scripts: { dev: "vite" } }) },
      });
      expect(check(run(dir).report, "H4").status).toBe("pass");
      const bad = fixture({ agents: (sha) => `${AGENTS(sha)}\n\`\`\`bash\ncd apps/web && pnpm dev\n\`\`\`\n` });
      expect(check(run(bad.dir).report, "H4").status).toBe("fail");
    });
    it("fails npm test when package.json has no test script", () => {
      const { dir } = fixture({
        agents: (sha) => `${AGENTS(sha)}\nRun \`npm test\`.\n`,
        extra: { "package.json": JSON.stringify({ scripts: { verify: "x", "env:ensure": "x" } }) },
      });
      expect(check(run(dir).report, "H4").details).toContain("`npm test`: no such script");
    });
  });

  describe("H6", () => {
    it("fails with no Done line", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace(/^- Done.*\n/m, "") });
      expect(check(run(dir).report, "H6").status).toBe("fail");
    });
    it("fails with two Done lines", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n- Done = \`pnpm test\` exits 0 (L3)\n` });
      expect(check(run(dir).report, "H6").details.join()).toContain("exactly one");
    });
    it("fails without an evidence level and when the command does not exist", () => {
      const noLevel = fixture({ agents: (sha) => AGENTS(sha).replace(" (evidence level L3)", "") });
      expect(check(run(noLevel.dir).report, "H6").details.join()).toContain("evidence level");
      const ghost = fixture({ agents: (sha) => AGENTS(sha).replace("`pnpm verify`", "`pnpm ghost`") });
      expect(check(run(ghost.dir).report, "H6").details.join()).toContain("does not exist");
    });
    it("accepts a command that is not a package-manager command", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace("`pnpm verify`", "`go test ./...`").replace("L3", "L2") });
      expect(check(run(dir).report, "H6").status).toBe("pass");
    });
  });

  describe("H7", () => {
    it("fails on status wording in a rule and names the line", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n## Status\n- Auth is implemented.\n- ✅ Checkout\n- TODO: add search\n` });
      const details = check(run(dir).report, "H7").details;
      expect(details).toHaveLength(3);
      expect(details[0]).toContain("Auth is implemented");
    });
    it("does not flag the Done line, fenced code, or a Greenfield section", () => {
      const agents = (sha: string) =>
        `${AGENTS(sha)}\n\`\`\`\nTODO in a code sample\n\`\`\`\n\n## Greenfield notes\n- Nothing is implemented yet; TODO: scaffold.\n\n## After\n- Keep it small.\n`;
      const { dir } = fixture({ agents });
      expect(check(run(dir).report, "H7").status).toBe("pass");
    });
  });

  describe("H8", () => {
    it("fails without an enforced-by column", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace("| Forbidden | Enforced by |", "| Forbidden | Notes |") });
      expect(check(run(dir).report, "H8").status).toBe("fail");
    });
    it("fails for a header with no rows", () => {
      const { dir } = fixture({ agents: (sha) => `# T\n\n<!-- Verified 2026-11-11 against ${sha}. -->\n\n| Path | Enforced by |\n| --- | --- |\n` });
      expect(check(run(dir).report, "H8").status).toBe("fail");
    });
  });

  describe("H10", () => {
    it("fails without docs/sdlc/environment.md", () => {
      const { dir } = fixture();
      rmSync(join(dir, "docs/sdlc/environment.md"));
      expect(check(run(dir).report, "H10").status).toBe("fail");
    });
    it("fails without the Ensure command line", () => {
      const { dir } = fixture({ extra: { "docs/sdlc/environment.md": "# Environment\n\nrun pnpm env:ensure\n" } });
      expect(check(run(dir).report, "H10").details.join()).toContain("Ensure command");
    });
    it("fails when AGENTS.md does not name the command or the file", () => {
      const { dir } = fixture({ extra: { "docs/sdlc/environment.md": "- Ensure command: `pnpm other`\n" } });
      expect(check(run(dir).report, "H10").details.join()).toContain("does not name the ensure command");
      const noPointer = fixture({ agents: (sha) => AGENTS(sha).replace("`docs/sdlc/environment.md`", "the environment doc") });
      expect(check(run(noPointer.dir).report, "H10").details.join()).toContain("does not point to");
    });
  });

  describe("warnings never block", () => {
    it("H5 fails without a version pin, and on a CI node version that differs from .nvmrc", () => {
      const none = fixture({ extra: { "package.json": JSON.stringify({ scripts: { test: "x", verify: "x", "env:ensure": "x" } }) } });
      expect(check(run(none.dir).report, "H5").status).toBe("fail");
      const drift = fixture({
        extra: { ".nvmrc": "22\n", ".github/workflows/ci.yml": "steps:\n  - with:\n      node-version: 20\n" },
      });
      expect(check(run(drift.dir).report, "H5").details.join()).toContain("Node 20");
    });
    it("H9 fails with no contributing docs and no commit convention", () => {
      const { dir } = fixture();
      rmSync(join(dir, "CONTRIBUTING.md"));
      expect(check(run(dir).report, "H9").status).toBe("fail");
    });
    it("H11 flags an un-ignored .env, a missing .env.example, and a tracked secret", () => {
      const { dir } = fixture({ extra: { ".gitignore": "", "src/key.ts": 'export const k = "AKIAABCDEFGHIJKLMNOP";\n' } });
      git(dir, "add", "src/key.ts");
      rmSync(join(dir, ".env.example"));
      const details = check(run(dir).report, "H11").details.join("\n");
      expect(details).toContain(".env is not ignored");
      expect(details).toContain(".env.example does not exist");
      expect(details).toContain("src/key.ts");
    });
    it("H12 flags a pitfall without an evidence reference", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}- The cache is flaky.\n` });
      expect(check(run(dir).report, "H12").details).toEqual(["pitfall without an evidence reference: - The cache is flaky."]);
    });
    it("H13 flags a must-read set above the budget unless a task index exists", () => {
      const big = "x".repeat(61 * 1024);
      const agents = (sha: string) => `${AGENTS(sha)}\nMust read: \`docs/big.md\`\n`;
      const over = fixture({ agents, extra: { "docs/big.md": big } });
      expect(check(run(over.dir).report, "H13").status).toBe("fail");
      const indexed = fixture({ agents: (sha) => `${agents(sha)}Read when: you touch billing.\n`, extra: { "docs/big.md": big } });
      expect(check(run(indexed.dir).report, "H13").status).toBe("pass");
    });
    it("a warning alone leaves ok true and the exit code 0", () => {
      const { dir } = fixture();
      rmSync(join(dir, "CONTRIBUTING.md"));
      const { code, report } = run(dir);
      expect(report.warnings).toEqual(["H9"]);
      expect(report.ok).toBe(true);
      expect(code).toBe(0);
    });
  });

  describe("--refresh", () => {
    it("reports no refresh needed on a clean harness at its provenance commit", () => {
      const { dir, sha } = fixture();
      const { refresh } = run(dir, "--refresh").report;
      expect(refresh).toMatchObject({ provenanceSha: sha, manifestsChanged: [], needsRefresh: false });
    });
    it("lists the manifests that changed since the provenance sha", () => {
      const { dir } = fixture();
      put(dir, { "package.json": JSON.stringify({ scripts: { test: "x", verify: "x", "env:ensure": "x" } }), "src/other.ts": "export {};\n" });
      git(dir, "add", "-A");
      git(dir, "commit", "-q", "-m", "change manifest");
      const { refresh } = run(dir, "--refresh").report;
      expect(refresh!.manifestsChanged).toContain("package.json");
      expect(refresh!.manifestsChanged).not.toContain("src/other.ts");
      expect(refresh!.needsRefresh).toBe(true);
    });
  });

  it("exits 2 on a bad argument or a missing directory", () => {
    expect(spawnSync("node", [script, "--bogus"], { encoding: "utf8" }).status).toBe(2);
    expect(spawnSync("node", [script, "--root", "/nonexistent-harness-dir"], { encoding: "utf8" }).status).toBe(2);
  });
});
