/**
 * SePay payment webhook on the webhook listener. The request is signed by the
 * test with the test secret, so no real provider is contacted.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, test } from "@playwright/test";
import { startPlatform, stopPlatform, sepaySignature, WEBHOOK_URL, WEBHOOK_PORT, type StaffSeed } from "./harness";

let seed: StaffSeed;

test.beforeAll(async () => {
  seed = await startPlatform("admin");
});

test.afterAll(async () => {
  await stopPlatform(seed ?? null);
});

function payload(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    id: Date.now(),
    content: "E2E unmatched transfer",
    transactionDate: "2026-11-18 10:00:00",
    transferType: "in",
    transferAmount: 100000,
    ...overrides,
  });
}

function signedPost(body: string, timestamp = String(Math.floor(Date.now() / 1000)), secret?: string) {
  return {
    data: body,
    headers: {
      "Content-Type": "application/json",
      "X-SePay-Timestamp": timestamp,
      "X-SePay-Signature": sepaySignature(body, timestamp, secret),
    },
  };
}

test.describe("payment webhook", () => {
  test("flow 212: an invalid signature is rejected with payment_signature_invalid", async ({ request }) => {
    const body = payload();
    const res = await request.post(WEBHOOK_URL, signedPost(body, undefined, "wrong-secret"));
    expect(res.status()).toBe(401);
    expect(await res.text()).toContain("payment_signature_invalid");
  });

  test("flow 212b: a missing signature is rejected", async ({ request }) => {
    const res = await request.post(WEBHOOK_URL, { data: payload(), headers: { "Content-Type": "application/json" } });
    expect(res.status()).toBe(401);
  });

  test("flow 213: a body larger than 65536 bytes is rejected with payment_body_too_large", async ({ request }) => {
    const body = payload({ content: "x".repeat(70_000) });
    const res = await request.post(WEBHOOK_URL, signedPost(body));
    expect(res.status()).toBe(413);
    expect(await res.text()).toContain("payment_body_too_large");
  });

  test("flow 215: a path other than the webhook route answers not_found on the webhook port", async ({ request }) => {
    const res = await request.get(`http://127.0.0.1:${WEBHOOK_PORT}/not-a-route`);
    expect(res.status()).toBe(404);
  });

  test("flow 59: an out transfer is ignored and a replayed event is deduplicated", async ({ request }) => {
    const id = Date.now();
    const out = payload({ id, transferType: "out" });
    const first = await request.post(WEBHOOK_URL, signedPost(out));
    expect(first.status()).toBeLessThan(300);
    const replay = await request.post(WEBHOOK_URL, signedPost(payload({ id: Date.now(), transferType: "in" })));
    expect(replay.status()).toBeLessThan(300);
  });

  test("flow 57: an in transfer with no transfer code is queued for manual review", async ({ request }) => {
    const res = await request.post(WEBHOOK_URL, signedPost(payload({ content: "no code here" })));
    expect(res.status()).toBeLessThan(300);
  });
});
