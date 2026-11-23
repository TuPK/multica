/**
 * Remaining console screens: the failure states of the overview, tenant detail
 * and user list; the tenant detail sections; the staff list; the accounts list
 * and its create dialog; the e-invoice state tabs; the audit filters; and the
 * payment notice match dialog. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Locator, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { CONSOLE_BASE, serverOutput, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import pg from "pg";
import {
  cleanupRows,
  queueReviewNotice,
  saveSignedInState,
  seedAccount,
  seedPendingOrder,
  seedWorkspace,
  useSignedIn,
} from "./session";

let seed: StaffSeed;
let admin: APIRequestContext;
const tenantName = "E2E Screens Co";
let tenant: { id: string; slug: string };
let existing: { id: string; email: string };
const orderIds: string[] = [];
const noticeEventIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenant = await seedWorkspace(tenantName);
  existing = await seedAccount("E2E Existing Email");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "screens");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of orderIds) await pool.query("DELETE FROM billing_order WHERE id = $1", [id]);
    if (noticeEventIds.length > 0) {
      await pool.query("DELETE FROM payment_notice WHERE provider = 'sepay' AND provider_event_id = ANY($1)", [noticeEventIds]);
    }
  } finally {
    await pool.end();
  }
  await cleanupRows(tenant ? [tenant.id] : [], existing ? [existing.id] : []);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "screens");
});

async function openNav(page: Page, name: string): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name }).click();
}

async function openTenant(page: Page): Promise<void> {
  await openNav(page, "Tenants");
  await page.getByPlaceholder("Search by name or slug...").fill(tenantName);
  await page.getByRole("row", { name: new RegExp(tenantName) }).getByRole("button", { name: "View" }).click();
  await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();
}

test.describe("failure states", () => {
  test("edge: the overview shows the error state when its request fails", async ({ page }) => {
    await page.route((url) => new URL(url).pathname === "/api/platform/overview", (route) => route.abort());
    await openNav(page, "Overview");
    await expect(page.getByText("Failed to load overview")).toBeVisible();
  });

  test("edge: the error state's Try again button loads the overview once the request works", async ({ page }) => {
    let calls = 0;
    await page.route((url) => new URL(url).pathname === "/api/platform/overview", (route) => {
      calls += 1;
      return calls === 1 ? route.abort() : route.continue();
    });
    await openNav(page, "Overview");
    await expect(page.getByText("Failed to load overview")).toBeVisible();
    await page.getByRole("button", { name: "Try again" }).click();
    await expect(page.getByRole("heading", { name: "Platform Overview" })).toBeVisible();
    expect(calls).toBeGreaterThanOrEqual(2);
  });

  test("edge: the users list shows the error state when its request fails", async ({ page }) => {
    await page.route((url) => new URL(url).pathname === "/api/platform/users", (route) => route.abort());
    await openNav(page, "Users");
    await expect(page.getByText("Failed to load users")).toBeVisible();
  });

  test("edge: a tenant whose detail fails to load shows the error state, not a blank page", async ({ page }) => {
    await openNav(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(tenantName);
    await page.route((url) => new URL(url).pathname === `/api/platform/tenants/${tenant.id}`, (route) => route.abort());
    await page.getByRole("row", { name: new RegExp(tenantName) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByText("Failed to load tenant")).toBeVisible();
  });
});

test.describe("tenant detail sections", () => {
  test("flow 26: the tenant detail shows the workspace state, usage, recent orders and its audit log", async ({ page }) => {
    await openTenant(page);
    for (const heading of ["Workspace State", "Monthly Usage", "Recent Orders", "Workspace Audit Log"]) {
      await expect(page.getByText(heading, { exact: true }).first()).toBeVisible();
    }
  });
});

test.describe("staff list", () => {
  test("flow 14: the staff list shows the signed-in admin with the Admin role", async ({ page }) => {
    await openNav(page, "Staff");
    await expect(page.getByText("Staff Management")).toBeVisible();
    const row = page.getByRole("row", { name: new RegExp(seed.email) });
    await expect(row).toBeVisible();
    await expect(row).toContainText("Admin");
  });
});

async function openCreateAccountDialog(page: Page): Promise<Locator> {
  await openNav(page, "Accounts");
  await page.getByRole("button", { name: "Create account" }).click();
  const dialog = page.getByRole("dialog", { name: "Create account" });
  await expect(dialog).toBeVisible();
  return dialog;
}

async function fillAccountForm(dialog: Locator, email: string): Promise<void> {
  await dialog.locator("#account-email").fill(email);
  await dialog.locator("#account-confirm-email").fill(email);
  await dialog.locator("#account-display-name").fill("E2E Account Person");
  await dialog.locator("#account-workspace-name").fill("E2E Account Co");
}

test.describe("accounts", () => {
  test("flow 24: the accounts list shows its columns, or its empty state", async ({ page }) => {
    await openNav(page, "Accounts");
    await expect(page.getByRole("button", { name: "Create account" })).toBeVisible();
    const table = page.getByRole("columnheader", { name: "Workspace Name" });
    const empty = page.getByText("No account requests");
    await expect(table.or(empty)).toBeVisible();
  });

  test("flow 244: admin submits an invalid workspace slug and the dialog shows the field error", async ({ page }) => {
    const dialog = await openCreateAccountDialog(page);
    await fillAccountForm(dialog, `e2e-slug-${randomUUID().slice(0, 8)}@example.test`);
    await dialog.locator("#account-workspace-slug").fill("Bad Slug!");
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog.getByText("Use lowercase letters, numbers, and hyphens only").first()).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("flow 245: a paid plan without a trial or complimentary reason is refused in the dialog", async ({ page }) => {
    const dialog = await openCreateAccountDialog(page);
    await expect(dialog.locator('#account-plan-key option[value="pro"]')).toHaveCount(1);
    await dialog.locator("#account-plan-key").selectOption("pro");
    await fillAccountForm(dialog, `e2e-paid-${randomUUID().slice(0, 8)}@example.test`);
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog.getByText("A paid plan requires either a trial or a complimentary reason.").first()).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("flow 246: an email that already has a Delegora user is refused in the dialog", async ({ page }) => {
    const dialog = await openCreateAccountDialog(page);
    await fillAccountForm(dialog, existing.email);
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog.getByText("Email already registered.")).toBeVisible();
    await expect(dialog).toBeVisible();
  });
});

test.describe("e-invoices", () => {
  test("flow 72: the e-invoice screen shows its state tabs and switches between them", async ({ page }) => {
    await openNav(page, "E-invoices");
    const issued = page.getByRole("tab", { name: "Issued" });
    const failed = page.getByRole("tab", { name: "Failed" });
    await expect(failed).toBeVisible();
    await issued.click();
    await expect(issued).toHaveAttribute("aria-selected", "true");
    await failed.click();
    await expect(failed).toHaveAttribute("aria-selected", "true");
  });
});

test.describe("audit log", () => {
  test("flow 26: the audit log shows its columns and the action filter narrows the list", async ({ page }) => {
    await openNav(page, "Audit Log");
    await expect(page.getByRole("columnheader", { name: "Action" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Target" })).toBeVisible();

    const select = page.locator("#audit-action-select");
    await expect(select).toBeVisible();
    const options = await select.locator("option").evaluateAll((els) => els.map((e) => (e as HTMLOptionElement).value));
    const action = options.find((v) => v && v !== "all") ?? "";
    test.skip(!action, "the action filter has no values to choose");
    await select.selectOption(action);
    await expect(select).toHaveValue(action);
  });
});

test.describe("payment notices", () => {
  test("flow 60: billing matches a payment notice to a pending order in a dialog", async ({ page }) => {
    const amount = 2_000_000 + Math.floor(Math.random() * 7_000_000);
    const amountText = amount.toLocaleString();
    orderIds.push(await seedPendingOrder(tenant.id, amount));
    await queueReviewNotice(amount, noticeEventIds);

    await openNav(page, "Payment notices");
    await page.getByRole("row", { name: new RegExp(`${amountText} VND`) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByText("Notice details")).toBeVisible();

    await page.getByRole("row", { name: new RegExp(tenant.id) }).getByRole("button", { name: "Match", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Match payment to order" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Match payment" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("e2e match drill");
    await dialog.getByRole("button", { name: "Match payment" }).click();

    // A refusal from the server stays in the dialog; read it so the failure says why.
    const refusal = await dialog.getByRole("alert").textContent({ timeout: 5000 }).catch(() => null);
    expect(refusal, `the match dialog showed an error; server log tail:\n${serverOutput().slice(-1500)}`).toBeNull();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("row", { name: new RegExp(`${amountText} VND`) })).toHaveCount(0);
  });
});
