import { errorCode } from "@multica/core/api";
import type { useT } from "../../i18n";

type DurableTranslate = ReturnType<typeof useT<"durable">>["t"];

const PRINCIPAL_ERROR_CODES = [
  "profile_consent_required",
  "principal_trait_limit",
  "principal_evidence_invalid",
  "principal_trait_invalid",
  "principal_trait_edited",
  "delegation_not_eligible",
  "delegation_forbidden_kind",
] as const;

export type PrincipalErrorCode = (typeof PRINCIPAL_ERROR_CODES)[number];

function isPrincipalErrorCode(code: string | undefined): code is PrincipalErrorCode {
  return !!code && (PRINCIPAL_ERROR_CODES as readonly string[]).includes(code);
}

/** Maps a server principal/delegation error code to localized copy, falling back to `fallback`. */
export function principalErrorMessage(err: unknown, t: DurableTranslate, fallback: string): string {
  const code = errorCode(err);
  if (isPrincipalErrorCode(code)) {
    return t(($) => $.principal.errors[code]);
  }
  return fallback;
}

/**
 * The four consent points from durable spec 6.6 joined into the exact string sent in
 * `PUT /api/principal/profile` so the server hashes the text the person actually read.
 */
export function buildConsentText(t: DurableTranslate): string {
  return [
    t(($) => $.principal.consent.point_data),
    t(($) => $.principal.consent.point_access),
    t(($) => $.principal.consent.point_no_eval),
    t(($) => $.principal.consent.point_delete),
    t(($) => $.principal.consent.checkbox),
  ].join("\n");
}

const KNOWN_KIND_TO_KEY = {
  general: "general",
  outcome: "outcome",
  inquiry: "inquiry",
  "proposal:general": "proposal_general",
  "proposal:hire": "proposal_hire",
  "proposal:promote": "proposal_promote",
  "effect:write": "effect_write",
  "effect:send": "effect_send",
  "proposal:org_change": "proposal_org_change",
  "proposal:launch": "proposal_launch",
  "proposal:stage_gate": "proposal_stage_gate",
  "effect:spend": "effect_spend",
  "effect:share": "effect_share",
  "proposal:budget": "proposal_budget",
  "proposal:autonomy": "proposal_autonomy",
  "proposal:envelope": "proposal_envelope",
  "proposal:policy": "proposal_policy",
  "proposal:profile": "proposal_profile",
} as const;

type KnownKind = keyof typeof KNOWN_KIND_TO_KEY;

function isKnownKind(kind: string): kind is KnownKind {
  return Object.prototype.hasOwnProperty.call(KNOWN_KIND_TO_KEY, kind);
}

/** Human-readable label for a decision kind, falling back to the raw identifier for custom kinds. */
export function decisionKindLabel(kind: string, t: DurableTranslate): string {
  if (isKnownKind(kind)) {
    const key = KNOWN_KIND_TO_KEY[kind];
    return t(($) => $.principal.delegation.kinds[key]);
  }
  return kind;
}
