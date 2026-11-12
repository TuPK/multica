import { z } from "zod";
import { parseWithFallback } from "../api/schema";
import type { ResearchSettings } from "../types/research";

/**
 * Zod schema for the Research Lab settings. Every field has a default, so a server that adds or drops
 * a field never blanks the tab. A malformed answer reads as nothing configured and keys that cannot
 * be stored, which keeps the form from offering a save that would fail.
 */
export const ResearchSettingsSchema = z
  .object({
    openalex_configured: z.boolean().default(false),
    semantic_scholar_configured: z.boolean().default(false),
    contact_email: z.string().default(""),
    keys_can_be_stored: z.boolean().default(false),
    updated_at: z.string().nullable().default(null),
  })
  .loose();

const EMPTY_SETTINGS: ResearchSettings = {
  openalex_configured: false,
  semantic_scholar_configured: false,
  contact_email: "",
  keys_can_be_stored: false,
  updated_at: null,
};

/** The settings of GET or PUT /api/research/settings. */
export function parseResearchSettings(raw: unknown): ResearchSettings {
  return parseWithFallback<ResearchSettings>(raw, ResearchSettingsSchema, EMPTY_SETTINGS, {
    endpoint: "research.settings",
  });
}
