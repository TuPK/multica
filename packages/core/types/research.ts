/**
 * The Research Lab source settings of a workspace, as GET /api/research/settings returns them.
 * Mirrors `research.Settings` in `server/internal/research/service.go`. The API keys are never
 * part of it: the server says only whether each one is stored.
 */
export interface ResearchSettings {
  openalex_configured: boolean;
  semantic_scholar_configured: boolean;
  contact_email: string;
  /** False when the server has no secret to seal a key with. A key cannot then be saved. */
  keys_can_be_stored: boolean;
  updated_at: string | null;
}

/**
 * The PUT /api/research/settings body. A field left out is not changed; an empty string removes
 * the stored value.
 */
export interface UpdateResearchSettingsInput {
  openalex_api_key?: string;
  semantic_scholar_api_key?: string;
  contact_email?: string;
}
