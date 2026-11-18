/**
 * Harness for the platform-management end-to-end tests.
 *
 * It starts the platform server (API on 8095, console on 30001) against the
 * dedicated test database named by PLATFORM_TEST_DATABASE_URL, with a Redis
 * container of its own. It never reads DATABASE_URL: e2e/env.ts loads .env,
 * whose DATABASE_URL points at the shared development database. It never
 * prints a DSN or a secret.
 *
 * External providers are not started. The e-invoice provider is empty, so no
 * e-invoice client exists; the payment webhook is driven by a signed request
 * the test builds itself.
 */

import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { createCipheriv, createHmac, randomBytes, randomUUID } from "node:crypto";
import { existsSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import type { APIRequestContext } from "@playwright/test";
import pg from "pg";

export const API_PORT = 8095;
export const CONSOLE_PORT = 30001;
export const WEBHOOK_PORT = 30003;
export const API_BASE = `http://127.0.0.1:${API_PORT}`;
export const CONSOLE_BASE = `http://localhost:${CONSOLE_PORT}`;
export const REDIS_HOST_PORT = 16379;
export const SEPAY_SECRET = "e2e-sepay-webhook-secret";

// Playwright runs from the repository root; the repo is ESM, so no __dirname.
const REPO_ROOT = process.cwd();
const BIN_DIR = resolve(REPO_ROOT, "test-results", "platform");
const BIN = resolve(BIN_DIR, "platform-server");
const REDIS_CONTAINER = `multica-platform-e2e-redis-${process.pid}`;

/** The test database URL, from the environment only. Never printed. */
export function testDatabaseUrl(): string {
  const url = process.env.PLATFORM_TEST_DATABASE_URL ?? "";
  if (!url) {
    throw new Error("PLATFORM_TEST_DATABASE_URL is not set; the platform e2e tests need a dedicated test database");
  }
  // The guard script refuses the shared database and any name not ending in _test.
  execFileSync("bash", [resolve(REPO_ROOT, "scripts", "require-test-database.sh")], {
    env: { PATH: process.env.PATH ?? "", DATABASE_URL: url },
    stdio: ["ignore", "ignore", "pipe"],
  });
  return url;
}

export function devVerificationCode(): string {
  const code = process.env.PLATFORM_DEV_VERIFICATION_CODE ?? "";
  if (!/^\d{6}$/.test(code)) {
    throw new Error("PLATFORM_DEV_VERIFICATION_CODE must be set to six digits for the platform e2e tests");
  }
  return code;
}

let serverProc: ChildProcess | null = null;
let serverLog = "";

function buildServer(): void {
  if (existsSync(BIN)) return;
  mkdirSync(BIN_DIR, { recursive: true });
  execFileSync("go", ["build", "-o", BIN, "./cmd/platform-server"], {
    cwd: resolve(REPO_ROOT, "platform-management"),
    stdio: "ignore",
    timeout: 300_000,
  });
}

function startRedis(): void {
  execFileSync(
    "docker",
    ["run", "-d", "--rm", "--name", REDIS_CONTAINER, "-p", `127.0.0.1:${REDIS_HOST_PORT}:6379`, "redis:7.0-alpine"],
    { stdio: "ignore", timeout: 120_000 },
  );
}

function stopRedis(): void {
  try {
    execFileSync("docker", ["rm", "-f", REDIS_CONTAINER], { stdio: "ignore", timeout: 60_000 });
  } catch {
    // The container may already be gone.
  }
}

async function waitFor(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown = null;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch (err) {
      lastErr = err;
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`timed out waiting for ${url}: ${String(lastErr)}`);
}

export interface StaffSeed {
  email: string;
  userId: string;
  role: "support" | "billing" | "admin";
  /** Raw TOTP key bytes, used to compute codes the server accepts. */
  totpKey: Buffer;
}

const MFA_KEY = randomBytes(32);
const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

/** RFC 4648 base32 without padding, the form the server stores and decodes. */
function base32Encode(buf: Buffer): string {
  let bits = 0;
  let value = 0;
  let out = "";
  for (const b of buf) {
    value = ((value << 8) | b) & 0xffff;
    bits += 8;
    while (bits >= 5) {
      out += B32[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += B32[(value << (5 - bits)) & 31];
  return out;
}

/** AES-256-GCM seal in the layout mfa.Crypto.Open reads: nonce, ciphertext, tag. */
function seal(plain: string): Buffer {
  const nonce = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", MFA_KEY, nonce);
  const ct = Buffer.concat([cipher.update(plain, "utf8"), cipher.final()]);
  return Buffer.concat([nonce, ct, cipher.getAuthTag()]);
}

/** The six-digit TOTP code at a Unix time, as the server computes it (30 s steps). */
export function totpAt(key: Buffer, unixSec: number): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(unixSec / 30)));
  const mac = createHmac("sha1", key).update(counter).digest();
  const off = mac[mac.length - 1] & 0x0f;
  const bin = ((mac[off] & 0x7f) << 24) | (mac[off + 1] << 16) | (mac[off + 2] << 8) | mac[off + 3];
  return String(bin % 1_000_000).padStart(6, "0");
}

async function lastUsedStep(userId: string): Promise<number> {
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    const res = await pool.query("SELECT last_used_step FROM user_mfa WHERE user_id = $1", [userId]);
    return Number(res.rows[0]?.last_used_step ?? 0);
  } finally {
    await pool.end();
  }
}

/**
 * A TOTP code the server will accept now. The server refuses a step it has
 * already used, so this waits for the next 30-second step when needed.
 */
export async function freshTotp(seed: StaffSeed): Promise<string> {
  const deadline = Date.now() + 65_000;
  for (;;) {
    const nowSec = Math.floor(Date.now() / 1000);
    if (Math.floor(nowSec / 30) > (await lastUsedStep(seed.userId))) return totpAt(seed.totpKey, nowSec);
    if (Date.now() > deadline) throw new Error("no fresh TOTP step became available");
    await new Promise((r) => setTimeout(r, 1000));
  }
}

/** Starts Redis and the platform server, and seeds one active staff user. */
export async function startPlatform(role: StaffSeed["role"] = "admin"): Promise<StaffSeed> {
  const dbUrl = testDatabaseUrl();
  const code = devVerificationCode();
  buildServer();
  startRedis();

  const email = `e2e-staff-${randomUUID().slice(0, 8)}@example.test`;
  const userId = randomUUID();
  const totpKey = randomBytes(20);
  const pool = new pg.Pool({ connectionString: dbUrl, max: 2 });
  try {
    await pool.query(
      `INSERT INTO "user" (id, name, email, avatar_url, created_at, updated_at, timezone)
       VALUES ($1, 'E2E Staff', $2, '', now(), now(), 'UTC')`,
      [userId, email],
    );
    await pool.query(
      `INSERT INTO platform_staff (user_id, role, created_by, created_at, revoked_at)
       VALUES ($1, $2, NULL, now(), NULL)`,
      [userId, role],
    );
    // The staff second factor is required: sign-in is refused without a confirmed TOTP.
    await pool.query(
      `INSERT INTO user_mfa (user_id, secret_enc, confirmed_at, recovery_hashes, last_used_step, created_at)
       VALUES ($1, $2, now(), '{}', 0, now())`,
      [userId, seal(base32Encode(totpKey))],
    );
  } finally {
    await pool.end();
  }

  const env: NodeJS.ProcessEnv = {
    PATH: process.env.PATH ?? "",
    HOME: process.env.HOME ?? "",
    APP_ENV: "test",
    DATABASE_URL: dbUrl,
    REDIS_URL: `redis://127.0.0.1:${REDIS_HOST_PORT}/0`,
    PLATFORM_ADMIN_EMAILS: email,
    PLATFORM_SESSION_SECRET: randomBytes(48).toString("hex"),
    MULTICA_MFA_SECRET_KEY: MFA_KEY.toString("base64"),
    PLATFORM_APP_URL: "http://localhost:3000",
    FRONTEND_ORIGIN: "http://localhost:3000",
    PLATFORM_API_PORT: String(API_PORT),
    PLATFORM_CONSOLE_PORT: String(CONSOLE_PORT),
    PLATFORM_WEBHOOK_PORT: String(WEBHOOK_PORT),
    PLATFORM_CONSOLE_ORIGIN: CONSOLE_BASE,
    PLATFORM_SECURE_COOKIES: "false",
    PLATFORM_DEV_VERIFICATION_CODE: code,
    PLATFORM_EINVOICE_PROVIDER: "",
    PAYMENT_PROVIDERS: "vnd=sepay",
    PAYMENT_WEBHOOK_SECRET_SEPAY: SEPAY_SECRET,
    BILLING_BANK_ACCOUNT_NAME: "E2E TEST ACCOUNT",
    BILLING_BANK_ACCOUNT_NUMBER: "0000000000",
    BILLING_BANK_NAME: "E2E TEST BANK",
  };

  serverLog = "";
  serverProc = spawn(BIN, [], { env, cwd: resolve(REPO_ROOT, "platform-management"), stdio: ["ignore", "pipe", "pipe"] });
  serverProc.stdout?.on("data", (d) => (serverLog += d.toString()));
  serverProc.stderr?.on("data", (d) => (serverLog += d.toString()));
  serverProc.once("exit", (code) => {
    serverLog += `\nplatform-server exited early with code ${String(code)}`;
  });
  try {
    await waitFor(`${API_BASE}/healthz`, 60_000);
    await waitFor(`${CONSOLE_BASE}/healthz`, 60_000);
  } catch (err) {
    throw new Error(`${String(err)}\nserver output (tail):\n${serverLog.slice(-2000)}`);
  }
  return { email, userId, role, totpKey };
}

/** Stops the server and removes the rows this run created. */
export async function stopPlatform(seed: StaffSeed | null): Promise<void> {
  const proc = serverProc;
  serverProc = null;
  if (proc && proc.exitCode === null && proc.signalCode === null) {
    // Wait for the process to exit so the next run can bind the same ports.
    const exited = new Promise<void>((resolve) => proc.once("exit", () => resolve()));
    proc.kill("SIGTERM");
    const timer = new Promise<void>((resolve) => setTimeout(resolve, 10_000));
    await Promise.race([exited, timer]);
    if (proc.exitCode === null && proc.signalCode === null) proc.kill("SIGKILL");
  }
  stopRedis();
  if (seed) {
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query("DELETE FROM user_mfa WHERE user_id = $1", [seed.userId]);
      await pool.query("DELETE FROM platform_staff WHERE user_id = $1", [seed.userId]);
      await pool.query(`DELETE FROM "user" WHERE id = $1`, [seed.userId]);
    } finally {
      await pool.end();
    }
  }
}

/**
 * Signs an API request context in as the seeded staff member: the email code,
 * then the TOTP code. The session cookie stays in the context for later calls.
 */
export async function signInApi(context: APIRequestContext, seed: StaffSeed): Promise<void> {
  const origin = { Origin: CONSOLE_BASE };
  const send = await context.post(`${CONSOLE_BASE}/api/platform/auth/send-code`, { headers: origin, data: { email: seed.email } });
  if (send.status() !== 204) throw new Error(`send-code answered ${send.status()}`);
  const first = await context.post(`${CONSOLE_BASE}/api/platform/auth/verify-code`, {
    headers: origin,
    data: { email: seed.email, code: devVerificationCode() },
  });
  if (first.status() !== 200) throw new Error(`verify-code (email) answered ${first.status()}`);
  const second = await context.post(`${CONSOLE_BASE}/api/platform/auth/verify-code`, {
    headers: origin,
    data: { email: seed.email, code: devVerificationCode(), totp: await freshTotp(seed) },
  });
  if (second.status() !== 200) throw new Error(`verify-code (totp) answered ${second.status()}`);
}

export function serverOutput(): string {
  return serverLog;
}

/** Builds the X-SePay-Signature header value for a raw body. */
export function sepaySignature(body: string, timestamp: string, secret = SEPAY_SECRET): string {
  return "sha256=" + createHmac("sha256", secret).update(`${timestamp}.${body}`).digest("hex");
}

export const WEBHOOK_URL = `http://127.0.0.1:${WEBHOOK_PORT}/api/payments/webhook/sepay`;
