import { expect, test } from "@playwright/test";
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

test.describe("hub skill import, edit, and version", () => {
  test("imports a skill from a zip, saves an edit, and sees the new version", async ({ page }) => {
    const api = new TestApiClient();
    await api.login(HUB_E2E_EMAIL, HUB_E2E_NAME);
    const workspace = await api.ensureWorkspace("E2E Hub Workspace", HUB_E2E_WORKSPACE_SLUG);
    await api.markUserOnboarded();
    const token = api.getToken();
    if (!token) throw new Error("E2E login did not return an auth token");

    const skillName = `hub-e2e-${Date.now().toString(36)}`;
    const original = `---\nname: ${skillName}\ndescription: Imported by the Hub e2e test\n---\n\n# ${skillName}\n`;
    const edited = "Edited in the Hub e2e test.";
    let skillId: string | null = null;

    try {
      await page.addInitScript((t) => {
        localStorage.setItem("multica_token", t);
        localStorage.setItem("multica:chat:isOpen", "false");
      }, token);
      await page.goto(`/${workspace.slug}/hub`, { waitUntil: "domcontentloaded" });

      // Import from an uploaded zip: the Upload tab's archive picker takes the file.
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
      await page.waitForURL(new RegExp(`/${workspace.slug}/hub/skill/[^/?#]+`));
      skillId = page.url().match(/\/hub\/skill\/([^/?#]+)/)?.[1] ?? null;
      expect(skillId).not.toBeNull();

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
});
