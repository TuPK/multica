import { z } from "zod";
import type { CreatePersonalAccessTokenResponse, PersonalAccessToken } from "../types";

// The R3 security responses: workspace policy, MFA, SSO domain claims and
// personal access tokens. Schemas are lenient: unknown enum values stay
// strings and `.loose()` keeps keys a newer server adds.

export interface WorkspacePolicy {
  allowed_invite_domains: string[];
  share_links_enabled: boolean;
  require_sso: boolean;
  require_mfa: boolean;
  /** Hours a session may live; 0 turns the limit off. */
  session_max_age_hours: number;
  updated_by: string | null;
  updated_at: string | null;
}

/** A partial update: only the fields present change. */
export type WorkspacePolicyInput = Partial<
  Pick<
    WorkspacePolicy,
    | "allowed_invite_domains"
    | "share_links_enabled"
    | "require_sso"
    | "require_mfa"
    | "session_max_age_hours"
  >
>;

export interface MFAEnrollResponse {
  secret: string;
  otpauth_uri: string;
}

export interface MFAConfirmResponse {
  recovery_codes: string[];
}

/** A TOTP code, or a recovery code where the endpoint accepts one. */
export interface MFACodeInput {
  code: string;
}

export interface SSODomain {
  domain: string;
  txt_name: string;
  txt_value: string;
  verified_at: string | null;
}

export interface SSODomainList {
  domains: SSODomain[];
}

/** The answer when the policy cannot be read: nothing restricted. */
export const DEFAULT_WORKSPACE_POLICY: WorkspacePolicy = {
  allowed_invite_domains: [],
  share_links_enabled: true,
  require_sso: false,
  require_mfa: false,
  session_max_age_hours: 0,
  updated_by: null,
  updated_at: null,
};

export const NO_SSO_DOMAINS: SSODomainList = { domains: [] };

export const WorkspacePolicySchema = z
  .object({
    allowed_invite_domains: z.array(z.string()).default([]),
    share_links_enabled: z.boolean().default(true),
    require_sso: z.boolean().default(false),
    require_mfa: z.boolean().default(false),
    session_max_age_hours: z.number().min(0).max(8760).default(0),
    updated_by: z.string().nullable().default(null),
    updated_at: z.string().nullable().default(null),
  })
  .loose();

export const MFAEnrollResponseSchema = z
  .object({ secret: z.string(), otpauth_uri: z.string() })
  .loose();

export const MFAConfirmResponseSchema = z
  .object({ recovery_codes: z.array(z.string()) })
  .loose();

export const SSODomainSchema = z
  .object({
    domain: z.string(),
    txt_name: z.string(),
    txt_value: z.string(),
    verified_at: z.string().nullable().default(null),
  })
  .loose();

export const SSODomainListSchema = z
  .object({ domains: z.array(SSODomainSchema).default([]) })
  .loose();

export const PersonalAccessTokenSchema = z
  .object({
    id: z.string(),
    name: z.string(),
    token_prefix: z.string().default(""),
    expires_at: z.string().nullable().default(null),
    last_used_at: z.string().nullable().default(null),
    created_at: z.string().default(""),
    workspace_id: z.string().nullable().default(null),
    read_only: z.boolean().default(false),
  })
  .loose();

export const PersonalAccessTokenListSchema = z.array(PersonalAccessTokenSchema);

/** The create response is a token plus the one-time secret. */
export const CreatePersonalAccessTokenResponseSchema = PersonalAccessTokenSchema.extend({
  token: z.string().min(1),
});

export type { PersonalAccessToken, CreatePersonalAccessTokenResponse };
