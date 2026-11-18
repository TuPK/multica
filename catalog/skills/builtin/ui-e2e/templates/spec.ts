/**
 * Spec skeleton for a UI journey. Fill the selectors from the app's own labels.
 */
import { expect, test } from "@playwright/test";
import { useSignedIn, saveSignedInState } from "./session";

test.beforeAll(async ({ browser }) => {
  // Seed the rows the screens need (SQL or an API helper), then save a session once.
  await saveSignedInState(browser, "admin", async (page) => {
    // sign in through the real form here
  });
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "admin");
});

test.describe("<feature>", () => {
  test("flow <id>: <what the user does and sees>", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "<Nav item>" }).click();
    await expect(page.getByRole("heading", { name: "<Screen title>" })).toBeVisible();
  });

  test.fixme("flow <id>: <edge case>", async () => {
    // Product failure: <observed behaviour>; likely cause: <cause>. Product code not changed.
  });
});
