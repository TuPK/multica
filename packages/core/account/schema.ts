import { z } from "zod";
import type { ConsentRequirement } from "../types/workspace";

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

export const ConsentRequirementSchema = z
  .object({
    document: z.string(),
    version: z.string(),
    url: z.string().default(""),
  })
  .loose();

/** Absent or malformed means nothing is required: the server never blocks calls. */
export const ConsentRequiredSchema = z.array(ConsentRequirementSchema).default([]).catch([]);

export interface AcceptConsentItem {
  document: string;
  version: string;
}

/** Response of POST /api/me/consents: the documents still outstanding. */
export interface AcceptConsentsResponse {
  consent_required: ConsentRequirement[];
}

export const AcceptConsentsResponseSchema = z
  .object({ consent_required: ConsentRequiredSchema })
  .loose();

export const EMPTY_ACCEPT_CONSENTS_RESPONSE: AcceptConsentsResponse = {
  consent_required: [],
};
