// Types of the knowledge taxonomy. The schemas in taxonomy.ts parse the server's
// answers into these; a field the server leaves out takes its default there.

import type { KnowledgeDocument } from "./types";

/** The publish policy of a scope. "unknown" is a mode this client cannot read. */
export type KnowledgePublishMode = "manual" | "auto_after_review" | "auto" | "unknown";

/** A field a kind declares. "unknown" is a type this client cannot read: shown as text, never guessed. */
export type KindFieldType = "text" | "select" | "number" | "date" | "bool" | "url" | "unknown";

export interface KindField {
  key: string;
  label: string;
  type: KindFieldType;
  options: string[];
  required: boolean;
  /** A select whose options run from the first value to the last, such as a pipeline stage. */
  ordered: boolean;
}

/** The default mode, and the knowledge kind's freshness, template and fields. */
export interface KnowledgeKind {
  key: string;
  name: string;
  default_base_key: string;
  template: string;
  review_days: number;
  required_sections: string[];
  fields: KindField[];
}

/** The Collections view of a kind: its fields, and the documents that match, with their properties. */
export interface KnowledgeCollection {
  kind: string;
  fields: KindField[];
  documents: KnowledgeDocument[];
}



export interface KnowledgeKinds {
  kinds: KnowledgeKind[];
}

/** An admin's edit of a kind. A field left out is kept. */
export interface UpdateKnowledgeKindRequest {
  name?: string;
  template?: string;
  review_days?: number;
  required_sections?: string[];
}

export interface KnowledgePublishPolicy {
  /** "base" or "kind". */
  scope: string;
  key: string;
  mode: KnowledgePublishMode;
  updated_at: string;
}

export interface KnowledgePublishPolicies {
  policies: KnowledgePublishPolicy[];
  default_mode: KnowledgePublishMode;
}

export interface SetKnowledgePolicyRequest {
  scope: "base" | "kind";
  key: string;
  mode: "manual" | "auto_after_review" | "auto";
}

export interface ReviewKnowledgeDocumentRequest {
  verdict: "ok" | "changes";
  note?: string;
}

/** One document a Librarian routine points at. */
export interface KnowledgeLibrarianFinding {
  link_id: string;
  title: string;
  knowledge_base: string;
  knowledge_kind: string;
  squad_id: string;
  reason: string;
  missing_sections: string[];
  other_link_id: string;
}

export interface KnowledgeLibrarianReport {
  job: string;
  ran_at: string;
  counts: Record<string, number>;
  findings: KnowledgeLibrarianFinding[];
  truncated: boolean;
}

export interface KnowledgeHealthJob {
  job: string;
  ran_at: string;
  report: KnowledgeLibrarianReport;
}

export interface KnowledgeHealth {
  jobs: KnowledgeHealthJob[];
}

export interface KnowledgeEvalCase {
  id: string;
  question: string;
  expected_kind: string;
  expected_document_id: string;
  owner_squad_id: string | null;
  created_by_type: string;
  created_at: string;
}

export interface KnowledgeEvalCases {
  cases: KnowledgeEvalCase[];
}

export interface CreateKnowledgeEvalCaseRequest {
  question: string;
  expected_kind?: string;
  expected_document_id?: string;
  owner_squad_id?: string;
}

export interface KnowledgeEvalResult {
  case_id: string;
  question: string;
  expected_kind: string;
  expected_document_id: string;
  hit: boolean;
  rank: number;
}

/** "offline": WeKnora did not answer, and nothing was recorded. */
export interface KnowledgeEvalRun {
  id: string;
  state: "ready" | "offline" | "unknown";
  total: number;
  hits: number;
  hit_rate: number;
  ran_at: string;
  results: KnowledgeEvalResult[];
}

export interface KnowledgeEvalRuns {
  runs: KnowledgeEvalRun[];
}
