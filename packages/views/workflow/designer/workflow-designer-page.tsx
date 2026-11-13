"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
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
  issueWorkflowStatusCreateForbidden,
  useCreateIssueWorkflow,
  useIssueWorkflowDetail,
  useUpdateIssueWorkflow,
} from "@multica/core/issue-workflows";
import { useWorkspacePaths } from "@multica/core/paths";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type {
  IssueWorkflow,
  IssueWorkflowMappingPlan,
  IssueWorkflowTemplate,
} from "@multica/core/types";
import { useStatusLabel } from "../../issues/utils/status-label";
import { AppLink, useNavigation } from "../../navigation";
import { useT } from "../../i18n";
import { WorkflowMappingDialog } from "../workflow-mapping-dialog";
import type {
  IssueWorkflowStep,
  IssueWorkflowTransition,
} from "../types";
import { useValidationMessage } from "../use-validation-message";
import { validateWorkflow } from "../validation";
import { workflowStepInputs } from "../workflow-data";
import { DesignerCanvas } from "./designer-canvas";
import { StepInspector } from "./step-inspector";
import { StatusPalette } from "./status-palette";
import { TransitionEditorDialog } from "./transition-editor-dialog";
import { ValidationList } from "./validation-list";
import { StartPointBar, type StartingPointKind } from "./start-point-bar";
import { CloneWorkflowDialog } from "./clone-workflow-dialog";
import { TemplatePickerDialog } from "./template-picker-dialog";
import { DiscardConfirmDialog } from "./discard-confirm-dialog";
import {
  cloneWorkflowSteps,
  deriveStepOrder,
  ensureStepLayouts,
  getAutoLayoutPosition,
  isUntouchedBlankSeed,
  newTransitionId,
} from "./clone-workflow-utils";

export interface WorkflowDesignerPageProps {
  workflowId?: string;
  isNew?: boolean;
}

function getInitialBlankSteps(t: ReturnType<typeof useT<"workflow">>["t"]): IssueWorkflowStep[] {
  return [
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
      layout: getAutoLayoutPosition(0),
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
      layout: getAutoLayoutPosition(1),
    },
    {
      status_key: "done",
      handler: { type: "none" },
      instructions: "",
      layout: getAutoLayoutPosition(2),
    },
    {
      status_key: "cancelled",
      handler: { type: "none" },
      instructions: "",
      layout: getAutoLayoutPosition(3),
    },
  ];
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
  const isTemplateManaged = Boolean(existingWorkflow?.template_ref);
  // A workflow Kit Việt manages (template) or a flow workflow is read-only here: its
  // server rules, not the editor, decide what changes, and the list hides edit too.
  const isReadOnly = isTemplateManaged || Boolean(existingWorkflow?.flow_config);
  const createWorkflow = useCreateIssueWorkflow();
  const updateWorkflow = useUpdateIssueWorkflow();

  // Draft state
  const [name, setName] = useState(isNew ? "New Workflow" : "");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<IssueWorkflowStep[]>([]);
  const [isInitialized, setIsInitialized] = useState(false);
  const [hasUserEdits, setHasUserEdits] = useState(false);
  const [activeStartingPoint, setActiveStartingPoint] = useState<StartingPointKind>("blank");

  const [paletteOpen, setPaletteOpen] = useState(false);
  const [selectedStepKey, setSelectedStepKey] = useState<string | undefined>();
  // Set when the server needs a status mapping for the statuses this edit removes.
  const [mappingPlan, setMappingPlan] = useState<IssueWorkflowMappingPlan | null>(null);

  // Dialog states for starting points
  const [cloneDialogOpen, setCloneDialogOpen] = useState(false);
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [discardDialogOpen, setDiscardDialogOpen] = useState(false);
  const [pendingDiscardAction, setPendingDiscardAction] = useState<(() => void) | null>(null);

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
      setSteps(getInitialBlankSteps(t));
      setIsInitialized(true);
      setHasUserEdits(false);
      setActiveStartingPoint("blank");
    } else if (existingWorkflow) {
      setName(existingWorkflow.name);
      setDescription(existingWorkflow.description || "");
      setSteps(ensureStepLayouts(existingWorkflow.steps));
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

  const selectedStep = useMemo(() => {
    return steps.find((s) => s.status_key === selectedStepKey) ?? null;
  }, [steps, selectedStepKey]);

  const availableStatusOptions = useMemo(() => {
    return steps.map((s) => {
      const def = statuses.find((st) => st.key === s.status_key);
      return {
        key: s.status_key,
        label: s.new_status ? s.new_status.name : def ? getStatusLabel(s.status_key) : s.status_key,
      };
    });
  }, [steps, statuses, getStatusLabel]);

  // Check if draft is still untouched blank seed
  const isDraftUntouched = useMemo(() => {
    if (!isNew) return false;
    if (activeStartingPoint !== "blank" || hasUserEdits) return false;
    return isUntouchedBlankSeed({
      name,
      description,
      steps,
      defaultName: t(($) => $.designer.default_name),
      defaultBranchStart: t(($) => $.designer.default_branch_start),
      defaultBranchDone: t(($) => $.designer.default_branch_done),
    });
  }, [isNew, activeStartingPoint, hasUserEdits, name, description, steps, t]);

  const handleSelectBlank = () => {
    if (activeStartingPoint === "blank" && isDraftUntouched) return;

    const action = () => {
      setName(t(($) => $.designer.default_name));
      setDescription("");
      setSteps(getInitialBlankSteps(t));
      setSelectedStepKey(undefined);
      setActiveStartingPoint("blank");
      setHasUserEdits(false);
    };

    if (isDraftUntouched) {
      action();
    } else {
      setPendingDiscardAction(() => action);
      setDiscardDialogOpen(true);
    }
  };

  const handleSelectWorkflowToClone = (sourceWf: IssueWorkflow) => {
    const action = () => {
      setName(t(($) => $.manage.copy_name, { name: sourceWf.name }));
      setDescription(sourceWf.description ?? "");
      setSteps(ensureStepLayouts(cloneWorkflowSteps(sourceWf.steps)));
      setSelectedStepKey(undefined);
      setActiveStartingPoint("clone");
      setHasUserEdits(false);
    };

    if (isDraftUntouched) {
      action();
    } else {
      setPendingDiscardAction(() => action);
      setDiscardDialogOpen(true);
    }
  };

  const handleSelectTemplate = (
    template: IssueWorkflowTemplate,
    tplName: string,
    tplDesc: string,
  ) => {
    const action = () => {
      setName(tplName);
      setDescription(tplDesc);
      // A created status is keyed the way the save will key it, against the live catalog.
      setSteps(ensureStepLayouts(cloneWorkflowSteps(template.steps, statuses)));
      setSelectedStepKey(undefined);
      setActiveStartingPoint("template");
      setHasUserEdits(false);
    };

    if (isDraftUntouched) {
      action();
    } else {
      setPendingDiscardAction(() => action);
      setDiscardDialogOpen(true);
    }
  };

  // Add status from palette
  const handleAddStatus = (statusKey: string) => {
    if (activeStatusKeys.has(statusKey)) return;

    const newIndex = steps.length;
    const newStep: IssueWorkflowStep = {
      status_key: statusKey,
      handler: { type: "none" },
      instructions: "",
      transitions: [],
      layout: getAutoLayoutPosition(newIndex),
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

    setSteps(ensureStepLayouts(nextSteps));
    setHasUserEdits(true);
    setSelectedStepKey(statusKey);
  };

  // Step Inspector & Keyboard Path Handlers
  const handleUpdateStep = (updated: IssueWorkflowStep) => {
    setSteps((prev) =>
      prev.map((s) => (s.status_key === updated.status_key ? updated : s)),
    );
    setHasUserEdits(true);
  };

  const handleMoveStepUp = (stepKey: string) => {
    const idx = steps.findIndex((s) => s.status_key === stepKey);
    if (idx <= 0) return;
    const next = [...steps];
    const prev = next[idx - 1]!;
    const curr = next[idx]!;
    // Swap layout positions
    const prevLayout = prev.layout ?? getAutoLayoutPosition(idx - 1);
    const currLayout = curr.layout ?? getAutoLayoutPosition(idx);
    next[idx - 1] = { ...curr, layout: prevLayout };
    next[idx] = { ...prev, layout: currLayout };
    setSteps(next);
    setHasUserEdits(true);
  };

  const handleMoveStepDown = (stepKey: string) => {
    const idx = steps.findIndex((s) => s.status_key === stepKey);
    if (idx < 0 || idx >= steps.length - 1) return;
    const next = [...steps];
    const curr = next[idx]!;
    const after = next[idx + 1]!;
    // Swap layout positions
    const currLayout = curr.layout ?? getAutoLayoutPosition(idx);
    const afterLayout = after.layout ?? getAutoLayoutPosition(idx + 1);
    next[idx] = { ...after, layout: currLayout };
    next[idx + 1] = { ...curr, layout: afterLayout };
    setSteps(next);
    setHasUserEdits(true);
  };

  const handleRemoveStep = (stepKey: string) => {
    if (stepKey === "todo" || stepKey === "done" || stepKey === "cancelled") return;
    const next = steps.filter((s) => s.status_key !== stepKey);
    setSteps(ensureStepLayouts(next));
    setHasUserEdits(true);
    if (selectedStepKey === stepKey) {
      setSelectedStepKey(undefined);
    }
  };

  // Transition dialog handlers
  const handleOpenAddTransition = (sourceKey: string, targetKey?: string) => {
    const destinationKey =
      targetKey ??
      steps.find((s) => s.status_key !== sourceKey)?.status_key ??
      sourceKey;

    const newTrans: IssueWorkflowTransition = {
      id: newTransitionId(),
      label: "",
      when: null,
      to_status_key: destinationKey,
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
    setHasUserEdits(true);
  };

  const handleDeleteTransition = (sourceKeyParam?: string, transitionIdParam?: string) => {
    const sourceKey = sourceKeyParam ?? editingTransition?.sourceKey;
    const transId = transitionIdParam ?? editingTransition?.transition.id;
    if (!sourceKey || !transId) return;

    setSteps((prevSteps) =>
      prevSteps.map((step) => {
        if (step.status_key !== sourceKey) return step;
        const transitions = step.transitions ?? [];
        return {
          ...step,
          transitions: transitions.filter((t) => t.id !== transId),
        };
      }),
    );
    setHasUserEdits(true);
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

    // Board order is the order of the saved steps, which the designer derives
    // from node position (left to right, then top to bottom) when saving.
    const orderedSteps = deriveStepOrder(ensureStepLayouts(steps));

    // A default workflow keeps its reserved name, so the name is not sent.
    const body = {
      ...(isDefault ? {} : { name: savedName }),
      description: description.trim(),
      steps: workflowStepInputs(orderedSteps),
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
      if (issueWorkflowStatusCreateForbidden(err)) {
        toast.error(t(($) => $.designer.status_create_forbidden));
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
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setHasUserEdits(true);
                }}
                disabled={isDefault || isReadOnly}
                placeholder={t(($) => $.designer.name_placeholder)}
                className="text-title font-semibold text-foreground bg-transparent border-none outline-none focus-visible:ring-1 focus-visible:ring-ring rounded px-1 -ml-1 placeholder:text-muted-foreground truncate disabled:opacity-70"
              />
              {isTemplateManaged && (
                <Badge variant="secondary" className="gap-1 border border-border/60 bg-muted/30 shrink-0">
                  {t(($) => $.manage.managed_by_template)}
                </Badge>
              )}
            </div>
            <input
              type="text"
              value={description}
              onChange={(e) => {
                setDescription(e.target.value);
                setHasUserEdits(true);
              }}
              disabled={isReadOnly}
              placeholder={t(($) => $.designer.description_placeholder)}
              className="text-caption text-muted-foreground bg-transparent border-none outline-none focus-visible:ring-1 focus-visible:ring-ring rounded px-1 -ml-1 placeholder:text-muted-foreground truncate disabled:opacity-70"
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

          {!isReadOnly && (
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
          )}
        </div>
      </header>

      {/* Starting point bar (only when isNew) */}
      {isNew && (
        <StartPointBar
          activeChoice={activeStartingPoint}
          onSelectBlank={handleSelectBlank}
          onOpenClone={() => setCloneDialogOpen(true)}
          onOpenTemplate={() => setTemplateDialogOpen(true)}
        />
      )}

      {/* Main Designer Area: Canvas + Inspector */}
      <div className="relative flex flex-1 overflow-hidden">
        <div className="flex-1 h-full relative overflow-hidden">
          <DesignerCanvas
            steps={steps}
            wsId={wsId}
            onUpdateSteps={(nextSteps) => {
              setSteps(nextSteps);
              setHasUserEdits(true);
            }}
            onAddTransition={handleOpenAddTransition}
            onEditTransition={handleOpenEditTransition}
            selectedStepKey={selectedStepKey}
            onSelectStep={setSelectedStepKey}
            readOnly={isReadOnly}
          />
        </div>

        {/* Step Inspector Panel */}
        <StepInspector
          step={selectedStep}
          steps={steps}
          wsId={wsId}
          onUpdateStep={handleUpdateStep}
          onMoveStepUp={handleMoveStepUp}
          onMoveStepDown={handleMoveStepDown}
          onRemoveStep={handleRemoveStep}
          onAddStep={() => setPaletteOpen(true)}
          onSelectStep={setSelectedStepKey}
          onAddTransition={handleOpenAddTransition}
          onEditTransition={handleOpenEditTransition}
          onDeleteTransition={(sourceKey, id) => handleDeleteTransition(sourceKey, id)}
          onClose={() => setSelectedStepKey(undefined)}
          readOnly={isReadOnly}
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
        onDelete={() => handleDeleteTransition()}
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

      {/* Clone Workflow Dialog */}
      {isNew && (
        <CloneWorkflowDialog
          open={cloneDialogOpen}
          onOpenChange={setCloneDialogOpen}
          wsId={wsId}
          onSelectWorkflow={handleSelectWorkflowToClone}
        />
      )}

      {/* Template Picker Dialog */}
      {isNew && (
        <TemplatePickerDialog
          open={templateDialogOpen}
          onOpenChange={setTemplateDialogOpen}
          wsId={wsId}
          onSelectTemplate={handleSelectTemplate}
        />
      )}

      {/* Discard confirmation dialog */}
      {isNew && (
        <DiscardConfirmDialog
          open={discardDialogOpen}
          onOpenChange={setDiscardDialogOpen}
          onConfirm={() => {
            pendingDiscardAction?.();
            setPendingDiscardAction(null);
          }}
        />
      )}
    </div>
  );
}
