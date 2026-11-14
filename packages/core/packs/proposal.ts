import type { Attachment, FlowProposalSource } from "../types";

/** What `source_ref` of a draft of origin proposal says: the issue the agent was given, and the agent. */
export function proposalSource(sourceRef: unknown): FlowProposalSource | null {
  if (!sourceRef || typeof sourceRef !== "object" || Array.isArray(sourceRef)) return null;
  const ref = sourceRef as Record<string, unknown>;
  if (typeof ref.issue_id !== "string" || !ref.issue_id) return null;
  return { issue_id: ref.issue_id, agent_id: typeof ref.agent_id === "string" ? ref.agent_id : "" };
}

/**
 * The file an import would read: the newest whose name ends in flow.json and that the agent of the
 * proposal attached. The server applies the same rule; this only says whether there is one to import.
 */
export function latestProposalFile(attachments: Attachment[], agentId: string): Attachment | null {
  if (!agentId) return null;
  let newest: Attachment | null = null;
  for (const att of attachments) {
    if (att.uploader_type !== "agent" || att.uploader_id !== agentId || !att.filename.endsWith("flow.json")) continue;
    if (!newest || Date.parse(att.created_at) >= Date.parse(newest.created_at)) newest = att;
  }
  return newest;
}
