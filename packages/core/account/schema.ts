import { z } from "zod";

/**
 * Response of POST /api/me/sessions/revoke-all.
 *
 * `token` is the caller's replacement session. The old one is revoked by the
 * same call, so a client that holds its session as a string has to adopt this
 * one or its next request is refused.
 */
export interface RevokeAllSessionsResponse {
  revoked_at: string;
  token: string;
}

export const RevokeAllSessionsResponseSchema = z
  .object({
    revoked_at: z.string().default(""),
    token: z.string().default(""),
  })
  .loose();

// Fail closed on an unreadable body: no token means nothing is adopted, so the
// client keeps what it has. A fallback that invented a token would replace a
// working session with garbage.
export const EMPTY_REVOKE_ALL_SESSIONS_RESPONSE: RevokeAllSessionsResponse = {
  revoked_at: "",
  token: "",
};
