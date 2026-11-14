"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { TooltipHint } from "@multica/ui/components/common/tooltip-hint";
import { ChevronDown, Workflow } from "@multica/ui/icons";
import { flowCapabilitiesOptions } from "@multica/core/flow-drafts";
import { squadWorkflowsOptions } from "@multica/core/issue-workflows";
import type { IssueWorkflow } from "@multica/core/types";
import { useT } from "../i18n";
import { PillButton } from "../common/pill-button";

/** The workflows of a squad that run on a flow, in the order the server lists them. */
export function squadFlowWorkflows(workflows: IssueWorkflow[]): IssueWorkflow[] {
  return workflows.filter((w) => w.flow_config != null);
}

/** How many separate flows (lineages) a list of workflows holds. */
export function countFlowLineages(workflows: IssueWorkflow[]): number {
  return new Set(squadFlowWorkflows(workflows).map((w) => w.lineage_id ?? w.id)).size;
}

export interface FlowWorkflowPickerProps {
  wsId: string;
  squadId: string;
  /** The workflow chosen for the request; undefined means the squad's active one. */
  value: string | undefined;
  onChange: (workflowId: string | undefined) => void;
}

/**
 * Chooses which of a squad's workflows runs a new request (spec 16.5). It shows only when the squad
 * has more than one flow to choose between, and the server has flow authoring on.
 */
export function FlowWorkflowPicker({ wsId, squadId, value, onChange }: FlowWorkflowPickerProps) {
  const { t } = useT("modals");
  const query = useQuery(squadWorkflowsOptions(wsId, squadId));
  // The server refuses the choice with authoring off; a server that does not say is taken to allow it.
  const authoring = useQuery(flowCapabilitiesOptions(wsId)).data?.features?.authoring ?? true;
  const flows = useMemo(() => squadFlowWorkflows(query.data?.workflows ?? []), [query.data?.workflows]);
  const activeId = query.data?.active_workflow_id ?? null;
  if (!authoring || countFlowLineages(query.data?.workflows ?? []) < 2) return null;

  const chosen = flows.find((w) => w.id === (value ?? activeId));
  const label = (w: IssueWorkflow) => (w.id === activeId ? t(($) => $.create_issue.flow_workflow_active, { name: w.name }) : w.name);
  const tooltip = t(($) => $.create_issue.flow_workflow);

  return (
    <DropdownMenu>
      <TooltipHint label={tooltip}>
        <DropdownMenuTrigger
          render={
            <PillButton aria-label={tooltip}>
              <Workflow className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
              <span className="truncate">{chosen ? label(chosen) : tooltip}</span>
              <ChevronDown className="size-3 shrink-0 text-muted-foreground" aria-hidden />
            </PillButton>
          }
        />
      </TooltipHint>
      <DropdownMenuContent align="start" className="min-w-56 max-w-80">
        <DropdownMenuRadioGroup
          value={value ?? activeId ?? ""}
          onValueChange={(id) => onChange(id === activeId ? undefined : id)}
        >
          {flows.map((w) => (
            <DropdownMenuRadioItem key={w.id} value={w.id}>
              <span className="truncate">{label(w)}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
