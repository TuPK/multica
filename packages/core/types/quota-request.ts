export type QuotaRequestState =
  | "pending"
  | "approved"
  | "denied"
  | "canceled"
  | (string & {});

export interface QuotaRequest {
  id: string;
  workspace_id: string;
  gate: string;
  current_value: number | null;
  requested_value: number;
  approved_value: number | null;
  state: QuotaRequestState;
  reason: string;
  decision_reason: string;
  requested_by: string;
  requested_by_label: string;
  decided_by: string;
  created_at: string;
  decided_at: string | null;
}

export interface CreateQuotaRequestInput {
  gate: string;
  requested_value: number;
  reason: string;
}

export interface QuotaRequestListResponse {
  requests: QuotaRequest[];
}
