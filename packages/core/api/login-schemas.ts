import { z } from "zod";
import type { User } from "../types";
import { UserSchema } from "./schemas";

// Login answers carry credentials, so these schemas are strict about the
// fields that matter (token, user, mfa_token, expires_in) and are used with a
// null fallback that the client turns into a thrown error. Unknown keys pass.

export type LoginAttempt =
  | { kind: "session"; token: string; user: User }
  | { kind: "mfa"; mfaToken: string; expiresIn: number };

export const LoginResponseSchema = z
  .object({
    token: z.string().min(1),
    user: UserSchema,
  })
  .loose();

const MFAChallengeSchema = z
  .object({
    mfa_required: z.literal(true),
    mfa_token: z.string().min(1),
    expires_in: z.number(),
  })
  .loose()
  .transform((v) => ({ kind: "mfa" as const, mfaToken: v.mfa_token, expiresIn: v.expires_in }));

const SessionAttemptSchema = LoginResponseSchema.transform((v) => ({
  kind: "session" as const,
  token: v.token,
  user: v.user as unknown as User,
}));

/** Discriminated on `mfa_required`: present and true means a challenge. */
export const LoginAttemptSchema = z.union([MFAChallengeSchema, SessionAttemptSchema]);
