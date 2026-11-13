"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Download,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Workflow,
  Zap,
} from "@multica/ui/icons";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import { clientErrorMessage } from "@multica/core/api";
import {
  flowDraftsOptions,
  useCreateFlowDraft,
  useExportIssueWorkflowManifest,
  useFlowCapabilities,
} from "@multica/core/flow-drafts";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink, useNavigation } from "../../navigation";
import { useT } from "../../i18n";
import {
  squadWorkflowsOptions,
  useSetFlowAuto,
  useSetSquadActiveWorkflow,
} from "@multica/core/issue-workflows";
import { WorkflowGraphSheet } from "./workflow-graph-sheet";
import { cutDraftName } from "../flow-designer/draft-name";
import { downloadJson, manifestFileName } from "../flow-designer/download-json";
import { StartDialog } from "../flow-designer/start-dialog";
import { ImportDialog } from "../flow-designer/import-dialog";

export interface SquadWorkflowsSectionProps {
  wsId: string;
  squad: Squad;
  canManage: boolean;
}

export function SquadWorkflowsSection({
  wsId,
  squad,
  canManage,
}: SquadWorkflowsSectionProps) {
  const { t } = useT("squads");
  const { push } = useNavigation();
  const paths = useWorkspacePaths();
  const [selectedWorkflow, setSelectedWorkflow] = useState<IssueWorkflow | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [startDialogOpen, setStartDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);

  const query = useQuery(squadWorkflowsOptions(wsId, squad.id));
  const capabilitiesQuery = useFlowCapabilities(wsId);
  const authoringEnabled = capabilitiesQuery.data?.features?.authoring ?? true;

  // Drafts are the manager's: the route answers 403 to anyone else.
  const draftsQuery = useQuery({ ...flowDraftsOptions(wsId, squad.id), enabled: canManage && authoringEnabled });
  const drafts = draftsQuery.data?.drafts ?? [];

  const setActiveMut = useSetSquadActiveWorkflow(wsId, squad.id);
  const setAutoMut = useSetFlowAuto(wsId);
  const createDraftMut = useCreateFlowDraft();
  const exportManifestMut = useExportIssueWorkflowManifest();

  const handleOpenGraph = (wf: IssueWorkflow) => {
    setSelectedWorkflow(wf);
    setSheetOpen(true);
  };

  const handleSetActive = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      await setActiveMut.mutateAsync({ workflow_id: wf.id });
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.workflows.error));
    }
  };

  const handleToggleAuto = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      await setAutoMut.mutateAsync({ workflowId: wf.id, auto: !wf.flow_auto });
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.workflows.error));
    }
  };

  // A copy of a flow workflow, of any version or origin, as a draft that carries its lineage. The
  // draft takes the workflow's name from the server.
  const handleEditAsDraft = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      const draft = await createDraftMut.mutateAsync({
        squadId: squad.id,
        data: { from: { kind: "workflow", workflow_id: wf.id } },
      });
      push(`${paths.squadDetail(squad.id)}/workflows/${draft.id}`);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.start.failed));
    }
  };

  const handleExport = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      const manifest = await exportManifestMut.mutateAsync(wf.id);
      downloadJson(manifestFileName(wf.name), manifest);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.editor.export_failed));
    }
  };

  const handleCreateBlank = async () => {
    try {
      const draft = await createDraftMut.mutateAsync({
        squadId: squad.id,
        data: { name: cutDraftName(squad.name), from: { kind: "blank" } },
      });
      push(`${paths.squadDetail(squad.id)}/workflows/${draft.id}`);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.start.failed));
    }
  };

  const rawWorkflows = query.data?.workflows;
  const workflows = useMemo(() => rawWorkflows ?? [], [rawWorkflows]);
  const activeId = query.data?.active_workflow_id ?? squad.active_workflow_id;

  // Group workflows by lineage_id if present, else by wf.id
  const groups = useMemo(() => {
    const map = new Map<string, IssueWorkflow[]>();
    for (const wf of workflows) {
      const key = wf.lineage_id ?? wf.id;
      const list = map.get(key) ?? [];
      list.push(wf);
      map.set(key, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => {
        const vA = a.version ?? 0;
        const vB = b.version ?? 0;
        if (vA !== vB) return vB - vA;
        return new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime();
      });
    }
    return Array.from(map.values());
  }, [workflows]);

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-title font-semibold text-foreground">
          <Workflow className="size-4 text-muted-foreground" aria-hidden />
          <span>{t(($) => $.workflows.section_title)}</span>
        </h2>
        {canManage && authoringEnabled && (
          <Button
            size="sm"
            onClick={() => setStartDialogOpen(true)}
            className="gap-1.5"
          >
            <Plus className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow_designer.new)}</span>
          </Button>
        )}
      </div>

      {query.isPending ? (
        <div className="flex items-center justify-center p-8 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" aria-hidden />
        </div>
      ) : query.isError ? (
        <div className="flex items-center justify-between rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-body text-destructive">
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" aria-hidden />
            <span>{t(($) => $.workflows.error)}</span>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
            className="gap-1.5"
          >
            <RefreshCw className="size-3.5" aria-hidden />
            <span>{t(($) => $.workflows.retry)}</span>
          </Button>
        </div>
      ) : workflows.length === 0 ? (
        canManage && authoringEnabled ? (
          <div className="rounded-xl border border-border/60 bg-muted/20 p-6 text-center space-y-4">
            <p className="text-body text-muted-foreground">{t(($) => $.workflows.empty)}</p>
            <div className="flex flex-wrap items-center justify-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleCreateBlank}
                disabled={createDraftMut.isPending}
              >
                {t(($) => $.flow_designer.start.blank)}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setImportDialogOpen(true)}
              >
                {t(($) => $.flow_designer.start.import)}
              </Button>
            </div>
          </div>
        ) : (
          <div className="rounded-xl border border-border/60 bg-muted/20 p-6 text-center text-body text-muted-foreground">
            {t(($) => $.workflows.empty)}
          </div>
        )
      ) : (
        <div className="space-y-4">
          {groups.map((group, groupIdx) => {
            const first = group[0];
            if (!first) return null;
            const hasMultipleInLineage = group.length > 1;
            return (
              <div
                key={first.lineage_id ?? `group-${groupIdx}`}
                data-testid={first.lineage_id ? `lineage-group-${first.lineage_id}` : undefined}
                className={hasMultipleInLineage ? "rounded-xl border border-border/70 p-2 space-y-2 bg-muted/10" : "space-y-2"}
              >
                {group.map((wf) => {
                  const isActive = wf.id === activeId;
                  const totalTasks = wf.steps.reduce(
                    (acc, step) => acc + (step.tasks?.length ?? 0),
                    0,
                  );
                  const isFlow = wf.flow_config != null;

                  return (
                    <div
                      key={wf.id}
                      tabIndex={0}
                      onClick={() => handleOpenGraph(wf)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          handleOpenGraph(wf);
                        }
                      }}
                      className="group flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl border border-border bg-card p-4 hover:border-sidebar-accent hover:shadow-xs transition-all cursor-pointer text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <div className="flex flex-col space-y-1.5 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-medium text-body text-foreground truncate">
                            {wf.name}
                          </span>
                          {wf.version != null && (
                            <Badge variant="outline" className="text-micro font-mono shrink-0">
                              {`v${wf.version}`}
                            </Badge>
                          )}
                          {isActive && (
                            <Badge variant="default" className="text-micro font-medium gap-1 shrink-0">
                              <CheckCircle2 className="size-3" aria-hidden />
                              <span>{t(($) => $.workflows.badge_active)}</span>
                            </Badge>
                          )}
                          {wf.template_ref && (
                            <Badge variant="secondary" className="text-micro shrink-0">
                              {wf.template_ref}
                            </Badge>
                          )}
                        </div>

                        <div className="flex items-center gap-2 text-caption text-muted-foreground flex-wrap">
                          <span>
                            {t(($) => $.workflows.steps_count, { count: wf.steps.length })}
                          </span>
                          <span>·</span>
                          <span>
                            {t(($) => $.workflows.tasks_count, { count: totalTasks })}
                          </span>
                          {typeof wf.open_flow_requests === "number" && wf.open_flow_requests > 0 && (
                            <>
                              <span>·</span>
                              <span className="text-amber-600 dark:text-amber-400 font-medium">
                                {t(($) => $.workflows.open_requests, { count: wf.open_flow_requests })}
                              </span>
                            </>
                          )}
                        </div>
                      </div>

                      {/* Right side: Admin actions & Chevron */}
                      <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                        {canManage && (
                          <>
                            {/* Set Active */}
                            {!isActive && (
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={setActiveMut.isPending}
                                onClick={(e) => void handleSetActive(e, wf)}
                                className="text-caption h-8"
                              >
                                {t(($) => $.workflows.set_active)}
                              </Button>
                            )}

                            {/* Auto Toggle (for flow workflows) */}
                            {wf.flow_config != null && (
                              <Button
                                variant={wf.flow_auto ? "secondary" : "ghost"}
                                size="sm"
                                disabled={setAutoMut.isPending}
                                onClick={(e) => void handleToggleAuto(e, wf)}
                                className="gap-1.5 text-caption h-8"
                              >
                                <Zap className="size-3.5" aria-hidden />
                                <span>
                                  {wf.flow_auto
                                    ? t(($) => $.workflows.auto_on)
                                    : t(($) => $.workflows.auto_off)}
                                </span>
                              </Button>
                            )}

                            {/* Edit as draft */}
                            {isFlow && authoringEnabled && (
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={createDraftMut.isPending}
                                onClick={(e) => void handleEditAsDraft(e, wf)}
                                className="gap-1 text-caption h-8"
                              >
                                <Pencil className="size-3.5" aria-hidden />
                                <span>{t(($) => $.flow_designer.edit_as_draft)}</span>
                              </Button>
                            )}
                          </>
                        )}

                        {/* Export */}
                        {canManage && authoringEnabled && wf.flow_config != null && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={(e) => void handleExport(e, wf)}
                            className="gap-1 text-caption h-8"
                          >
                            <Download className="size-3.5" aria-hidden />
                            <span>{t(($) => $.flow_designer.export)}</span>
                          </Button>
                        )}

                        <ChevronRight className="size-4 text-muted-foreground group-hover:translate-x-0.5 transition-transform" aria-hidden />
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      )}

      {/* Graph Sheet */}
      <WorkflowGraphSheet
        workflow={selectedWorkflow}
        open={sheetOpen}
        onOpenChange={setSheetOpen}
      />

      {drafts.length > 0 && (
        <div className="space-y-1.5">
          <h3 className="text-caption font-medium text-muted-foreground">{t(($) => $.flow_designer.drafts)}</h3>
          <ul className="divide-y divide-border/60 rounded-xl border border-border bg-card">
            {drafts.map((draft) => (
              <li key={draft.id}>
                <AppLink
                  href={`${paths.squadDetail(squad.id)}/workflows/${draft.id}`}
                  className="flex items-center justify-between gap-2 px-4 py-2.5 text-body hover:bg-muted/40"
                >
                  <span className="min-w-0 truncate font-medium text-foreground">{draft.name}</span>
                  <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                </AppLink>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Start Dialog */}
      <StartDialog
        open={startDialogOpen}
        onOpenChange={setStartDialogOpen}
        squad={squad}
        workflows={workflows}
        onOpenImport={() => {
          setStartDialogOpen(false);
          setImportDialogOpen(true);
        }}
      />

      {/* Import Dialog */}
      <ImportDialog
        open={importDialogOpen}
        onOpenChange={setImportDialogOpen}
        squad={squad}
      />
    </section>
  );
}
