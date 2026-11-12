"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import {
  Copy,
  FolderKanban,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  Workflow,
} from "@multica/ui/icons";
import { ApiError, clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import {
  parseIssueWorkflowBlockers,
  useCreateIssueWorkflow,
  useDeleteIssueWorkflow,
  useIssueWorkflows,
  useIssueWorkflowUsage,
} from "@multica/core/issue-workflows";
import { projectListOptions } from "@multica/core/projects/queries";
import type { IssueWorkflow } from "@multica/core/types";
import { useActorName } from "@multica/core/workspace/hooks";
import { ActorAvatar } from "../../common/actor-avatar";
import { AppLink, useNavigation } from "../../navigation";
import { useT, useTimeAgo } from "../../i18n";
import { localDeleteBlockers, workflowStepInputs, type DeleteBlocker } from "../workflow-data";
import { WorkflowDeleteDialog } from "./workflow-delete-dialog";
import { WorkflowDetailDrawer } from "./workflow-detail-drawer";

type FilterTab = "all" | "projects" | "squads" | "agents";

export function WorkflowManagePage() {
  const { t } = useT("workflow");
  const wsId = useWorkspaceId();
  const p = useWorkspacePaths();
  const navigation = useNavigation();
  const timeAgo = useTimeAgo();
  const { getActorName } = useActorName();

  const { workflows, isLoaded } = useIssueWorkflows(wsId);
  const { usageById } = useIssueWorkflowUsage(wsId);
  const createWorkflow = useCreateIssueWorkflow();
  const deleteWorkflow = useDeleteIssueWorkflow();
  const { data: projects = [] } = useQuery(projectListOptions(wsId));

  const [searchQuery, setSearchQuery] = useState("");
  const [activeFilter, setActiveFilter] = useState<FilterTab>("all");
  const [selectedWorkflow, setSelectedWorkflow] = useState<IssueWorkflow | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  // Delete dialog state. The blockers are what the page can see before the
  // delete; a refused delete replaces them with the server's reasons.
  const [deleteTarget, setDeleteTarget] = useState<IssueWorkflow | null>(null);
  const [deleteBlockers, setDeleteBlockers] = useState<DeleteBlocker[]>([]);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // Filtered workflows
  const filteredWorkflows = useMemo(() => {
    return workflows.filter((wf) => {
      // Search matching
      if (searchQuery.trim()) {
        const query = searchQuery.toLowerCase();
        const matchesName = wf.name.toLowerCase().includes(query);
        const matchesDesc = wf.description?.toLowerCase().includes(query);
        if (!matchesName && !matchesDesc) return false;
      }

      // Filter tab
      const usage = usageById[wf.id];
      if (activeFilter === "projects") {
        return (usage?.project_ids.length ?? 0) > 0;
      }
      if (activeFilter === "squads") {
        return (usage?.squad_ids.length ?? 0) > 0;
      }
      if (activeFilter === "agents") {
        return (usage?.agent_ids.length ?? 0) > 0;
      }

      return true;
    });
  }, [workflows, searchQuery, activeFilter, usageById]);

  const handleOpenDetail = (wf: IssueWorkflow) => {
    setSelectedWorkflow(wf);
    setDrawerOpen(true);
  };

  const handleDuplicate = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      await createWorkflow.mutateAsync({
        name: t(($) => $.manage.copy_name, { name: wf.name }),
        description: wf.description,
        steps: workflowStepInputs(wf.steps),
      });
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.manage.action_failed));
    }
  };

  const handleDeletePrompt = (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    setDeleteTarget(wf);
    setDeleteBlockers(localDeleteBlockers(wf, usageById[wf.id]));
    setDeleteDialogOpen(true);
  };

  const closeDeleteDialog = (open: boolean) => {
    setDeleteDialogOpen(open);
    if (!open) setDeleteTarget(null);
  };

  const handleConfirmDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteWorkflow.mutateAsync(deleteTarget.id);
      if (selectedWorkflow?.id === deleteTarget.id) setDrawerOpen(false);
      closeDeleteDialog(false);
    } catch (err) {
      const reasons = parseIssueWorkflowBlockers(err instanceof ApiError ? err.body : undefined);
      if (reasons.length > 0) {
        setDeleteBlockers(reasons.map((r) => ({ code: r.code, count: r.count })));
        return;
      }
      closeDeleteDialog(false);
      toast.error(clientErrorMessage(err) ?? t(($) => $.manage.action_failed));
    }
  };

  return (
    <div className="flex flex-col h-full overflow-y-auto p-6 space-y-6">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-display-sm font-semibold text-foreground">
            {t(($) => $.manage.title)}
          </h1>
        </div>

        <div className="flex items-center gap-2">
          <AppLink href={p.newWorkflow()}>
            <Button className="gap-1.5">
              <Plus className="size-4" />
              <span>{t(($) => $.manage.new_workflow)}</span>
            </Button>
          </AppLink>
        </div>
      </div>

      {/* Search and Filters */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
          <Input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t(($) => $.manage.search_placeholder)}
            className="pl-9"
          />
        </div>

        <div className="flex items-center gap-1.5 p-1 bg-muted/60 rounded-lg">
          {(["all", "projects", "squads", "agents"] as const).map((tab) => (
            <Button
              key={tab}
              variant={activeFilter === tab ? "brand" : "ghost"}
              size="sm"
              onClick={() => setActiveFilter(tab)}
              className="text-caption font-medium"
            >
              {tab === "all" && t(($) => $.manage.filter_all)}
              {tab === "projects" && t(($) => $.manage.filter_projects)}
              {tab === "squads" && t(($) => $.manage.filter_squads)}
              {tab === "agents" && t(($) => $.manage.filter_agents)}
            </Button>
          ))}
        </div>
      </div>

      {/* Workflows List */}
      {!isLoaded ? null : filteredWorkflows.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-center border border-dashed border-border rounded-xl">
          <Workflow className="size-10 text-faint-foreground mb-3" />
          <div className="text-body font-medium text-foreground">
            {t(($) => $.manage.empty_title)}
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3">
          {filteredWorkflows.map((wf) => {
            const usage = usageById[wf.id];
            const assignedProjects = projects.filter((proj) =>
              usage?.project_ids.includes(proj.id),
            );

            return (
              <div
                key={wf.id}
                role="button"
                tabIndex={0}
                onClick={() => handleOpenDetail(wf)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    handleOpenDetail(wf);
                  }
                }}
                className="group relative flex flex-col sm:flex-row sm:items-center justify-between gap-4 p-4 rounded-xl border border-border bg-card hover:border-sidebar-accent hover:shadow-xs transition-all cursor-pointer text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <div className="flex flex-col space-y-1.5 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <Workflow className="size-4 text-muted-foreground shrink-0" />
                    <span className="text-body-lg font-semibold text-foreground truncate">
                      {wf.name}
                    </span>
                    {wf.is_default && (
                      <Badge variant="secondary">
                        {t(($) => $.manage.default_badge)}
                      </Badge>
                    )}
                    <Badge variant="outline">
                      {t(($) => $.manage.steps_count, { count: wf.steps.length })}
                    </Badge>
                  </div>

                  {wf.description ? (
                    <p className="text-body-sm text-muted-foreground line-clamp-1">
                      {wf.description}
                    </p>
                  ) : null}

                  {/* Assigned Chips */}
                  <div className="flex items-center gap-1.5 flex-wrap pt-1">
                    {assignedProjects.slice(0, 3).map((proj) => (
                      <Badge
                        key={proj.id}
                        variant="ghost"
                        className="gap-1 text-micro text-muted-foreground border border-border/60 bg-muted/30"
                      >
                        <FolderKanban className="size-3" />
                        <span className="truncate max-w-[120px]">{proj.title}</span>
                      </Badge>
                    ))}
                    {assignedProjects.length > 3 && (
                      <Badge variant="ghost" className="text-micro text-muted-foreground">
                        +{assignedProjects.length - 3}
                      </Badge>
                    )}

                    {usage &&
                      usage.squad_ids.slice(0, 2).map((squadId) => (
                        <Badge
                          key={squadId}
                          variant="ghost"
                          className="gap-1 text-micro text-muted-foreground border border-border/60 bg-muted/30"
                        >
                          <ActorAvatar actorType="squad" actorId={squadId} className="size-3" />
                          <span className="truncate max-w-[100px]">
                            {getActorName("squad", squadId)}
                          </span>
                        </Badge>
                      ))}

                    {usage &&
                      usage.agent_ids.slice(0, 2).map((agentId) => (
                        <Badge
                          key={agentId}
                          variant="ghost"
                          className="gap-1 text-micro text-muted-foreground border border-border/60 bg-muted/30"
                        >
                          <ActorAvatar actorType="agent" actorId={agentId} className="size-3" />
                          <span className="truncate max-w-[100px]">
                            {getActorName("agent", agentId)}
                          </span>
                        </Badge>
                      ))}
                  </div>
                </div>

                {/* Right side: Updated time and Action menu */}
                <div className="flex items-center gap-3 shrink-0 self-end sm:self-center">
                  <span className="text-caption text-muted-foreground">
                    {timeAgo(wf.updated_at)}
                  </span>

                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={t(($) => $.manage.actions.menu)}
                          onClick={(e) => e.stopPropagation()}
                        >
                          <MoreHorizontal className="size-4" />
                        </Button>
                      }
                    />
                    <DropdownMenuContent align="end" className="w-40">
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation();
                          navigation.push(p.workflowDetail(wf.id));
                        }}
                      >
                        <Pencil className="size-4 mr-2" />
                        <span>{t(($) => $.manage.actions.edit)}</span>
                      </DropdownMenuItem>

                      <DropdownMenuItem
                        disabled={createWorkflow.isPending}
                        onClick={(e) => void handleDuplicate(e, wf)}
                      >
                        <Copy className="size-4 mr-2" />
                        <span>{t(($) => $.manage.actions.duplicate)}</span>
                      </DropdownMenuItem>

                      <DropdownMenuSeparator />

                      <DropdownMenuItem
                        variant="destructive"
                        onClick={(e) => handleDeletePrompt(e, wf)}
                      >
                        <Trash2 className="size-4 mr-2" />
                        <span>{t(($) => $.manage.actions.delete)}</span>
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Detail Drawer */}
      <WorkflowDetailDrawer
        workflow={selectedWorkflow}
        usage={selectedWorkflow ? usageById[selectedWorkflow.id] : undefined}
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        wsId={wsId}
      />

      {/* Delete Dialog */}
      <WorkflowDeleteDialog
        workflow={deleteTarget}
        blockers={deleteBlockers}
        open={deleteDialogOpen}
        onOpenChange={closeDeleteDialog}
        onConfirmDelete={() => void handleConfirmDelete()}
        isDeleting={deleteWorkflow.isPending}
      />
    </div>
  );
}
