/**
 * Reusable helper: sign in once through the real form, save the session
 * cookies, and add them to each test's page. Adjust the selectors to the app's
 * labels (read the sign-in component first).
 */
import { mkdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { Browser, BrowserContext, Page } from "@playwright/test";

type Cookies = Parameters<BrowserContext["addCookies"]>[0];

const STATE_DIR = resolve(process.cwd(), "test-results", "ui-e2e");

export function stateFile(name: string): string {
  mkdirSync(STATE_DIR, { recursive: true });
  return resolve(STATE_DIR, `session-${name}.json`);
}

/** Save the cookies of a signed-in session. Call from beforeAll with the browser fixture. */
export async function saveSignedInState(
  browser: Browser,
  name: string,
  signIn: (page: Page) => Promise<void>,
): Promise<string> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signIn(page);
  const path = stateFile(name);
  await context.storageState({ path });
  await context.close();
  return path;
}

/**
 * Add the saved cookies to the page's context. Call from beforeEach. Do not use
 * test.use({ storageState }) with a file written in beforeAll: the file may not
 * exist yet when the first test's context is created.
 */
export async function useSignedIn(page: Page, name: string): Promise<void> {
  const state = JSON.parse(readFileSync(stateFile(name), "utf8")) as { cookies: Cookies };
  await page.context().addCookies(state.cookies);
}
