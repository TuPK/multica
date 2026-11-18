/**
 * Staff console requests that the server refuses: the staff gate, the reason
 * and body checks, the query checks, the self-target and staff-role rules, and
 * the order and payment-notice id checks. Each test sends the request as a
 * signed-in admin (or without a session where the flow says so) and checks the
 * status and the error code. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type APIResponse } from "@playwright/test";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, type StaffSeed } from "./harness";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };
const UNKNOWN_ID = "00000000-0000-0000-0000-00000000dead";

let seed: StaffSeed;
let admin: APIRequestContext;

test.beforeAll(async () => {
  seed = await startPlatform("admin");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  await admin?.dispose();
  await stopPlatform(seed ?? null);
});

function post(path: string, data: unknown): Promise<APIResponse> {
  return admin.post(`${CONSOLE_BASE}${path}`, { headers: ORIGIN, data: data as object });
}

function postRaw(path: string, raw: string): Promise<APIResponse> {
  return admin.post(`${CONSOLE_BASE}${path}`, { headers: ORIGIN, data: raw });
}

async function expectCode(res: APIResponse, status: number, code: string): Promise<void> {
  expect(res.status()).toBe(status);
  expect((await res.json()).code).toBe(code);
}

test.describe("staff gate", () => {
  test("flow 146: a caller without a staff session gets the 404 of an unknown route", async ({ request }) => {
    const res = await request.get(`${CONSOLE_BASE}/api/platform/tenants`);
    expect(res.status()).toBe(404);
  });

  test("flow 146: a wrong method on a console route gets the 404 of an unknown route", async () => {
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/tenants`, { headers: ORIGIN, data: {} });
    expect(res.status()).toBe(404);
  });
});

test.describe("reason and note checks", () => {
  test("flow 260: a status change without a reason is refused with reason_required", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/status`, { status: "read_only" }), 400, "reason_required");
  });

  test("flow 260: a blank reason is refused with reason_required", async () => {
    await expectCode(
      await post(`/api/platform/tenants/${UNKNOWN_ID}/status`, { status: "read_only", reason: "   " }),
      400,
      "reason_required",
    );
  });

  test("flow 261: a reason over 1000 characters is refused with reason_too_long", async () => {
    await expectCode(
      await post(`/api/platform/tenants/${UNKNOWN_ID}/status`, { status: "read_only", reason: "x".repeat(1001) }),
      400,
      "reason_too_long",
    );
  });

  test("flow 263: an empty internal tenant note is refused with body_required", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/notes`, { body: "   " }), 400, "body_required");
  });

  test("flow 264: an internal tenant note over 4000 characters is refused with body_too_long", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/notes`, { body: "x".repeat(4001) }), 400, "body_too_long");
  });
});

test.describe("body checks", () => {
  test("flow 265: a body that is not JSON is refused with invalid_body", async () => {
    await expectCode(await postRaw(`/api/platform/tenants/${UNKNOWN_ID}/notes`, "{"), 400, "invalid_body");
  });

  test("flow 265: trailing data after the JSON object is refused with invalid_body", async () => {
    await expectCode(await postRaw(`/api/platform/tenants/${UNKNOWN_ID}/notes`, '{"body":"a"} {}'), 400, "invalid_body");
  });

  test("flow 265: an unknown field is refused with invalid_body", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/notes`, { body: "a", extra: 1 }), 400, "invalid_body");
  });

  test("flow 265: a NUL byte in the body is refused with invalid_body", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/notes`, { body: "a\u0000b" }), 400, "invalid_body");
  });
});

test.describe("query and id checks", () => {
  test("flow 266: a limit outside 1 to the maximum is refused with invalid_filter", async () => {
    await expectCode(await admin.get(`${CONSOLE_BASE}/api/platform/tenants?limit=0`), 400, "invalid_filter");
  });

  test("flow 267: a malformed cursor is refused with invalid_cursor", async () => {
    await expectCode(await admin.get(`${CONSOLE_BASE}/api/platform/tenants?cursor=not-a-cursor`), 400, "invalid_cursor");
  });

  test("flow 268: a malformed payment notice id is refused with invalid_id", async () => {
    await expectCode(await admin.get(`${CONSOLE_BASE}/api/platform/payment-notices/not-a-uuid`), 400, "invalid_id");
  });

  test("flow 269: an order_id that is not a UUID is refused with invalid_order_id", async () => {
    await expectCode(
      await post(`/api/platform/payment-notices/${UNKNOWN_ID}/resolve`, { order_id: "not-a-uuid", reason: "e2e" }),
      400,
      "invalid_order_id",
    );
  });

  test("flow 270: an unknown tenant status is refused with invalid_status", async () => {
    await expectCode(await post(`/api/platform/tenants/${UNKNOWN_ID}/status`, { status: "bogus", reason: "e2e" }), 400, "invalid_status");
  });

  test("flow 270: an unknown order status filter is refused with invalid_status", async () => {
    await expectCode(await admin.get(`${CONSOLE_BASE}/api/platform/orders?status=bogus`), 400, "invalid_status");
  });
});

test.describe("order checks", () => {
  test("flow 262: a transfer reference over 100 characters is refused with transfer_reference_too_long", async () => {
    await expectCode(
      await post(`/api/platform/orders/${UNKNOWN_ID}/confirm`, { reason: "e2e", transfer_reference: "x".repeat(101) }),
      400,
      "transfer_reference_too_long",
    );
  });

  test("flow 222: confirming an order id that does not exist is refused with order_not_found", async () => {
    await expectCode(
      await post(`/api/platform/orders/${UNKNOWN_ID}/confirm`, { reason: "e2e", transfer_reference: "E2E-REF-1" }),
      404,
      "order_not_found",
    );
  });
});

test.describe("staff rules on the admin's own account", () => {
  test("flow 252: staff cannot change their own account status", async () => {
    await expectCode(
      await post(`/api/platform/users/${seed.userId}/status`, { status: "suspended", reason: "e2e" }),
      400,
      "self_target",
    );
  });

  test("flow 253: staff cannot reset their own two-step sign-in", async () => {
    await expectCode(await post(`/api/platform/users/${seed.userId}/mfa-reset`, { reason: "e2e" }), 403, "self_reset_refused");
  });
});

test.describe("staff membership checks", () => {
  test("flow 255: adding an email with no Multica account is refused with no_such_account", async () => {
    await expectCode(
      await post("/api/platform/staff", { email: `nobody-${Date.now()}@example.test`, role: "support" }),
      404,
      "no_such_account",
    );
  });

  test("flow 256: adding a user who is already active staff is refused with already_staff", async () => {
    await expectCode(await post("/api/platform/staff", { email: seed.email, role: "support" }), 409, "already_staff");
  });

  test("flow 258: an unknown staff role is refused with invalid_role", async () => {
    await expectCode(await post("/api/platform/staff", { email: seed.email, role: "owner" }), 400, "invalid_role");
  });

  test("flow 259: a malformed staff email is refused with invalid_email", async () => {
    await expectCode(await post("/api/platform/staff", { email: "not-an-email", role: "support" }), 400, "invalid_email");
  });

  test("flow 259: a staff email over 254 characters is refused with invalid_email", async () => {
    const long = `${"a".repeat(250)}@example.test`;
    await expectCode(await post("/api/platform/staff", { email: long, role: "support" }), 400, "invalid_email");
  });
});

test.describe("price schedule guard", () => {
  test("flow 233: a PUT to a plan with a price field is refused with price_field_via_schedule", async () => {
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/plans/e2e-plan`, {
      headers: ORIGIN,
      data: { seat_price_month: 1000 },
    });
    await expectCode(res, 400, "price_field_via_schedule");
  });
});
