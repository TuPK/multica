/**
 * Staff sign-in through the console form, as a person would use it: what the
 * form shows, what it refuses, and how the session ends. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, test } from "@playwright/test";
import { CONSOLE_BASE, devVerificationCode, startPlatform, stopPlatform, type StaffSeed } from "../harness";
import { signInThroughForm } from "./session";

const NEUTRAL = "If this address can sign in, a code has been sent";
const INVALID = "That code is not valid.";

let seed: StaffSeed;

test.beforeAll(async () => {
  seed = await startPlatform("admin");
});

test.afterAll(async () => {
  await stopPlatform(seed ?? null);
});

test.describe("sign-in form", () => {
  test("flow 12/13: a staff member signs in, reloads the page, and signs out", async ({ page }) => {
    test.setTimeout(120_000);
    await signInThroughForm(page, seed);

    const nav = page.getByRole("navigation", { name: "Main Navigation" });
    for (const item of ["Overview", "Tenants", "Users", "Orders", "Payment notices", "Accounts", "Plans", "Staff", "Audit Log"]) {
      await expect(nav.getByRole("button", { name: item })).toBeVisible();
    }

    await page.reload();
    await expect(page.getByText("Platform Overview").first()).toBeVisible();

    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByText("Platform Console", { exact: true })).toBeVisible();
    await expect(page.locator("#email-input")).toBeVisible();
    await page.reload();
    await expect(page.locator("#email-input")).toBeVisible();
  });

  test("flow 142: the send-code button stays disabled for an empty or blank email", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    const send = page.getByRole("button", { name: "Send sign-in code" });
    await expect(send).toBeDisabled();
    await page.locator("#email-input").fill("   ");
    await expect(send).toBeDisabled();
  });

  test("flow 142: a malformed email is stopped by the form and no code step appears", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill("not-an-email");
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await expect(page.locator("#code-input")).toHaveCount(0);
    await expect(page.getByRole("status")).toHaveCount(0);
  });

  test("flow 142: an unknown email and a known email get the same neutral message", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(`nobody-${Date.now()}@example.test`);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await expect(page.getByRole("status")).toHaveText(NEUTRAL);
    await expect(page.locator("#code-input")).toBeVisible();

    await page.getByRole("button", { name: "Use a different email" }).click();
    await page.locator("#email-input").fill(seed.email);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await expect(page.getByRole("status")).toHaveText(NEUTRAL);
    await expect(page.locator("#code-input")).toBeVisible();
  });

  test("flow 143: a wrong email code shows the invalid-code message and stays on the code step", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(seed.email);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    const wrong = devVerificationCode() === "000000" ? "111111" : "000000";
    await page.locator("#code-input").fill(wrong);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert")).toHaveText(INVALID);
    await expect(page.locator("#code-input")).toBeVisible();
  });

  test("flow 143: a code with letters or a short code is refused with the invalid-code message", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(seed.email);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await page.locator("#code-input").fill("ab12");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert")).toHaveText(INVALID);
  });

  test('flow 143: Enter in the email field sends the code, and "Use a different email" returns to the email step with the address kept', async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(seed.email);
    await page.locator("#email-input").press("Enter");
    await expect(page.locator("#code-input")).toBeVisible();

    await page.getByRole("button", { name: "Use a different email" }).click();
    await expect(page.locator("#email-input")).toHaveValue(seed.email);
    await expect(page.locator("#code-input")).toHaveCount(0);
  });

  test("flow 143: Start over on the authenticator step returns to the email step", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(seed.email);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await page.locator("#code-input").fill(devVerificationCode());
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.locator("#totp-input")).toBeVisible();
    await page.getByRole("button", { name: "Start over" }).click();
    await expect(page.locator("#email-input")).toBeVisible();
    await expect(page.locator("#totp-input")).toHaveCount(0);
  });

  test("flow 143: a wrong authenticator code after a correct email code shows the invalid-code message", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.locator("#email-input").fill(seed.email);
    await page.getByRole("button", { name: "Send sign-in code" }).click();
    await page.locator("#code-input").fill(devVerificationCode());
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.locator("#totp-input")).toBeVisible();

    await page.locator("#totp-input").fill("000000");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert")).toHaveText(INVALID);
    await expect(page.locator("#totp-input")).toBeVisible();
  });
});
