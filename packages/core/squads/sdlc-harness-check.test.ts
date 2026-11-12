// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const script = fileURLToPath(new URL("./sdlc-skills/sdlc-harness/scripts/check-harness.mjs", import.meta.url));

interface Check {
  id: string;
  level: "gate" | "cert" | "warning";
  status: "pass" | "fail" | "skipped";
  details: string[];
}
interface Report {
  ok: boolean;
  certified: boolean;
  mode: string;
  blocking: string[];
  cert: string[];
  accepted: string[];
  footprint: { lines: number; bytes: number; instructions: number; proseRules: number };
  warnings: string[];
  checks: Check[];
  executed?: { ensure?: { exit: number | null }; verify?: { exit: number | null; treeClean?: boolean } };
  refresh?: { certSha: string | null; staleDays: number | null; manifestsChanged: string[]; needsRefresh: boolean; reasons: string[] };
}

const GATE = ["H1", "H3", "H4"];
const CERT = ["H6", "H7", "H10", "H14"];

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

## Verify
- Ensure: \`pnpm env:ensure\`. Read \`docs/sdlc/environment.md\` when the app will not start.
- Quick check: \`pnpm test\`
- Done = \`pnpm verify\` exits 0 (evidence level L3)

## Rules
- Never call fetch from a component; use the client in \`src/lib/api.ts\`. Enforced by: \`pnpm test\`
- Screens stay in \`src/pages/\`. Enforced by: prose

## Known pitfalls
- A stale session returns 401 (TUPK-182).

## Read when
- Changing the API client: read \`src/lib/api.ts\`.
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

interface RunnableOptions {
  env?: string;
  verify?: string;
  commit?: boolean;
}
/** A harness whose Ensure and Done commands are small shell scripts, committed so the tree is clean. */
function runnable({ env = "exit 0\n", verify = "exit 0\n", commit = true }: RunnableOptions = {}) {
  const f = fixture({
    agents: (sha) => AGENTS(sha).replaceAll("`pnpm env:ensure`", "`sh scripts/env.sh`").replace("`pnpm verify`", "`sh scripts/verify.sh`"),
    extra: {
      "scripts/env.sh": env,
      "scripts/verify.sh": verify,
      "docs/sdlc/environment.md": "- Ensure command: `sh scripts/env.sh`\n",
    },
  });
  if (commit) {
    git(f.dir, "add", "-A");
    git(f.dir, "commit", "-q", "-m", "harness");
  }
  return f;
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
    expect(report.checks.map((c) => c.id)).toEqual(["H1", "H2", "H3", "H4", "H5", "H6", "H7", "H8", "H9", "H10", "H11", "H12", "H13", "H14", "H15", "H16"]);
    expect(report.checks.filter((c) => c.status === "fail").map((c) => `${c.id}: ${c.details.join("; ")}`)).toEqual([]);
    expect(report.ok).toBe(true);
    expect(code).toBe(0);
  });

  it("gates on exactly H1, H3 and H4 and certifies on H6, H7, H10 and H14", () => {
    const { dir } = fixture();
    const { report } = run(dir);
    expect(report.checks.filter((c) => c.level === "gate").map((c) => c.id)).toEqual(GATE);
    expect(report.checks.filter((c) => c.level === "cert").map((c) => c.id)).toEqual(CERT);
    expect(report.certified).toBe(true);
  });

  it("fails every gate check on an empty repository, and every cert check with --cert", () => {
    const dir = mkdtempSync(join(tmpdir(), "harness-"));
    dirs.push(dir);
    const plain = run(dir);
    expect(plain.report.ok).toBe(false);
    expect(plain.report.blocking).toEqual(GATE);
    expect(plain.code).toBe(1);
    const cert = run(dir, "--cert");
    expect(cert.report.blocking).toEqual([...GATE, ...CERT]);
    expect(cert.report.mode).toBe("cert");
    expect(cert.code).toBe(1);
  });

  it("a harness that is long and has no Done line passes the gate (exit 0) but not --cert (exit 1)", () => {
    const filler = "- Keep the diff small.\n".repeat(110);
    const { dir } = fixture({
      agents: (sha) => AGENTS(sha).replace(/^- Done.*\n/m, "") + filler,
    });
    const quick = run(dir, "--quick");
    expect(quick.code).toBe(0);
    expect(quick.report.ok).toBe(true);
    const full = run(dir);
    expect(full.code).toBe(0);
    expect(full.report.cert).toEqual(["H6"]);
    expect(full.report.certified).toBe(false);
    const cert = run(dir, "--cert");
    expect(cert.code).toBe(1);
    expect(cert.report.blocking).toEqual(["H6"]);
  });

  it("--quick runs only the gate checks", () => {
    const { dir } = fixture({ extra: { ".gitignore": "" } });
    const { report } = run(dir, "--quick");
    expect(report.mode).toBe("quick");
    for (const c of report.checks) expect(c.status === "skipped", c.id).toBe(c.level !== "gate");
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
  });

  describe("H14", () => {
    it("passes a small file and warns above 120 lines without blocking certification", () => {
      const small = fixture();
      expect(check(run(small.dir).report, "H14").status).toBe("pass");
      const soft = fixture({ agents: (sha) => `${AGENTS(sha)}${"- filler\n".repeat(110)}` });
      const c = check(run(soft.dir, "--cert").report, "H14");
      expect(c).toMatchObject({ status: "fail", level: "warning" });
      expect(run(soft.dir, "--cert").report.certified).toBe(true);
    });
    it("fails certification above 200 lines or 24 KiB", () => {
      const long = fixture({ agents: (sha) => `${AGENTS(sha)}${"- filler\n".repeat(200)}` });
      const c = check(run(long.dir).report, "H14");
      expect(c).toMatchObject({ status: "fail", level: "cert" });
      expect(c.details.join()).toContain("lines");
      const wide = fixture({ agents: (sha) => `${AGENTS(sha)}\n- ${"x".repeat(24 * 1024)}\n` });
      expect(check(run(wide.dir).report, "H14")).toMatchObject({ status: "fail", level: "cert" });
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
    it("does not mistake a command from another tool for a package script", () => {
      const { dir } = fixture({
        agents: (sha) => AGENTS(sha).replace("`pnpm verify`", "`go test ./...`").replace("L3", "L2"),
        extra: { "package.json": JSON.stringify({ scripts: { verify: "x", "env:ensure": "x" } }) },
      });
      expect(check(run(dir).report, "H6").status).toBe("pass");
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
    it("does not flag \"implemented\" used as a requirement rather than a status", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n- Tooling MUST be implemented in Go.\n` });
      expect(check(run(dir).report, "H7").status).toBe("pass");
    });
    it("does not flag the Done line, fenced code, or a Greenfield section", () => {
      const agents = (sha: string) =>
        `${AGENTS(sha)}\n\`\`\`\nTODO in a code sample\n\`\`\`\n\n## Greenfield notes\n- Nothing is implemented yet; TODO: scaffold.\n\n## After\n- Keep it small.\n`;
      const { dir } = fixture({ agents });
      expect(check(run(dir).report, "H7").status).toBe("pass");
    });
  });

  describe("H8", () => {
    it("fails a rule without Enforced by and names the line", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n## More rules\n- Never skip the tests.\n`.replace("## Known pitfalls", "## Known pitfalls") });
      expect(check(run(dir).report, "H8").status).toBe("pass");
      const bare = fixture({ agents: (sha) => AGENTS(sha).replace(" Enforced by: prose", "") });
      expect(check(run(bare.dir).report, "H8").details.join()).toContain("rule without");
    });
    it("fails when Enforced by names a path or a command that does not exist, passes for prose", () => {
      const path = fixture({ agents: (sha) => AGENTS(sha).replace("Enforced by: `pnpm test`", "Enforced by: `eslint/none.js`") });
      expect(check(run(path.dir).report, "H8").details.join()).toContain("path that does not exist: eslint/none.js");
      const cmd = fixture({ agents: (sha) => AGENTS(sha).replace("Enforced by: `pnpm test`", "Enforced by: `pnpm ghost`") });
      expect(check(run(cmd.dir).report, "H8").details.join()).toContain("command that does not exist");
      expect(check(run(fixture().dir).report, "H8").status).toBe("pass");
    });
    it("counts prose rules in the footprint", () => {
      const { dir } = fixture();
      expect(run(dir).report.footprint).toMatchObject({ proseRules: 1 });
    });
  });

  describe("H15", () => {
    it("fails a bare pointer and passes one with a condition", () => {
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}- \`docs/sdlc/environment.md\`\n- [arch](docs/sdlc/environment.md)\n` });
      expect(check(run(dir).report, "H15").details).toHaveLength(2);
      expect(check(run(fixture().dir).report, "H15").status).toBe("pass");
    });
  });

  describe("H16", () => {
    it("fails a directory tree, an overview heading and a narrative paragraph", () => {
      const tree = fixture({ agents: (sha) => `${AGENTS(sha)}\n\`\`\`\nsrc/\n├── lib\n└── pages\n\`\`\`\n` });
      expect(check(run(tree.dir).report, "H16").details.join()).toContain("directory tree");
      const head = fixture({ agents: (sha) => `${AGENTS(sha)}\n## Project structure\n- Keep it.\n` });
      expect(check(run(head.dir).report, "H16").details.join()).toContain("overview-style heading");
      const para = fixture({ agents: (sha) => `${AGENTS(sha)}\nDemo is a web app.\nIt uses React.\nIt has a backend.\n` });
      expect(check(run(para.dir).report, "H16").details.join()).toContain("paragraph");
    });
    it("passes the template and exempts a Greenfield section", () => {
      expect(check(run(fixture().dir).report, "H16").status).toBe("pass");
      const { dir } = fixture({ agents: (sha) => `${AGENTS(sha)}\n## Greenfield\nThe repo is empty.\nIt will be a web app.\nStack is open.\n` });
      expect(check(run(dir).report, "H16").status).toBe("pass");
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
    it("H11 does not flag a tracked .env file that holds only public-prefix keys", () => {
      const pub = fixture({ extra: { ".env.production": "EXPO_PUBLIC_API_URL=https://x\n" } });
      git(pub.dir, "add", ".env.production");
      expect(check(run(pub.dir).report, "H11").status).toBe("pass");
      const priv = fixture({ extra: { ".env.production": "API_URL=https://x\nDB_PASSWORD=x\n" } });
      git(priv.dir, "add", ".env.production");
      expect(check(run(priv.dir).report, "H11").details.join()).toContain("a real .env file is tracked");
    });
    it("H12 reads a Gotchas heading like Known pitfalls", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace("## Known pitfalls", "## Gotchas").replace(" (TUPK-182)", "") });
      expect(check(run(dir).report, "H12").status).toBe("fail");
    });
    it("H12 flags a pitfall without an evidence reference", () => {
      const { dir } = fixture({ agents: (sha) => AGENTS(sha).replace("## Read when", "- The cache is flaky.\n\n## Read when") });
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

  describe("--run (E1, E2)", () => {
    it("runs both commands, reports them, and certifies when both exit 0", () => {
      const { dir } = runnable();
      const { code, report } = run(dir, "--run");
      expect(report.mode).toBe("run");
      expect(check(report, "E1").status).toBe("pass");
      expect(check(report, "E2").status).toBe("pass");
      expect(report.executed).toMatchObject({ ensure: { exit: 0 }, verify: { exit: 0, treeClean: true } });
      expect(report.certified).toBe(true);
      expect(code).toBe(0);
    });

    it("fails E1 when Ensure exits non-zero and shows the output", () => {
      const { dir } = runnable({ env: "echo no database >&2\nexit 3\n" });
      const { code, report } = run(dir, "--run");
      expect(check(report, "E1").status).toBe("fail");
      expect(check(report, "E1").details.join("\n")).toContain("exited 3");
      expect(check(report, "E1").details.join("\n")).toContain("no database");
      expect(report.blocking).toContain("E1");
      expect(report.certified).toBe(false);
      expect(code).toBe(1);
    });

    it("fails E2 when Done exits non-zero", () => {
      const { dir } = runnable({ verify: "echo type error\nexit 1\n" });
      const { code, report } = run(dir, "--run");
      expect(check(report, "E2")).toMatchObject({ status: "fail", level: "cert" });
      expect(report.executed!.verify).toMatchObject({ exit: 1 });
      expect(code).toBe(1);
    });

    it("fails E2 without running Done when the git tree is not clean", () => {
      const { dir } = runnable({ verify: "touch ran.marker\nexit 0\n" });
      put(dir, { "src/new.ts": "export {};\n" });
      const { report } = run(dir, "--run");
      expect(check(report, "E2").details.join("\n")).toContain("not clean");
      expect(existsSync(join(dir, "ran.marker"))).toBe(false);
    });

    it("kills a command that outlives --timeout, children included", async () => {
      const { dir } = runnable({ verify: "sleep 30 &\necho $! > child.pid\nwait\n" });
      put(dir, { ".gitignore": ".env\n.multica/\nnode_modules/\nchild.pid\n" });
      git(dir, "add", ".gitignore");
      git(dir, "commit", "-q", "-m", "ignore pid");
      const started = Date.now();
      const { report } = run(dir, "--run", "--timeout", "1");
      expect(Date.now() - started).toBeLessThan(15000);
      expect(check(report, "E2").details.join("\n")).toContain("killed");
      const pid = Number(readFileSync(join(dir, "child.pid"), "utf8").trim());
      await new Promise((r) => setTimeout(r, 300));
      expect(() => process.kill(pid, 0)).toThrow();
    });

    it("redacts secret-looking lines in the output tail", () => {
      const { dir } = runnable({ verify: "echo AKIAABCDEFGHIJKLMNOP\nexit 1\n" });
      const { report } = run(dir, "--run");
      const text = JSON.stringify(report.executed);
      expect(text).toContain("redacted");
      expect(text).not.toContain("AKIAABCDEFGHIJKLMNOP");
    });

    it("does not run a command whose static check fails", () => {
      const { dir } = runnable();
      writeFileSync(join(dir, "AGENTS.md"), readFileSync(join(dir, "AGENTS.md"), "utf8").replace(/^- Done.*\n/m, ""));
      git(dir, "commit", "-q", "-am", "drop done");
      const { report } = run(dir, "--run");
      expect(check(report, "E2").details.join("\n")).toContain("H6 fails");
    });

    it("is refused together with --quick", () => {
      expect(spawnSync("node", [script, "--quick", "--run"], { encoding: "utf8" }).status).toBe(2);
    });
  });

  describe("--write-cert and the certificate", () => {
    const certOf = (dir: string) => JSON.parse(readFileSync(join(dir, "docs/sdlc/harness-cert.json"), "utf8"));

    it("writes docs/sdlc/harness-cert.json only after a certifying run", () => {
      const { dir } = runnable();
      const { code, report } = run(dir, "--run", "--write-cert");
      expect(code).toBe(0);
      const cert = certOf(dir);
      expect(cert).toMatchObject({
        schema: 1,
        sha: git(dir, "rev-parse", "HEAD"),
        checks: { gate: ["H1", "H3", "H4"] },
        executed: { ensure: { exit: 0 }, verify: { exit: 0, treeClean: true } },
        assumptions: [],
      });
      expect(cert.checks.cert).toEqual(["H6", "H7", "H10", "H14", "E1", "E2"]);
      expect(cert.date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
      expect(cert.footprint.lines).toBeGreaterThan(0);
      expect(report.certified).toBe(true);
      expect(check(run(dir).report, "H2").details.join()).toContain("harness-cert.json");
    });

    it("writes nothing and exits 1 when the run does not certify", () => {
      const { dir } = runnable({ verify: "exit 1\n" });
      const { code, report } = run(dir, "--run", "--write-cert");
      expect(code).toBe(1);
      expect(existsSync(join(dir, "docs/sdlc/harness-cert.json"))).toBe(false);
      expect(report.certified).toBe(false);
    });

    it("needs --run, and refuses a bad --accept", () => {
      expect(spawnSync("node", [script, "--write-cert"], { encoding: "utf8" }).status).toBe(2);
      expect(spawnSync("node", [script, "--accept", "nope"], { encoding: "utf8" }).status).toBe(2);
    });

    it("lowers a cert-level check on purpose with --accept, recording the assumption in the certificate", () => {
      const { dir } = runnable();
      writeFileSync(join(dir, "AGENTS.md"), readFileSync(join(dir, "AGENTS.md"), "utf8").replace(/^- Done.*\n/m, ""));
      git(dir, "commit", "-q", "-am", "no done");
      const refused = run(dir, "--run");
      expect(refused.report.certified).toBe(false);
      const { code, report } = run(dir, "--run", "--write-cert", "--accept", "H6=A-3", "--accept", "E2=A-3");
      expect(code).toBe(0);
      expect(report.accepted).toEqual(["H6", "E2"]);
      expect(certOf(dir)).toMatchObject({ accepted: ["H6", "E2"], assumptions: ["A-3"], executed: { verify: null } });
    });

    it("H2 fails for a certificate that is not schema 1, is not JSON, or names a sha that is not in git", () => {
      const bad = fixture({ extra: { "docs/sdlc/harness-cert.json": "{ not json" } });
      expect(check(run(bad.dir).report, "H2").details.join()).toContain("not valid JSON");
      const schema = fixture({ extra: { "docs/sdlc/harness-cert.json": JSON.stringify({ schema: 2 }) } });
      expect(check(run(schema.dir).report, "H2").details.join()).toContain("schema 1");
      const ghost = fixture({ extra: { "docs/sdlc/harness-cert.json": JSON.stringify({ schema: 1, sha: "deadbeefcafe", date: "2026-11-11" }) } });
      expect(check(run(ghost.dir).report, "H2").details.join()).toContain("not a commit");
    });

    it("--refresh reads the certificate: clean at its commit, stale after a manifest or environment change", () => {
      const { dir } = runnable();
      run(dir, "--run", "--write-cert");
      git(dir, "add", "-A");
      git(dir, "commit", "-q", "-m", "certificate");
      const certSha = certOf(dir).sha;
      const fresh = run(dir, "--refresh").report.refresh!;
      expect(fresh).toMatchObject({ certSha, manifestsChanged: [], needsRefresh: false });
      expect(fresh.staleDays).toBe(0);
      put(dir, { "docs/sdlc/environment.md": "- Ensure command: `sh scripts/env.sh`\nChanged.\n", "src/other.ts": "export {};\n" });
      git(dir, "add", "-A");
      git(dir, "commit", "-q", "-m", "env changed");
      const stale = run(dir, "--refresh").report.refresh!;
      expect(stale.manifestsChanged).toEqual(["docs/sdlc/environment.md"]);
      expect(stale.needsRefresh).toBe(true);
    });

    it("--refresh asks for a refresh when there is no certificate, and when its sha is not an ancestor of HEAD", () => {
      const none = fixture({ agents: (sha) => AGENTS(sha).replace(/<!--.*-->\n/, "") });
      expect(run(none.dir, "--refresh").report.refresh).toMatchObject({ certSha: null, needsRefresh: true, reasons: expect.arrayContaining(["no certificate"]) });
      const { dir } = runnable();
      const unrelated = git(dir, "commit-tree", "HEAD^{tree}", "-m", "unrelated");
      put(dir, { "docs/sdlc/harness-cert.json": JSON.stringify({ schema: 1, sha: unrelated, date: "2026-11-11" }) });
      const res = run(dir, "--refresh").report.refresh!;
      expect(res.needsRefresh).toBe(true);
      expect(res.reasons.join()).toContain("not an ancestor");
    });
  });

  describe("the AGENTS.md template", () => {
    it("fills in to a harness that certifies and stays within 80 lines", () => {
      const template = readFileSync(script.replace("scripts/check-harness.mjs", "references/agents-md-template.md"), "utf8");
      const block = /````markdown\n([\s\S]*?)\n````/.exec(template)![1]!;
      const filled = block
        .replace("# <project name>", "# Demo")
        .replace("`<ensure command>`", "`pnpm env:ensure`")
        .replace("`<fast command>`", "`pnpm test`")
        .replace("`<verify command>`", "`pnpm verify`")
        .replace("L<1-3>", "L3")
        .replace("- Never <X>; do <Y>. Enforced by: <lint rule | test | CI job | hook | prose>", "- Never call fetch from a component; use `src/lib/api.ts`. Enforced by: prose")
        .replace("- <What goes wrong, and what to do instead> (<ruling id, issue id, or command and exit code>)", "- A stale session returns 401; clear it first (TUPK-182).")
        .replace("- Changing <area>: read `<path>`", "- Changing the API client: read `src/lib/api.ts`.");
      expect(filled).not.toMatch(/<[a-z]/i);
      expect(filled.split("\n").length).toBeLessThanOrEqual(80);
      const { dir } = fixture({ agents: () => filled });
      const { report } = run(dir, "--cert");
      expect(report.checks.filter((c) => c.status === "fail" && c.level !== "warning").map((c) => `${c.id}: ${c.details.join("; ")}`)).toEqual([]);
      expect(report.certified).toBe(true);
      expect(report.warnings).toEqual(["H2"]);
    });
  });

  describe("--refresh", () => {
    it("falls back to the v1 provenance line and reports no refresh needed at that commit", () => {
      const { dir, sha } = fixture();
      const { refresh } = run(dir, "--refresh").report;
      expect(refresh).toMatchObject({ certSha: sha, manifestsChanged: [], needsRefresh: false });
    });
    it("lists the manifests that changed since the certificate sha", () => {
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
