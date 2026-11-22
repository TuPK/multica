"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Settings, Workflow } from "@multica/ui/icons";
import { useWorkspaceId } from "@multica/core/hooks";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import {
  issueFlowOptions,
  issueRoomWorkflow,
  useIssueWorkflows,
  useRoomRoster,
} from "@multica/core/issue-workflows";
import { useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { cn } from "@multica/ui/lib/utils";
import { ActorAvatar } from "../common/actor-avatar";
import { StatusIcon } from "../issues/components/status-icon";
import { useStatusLabel } from "../issues/utils/status-label";
import { useT } from "../i18n";
import { useNavigation } from "../navigation";
import { useWorkflowPermissions } from "./permissions";
import { stepHandlerActor, useStepHandlerLabel } from "./step-handler";
import { IssueFlowStatePanel } from "./issue-flow-state-panel";

export interface IssueFlowCardProps {
  issueId?: string;
  projectId: string | null;
  assigneeType?: string | null;
  assigneeId?: string | null;
  currentStatus: string;
  canManage?: boolean;
}

/**
 * The workflow an issue follows:
 * - If it is an enforced flow request, renders the full IssueFlowStatePanel.
 * - Otherwise follows its room's workflow (or Default), opening a read-only
 *   card of its columns and handoffs.
 */
export function IssueFlowCard({
  issueId,
  projectId,
  assigneeType = null,
  assigneeId = null,
  currentStatus,
  canManage = false,
}: IssueFlowCardProps) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const catalog = useIssueStatuses(wsId);
  const labelOf = useStatusLabel(wsId);

  // Flow query for enforced squad flow
  const flowQuery = useQuery({ ...issueFlowOptions(wsId, issueId ?? ""), enabled: Boolean(issueId) });

  const { workflows } = useIssueWorkflows(wsId);
  const roster = useRoomRoster(wsId);
  const { data: projects = [] } = useQuery({
    ...projectListOptions(wsId),
    enabled: Boolean(wsId) && Boolean(projectId),
  });
  const project = projectId ? projects.find((p) => p.id === projectId) ?? null : null;
  const workflow = issueRoomWorkflow(workflows, roster, {
    project_id: projectId,
    assignee_type: assigneeType,
    assignee_id: assigneeId,
  });
  const { canCreateWorkflow } = useWorkflowPermissions(project);
  const handlerLabel = useStepHandlerLabel(project);
  const [open, setOpen] = useState(false);

  // If this issue is managed by flow engine, render the flow panel
  if (flowQuery.data && flowQuery.data.managed) {
    return (
      <IssueFlowStatePanel
        wsId={wsId}
        issueId={issueId!}
        flow={flowQuery.data}
        canManage={canManage}
      />
    );
  }

  if (!workflow) return null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="-ml-1 inline-flex min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-caption transition-colors hover:bg-accent data-popup-open:bg-accent"
          >
            <Workflow aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate text-foreground">{workflow.name}</span>
          </button>
        }
      />
      <PopoverContent align="start" className="w-72 p-3">
        <p className="mb-2 text-caption text-muted-foreground">
          {project
            ? t(($) => $.workflows.detail.card_title, { workflow: workflow.name, project: project.title })
            : workflow.name}
        </p>
        <ol className="space-y-0.5">
          {workflow.steps.map((step) => {
            const label = handlerLabel(step);
            const actor = stepHandlerActor(step, project);
            const current = step.status_key === currentStatus;
            return (
              <li
                key={step.status_key}
                className={cn("flex items-center gap-2 rounded-md px-2 py-1.5", current && "bg-accent")}
                aria-current={current ? "step" : undefined}
              >
                <StatusIcon
                  status={step.status_key}
                  category={catalog.categoryOf(step.status_key)}
                  color={catalog.colorOf(step.status_key)}
                  icon={catalog.iconOf(step.status_key)}
                  className="h-3.5 w-3.5 shrink-0"
                />
                <span className={cn("min-w-0 flex-1 truncate text-body", current && "font-medium")}>
                  {labelOf(step.status_key)}
                </span>
                {label && (
                  <span className="flex min-w-0 items-center gap-1 text-caption text-muted-foreground">
                    {actor && <ActorAvatar actorType={actor.type} actorId={actor.id} size="xs" profileLink={false} />}
                    <span className="truncate">{label}</span>
                  </span>
                )}
              </li>
            );
          })}
        </ol>
        {canCreateWorkflow && (
          <div className="-mx-3 -mb-3 mt-2 border-t border-surface-border p-1">
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start"
              onClick={() => {
                setOpen(false);
                navigation.push(`${paths.settings()}?tab=workflows`);
              }}
            >
              <Settings aria-hidden className="size-3.5" />
              {t(($) => $.workflows.project.manage)}
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
