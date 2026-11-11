import { expect, test, type Page } from "@playwright/test";
import { TestApiClient } from "./fixtures";

// A fixed user and workspace, as loginAsDefault does, so repeated runs reuse
// one workspace. Each run still imports a skill with its own name and removes it.
const HUB_E2E_EMAIL = "e2e-hub@multica.ai";
const HUB_E2E_NAME = "E2E Hub User";
const HUB_E2E_WORKSPACE_SLUG = "e2e-hub-workspace";

// CRC-32 (IEEE), the checksum every zip entry carries.
const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(data: Buffer): number {
  let crc = 0xffffffff;
  for (const byte of data) crc = CRC_TABLE[(crc ^ byte) & 0xff]! ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

/** A zip with stored (uncompressed) entries, built in memory so the spec needs no binary fixture. */
function buildZip(files: Record<string, string>): Buffer {
  const locals: Buffer[] = [];
  const centrals: Buffer[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(files)) {
    const nameBuf = Buffer.from(name, "utf8");
    const data = Buffer.from(text, "utf8");
    const crc = crc32(data);

    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4); // version needed
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(nameBuf.length, 26);
    locals.push(local, nameBuf, data);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(20, 4); // version made by
    central.writeUInt16LE(20, 6); // version needed
    central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(nameBuf.length, 28);
    central.writeUInt32LE(offset, 42);
    centrals.push(central, nameBuf);

    offset += local.length + nameBuf.length + data.length;
  }

  const centralBytes = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(Object.keys(files).length, 8);
  end.writeUInt16LE(Object.keys(files).length, 10);
  end.writeUInt32LE(centralBytes.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, centralBytes, end]);
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;

interface HubE2eContext {
  slug: string;
  token: string;
  api: TestApiClient;
}

/** Signs in, opens the workspace, and returns what the tests need to script the API. */
async function signIn(page: Page): Promise<HubE2eContext> {
  const api = new TestApiClient();
  await api.login(HUB_E2E_EMAIL, HUB_E2E_NAME);
  const workspace = await api.ensureWorkspace("E2E Hub Workspace", HUB_E2E_WORKSPACE_SLUG);
  await api.markUserOnboarded();
  const token = api.getToken();
  if (!token) throw new Error("E2E login did not return an auth token");
  await page.addInitScript((t) => {
    localStorage.setItem("multica_token", t);
    localStorage.setItem("multica:chat:isOpen", "false");
  }, token);
  return { slug: workspace.slug, token, api };
}

/** Imports one skill from an uploaded zip through the Add wizard, and returns its skill id. */
async function importSkill(page: Page, slug: string, skillName: string, original: string): Promise<string> {
  await page.goto(`/${slug}/hub`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Add", exact: true }).click();
  const wizard = page.getByRole("dialog");
  await wizard.getByRole("tab", { name: "Upload" }).click();
  await wizard.locator('input[type="file"][accept*="zip"]').setInputFiles({
    name: `${skillName}.zip`,
    mimeType: "application/zip",
    buffer: buildZip({ "SKILL.md": original }),
  });
  await expect(wizard.getByRole("heading", { name: skillName })).toBeVisible();
  await wizard.getByRole("button", { name: "Import", exact: true }).click();

  // The commit opens the new item; its id is the skill id the teardown removes.
  await page.waitForURL(new RegExp(`/${slug}/hub/skill/[^/?#]+`));
  const skillId = page.url().match(/\/hub\/skill\/([^/?#]+)/)?.[1] ?? null;
  if (!skillId) throw new Error("The import did not open the new skill");
  return skillId;
}

test.describe("hub skill import, edit, and version", () => {
  test("imports a skill from a zip, saves an edit, and sees the new version", async ({ page }) => {
    const { slug, api } = await signIn(page);
    const skillName = `hub-e2e-${Date.now().toString(36)}`;
    const original = `---\nname: ${skillName}\ndescription: Imported by the Hub e2e test\n---\n\n# ${skillName}\n`;
    const edited = "Edited in the Hub e2e test.";
    let skillId: string | null = null;

    try {
      skillId = await importSkill(page, slug, skillName, original);

      // Edit SKILL.md and save it.
      await page.getByRole("treeitem", { name: "SKILL.md" }).click();
      await page.getByRole("button", { name: "Edit", exact: true }).click();
      const editor = page.getByRole("textbox", { name: "Contents of SKILL.md" });
      await editor.fill(`${original}\n${edited}\n`);
      await page.getByRole("button", { name: "Save", exact: true }).click();
      await expect(page.getByText("Saved SKILL.md")).toBeVisible();

      // Every save records a version: the import is v1 and the save is v2.
      await page.getByRole("tab", { name: "Versions" }).click();
      const versions = page.getByRole("tablist", { name: "Skill versions" });
      await expect(versions.getByRole("tab", { name: "v2" })).toBeVisible();
      await expect(versions.getByRole("tab", { name: "v1" })).toBeVisible();
      await versions.getByRole("tab", { name: "v2" }).click();
      await expect(page.getByText(edited)).toBeVisible();
    } finally {
      if (skillId) await api.deleteSkill(skillId);
    }
  });

  test("filters and sorts the Hub list, and keeps the view preference after a reload", async ({ page }) => {
    const { slug, api } = await signIn(page);
    const skillName = `hub-e2e-filter-${Date.now().toString(36)}`;
    const original = `---\nname: ${skillName}\ndescription: Filter check\n---\n\n# ${skillName}\n`;
    let skillId: string | null = null;

    try {
      skillId = await importSkill(page, slug, skillName, original);
      await page.goto(`/${slug}/hub`, { waitUntil: "domcontentloaded" });
      const row = page.getByRole("link", { name: skillName });
      await expect(row).toBeVisible();

      // The kind control narrows to plugins, which hides the imported skill.
      await page.getByRole("button", { name: "Plugins", exact: true }).click();
      await expect(row).toBeHidden();
      await page.getByRole("button", { name: "All", exact: true }).click();
      await expect(row).toBeVisible();

      // A source filter keeps the upload, and the count reports what matched.
      await page.getByRole("button", { name: /^Filters/ }).click();
      await page.getByRole("menuitemcheckbox", { name: "Upload" }).click();
      await page.keyboard.press("Escape");
      await expect(page.locator('span[aria-live="polite"]')).toHaveText(/^\d+ of \d+$/);
      await expect(row).toBeVisible();
      await page.getByRole("button", { name: "Clear filters" }).click();
      await expect(page.getByRole("button", { name: "Clear filters" })).toBeHidden();

      // The chosen sort survives a reload, because it is saved on this device.
      await page.getByRole("button", { name: /^Sort:/ }).click();
      await page.getByRole("menuitemradio", { name: "Name" }).click();
      await expect(page.getByRole("button", { name: /^Sort:/ })).toContainText("Name");
      await page.reload({ waitUntil: "domcontentloaded" });
      await expect(page.getByRole("button", { name: /^Sort:/ })).toContainText("Name");
    } finally {
      if (skillId) await api.deleteSkill(skillId);
    }
  });

  test("warns when the skill changed on the server, and overwrites on request", async ({ page, request }) => {
    const { slug, token, api } = await signIn(page);
    const skillName = `hub-e2e-conflict-${Date.now().toString(36)}`;
    const original = `---\nname: ${skillName}\ndescription: Conflict check\n---\n\n# ${skillName}\n`;
    let skillId: string | null = null;

    try {
      skillId = await importSkill(page, slug, skillName, original);
      await page.getByRole("treeitem", { name: "SKILL.md" }).click();
      await page.getByRole("button", { name: "Edit", exact: true }).click();
      await page.getByRole("textbox", { name: "Contents of SKILL.md" }).fill(`${original}\nLocal edit.\n`);

      // Another client saves the skill while this editor is open.
      const remote = await request.put(`${API_BASE}/api/skills/${skillId}`, {
        headers: { Authorization: `Bearer ${token}`, "X-Workspace-Slug": slug },
        data: { content: `${original}\nChanged elsewhere.\n` },
      });
      expect(remote.ok()).toBeTruthy();

      await page.getByRole("button", { name: "Save", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "SKILL.md changed on the server" });
      await expect(dialog).toBeVisible();
      await dialog.getByRole("button", { name: "Overwrite" }).click();
      await expect(page.getByText("Saved SKILL.md")).toBeVisible();
    } finally {
      if (skillId) await api.deleteSkill(skillId);
    }
  });
});
