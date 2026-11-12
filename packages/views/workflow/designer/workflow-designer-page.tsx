"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@multica/ui/components/ui/sheet";
import {
  ArrowLeft,
  PanelRightClose,
  PanelRightOpen,
  Save,
} from "@multica/ui/icons";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  issueWorkflowMappingPlanFrom,
  useCreateIssueWorkflow,
  useIssueWorkflowDetail,
  useUpdateIssueWorkflow,
} from "@multica/core/issue-workflows";
import { useWorkspacePaths } from "@multica/core/paths";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type { IssueWorkflowMappingPlan } from "@multica/core/types";
import { useStatusLabel } from "../../issues/utils/status-label";
import { AppLink, useNavigation } from "../../navigation";
import { useT } from "../../i18n";
import { WorkflowMappingDialog } from "../../workflows/workflow-mapping-dialog";
import type {
  IssueWorkflowStep,
  IssueWorkflowTransition,
} from "../types";
import { useValidationMessage } from "../use-validation-message";
import { validateWorkflow } from "../validation";
import { workflowStepInputs } from "../workflow-data";
import { DesignerCanvas } from "./designer-canvas";
import { StatusPalette } from "./status-palette";
import { TransitionEditorDialog } from "./transition-editor-dialog";
import { ValidationList } from "./validation-list";

export interface WorkflowDesignerPageProps {
  workflowId?: string;
  isNew?: boolean;
}

/** A new transition id, unique within the draft. */
function newTransitionId(): string {
  return `trans-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export function WorkflowDesignerPage({
  workflowId,
  isNew = false,
}: WorkflowDesignerPageProps) {
  const { t } = useT("workflow");
  const renderValidationMessage = useValidationMessage();
  const wsId = useWorkspaceId();
  const p = useWorkspacePaths();
  const navigation = useNavigation();
  const { statuses } = useIssueStatuses(wsId);
  const getStatusLabel = useStatusLabel(wsId);

  const detail = useIssueWorkflowDetail(wsId, isNew ? undefined : workflowId);
  const existingWorkflow = detail.workflow;
  const isDefault = existingWorkflow?.is_default ?? false;
  const createWorkflow = useCreateIssueWorkflow();
  const updateWorkflow = useUpdateIssueWorkflow();

  // Draft state
  const [name, setName] = useState(isNew ? "New Workflow" : "");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<IssueWorkflowStep[]>([]);
  const [isInitialized, setIsInitialized] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [selectedStepKey, setSelectedStepKey] = useState<string | undefined>();
  // Set when the server needs a status mapping for the statuses this edit removes.
  const [mappingPlan, setMappingPlan] = useState<IssueWorkflowMappingPlan | null>(null);

  // Transition editing dialog
  const [editingTransition, setEditingTransition] = useState<{
    sourceKey: string;
    transition: IssueWorkflowTransition;
  } | null>(null);
  const [transitionDialogOpen, setTransitionDialogOpen] = useState(false);

  // Initialize draft once, from the new-workflow seed or the stored workflow.
  useEffect(() => {
    if (isInitialized) return;

    if (isNew) {
      setName(t(($) => $.designer.default_name));
      setDescription("");
      setSteps([
        {
          status_key: "todo",
          handler: { type: "none" },
          instructions: "",
          transitions: [
            {
              id: "todo-default",
              label: t(($) => $.designer.default_branch_start),
              when: null,
              to_status_key: "in_progress",
            },
          ],
        },
        {
          status_key: "in_progress",
          handler: { type: "none" },
          instructions: "",
          transitions: [
            {
              id: "in_progress-done",
              label: t(($) => $.designer.default_branch_done),
              when: null,
              to_status_key: "done",
            },
          ],
        },
        {
          status_key: "done",
          handler: { type: "none" },
          instructions: "",
        },
        {
          status_key: "cancelled",
          handler: { type: "none" },
          instructions: "",
        },
      ]);
      setIsInitialized(true);
    } else if (existingWorkflow) {
      setName(existingWorkflow.name);
      setDescription(existingWorkflow.description || "");
      setSteps(existingWorkflow.steps);
      setIsInitialized(true);
    }
  }, [isNew, existingWorkflow, isInitialized, t]);

  // Live validation
  const validationIssues = useMemo(() => {
    return validateWorkflow(steps);
  }, [steps]);

  const activeStatusKeys = useMemo(() => {
    return new Set(steps.map((s) => s.status_key));
  }, [steps]);

  const availableStatusOptions = useMemo(() => {
    return steps.map((s) => {
      const def = statuses.find((st) => st.key === s.status_key);
      return {
        key: s.status_key,
        label: def ? getStatusLabel(s.status_key) : s.status_key,
      };
    });
  }, [steps, statuses, getStatusLabel]);

  // Add status from palette
  const handleAddStatus = (statusKey: string) => {
    if (activeStatusKeys.has(statusKey)) return;

    const newStep: IssueWorkflowStep = {
      status_key: statusKey,
      handler: { type: "none" },
      instructions: "",
      transitions: [],
    };

    // Insert before fixed tail (blocked, done, cancelled)
    const hasBlocked = activeStatusKeys.has("blocked");
    const tailStartIndex = steps.findIndex(
      (s) =>
        s.status_key === (hasBlocked ? "blocked" : "done") ||
        s.status_key === "cancelled",
    );

    const nextSteps = [...steps];
    if (tailStartIndex !== -1) {
      nextSteps.splice(tailStartIndex, 0, newStep);
    } else {
      nextSteps.push(newStep);
    }

    setSteps(nextSteps);
    setSelectedStepKey(statusKey);
  };

  // Transition dialog handlers
  const handleOpenAddTransition = (sourceKey: string) => {
    const targetKey =
      steps.find((s) => s.status_key !== sourceKey)?.status_key || sourceKey;

    const newTrans: IssueWorkflowTransition = {
      id: newTransitionId(),
      label: "",
      when: null,
      to_status_key: targetKey,
    };

    setEditingTransition({ sourceKey, transition: newTrans });
    setTransitionDialogOpen(true);
  };

  const handleOpenEditTransition = (
    sourceKey: string,
    transition: IssueWorkflowTransition,
  ) => {
    setEditingTransition({ sourceKey, transition });
    setTransitionDialogOpen(true);
  };

  const handleSaveTransition = (updated: IssueWorkflowTransition) => {
    if (!editingTransition) return;
    const { sourceKey } = editingTransition;

    setSteps((prevSteps) =>
      prevSteps.map((step) => {
        if (step.status_key !== sourceKey) return step;

        const transitions = step.transitions ?? [];
        const exists = transitions.some((t) => t.id === updated.id);
        const nextTransitions = exists
          ? transitions.map((t) => (t.id === updated.id ? updated : t))
          : [...transitions, updated];

        return { ...step, transitions: nextTransitions };
      }),
    );
  };

  const handleDeleteTransition = () => {
    if (!editingTransition) return;
    const { sourceKey, transition } = editingTransition;

    setSteps((prevSteps) =>
      prevSteps.map((step) => {
        if (step.status_key !== sourceKey) return step;
        const transitions = step.transitions ?? [];
        return {
          ...step,
          transitions: transitions.filter((t) => t.id !== transition.id),
        };
      }),
    );
  };

  const isSaving = createWorkflow.isPending || updateWorkflow.isPending;
  const savedName = name.trim() || t(($) => $.designer.untitled_name);

  const save = async (mapping?: Record<string, string>) => {
    const errors = validationIssues.filter((i) => i.severity === "error");
    if (errors.length > 0) {
      toast.error(
        errors[0] ? renderValidationMessage(errors[0].message) : t(($) => $.designer.validation.fix_errors),
      );
      return;
    }

    // A default workflow keeps its reserved name, so the name is not sent.
    const body = {
      ...(isDefault ? {} : { name: savedName }),
      description: description.trim(),
      steps: workflowStepInputs(steps),
      ...(mapping ? { status_mapping: mapping } : {}),
    };

    try {
      if (!isNew && workflowId) {
        await updateWorkflow.mutateAsync({ id: workflowId, ...body });
        setMappingPlan(null);
        toast.success(t(($) => $.designer.saved));
      } else {
        const created = await createWorkflow.mutateAsync({ ...body, name: savedName });
        toast.success(t(($) => $.designer.saved));
        navigation.replace(p.workflowDetail(created.id));
      }
    } catch (err) {
      const plan = issueWorkflowMappingPlanFrom(err);
      if (plan) {
        setMappingPlan(plan);
        return;
      }
      toast.error(clientErrorMessage(err) ?? t(($) => $.designer.save_failed));
    }
  };

  if (!isNew && detail.isError) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-body text-muted-foreground">
        {t(($) => $.designer.load_failed)}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full w-full bg-background overflow-hidden">
      {/* Top Bar / Toolbar */}
      <header className="flex items-center justify-between px-6 py-3 border-b border-border bg-card shrink-0 gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <AppLink href={p.workflow()}>
            <Button variant="ghost" size="icon-sm" aria-label={t(($) => $.designer.back_to_workflows)}>
              <ArrowLeft className="size-4" />
            </Button>
          </AppLink>

          <div className="flex flex-col min-w-0">
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isDefault}
              placeholder={t(($) => $.designer.name_placeholder)}
              className="text-title font-semibold text-foreground bg-transparent border-none outline-none focus-visible:ring-1 focus-visible:ring-ring rounded px-1 -ml-1 placeholder:text-muted-foreground truncate disabled:opacity-70"
            />
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t(($) => $.designer.description_placeholder)}
              className="text-caption text-muted-foreground bg-transparent border-none outline-none focus-visible:ring-1 focus-visible:ring-ring rounded px-1 -ml-1 placeholder:text-muted-foreground truncate"
            />
          </div>
        </div>

        {/* Right actions: Palette trigger, Live validation list, Save */}
        <div className="flex items-center gap-2 shrink-0">
          <Button
            type="button"
            variant={paletteOpen ? "secondary" : "outline"}
            size="sm"
            onClick={() => setPaletteOpen((prev) => !prev)}
            className="gap-1.5 h-8 text-caption"
          >
            {paletteOpen ? (
              <PanelRightClose className="size-4" />
            ) : (
              <PanelRightOpen className="size-4" />
            )}
            <span>{t(($) => $.designer.status_palette)}</span>
          </Button>

          <ValidationList
            issues={validationIssues}
            onSelectStep={setSelectedStepKey}
          />

          <Button
            type="button"
            size="sm"
            onClick={() => void save()}
            disabled={isSaving || !isInitialized}
            className="gap-1.5 h-8"
          >
            {isSaving ? (
              <span>{t(($) => $.designer.saving)}</span>
            ) : (
              <>
                <Save className="size-4" />
                <span>{t(($) => $.designer.save)}</span>
              </>
            )}
          </Button>
        </div>
      </header>

      {/* Main Designer Area */}
      <div className="relative flex flex-1 overflow-hidden">
        <DesignerCanvas
          steps={steps}
          wsId={wsId}
          onUpdateSteps={setSteps}
          onAddTransition={handleOpenAddTransition}
          onEditTransition={handleOpenEditTransition}
          selectedStepKey={selectedStepKey}
          onSelectStep={setSelectedStepKey}
        />

        {/* Status Palette Drawer/Sheet */}
        <Sheet open={paletteOpen} onOpenChange={setPaletteOpen}>
          <SheetContent side="right" className="w-80 sm:w-96 p-6">
            <SheetHeader className="pb-2">
              <SheetTitle>{t(($) => $.designer.status_palette)}</SheetTitle>
            </SheetHeader>
            <StatusPalette
              wsId={wsId}
              activeStatusKeys={activeStatusKeys}
              onAddStatus={handleAddStatus}
            />
          </SheetContent>
        </Sheet>
      </div>

      {/* Transition / Edge Editor Dialog */}
      <TransitionEditorDialog
        open={transitionDialogOpen}
        onOpenChange={setTransitionDialogOpen}
        transition={editingTransition?.transition ?? null}
        sourceStepKey={editingTransition?.sourceKey ?? ""}
        availableStatuses={availableStatusOptions}
        onSave={handleSaveTransition}
        onDelete={handleDeleteTransition}
      />

      {/* Status mapping for the statuses this edit removes */}
      <WorkflowMappingDialog
        open={mappingPlan !== null}
        onOpenChange={(open) => {
          if (!open) setMappingPlan(null);
        }}
        variant={{ kind: "edit", workflowName: savedName }}
        plan={mappingPlan}
        targetKeys={steps.map((s) => s.status_key)}
        pending={updateWorkflow.isPending}
        onConfirm={(mapping) => void save(mapping)}
      />
    </div>
  );
}
