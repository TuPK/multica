"use client";

import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { flowErrorCode } from "@multica/core/flow-drafts";
import { latestProposalFile, proposalFilesOptions, proposalSource, useImportFlowProposal } from "@multica/core/packs";
import { useWorkspacePaths } from "@multica/core/paths";
import { agentListOptions } from "@multica/core/workspace/queries";
import type { FlowDraft } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Bot, Loader2 } from "@multica/ui/icons";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";

export interface ProposalBannerProps {
  squadId: string;
  draft: FlowDraft;
  /** Saves the draft and resolves to the revision the server has; null when it could not be saved. */
  saveNow: () => Promise<number | null>;
  /** Gets the draft the import answered with, to show it. */
  onImported: (draft: FlowDraft) => void;
}

/**
 * The strip of a draft that an agent was asked to write (spec 11.9): waiting for the agent, Import proposal once
 * it has attached a flow.json, and who proposed it after that.
 */
export function ProposalBanner({ squadId, draft, saveNow, onImported }: ProposalBannerProps) {
  const { t } = useT("squads");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const importMut = useImportFlowProposal();
  const source = draft.origin === "proposal" ? proposalSource(draft.source_ref) : null;

  const filesQuery = useQuery({ ...proposalFilesOptions(wsId, source?.issue_id ?? "", { poll: true }), enabled: Boolean(wsId && source) });
  const agentsQuery = useQuery({ ...agentListOptions(wsId), enabled: Boolean(wsId && source) });
  if (draft.origin !== "proposal") return null;

  const file = source ? latestProposalFile(filesQuery.data ?? [], source.agent_id) : null;
  const agentName = agentsQuery.data?.find((a) => a.id === source?.agent_id)?.name ?? "";
  // The file counts as read when the draft has been saved since it was attached and has moved on from its first
  // revision; a newer file asks to be imported again.
  const imported = file !== null && draft.revision > 1 && Date.parse(file.created_at) <= Date.parse(draft.updated_at);

  const handleImport = async () => {
    try {
      const revision = await saveNow();
      if (revision === null) {
        toast.error(t(($) => $.flow_designer.editor.not_saved));
        return;
      }
      const next = await importMut.mutateAsync({ squadId, draftId: draft.id, revision });
      onImported(next);
    } catch (err) {
      if (flowErrorCode(err) === "flow_proposal_missing") {
        toast.error(t(($) => $.flow_designer.proposal.missing));
        return;
      }
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.proposal.import_failed));
    }
  };

  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border bg-muted/30 px-4 py-2 text-caption">
      <Bot className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      {imported ? (
        <span className="font-medium text-foreground">{t(($) => $.flow_designer.proposal.by, { agent: agentName })}</span>
      ) : file ? (
        <Button type="button" size="xs" disabled={importMut.isPending} onClick={() => void handleImport()} className="gap-1.5">
          {importMut.isPending && <Loader2 className="size-3 animate-spin" aria-hidden />}
          <span>{t(($) => $.flow_designer.proposal.import)}</span>
        </Button>
      ) : (
        <span className="text-muted-foreground">{t(($) => $.flow_designer.proposal.waiting)}</span>
      )}
      {source && (
        <AppLink href={paths.issueDetail(source.issue_id)} className="text-muted-foreground underline-offset-2 hover:underline">
          {t(($) => $.flow_designer.proposal.open_issue)}
        </AppLink>
      )}
    </div>
  );
}
