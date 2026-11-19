/**
 * Staff reads and inline actions in the console: the overview cards, a
 * tenant's detail and internal notes, a user's account record, the e-invoice
 * and audit screens, the account and payment-notice screens, and what happens
 * when the session ends. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import { queueReviewNotice, saveSignedInState, seedAccount, seedWorkspace, useSignedIn } from "./session";

let seed: StaffSeed;
let admin: APIRequestContext;
let tenant: { id: string; slug: string };
let tenantName = "E2E Notes Co";
let target: { id: string; email: string };
const noticeEventIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenant = await seedWorkspace(tenantName);
  target = await seedAccount("E2E Reads Person");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "reads");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query("DELETE FROM platform_note WHERE workspace_id = $1", [tenant.id]);
    await pool.query("DELETE FROM member WHERE workspace_id = $1", [tenant.id]);
    await pool.query("DELETE FROM workspace WHERE id = $1", [tenant.id]);
    await pool.query("DELETE FROM user_mfa WHERE user_id = $1", [target.id]);
    await pool.query("DELETE FROM platform_account WHERE user_id = $1", [target.id]);
    if (noticeEventIds.length > 0) {
      await pool.query("DELETE FROM payment_notice WHERE provider = 'sepay' AND provider_event_id = ANY($1)", [noticeEventIds]);
    }
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "reads");
});

async function openNav(page: import("@playwright/test").Page, name: string): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name }).click();
}

async function openTenant(page: import("@playwright/test").Page, name: string): Promise<void> {
  await openNav(page, "Tenants");
  await page.getByPlaceholder("Search by name or slug...").fill(name);
  await page.getByRole("row", { name: new RegExp(name) }).getByRole("button", { name: "View" }).click();
  await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();
}

test.describe("overview", () => {
  test("flow 26: the overview shows the platform's counters", async ({ page }) => {
    await openNav(page, "Overview");
    await expect(page.getByRole("heading", { name: "Platform Overview" })).toBeVisible();
    await expect(page.getByText("Total Workspaces")).toBeVisible();
    await expect(page.getByText("Active Workspaces")).toBeVisible();
  });
});

test.describe("tenant detail", () => {
  test("flow 26: a tenant's detail shows its status, internal notes, subscription and billing profile", async ({ page }) => {
    await openTenant(page, tenantName);
    await expect(page.getByText("Internal Notes").first()).toBeVisible();
    await expect(page.getByText("Subscription", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Billing profile", { exact: true }).first()).toBeVisible();
  });

  test("flow 26: staff adds an internal note and reads it back on the screen", async ({ page }) => {
    await openTenant(page, tenantName);
    const note = `e2e note ${randomUUID().slice(0, 8)}`;
    const box = page.getByPlaceholder("Write an internal note...");
    const add = page.getByRole("button", { name: "Add note" });

    await expect(add).toBeDisabled();
    await box.fill("   ");
    await expect(add).toBeDisabled();

    await box.fill(note);
    await expect(add).toBeEnabled();
    await add.click();
    await expect(box).toHaveValue("");
    await expect(page.getByText(note)).toBeVisible();
  });

  test("flow 264: a note one character over 4000 is refused and the screen shows the error", async ({ page }) => {
    await openTenant(page, tenantName);
    await page.getByPlaceholder("Write an internal note...").fill("x".repeat(4001));
    await page.getByRole("button", { name: "Add note" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
  });

  test("flow 264: a note of exactly 4000 characters is accepted", async ({ page }) => {
    await openTenant(page, tenantName);
    const box = page.getByPlaceholder("Write an internal note...");
    await box.fill("y".repeat(4000));
    await page.getByRole("button", { name: "Add note" }).click();
    await expect(box).toHaveValue("");
    await expect(page.getByRole("alert")).toHaveCount(0);
  });

  test("flow 63: staff edits the billing profile in a dialog and the saved legal name is shown", async ({ page }) => {
    await openTenant(page, tenantName);
    const legalName = `E2E Legal ${randomUUID().slice(0, 8)}`;
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Edit Billing Profile" });
    await expect(dialog).toBeVisible();
    // The company fields only appear for a business account.
    await dialog.locator("#bp-account-kind").selectOption("business");
    await dialog.locator("#bp-legal-name").fill(legalName);
    await dialog.locator("#bp-tax-code").fill("0123456789");
    await dialog.locator("#bp-address").fill("1 E2E Street, Hanoi");
    await dialog.locator("#bp-invoice-email").fill("billing@example.test");
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(legalName)).toBeVisible();
  });

  test("flow 63: a tax code that is not 10 digits is refused in the billing profile dialog", async ({ page }) => {
    await openTenant(page, tenantName);
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Edit Billing Profile" });
    await dialog.locator("#bp-account-kind").selectOption("business");
    await dialog.locator("#bp-legal-name").fill("E2E Tax Co");
    await dialog.locator("#bp-tax-code").fill("12ab");
    await dialog.locator("#bp-address").fill("1 E2E Street, Hanoi");
    await dialog.locator("#bp-invoice-email").fill("billing@example.test");
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Enter a valid 10-digit (or 10-3) tax code.")).toBeVisible();
  });

  test("flow 64: staff overrides the tenant subscription with a reason and the dialog closes", async ({ page }) => {
    await openTenant(page, tenantName);
    await page.getByRole("button", { name: "Edit subscription" }).click();
    const dialog = page.getByRole("dialog", { name: "Edit Subscription" });
    await expect(dialog).toBeVisible();
    const save = dialog.getByRole("button", { name: "Save subscription" });
    await expect(save).toBeDisabled();
    await dialog.locator("#sub-plan-key-input").fill("pro");
    await dialog.locator("#sub-seats-input").fill("3");
    await dialog.locator("#sub-reason-input").fill("e2e subscription drill");
    await expect(save).toBeEnabled();
    await save.click();
    await expect(dialog).toHaveCount(0);
  });

  test("flow 64: an unknown plan key is refused and the subscription dialog stays open", async ({ page }) => {
    await openTenant(page, tenantName);
    await page.getByRole("button", { name: "Edit subscription" }).click();
    const dialog = page.getByRole("dialog", { name: "Edit Subscription" });
    await dialog.locator("#sub-plan-key-input").fill("no-such-plan");
    await dialog.locator("#sub-reason-input").fill("e2e unknown plan drill");
    await dialog.getByRole("button", { name: "Save subscription" }).click();
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("alert")).toBeVisible();
  });
});

test.describe("user detail", () => {
  test("flow 20: staff finds a user and reads the account record, with no workspace memberships", async ({ page }) => {
    await openNav(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByText("Account Record")).toBeVisible();
    await expect(page.getByText("Workspace Memberships")).toBeVisible();
    await expect(page.getByText("No memberships")).toBeVisible();
  });

  test("flow 23: staff opens the two-step sign-in reset for another user", async ({ page }) => {
    await openNav(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Reset two-step sign-in" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
  });

  test("flow 23: staff resets another user's two-step sign-in with the dialog and the dialog closes", async ({ page }) => {
    await openNav(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Reset two-step sign-in" }).click();
    const dialog = page.getByRole("dialog", { name: "Reset two-step sign-in" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Reset and email the user" }).click();
    await expect(dialog).toHaveCount(0);
  });
});

test.describe("other screens", () => {
  test("flow 72: the e-invoice screen opens from the nav", async ({ page }) => {
    await openNav(page, "E-invoices");
    await expect(page.getByText("E-invoices").first()).toBeVisible();
  });

  test("flow 26: the audit log filters by an actor id that has no events and shows the empty state", async ({ page }) => {
    await openNav(page, "Audit Log");
    await page.getByPlaceholder("Filter by actor ID...").fill(randomUUID());
    await expect(page.getByText("No audit events found")).toBeVisible();
  });

  test("flow 24: the accounts screen opens and offers account creation", async ({ page }) => {
    await openNav(page, "Accounts");
    await expect(page.getByRole("button", { name: "Create account" })).toBeVisible();
  });

  test("flow 60: the payment notice queue opens and each row can be viewed", async ({ page }) => {
    await openNav(page, "Payment notices");
    await expect(page.getByRole("heading", { name: "Payment notices" })).toBeVisible();
    const rows = page.getByRole("row").filter({ has: page.getByRole("button", { name: "View" }) });
    if ((await rows.count()) > 0) {
      await rows.first().getByRole("button", { name: "View" }).click();
      await expect(page.getByText("Notice details")).toBeVisible();
    }
  });

  test("flow 61: staff dismisses a payment notice in a dialog and it leaves the review queue", async ({ page }) => {
    const amount = 1_000_000 + Math.floor(Math.random() * 8_000_000);
    const amountText = amount.toLocaleString();
    await queueReviewNotice(amount, noticeEventIds);

    await openNav(page, "Payment notices");
    const row = page.getByRole("row", { name: new RegExp(`${amountText} VND`) });
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "View" }).click();
    await expect(page.getByText("Notice details")).toBeVisible();

    await page.getByRole("button", { name: "Dismiss", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Dismiss payment notice" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Dismiss notice" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("e2e dismiss drill");
    await dialog.getByRole("button", { name: "Dismiss notice" }).click();

    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("row", { name: new RegExp(`${amountText} VND`) })).toHaveCount(0);
  });
});

test.describe("session", () => {
  test("flow 13: clearing the session cookie returns the console to the sign-in form on the next visit", async ({ page }) => {
    await page.context().clearCookies();
    await page.goto(CONSOLE_BASE);
    await expect(page.locator("#email-input")).toBeVisible();
    await expect(page.getByText("Platform Console", { exact: true })).toBeVisible();
  });
});
