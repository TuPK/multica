"use client";

import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@multica/ui/components/ui/sheet";
import { ArrowRight, Bot, FolderKanban, Users, Workflow } from "@multica/ui/icons";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import { useActorName } from "@multica/core/workspace/hooks";
import { ActorAvatar } from "../../common/actor-avatar";
import { StatusIcon } from "../../issues/components/status-icon";
import { useStatusLabel } from "../../issues/utils/status-label";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import type { IssueWorkflow, IssueWorkflowUsage } from "../types";

export interface WorkflowDetailDrawerProps {
  workflow: IssueWorkflow | null;
  usage?: IssueWorkflowUsage;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
}

export function WorkflowDetailDrawer({
  workflow,
  usage,
  open,
  onOpenChange,
  wsId,
}: WorkflowDetailDrawerProps) {
  const { t } = useT("workflow");
  const p = useWorkspacePaths();
  const { statuses } = useIssueStatuses(wsId);
  const getStatusLabel = useStatusLabel(wsId);
  const { getActorName } = useActorName();
  const { data: projects = [] } = useQuery(projectListOptions(wsId));

  if (!workflow) return null;

  const assignedProjects = projects.filter((proj) =>
    usage?.project_ids.includes(proj.id),
  );

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full sm:max-w-xl overflow-y-auto p-6"
      >
        <SheetHeader className="space-y-2 border-b border-border pb-4">
          <div className="text-caption font-medium text-muted-foreground">
            {t(($) => $.manage.detail_title)}
          </div>
          <div className="flex items-center gap-2">
            <Workflow className="size-5 text-muted-foreground" />
            <SheetTitle className="text-title font-semibold text-foreground">
              {workflow.name}
            </SheetTitle>
            {workflow.is_default && (
              <Badge variant="secondary">
                {t(($) => $.manage.default_badge)}
              </Badge>
            )}
            {workflow.template_ref && (
              <Badge variant="secondary" className="gap-1 border border-border/60 bg-muted/30">
                {t(($) => $.manage.managed_by_template)}
              </Badge>
            )}
          </div>
          {workflow.description ? (
            <SheetDescription className="text-body-sm text-muted-foreground">
              {workflow.description}
            </SheetDescription>
          ) : null}
          {!workflow.template_ref && (
            <div className="pt-2">
              <AppLink href={p.workflowDetail(workflow.id)}>
                <Button size="sm" className="gap-1.5">
                  <Workflow className="size-4" />
                  {t(($) => $.manage.open_designer)}
                </Button>
              </AppLink>
            </div>
          )}
        </SheetHeader>

        {/* Assigned entities */}
        <div className="space-y-4 py-4 border-b border-border">
          {assignedProjects.length > 0 && (
            <div>
              <div className="flex items-center gap-1.5 text-caption font-medium text-muted-foreground mb-1.5">
                <FolderKanban className="size-3.5" />
                <span>{t(($) => $.manage.assigned_projects)}</span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {assignedProjects.map((proj) => (
                  <Badge key={proj.id} variant="outline" className="gap-1 text-caption">
                    {proj.title}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {usage && usage.squad_ids.length > 0 && (
            <div>
              <div className="flex items-center gap-1.5 text-caption font-medium text-muted-foreground mb-1.5">
                <Users className="size-3.5" />
                <span>{t(($) => $.manage.assigned_squads)}</span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {usage.squad_ids.map((squadId) => (
                  <Badge key={squadId} variant="outline" className="gap-1 text-caption">
                    <ActorAvatar actorType="squad" actorId={squadId} className="size-3.5" />
                    <span>{getActorName("squad", squadId)}</span>
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {usage && usage.agent_ids.length > 0 && (
            <div>
              <div className="flex items-center gap-1.5 text-caption font-medium text-muted-foreground mb-1.5">
                <Bot className="size-3.5" />
                <span>{t(($) => $.manage.assigned_agents)}</span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {usage.agent_ids.map((agentId) => (
                  <Badge key={agentId} variant="outline" className="gap-1 text-caption">
                    <ActorAvatar actorType="agent" actorId={agentId} className="size-3.5" />
                    <span>{getActorName("agent", agentId)}</span>
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Steps list */}
        <div className="py-4 space-y-3">
          <div className="text-label font-medium text-foreground">
            {t(($) => $.manage.steps_count, { count: workflow.steps.length })}
          </div>
          <div className="space-y-2">
            {workflow.steps.map((step, idx) => {
              const statusDef = statuses.find((s) => s.key === step.status_key);
              const handlerType = step.handler.type;
              const hasHandler = handlerType !== "none";

              return (
                <div
                  key={step.status_key}
                  className="rounded-lg border border-border p-3 space-y-2 bg-card"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="text-micro font-mono text-muted-foreground">
                        #{idx + 1}
                      </span>
                      <StatusIcon status={step.status_key} className="size-4 shrink-0" />
                      <span className="text-body font-medium text-foreground">
                        {statusDef ? getStatusLabel(statusDef.key) : step.status_key}
                      </span>
                    </div>

                    {hasHandler && (
                      <div className="flex items-center gap-1 text-caption text-muted-foreground">
                        {step.handler.id && (
                          <ActorAvatar
                            actorType={handlerType === "squad" ? "squad" : "agent"}
                            actorId={step.handler.id}
                            className="size-4"
                          />
                        )}
                        <span>
                          {step.handler.id
                            ? getActorName(
                                handlerType === "squad" ? "squad" : "agent",
                                step.handler.id,
                              )
                            : handlerType}
                        </span>
                      </div>
                    )}
                  </div>

                  {step.instructions ? (
                    <p className="text-caption text-muted-foreground line-clamp-2 bg-muted/40 p-1.5 rounded">
                      {step.instructions}
                    </p>
                  ) : null}

                  {step.transitions && step.transitions.length > 0 && (
                    <div className="pt-1 flex flex-wrap items-center gap-1.5">
                      {step.transitions.map((trans) => (
                        <div
                          key={trans.id}
                          className="inline-flex items-center gap-1 text-micro px-2 py-0.5 rounded-full bg-muted text-muted-foreground border border-border/60"
                        >
                          <span className="font-medium text-foreground">
                            {trans.label || (trans.when === null ? "Default" : "Branch")}
                          </span>
                          <ArrowRight className="size-3" />
                          <span>{trans.to_status_key}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
