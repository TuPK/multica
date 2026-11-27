import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the seat dossier routes (durable spec 9.2, server durable_seat_fork.go).
// Unknown enum values degrade to safe defaults and malformed items are dropped individually
// so one unexpected row never breaks the seat view.

export const DOSSIER_KINDS = ["lesson", "pitfall", "preference", "context", "contact"] as const;
export const DOSSIER_ORIGINS = ["outcome", "retro", "agent", "member"] as const;
export const DOSSIER_STATUSES = ["active", "retired"] as const;
export const DOSSIER_STATUS_FILTERS = ["active", "retired", "all"] as const;
export const DOSSIER_EVIDENCE_TYPES = ["outcome", "retro", "comment", "document"] as const;

export const DOSSIER_TEXT_MAX = 800;
export const DOSSIER_WORK_CLASS_MAX = 200;
export const DOSSIER_EVIDENCE_REF_MAX = 200;
export const DOSSIER_MAX_PER_WRITE = 3;

export type DossierKind = (typeof DOSSIER_KINDS)[number];
export type DossierOrigin = (typeof DOSSIER_ORIGINS)[number];
export type DossierStatus = (typeof DOSSIER_STATUSES)[number];
export type DossierStatusFilter = (typeof DOSSIER_STATUS_FILTERS)[number];
export type DossierEvidenceType = (typeof DOSSIER_EVIDENCE_TYPES)[number];

export const DossierEvidenceSchema = z
  .object({
    type: enumOr(DOSSIER_EVIDENCE_TYPES, "document"),
    ref: z.string().min(1),
  })
  .loose();

export const SeatDossierEntrySchema = z
  .object({
    id: z.string().min(1),
    agent_id: str,
    kind: enumOr(DOSSIER_KINDS, "lesson"),
    text: str,
    work_class: str,
    evidence: listOfItems(DossierEvidenceSchema),
    origin: enumOr(DOSSIER_ORIGINS, "agent"),
    status: enumOr(DOSSIER_STATUSES, "active"),
    created_at: str,
    retired_at: nullableStr,
  })
  .loose();

export const SeatDossierListSchema = z
  .object({
    entries: listOfItems(SeatDossierEntrySchema),
  })
  .loose();

export type DossierEvidence = z.infer<typeof DossierEvidenceSchema>;
export type SeatDossierEntry = z.infer<typeof SeatDossierEntrySchema>;

/** Parses a list or add response (`{ entries: [...] }`). Malformed items are dropped. */
export function parseSeatDossierList(raw: unknown, endpoint: string): SeatDossierEntry[] {
  return parseWithFallback<{ entries: SeatDossierEntry[] }>(
    raw,
    SeatDossierListSchema,
    { entries: [] },
    { endpoint },
  ).entries;
}

/** Parses one entry (e.g. the retire response). A body of the wrong shape reads as null. */
export function parseSeatDossierEntry(raw: unknown, endpoint: string): SeatDossierEntry | null {
  return parseWithFallback<SeatDossierEntry | null>(raw, SeatDossierEntrySchema, null, { endpoint });
}
