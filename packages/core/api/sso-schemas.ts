import { z } from "zod";

// The SSO responses of platform spec 6.3. Schemas are lenient: roles and
// statuses stay strings so a newer server's value still parses, and `.loose()`
// keeps unknown keys. The client secret is never part of a response; the
// connection says only whether one is stored.

export interface SSOConnection {
  id: string;
  workspace_id: string;
  issuer: string;
  client_id: string;
  client_secret_set: boolean;
  default_role: string;
  status: string;
  /** The callback URL the owner registers at the identity provider. */
  redirect_uri: string;
  created_at: string;
  updated_at: string;
}

export interface SSOConnectionInput {
  issuer: string;
  client_id: string;
  /** Left out on an update that keeps the stored secret. */
  client_secret?: string;
  default_role?: string;
  status?: string;
}

export interface SSODiscovery {
  sso: boolean;
  /** Where the browser goes to sign in; set only when `sso` is true. */
  start_url: string | null;
}

/** An email that does not sign in through SSO, and the answer when the call fails. */
export const NO_SSO: SSODiscovery = { sso: false, start_url: null };

export const SSOConnectionSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().default(""),
    issuer: z.string(),
    client_id: z.string(),
    client_secret_set: z.boolean().default(false),
    default_role: z.string().default("member"),
    status: z.string().default("active"),
    redirect_uri: z.string().default(""),
    created_at: z.string().default(""),
    updated_at: z.string().default(""),
  })
  .loose();

export const SSODiscoverySchema = z
  .object({
    sso: z.boolean().default(false),
    start_url: z.string().nullish(),
  })
  .loose()
  .transform(
    (value): SSODiscovery => ({
      // A discovery without a place to go is no discovery: the sign-in page
      // must fall back to the other methods rather than offer a dead button.
      sso: value.sso && !!value.start_url,
      start_url: value.sso && value.start_url ? value.start_url : null,
    }),
  );
