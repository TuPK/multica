import { z } from "zod";
import { FALLBACK_KNOWLEDGE_DOCUMENT, KnowledgeDocumentSchema } from "../knowledge/schemas";
import type { KnowledgeDocument } from "../knowledge/types";

// The answer of the Sales Loop intake endpoints (spec 7.5): the knowledge document the input was
// written to, and the request issue the squad's leader routes. A response the client cannot read
// falls back to an empty document and no issue, never to a document that looks real.

export interface SalesIntake {
  document: KnowledgeDocument;
  issue_id: string;
}

export const SalesIntakeSchema = z
  .object({
    document: KnowledgeDocumentSchema,
    issue_id: z.string().default(""),
  })
  .loose();

export const FALLBACK_SALES_INTAKE: SalesIntake = {
  document: FALLBACK_KNOWLEDGE_DOCUMENT,
  issue_id: "",
};
