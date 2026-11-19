/**
 * Staff actions on user accounts and on the staff list: suspend and reactivate,
 * revoke sessions, reset two-step sign-in, add and remove staff, and the
 * account_locked refusal. Each test seeds its own target user. Flow numbers
 * refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type APIResponse } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";
import { startFakeSmtp, type FakeSmtp } from "./fake-smtp";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };

let seed: StaffSeed;
let admin: APIRequestContext;
let mail: FakeSmtp;
const created: string[] = [];

test.beforeAll(async () => {
  test.setTimeout(300_000);
  mail = await startFakeSmtp();
  seed = await startPlatform("admin", { smtpPort: mail.port });
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  await mail?.close();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of created) {
      await pool.query("DELETE FROM platform_staff WHERE user_id = $1", [id]);
      await pool.query("DELETE FROM platform_account WHERE user_id = $1", [id]);
      await pool.query("DELETE FROM user_mfa WHERE user_id = $1", [id]);
      await pool.query(`DELETE FROM "user" WHERE id = $1`, [id]);
    }
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

/** Inserts a Multica account the console can act on. Returns its id and email. */
async function seedUser(withMfa = false): Promise<{ id: string; email: string }> {
  const id = randomUUID();
  const email = `e2e-target-${id.slice(0, 8)}@example.test`;
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO "user" (id, name, email, avatar_url, created_at, updated_at, timezone)
       VALUES ($1, 'E2E Target', $2, '', now(), now(), 'UTC')`,
      [id, email],
    );
    if (withMfa) {
      // The reset deletes the row without reading the secret, so placeholder bytes are enough.
      await pool.query(
        `INSERT INTO user_mfa (user_id, secret_enc, confirmed_at, recovery_hashes, last_used_step, created_at)
         VALUES ($1, '\\x00'::bytea, now(), '{}', 0, now())`,
        [id],
      );
    }
  } finally {
    await pool.end();
  }
  created.push(id);
  return { id, email };
}

async function setAccountStatus(id: string, status: string): Promise<void> {
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO platform_account (user_id, status, reason, changed_by) VALUES ($1, $2, 'e2e', NULL)
       ON CONFLICT DO NOTHING`,
      [id, status],
    );
    await pool.query("UPDATE platform_account SET status = $2 WHERE user_id = $1", [id, status]);
  } finally {
    await pool.end();
  }
}

function post(path: string, data: unknown): Promise<APIResponse> {
  return admin.post(`${CONSOLE_BASE}${path}`, { headers: ORIGIN, data: data as object });
}

test.describe("account status", () => {
  test("flow 21: staff suspends an account, then reactivates it, and a repeat is a no-op", async () => {
    const target = await seedUser();

    const suspend = await post(`/api/platform/users/${target.id}/status`, { status: "suspended", reason: "e2e suspend" });
    expect(suspend.status()).toBe(200);
    expect(await suspend.json()).toEqual({ changed: true, status: "suspended" });

    const reactivate = await post(`/api/platform/users/${target.id}/status`, { status: "active", reason: "e2e reactivate" });
    expect(reactivate.status()).toBe(200);
    expect(await reactivate.json()).toEqual({ changed: true, status: "active" });

    const repeat = await post(`/api/platform/users/${target.id}/status`, { status: "active", reason: "e2e repeat" });
    expect(await repeat.json()).toEqual({ changed: false, status: "active" });
  });

  test("flow 254: an account that is pending deletion or deleted cannot be changed", async () => {
    const target = await seedUser();
    await setAccountStatus(target.id, "deleted");
    const res = await post(`/api/platform/users/${target.id}/status`, { status: "active", reason: "e2e" });
    expect(res.status()).toBe(409);
    expect((await res.json()).code).toBe("account_locked");
  });
});

test.describe("sessions and two-step sign-in", () => {
  test("flow 22: staff revokes every session of a user", async () => {
    const target = await seedUser();
    const res = await post(`/api/platform/users/${target.id}/revoke-sessions`, { reason: "e2e revoke" });
    expect(res.status()).toBe(200);
  });

  test("flow 23: staff resets a user's two-step sign-in and the row is removed", async () => {
    const target = await seedUser(true);
    const res = await post(`/api/platform/users/${target.id}/mfa-reset`, { reason: "e2e reset" });
    expect(res.status()).toBe(200);
    expect(await res.json()).toEqual({ reset: true });

    const again = await post(`/api/platform/users/${target.id}/mfa-reset`, { reason: "e2e reset again" });
    expect(await again.json()).toEqual({ reset: false });
  });

  test("flow 23: the reset emails the user that two-step sign-in was turned off", async () => {
    // The fake SMTP server (fake-smtp.ts) records what the platform sends; the user's inbox is that record.
    const target = await seedUser(true);
    const before = mail.sent.length;
    const res = await post(`/api/platform/users/${target.id}/mfa-reset`, { reason: "e2e reset email" });
    expect(res.status()).toBe(200);
    const sent = mail.sent.slice(before).filter((m) => m.to === target.email);
    expect(sent.length, "no reset notice reached the user").toBe(1);
    expect(sent[0].data).toContain("Subject: Two-step sign-in was turned off on your Multica account");
  });
});

test.describe("staff list", () => {
  test("flow 14: an admin adds a staff member, sees them listed, and revokes them", async () => {
    const target = await seedUser();

    const added = await post("/api/platform/staff", { email: target.email, role: "billing" });
    expect(added.status()).toBe(201);
    expect((await added.json()).role).toBe("billing");

    const list = await admin.get(`${CONSOLE_BASE}/api/platform/staff`);
    expect(list.status()).toBe(200);
    expect(JSON.stringify(await list.json())).toContain(target.email);

    const removed = await admin.delete(`${CONSOLE_BASE}/api/platform/staff/${target.id}`, { headers: { Origin: CONSOLE_BASE } });
    expect(removed.status()).toBeLessThan(300);
  });
});
