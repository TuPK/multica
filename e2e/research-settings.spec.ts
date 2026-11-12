import { expect, test } from "@playwright/test";
import { loginAsDefault } from "./helpers";

/**
 * Settings > Research sources. The page is real; the research endpoints are mocked at the network
 * boundary, so the spec writes nothing to the workspace and never needs a sealing secret on the
 * server. It checks what the unit tests of the tab cannot: that the tab is registered in the settings
 * page, reachable by `?tab=research`, saves only what was typed, and never shows a key back.
 */

const SETTINGS = {
  openalex_configured: false,
  semantic_scholar_configured: false,
  contact_email: "",
  keys_can_be_stored: true,
  updated_at: null,
};

test.describe("Research sources settings", () => {
  test("saves an OpenAlex key without ever showing it again", async ({ page }) => {
    let stored = { ...SETTINGS };
    const puts: Record<string, unknown>[] = [];
    await page.route("**/api/research/settings", async (route) => {
      const req = route.request();
      if (req.method() === "PUT") {
        const body = req.postDataJSON() as Record<string, string>;
        puts.push(body);
        stored = {
          ...stored,
          openalex_configured: body.openalex_api_key ? true : stored.openalex_configured,
          contact_email: body.contact_email ?? stored.contact_email,
        };
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(stored) });
    });

    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/settings?tab=research`, { waitUntil: "domcontentloaded" });

    const key = page.getByLabel("OpenAlex API key");
    await expect(key).toBeVisible({ timeout: 30000 });
    await expect(key).toHaveAttribute("type", "password");
    await expect(page.getByText("Not set").first()).toBeVisible();

    await key.fill("OA-E2E-SECRET");
    await page.getByRole("button", { name: "Save" }).first().click();

    await expect(page.getByText("Configured").first()).toBeVisible();
    expect(puts).toEqual([{ openalex_api_key: "OA-E2E-SECRET" }]);
    await expect(key).toHaveValue("");
    await expect(page.locator("body")).not.toContainText("OA-E2E-SECRET");
  });

  test("tells the operator when the server cannot store a key", async ({ page }) => {
    await page.route("**/api/research/settings", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ...SETTINGS, keys_can_be_stored: false }),
      }),
    );

    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/settings?tab=research`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("status")).toContainText("This server cannot store API keys", { timeout: 30000 });
    await expect(page.getByLabel("OpenAlex API key")).toBeDisabled();
  });
});
