import { expect, test, type Page } from "@playwright/test";
import { TestApiClient } from "./fixtures";

// A fixed user and workspace, as loginAsDefault does, so repeated runs reuse
// one workspace. The test saves nothing, so there is no data to remove.
const WORKFLOW_E2E_EMAIL = "e2e-workflow@multica.ai";
const WORKFLOW_E2E_NAME = "E2E Workflow User";
const WORKFLOW_E2E_WORKSPACE_SLUG = "e2e-workflow-workspace";

/** Signs in and returns the workspace slug the tests open. */
async function signIn(page: Page): Promise<string> {
  const api = new TestApiClient();
  await api.login(WORKFLOW_E2E_EMAIL, WORKFLOW_E2E_NAME);
  const workspace = await api.ensureWorkspace("E2E Workflow Workspace", WORKFLOW_E2E_WORKSPACE_SLUG);
  await api.markUserOnboarded();
  const token = api.getToken();
  if (!token) throw new Error("E2E login did not return an auth token");
  await page.addInitScript((t) => {
    localStorage.setItem("multica_token", t);
    localStorage.setItem("multica:chat:isOpen", "false");
  }, token);
  return workspace.slug;
}

test.describe("workflow tab", () => {
  test("opens the workflow list, starts a new designer, and shows the live validation badge", async ({ page }) => {
    const slug = await signIn(page);

    await page.goto(`/${slug}/workflow`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: "Workflows", exact: true })).toBeVisible();

    await page.getByRole("link", { name: "New workflow", exact: true }).click();
    await page.waitForURL(new RegExp(`/${slug}/workflow/new$`));

    // A new workflow starts from a valid seed: todo, in progress, done, cancelled.
    await expect(page.getByRole("button", { name: "Workflow is valid", exact: true })).toBeVisible();

    // Removing in progress leaves the todo branch pointing at a missing status,
    // and the badge changes without a save.
    await page.getByRole("button", { name: "Remove step", exact: true }).click();
    await expect(page.getByRole("button", { name: "1 Errors", exact: true })).toBeVisible();
  });
});
