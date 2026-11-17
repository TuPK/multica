// @vitest-environment node
import { describe, expect, it } from "vitest";
import { LoginAttemptSchema, LoginResponseSchema } from "./login-schemas";

const user = { id: "u1", name: "Alice", email: "a@x.test", avatar_url: null };

describe("LoginAttemptSchema", () => {
  it("parses a session answer", () => {
    const r = LoginAttemptSchema.safeParse({ token: "t", user });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data).toMatchObject({ kind: "session", token: "t", user: { id: "u1" } });
  });
  it("parses an mfa challenge", () => {
    const r = LoginAttemptSchema.safeParse({ mfa_required: true, mfa_token: "m", expires_in: 300 });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data).toEqual({ kind: "mfa", mfaToken: "m", expiresIn: 300 });
  });
  it("tolerates unknown extra keys", () => {
    expect(LoginAttemptSchema.safeParse({ token: "t", user, extra: 1 }).success).toBe(true);
    expect(
      LoginAttemptSchema.safeParse({ mfa_required: true, mfa_token: "m", expires_in: 1, extra: 1 }).success,
    ).toBe(true);
  });
  it.each([
    ["missing token", { user }],
    ["missing user", { token: "t" }],
    ["empty token", { token: "", user }],
    ["mfa without mfa_token", { mfa_required: true, expires_in: 300 }],
    ["expires_in as a string", { mfa_required: true, mfa_token: "m", expires_in: "300" }],
    ["empty object", {}],
  ])("rejects %s", (_n, body) => {
    expect(LoginAttemptSchema.safeParse(body).success).toBe(false);
  });
});

describe("LoginResponseSchema", () => {
  it("parses a session and rejects malformed ones", () => {
    expect(LoginResponseSchema.safeParse({ token: "t", user }).success).toBe(true);
    expect(LoginResponseSchema.safeParse({ token: "t" }).success).toBe(false);
    expect(LoginResponseSchema.safeParse({ user }).success).toBe(false);
    expect(LoginResponseSchema.safeParse({ mfa_required: true, mfa_token: "m", expires_in: 1 }).success).toBe(false);
  });
});
