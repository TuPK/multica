import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";
import type { TestApiClient } from "./fixtures";
import { callApi, noHorizontalScroll, persona, removeRun, shot, signIn } from "./flow-e2e-people";

// P21 of the flow engine plan (spec 11.4, C15, K13) as an automated test: the draft editor on a laptop (1280x800)
// and on a phone (390x844), driven in a real browser. A person is simulated by the browser; no agent runs.
//
// Each test is one verdict. The draft is made by the test through the API in a workspace of its own, and the
// workspace, the squad, the agent, the draft and the users are removed afterwards.
//
// Needs a backend with FF_FLOW_AUTHORING and FF_FLOW_GRAPH_V2 on and a web app pointing at it (see the run
// instructions in the ledger). FLOW_E2E_SHOTS names the folder the screenshots go to.

const RUN = Date.now().toString(36);
const EMAIL = `p21-owner-${RUN}@multica.ai`;
const SLUG = `p21-${RUN}`;

function tradeManifest() {
  const doc = JSON.parse(readFileSync(resolve(process.cwd(), "server/internal/issueworkflow/testdata/flows/trade.json"), "utf8"));
  for (const step of doc.steps) if (step.status?.name) step.status.name = `${step.status.name} ${RUN}`.slice(0, 40);
  return { format: "multica.flow-manifest/v1", name: `Trade copy ${RUN}`, doc };
}

/** A flow of `count` lead steps in a row, longer than any canvas of a laptop window can show whole. */
function longChainManifest(count: number) {
  const keys = Array.from({ length: count }, (_, i) => `s${String(i + 1).padStart(2, "0")}`);
  return {
    format: "multica.flow-manifest/v1",
    name: `Long chain ${RUN}`,
    doc: {
      version: 2,
      start: keys[0],
      roles: [{ key: "worker", kind: "agent" }],
      terminals: [{ key: "done", status: "done" }],
      steps: keys.map((key, i) => ({
        key,
        kind: "lead",
        title: `Step ${i + 1}`,
        status: { key: "todo" },
        edges: [{ id: `${key}_ok`, label: "Next", to: keys[i + 1] ?? "done" }],
      })),
    },
  };
}

/** A flow whose Check finds no error and many warnings: one for the status the steps share, one for each input. */
function manyWarningsManifest(steps: number) {
  const keys = Array.from({ length: steps }, (_, i) => `s${i + 1}`);
  return {
    format: "multica.flow-manifest/v1",
    name: `Many warnings ${RUN}`,
    doc: {
      version: 2,
      start: "plan",
      roles: [{ key: "worker", kind: "agent" }],
      terminals: [{ key: "done", status: "done" }],
      steps: [
        { key: "plan", kind: "lead", title: "Plan", status: { key: "todo" }, edges: [{ id: "plan_ok", label: "Planned", to: keys[0] }] },
        ...keys.map((key, i) => ({
          key,
          kind: "tasks",
          title: `Step ${i + 1}`,
          status: { key: "in_progress" },
          tasks: [{ key: `t${i + 1}`, title: `Task ${i + 1}`, kind: "work", role: "worker", ...(i > 0 ? { inputs: [{ from: `${keys[i - 1]}.t${i}` }] } : {}) }],
          edges: [{ id: `${key}_ok`, label: "Next", to: keys[i + 1] ?? "done" }],
        })),
      ],
    },
  };
}

/** The control is where a finger or a click lands: it has a box inside the page and nothing covers its centre. */
async function expectUsable(page: Page, control: Locator, what: string) {
  await expect(control, `${what}: not visible`).toBeVisible();
  await control.scrollIntoViewIfNeeded();
  const box = await control.boundingBox();
  expect(box, `${what}: no bounding box`).not.toBeNull();
  expect(box!.width, `${what}: empty box ${JSON.stringify(box)}`).toBeGreaterThan(0);
  expect(box!.height, `${what}: empty box ${JSON.stringify(box)}`).toBeGreaterThan(0);
  const viewport = page.viewportSize()!;
  expect(box!.x, `${what}: starts left of the page ${JSON.stringify(box)}`).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width, `${what}: sticks out right of the page ${JSON.stringify(box)}`).toBeLessThanOrEqual(viewport.width);
  const covered = await control.evaluate((el) => {
    const r = el.getBoundingClientRect();
    const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return top === el || (top !== null && el.contains(top)) ? null : (top ? `${top.tagName}.${(top as HTMLElement).className}`.slice(0, 120) : "nothing");
  });
  expect(covered, `${what}: covered at its centre by ${covered}`).toBeNull();
  await expect(control, `${what}: disabled`).toBeEnabled();
}

test.describe.serial("P21: the flow draft editor on a laptop and on a phone", () => {
  let owner: TestApiClient | undefined;
  let squadId = "";
  let draftId = "";
  let slug = "";

  test.beforeAll(async () => {
    owner = await persona(EMAIL, `P21 Owner ${RUN}`);
    const ws = await owner.ensureWorkspace(`P21 ${RUN}`, SLUG);
    slug = ws.slug;
    const runtimeId = await owner.seedRuntime(`P21 runtime ${RUN}`);
    const lead = await owner.createAgent(`P21 lead ${RUN}`, runtimeId);
    const squad = await owner.createSquad(`P21 squad ${RUN}`, lead.id, "P21 layout check");
    squadId = squad.id;
    const draft = await callApi<{ id: string }>(owner, "POST", `/api/squads/${squadId}/flow-drafts`, {
      from: { kind: "manifest", manifest: tradeManifest() },
    });
    draftId = draft.id;
  });

  test.afterAll(async () => {
    await removeRun(owner, [EMAIL]);
  });

  async function openEditor(page: Page, size: { width: number; height: number }) {
    await page.setViewportSize(size);
    await signIn(page, owner!);
    await page.goto(`/${slug}/squads/${squadId}/workflows/${draftId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Trade copy ${RUN}`, { timeout: 90_000 });
  }

  for (const size of [
    { name: "1280x800 desktop", width: 1280, height: 800, tag: "1280" },
    { name: "390x844 phone", width: 390, height: 844, tag: "390" },
  ]) {
    test(`${size.name}: no sideways scroll, Check and Publish are visible and clickable`, async ({ page }) => {
      test.setTimeout(240_000);
      await openEditor(page, size);
      await shot(page, `p21-editor-${size.tag}`);

      // A. The page does not scroll sideways.
      await noHorizontalScroll(page, `editor at ${size.tag}px`);

      // B. Check and Publish are on screen and nothing covers them.
      const check = page.getByRole("button", { name: "Check", exact: true });
      const publish = page.getByRole("button", { name: "Publish", exact: true });
      await expectUsable(page, check, `Check at ${size.tag}px`);
      await expectUsable(page, publish, `Publish at ${size.tag}px`);

      // C. A click on Check reaches the server and the answer comes back.
      const validated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/validate`) && r.request().method() === "POST");
      await check.click();
      expect((await validated).status(), "Check: the validate call").toBe(200);
      await shot(page, `p21-check-${size.tag}`);
      await noHorizontalScroll(page, `after Check at ${size.tag}px`);

      // D. A click on Publish opens the publish dialog, which also does not scroll sideways.
      await publish.click();
      const dialog = page.getByRole("dialog");
      await expect(dialog, "Publish: the dialog did not open").toBeVisible();
      await expect(dialog).toHaveCSS("opacity", "1");
      await shot(page, `p21-publish-dialog-${size.tag}`);
      await noHorizontalScroll(page, `publish dialog at ${size.tag}px`);
      const box = await dialog.boundingBox();
      expect(box!.x, "publish dialog starts left of the page").toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width, "publish dialog sticks out right of the page").toBeLessThanOrEqual(size.width);
      await page.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
    });
  }

  // K13: the Outline is usable with a keyboard. Tab reaches the steps in order; Enter and Space open the step.
  for (const size of [
    { name: "1280x800 desktop", width: 1280, height: 800, tag: "1280" },
    { name: "390x844 phone", width: 390, height: 844, tag: "390" },
  ]) {
    test(`${size.name}: the Outline works by keyboard (Tab, then Enter or Space on a step)`, async ({ page }) => {
      test.setTimeout(240_000);
      await openEditor(page, size);
      const phone = size.width < 768;
      const search = page.getByPlaceholder("Search steps…");
      const stepButton = (title: string) => page.getByRole("button", { name: new RegExp(`^${title}\\b`) }).first();

      // Tab from the search box reaches the first step, then the next one, in the order of the list.
      await search.focus();
      await page.keyboard.press("Tab");
      const names: string[] = [];
      for (let i = 0; i < 2; i++) {
        names.push(await page.evaluate(() => (document.activeElement as HTMLElement | null)?.textContent ?? ""));
        await page.keyboard.press("Tab");
      }
      expect(names[0], "the first Tab from the search box did not land on a step").toMatch(/^Plan/);
      expect(names[1], "the second Tab did not land on the next step").toMatch(/^Collect/);

      // Enter on a step opens it: its title shows in the inspector.
      const collect = stepButton("Collect");
      await collect.focus();
      await page.keyboard.press("Enter");
      const title = (phone ? page.locator('[data-slot="sheet-content"]') : page).getByPlaceholder("Step title");
      await expect(title, "Enter did not open the step").toHaveValue("Collect");
      if (phone) {
        // The Sheet takes the focus into itself; Escape closes it and the focus goes back to the step.
        await expect(page.locator('[data-slot="sheet-content"]').locator(":focus, :has(:focus)").first(), "focus is not inside the Sheet").toBeAttached();
        await page.keyboard.press("Escape");
        await expect(page.locator('[data-slot="sheet-content"]')).toBeHidden();
        await expect(collect, "focus did not return to the step").toBeFocused();
      } else {
        await expect(collect, "focus left the step after Enter").toBeFocused();
      }
      await noHorizontalScroll(page, `Outline by keyboard at ${size.tag}px`);

      // Space does the same on another step.
      const next = stepButton("Draft");
      await next.focus();
      await page.keyboard.press("Space");
      await expect(title, "Space did not open the step").toHaveValue(/^Draft/);
      if (!phone) await expect(next, "focus left the step after Space").toBeFocused();
      await shot(page, `p21-outline-keyboard-${size.tag}`);
      await noHorizontalScroll(page, `Outline after Space at ${size.tag}px`);
    });
  }

  // K13: a graph is read, not hunted for. At 1280px every step of the draft is on screen the moment the Graph opens,
  // and the canvas never scrolls the page sideways; on a phone the graph is the vertical list of the steps.
  test("1280x800 desktop: the Graph shows every step inside its canvas as soon as it opens", async ({ page }) => {
    test.setTimeout(240_000);
    await openEditor(page, { width: 1280, height: 800 });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    const canvas = page.getByTestId("flow-graph-canvas");
    await expect(canvas).toBeVisible();

    // Read at once, with no wait for an animation to end: a view that fits only after a moment is not fitted.
    const cards = await canvas.locator("[data-step-key]").evaluateAll((nodes) => {
      const area = nodes[0]!.closest(".overflow-auto")!.getBoundingClientRect();
      return {
        area: { left: area.left, right: area.right, top: area.top, bottom: area.bottom },
        cards: nodes.map((n) => {
          const r = n.getBoundingClientRect();
          return { key: n.getAttribute("data-step-key"), left: r.left, right: r.right, top: r.top, bottom: r.bottom };
        }),
      };
    });
    expect(cards.cards.map((c) => c.key), "the graph does not draw every step of the draft").toEqual(
      expect.arrayContaining(["plan", "collect", "draft", "gate", "handover", "release"]),
    );
    for (const c of cards.cards) {
      expect(c.left, `step ${c.key} starts left of the canvas ${JSON.stringify(cards.area)}`).toBeGreaterThanOrEqual(cards.area.left);
      expect(c.right, `step ${c.key} is cut at the right of the canvas ${JSON.stringify({ card: c, area: cards.area })}`).toBeLessThanOrEqual(cards.area.right);
      expect(c.bottom, `step ${c.key} is cut at the bottom of the canvas`).toBeLessThanOrEqual(cards.area.bottom);
    }
    await shot(page, "p21-graph-1280");
    await noHorizontalScroll(page, "graph at 1280px");
  });

  // A graph that does not fit even at the smallest size the canvas shrinks to is panned: it scrolls inside the canvas,
  // never the page, and its last step can be brought into view.
  test("1280x800 desktop: a long graph is panned inside its canvas up to its last step", async ({ page }) => {
    test.setTimeout(240_000);
    const draft = await callApi<{ id: string }>(owner!, "POST", `/api/squads/${squadId}/flow-drafts`, {
      from: { kind: "manifest", manifest: longChainManifest(14) },
    });
    await page.setViewportSize({ width: 1280, height: 800 });
    await signIn(page, owner!);
    await page.goto(`/${slug}/squads/${squadId}/workflows/${draft.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Long chain ${RUN}`, { timeout: 90_000 });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    const canvas = page.getByTestId("flow-graph-canvas");
    await expect(canvas).toBeVisible();

    const area = canvas.locator("xpath=ancestor::div[contains(@class,'overflow-auto')][1]");
    const sizes = await area.evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
    expect(sizes.scrollWidth, `the canvas does not scroll, so the steps past its edge cannot be reached ${JSON.stringify(sizes)}`).toBeGreaterThan(sizes.clientWidth);
    await shot(page, "p21-graph-long-start-1280");

    await area.evaluate((el) => el.scrollTo({ left: el.scrollWidth }));
    const last = canvas.locator('[data-step-key="s14"]');
    const box = await last.boundingBox();
    const bounds = await area.evaluate((el) => el.getBoundingClientRect().right);
    expect(box, "the last step has no box").not.toBeNull();
    expect(box!.x + box!.width, `the last step is out of reach at the end of the canvas ${JSON.stringify({ box, bounds })}`).toBeLessThanOrEqual(bounds + 1);
    await shot(page, "p21-graph-long-end-1280");
    await noHorizontalScroll(page, "long graph at 1280px");
  });

  // The Problems list is read from top to bottom with the page: it has no scroll area of its own that hides rows, and a
  // long path wraps inside the row instead of being cut at the edge of the screen.
  for (const size of [
    { name: "1280x800 desktop", width: 1280, height: 800, tag: "1280" },
    { name: "390x844 phone", width: 390, height: 844, tag: "390" },
  ]) {
    test(`${size.name}: every row of Problems is in the page, whole`, async ({ page }) => {
      test.setTimeout(240_000);
      const draft = await callApi<{ id: string }>(owner!, "POST", `/api/squads/${squadId}/flow-drafts`, {
        from: { kind: "manifest", manifest: manyWarningsManifest(8) },
      });
      await page.setViewportSize({ width: size.width, height: size.height });
      await signIn(page, owner!);
      await page.goto(`/${slug}/squads/${squadId}/workflows/${draft.id}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Many warnings ${RUN}`, { timeout: 90_000 });
      const validated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draft.id}/validate`) && r.request().method() === "POST");
      await page.getByRole("button", { name: "Check", exact: true }).click();
      expect((await validated).status()).toBe(200);
      await page.getByRole("button", { name: /^Problems/ }).click();
      const rows = page.getByRole("listitem").filter({ hasText: /status_shared|input_no_files/ });
      await expect(rows.first()).toBeVisible();
      expect(await rows.count(), "Check found fewer warnings than the draft was built to give").toBeGreaterThanOrEqual(8);

      const report = await rows.evaluateAll((items) =>
        items.map((li) => {
          const scrollers: string[] = [];
          for (let el = li.parentElement; el; el = el.parentElement) {
            const overflowY = getComputedStyle(el).overflowY;
            // A box that scrolls and ends well above the bottom of the screen is a window onto the list, not the page.
            if ((overflowY === "auto" || overflowY === "scroll") && el.scrollHeight > el.clientHeight && el.getBoundingClientRect().bottom < window.innerHeight * 0.8) {
              scrollers.push(el.className.slice(0, 60));
            }
          }
          const box = li.getBoundingClientRect();
          const path = li.querySelector("span.font-mono:nth-of-type(2)")?.getBoundingClientRect();
          return { text: (li.textContent ?? "").slice(0, 40), scrollers, right: box.right, pathRight: path?.right ?? 0, scrollWidth: li.scrollWidth, clientWidth: li.clientWidth };
        }),
      );
      const own = report.filter((r) => r.scrollers.length > 0);
      expect(own, `rows sit in a box that scrolls on its own, short of the page bottom: ${JSON.stringify(own.slice(0, 2))}`).toEqual([]);
      for (const r of report) {
        expect(r.scrollWidth, `row "${r.text}" is cut at its right edge`).toBeLessThanOrEqual(r.clientWidth);
        expect(r.pathRight, `the path of row "${r.text}" sticks out of its row`).toBeLessThanOrEqual(r.right);
      }
      await shot(page, `p21-problems-${size.tag}`);
      await noHorizontalScroll(page, `Problems at ${size.tag}px`);
    });
  }

  test("390x844 phone: the Graph is a list that shows every step and does not scroll sideways", async ({ page }) => {
    test.setTimeout(240_000);
    await openEditor(page, { width: 390, height: 844 });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    for (const title of ["Plan", "Collect", "Draft", "Gate", "Handover", "Release"]) {
      await expect(page.getByText(new RegExp(`^#\\d+ ${title}`)).first(), `the list has no step ${title}`).toBeVisible();
    }
    await shot(page, "p21-graph-390");
    await noHorizontalScroll(page, "graph at 390px");
  });

  test("390x844 phone: the inspector opens as a Sheet inside the screen", async ({ page }) => {
    test.setTimeout(240_000);
    await openEditor(page, { width: 390, height: 844 });
    // The inspector of a step is not beside the outline on a phone: no dialog until a step is chosen.
    await expect(page.getByRole("dialog")).toHaveCount(0);

    await page.getByRole("button", { name: /^Collect\b/ }).first().click();
    const sheet = page.locator('[data-slot="sheet-content"]');
    await expect(sheet, "the inspector is not a Sheet (no [data-slot=sheet-content])").toBeVisible();
    await expect(sheet).toHaveAttribute("role", "dialog");
    await expect(sheet).toHaveAttribute("data-side", "right");
    await expect(sheet).toHaveCSS("opacity", "1");
    await page.waitForTimeout(600);
    await shot(page, "p21-inspector-sheet-390");

    const box = await sheet.boundingBox();
    expect(box, "the Sheet has no box").not.toBeNull();
    expect(box!.x, `the Sheet starts left of the screen ${JSON.stringify(box)}`).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width, `the Sheet spills right of the screen ${JSON.stringify(box)}`).toBeLessThanOrEqual(390);
    expect(box!.y + box!.height, `the Sheet spills below the screen ${JSON.stringify(box)}`).toBeLessThanOrEqual(844);
    await expect(sheet.getByPlaceholder("Step title")).toHaveValue("Collect");
    await expect(page.locator('[data-slot="sheet-overlay"]'), "the Sheet has no overlay").toBeVisible();
    await noHorizontalScroll(page, "inspector Sheet at 390px");
    const inner = await sheet.evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
    expect(inner.scrollWidth, `the Sheet scrolls sideways ${JSON.stringify(inner)}`).toBeLessThanOrEqual(inner.clientWidth);

    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
  });
});
