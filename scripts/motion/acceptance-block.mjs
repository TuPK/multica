// Scenario for scripts/motion/capture.mjs: adding and archiving a criterion in the issue
// sidebar's Acceptance block (durable spec 2.9, packages/views/durable/outcomes). The API of the
// environment must run with the durable_outcomes flag on; `make up` takes it from FF_DURABLE_OUTCOMES
// when the API starts:
//
//   FF_DURABLE_OUTCOMES=true make up C=api,web
//   make env-exec ARGS="-- node scripts/motion/capture.mjs scripts/motion/acceptance-block.mjs --out <dir>"
//
// Each run seeds one issue with three manual criteria, then: adds a criterion through the dialog
// (its row fades in), and archives the first row (it collapses out). Both passes run, the
// reduced-motion pass with the same actions, which must land in one frame.

const SECTION = 'section[aria-label="Acceptance"]';
const TARGETS = {
  section: SECTION,
  list: `${SECTION} ul`,
  first: `${SECTION} ul > li:first-child`,
  // Every run starts with three criteria, so the fourth row is the one the dialog adds.
  added: `${SECTION} ul > li:nth-child(4)`,
  // The row the archive action tags before it clicks: once it leaves, the first child is a
  // different row, so the collapse is tracked on this element alone.
  archived: `${SECTION} ul > li[data-motion-archived]`,
};
const SEED_CRITERIA = ["Unit tests pass", "Docs updated", "Changelog entry"];

async function seed(api, log) {
  const issue = await api("POST", "/api/issues", { title: "Acceptance block motion check", allow_duplicate: true });
  for (const text of SEED_CRITERIA) {
    await api("POST", `/api/issues/${issue.id}/criteria`, { text, check_kind: "manual" });
  }
  log(`seeded issue ${issue.identifier ?? issue.id} (${issue.id}) with ${SEED_CRITERIA.length} criteria`);
  return issue;
}

async function openIssue(open, issue, options) {
  const page = await open(`/issues/${issue.identifier ?? issue.id}`, options);
  await page.waitForSelector(`${TARGETS.first}`, { timeout: 60000 });
  await page.waitForTimeout(1200); // fonts, queries, the sidebar's entry transition
  return page;
}

// The row appears when the criteria are read again after the write, so each action waits for
// that read: sampling that stopped at the write would miss the motion.
const refetchedCriteria = (page) =>
  page.waitForResponse((r) => r.request().method() === "GET" && /\/criteria$/.test(new URL(r.url()).pathname), { timeout: 15000 });

async function addCriterion(page, text) {
  await page.getByRole("button", { name: "Add criterion" }).click();
  const dialog = page.getByRole("dialog", { name: "Add criterion" });
  await dialog.getByLabel("Criterion").fill(text);
  const refetch = refetchedCriteria(page);
  await dialog.getByRole("button", { name: "Add", exact: true }).click();
  await refetch;
  await dialog.waitFor({ state: "detached", timeout: 10000 });
}

async function archiveFirst(page) {
  await page.locator(TARGETS.first).evaluate((row) => {
    row.dataset.motionArchived = "";
  });
  const refetch = refetchedCriteria(page);
  await page.getByRole("button", { name: "Archive criterion" }).first().click();
  await refetch;
}

export default async function acceptanceBlock({ open, sample, record, check, checks, api, log }) {
  const { track, monotonic, animated, instant, reachesEnd } = checks;
  const issue = await seed(api, log);

  for (const reducedMotion of [false, true]) {
    const pass = reducedMotion ? "reduced motion" : "motion";
    const page = await openIssue(open, issue, { reducedMotion });

    const addLabel = `${pass}, add a criterion`;
    const addFrames = await sample(page, addLabel, TARGETS, () => addCriterion(page, "Screenshots attached"), { quietMs: 500 });
    // The sampler records boxes only, so the fade is not measured; the rise is, and the GIF shows both.
    const newY = track(addFrames, "added", "y");
    if (reducedMotion) {
      check(`${addLabel}: list grows in one frame`, instant(track(addFrames, "list", "h")));
    } else {
      check(`${addLabel}: new row rises into place`, animated(newY));
      check(`${addLabel}: new row rises one way`, monotonic(newY));
      const rise = Math.max(...newY) - Math.min(...newY);
      check(`${addLabel}: new row moves at most 4px`, { ok: rise <= 4.5, detail: `moved ${rise.toFixed(1)}px` });
    }

    const archiveLabel = `${pass}, archive the first criterion`;
    const archiveFrames = await sample(page, archiveLabel, TARGETS, () => archiveFirst(page), { quietMs: 500 });
    const listHeight = track(archiveFrames, "list", "h");
    const archivedHeight = track(archiveFrames, "archived", "h");
    if (reducedMotion) {
      check(`${archiveLabel}: list height changes instantly`, instant(listHeight));
      check(`${archiveLabel}: archived row closes in one frame`, instant(archivedHeight));
      // Reduced motion removes the row outright, so its end state is gone, not zero high.
      const removed = archiveFrames.at(-1)?.rects.archived == null;
      check(`${archiveLabel}: archived row is gone by the last frame`, { ok: removed, detail: removed ? "gone" : "still on the page" });
    } else {
      check(`${archiveLabel}: list height changes one way`, monotonic(listHeight));
      check(`${archiveLabel}: list height eases`, animated(listHeight));
      check(`${archiveLabel}: archived row collapses over several frames`, animated(archivedHeight));
      check(`${archiveLabel}: archived row ends at zero`, reachesEnd(archivedHeight, 0));
    }
  }

  // The same two actions, recorded in motion, from a fresh page.
  const issueForGif = await seed(api, log);
  const page = await openIssue(open, issueForGif, {});
  const box = await page.locator(SECTION).boundingBox();
  const clip = { x: box.x - 8, y: box.y - 8, width: box.width + 16, height: box.height + 16 };
  await record(page, "acceptance-block", clip, async () => {
    await page.waitForTimeout(400);
    await addCriterion(page, "Screenshots attached");
    await page.waitForTimeout(700);
    await archiveFirst(page);
    await page.waitForTimeout(900);
  });
}
