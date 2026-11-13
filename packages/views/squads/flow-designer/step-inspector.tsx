/* eslint-disable i18next/no-literal-string, no-restricted-syntax */
import { useMemo } from "react";
import type { FlowDoc, FlowDocJoin, FlowDocStep, FlowDocTask, Skill, SquadMember } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Plus } from "@multica/ui/icons";
import { TaskCard } from "./task-card";
import { EdgeList } from "./edge-list";
import { useT } from "../../i18n";

export interface StepInspectorProps {
  step: FlowDocStep;
  onChangeStep: (updated: FlowDocStep) => void;
  doc: FlowDoc;
  squadMembers?: SquadMember[];
  squadSkills?: Skill[];
  taskLimit?: number;
}

export function StepInspector({
  step,
  onChangeStep,
  doc,
  squadMembers = [],
  squadSkills = [],
  taskLimit = 12,
}: StepInspectorProps) {
  const { t } = useT("squads");

  const atTaskLimit = (step.tasks?.length ?? 0) >= taskLimit;

  // Predecessor tasks: tasks from steps that have a path reaching this step
  const predecessorTasks = useMemo(() => {
    // Collect all steps other than this step
    const result: { stepKey: string; taskKey: string; label: string }[] = [];
    for (const otherStep of doc.steps) {
      if (otherStep.key === step.key) continue;
      for (const task of otherStep.tasks ?? []) {
        result.push({
          stepKey: otherStep.key,
          taskKey: task.key,
          label: task.title || task.key,
        });
      }
    }
    return result;
  }, [doc.steps, step.key]);

  const handleAddTask = () => {
    if (atTaskLimit) return;
    const taskKey = `${step.key}_t${(step.tasks?.length ?? 0) + 1}`;
    const newTask: FlowDocTask = {
      key: taskKey,
      title: `Task ${(step.tasks?.length ?? 0) + 1}`,
      kind: "work",
      role: doc.roles?.[0]?.key || "worker",
    };
    onChangeStep({
      ...step,
      tasks: [...(step.tasks ?? []), newTask],
    });
  };

  const handleUpdateTask = (index: number, updated: FlowDocTask) => {
    const nextTasks = [...(step.tasks ?? [])];
    nextTasks[index] = updated;
    onChangeStep({ ...step, tasks: nextTasks });
  };

  const handleDeleteTask = (index: number) => {
    const nextTasks = (step.tasks ?? []).filter((_, i) => i !== index);
    onChangeStep({ ...step, tasks: nextTasks });
  };

  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-6 text-foreground max-w-3xl">
      {/* Step Basic Info */}
      <div className="space-y-3 pb-4 border-b border-border">
        <div className="flex items-center justify-between gap-2">
          <Input
            value={step.title}
            onChange={(e) => onChangeStep({ ...step, title: e.target.value })}
            placeholder="Step title"
            className="text-body font-semibold h-9 max-w-sm"
          />
          <Badge variant="outline" className="font-mono text-micro">
            {step.key}
          </Badge>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label className="text-micro text-muted-foreground block mb-0.5">Kind</label>
            <select
              value={step.kind}
              onChange={(e) =>
                onChangeStep({
                  ...step,
                  kind: e.target.value as FlowDocStep["kind"],
                })
              }
              className="w-full h-8 rounded-md border border-input bg-background px-2 text-caption"
            >
              <option value="tasks">{t(($) => $.flow_designer.step.kind.tasks)}</option>
              <option value="lead">{t(($) => $.flow_designer.step.kind.lead)}</option>
              <option value="manual">{t(($) => $.flow_designer.step.kind.manual)}</option>
            </select>
          </div>

          {step.kind === "tasks" && (
            <div>
              <label className="text-micro text-muted-foreground block mb-0.5">
                {t(($) => $.flow_designer.join.title)}
              </label>
              <select
                value={step.join?.mode || "all"}
                onChange={(e) => {
                  const mode = e.target.value as FlowDocJoin["mode"];
                  onChangeStep({
                    ...step,
                    join: {
                      over: step.join?.over || "all",
                      mode,
                      block: step.join?.block || "immediate",
                      k: mode === "quorum" ? step.join?.k || 2 : undefined,
                    },
                  });
                }}
                className="w-full h-8 rounded-md border border-input bg-background px-2 text-caption"
              >
                <option value="all">{t(($) => $.flow_designer.join.all)}</option>
                <option value="any">{t(($) => $.flow_designer.join.any)}</option>
                <option value="quorum">Quorum</option>
              </select>
            </div>
          )}

          {step.kind === "manual" && (
            <div>
              <label className="text-micro text-muted-foreground block mb-0.5">Exit Role</label>
              <select
                value={step.exit_role || "managers"}
                onChange={(e) => onChangeStep({ ...step, exit_role: e.target.value })}
                className="w-full h-8 rounded-md border border-input bg-background px-2 text-caption"
              >
                <option value="managers">{t(($) => $.flow_designer.roles.kind.managers)}</option>
                <option value="requester">Requester</option>
                {(doc.roles ?? [])
                  .filter((r) => r.kind === "member")
                  .map((r) => (
                    <option key={r.key} value={r.key}>
                      {r.label}
                    </option>
                  ))}
              </select>
            </div>
          )}

          <div>
            <label className="text-micro text-muted-foreground block mb-0.5">Notice message key</label>
            <Input
              value={step.notice || ""}
              onChange={(e) => onChangeStep({ ...step, notice: e.target.value || undefined })}
              placeholder="e.g. step_notice"
              className="h-8 text-caption"
            />
          </div>
        </div>
      </div>

      {/* Tasks Section (only if kind === "tasks") */}
      {step.kind === "tasks" && (
        <div className="space-y-3 pb-4 border-b border-border">
          <div className="flex items-center justify-between">
            <span className="text-caption font-semibold">
              Tasks ({step.tasks?.length ?? 0} / {taskLimit})
            </span>
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled={atTaskLimit}
              onClick={handleAddTask}
              className="h-7 gap-1 text-caption"
            >
              <Plus className="size-3" aria-hidden />
              <span>{t(($) => $.flow_designer.task.add)}</span>
            </Button>
          </div>

          <div className="space-y-3">
            {(step.tasks ?? []).map((task, idx) => (
              <TaskCard
                key={task.key || idx}
                task={task}
                onChangeTask={(updated) => handleUpdateTask(idx, updated)}
                onDeleteTask={() => handleDeleteTask(idx)}
                doc={doc}
                currentStepKey={step.key}
                squadMembers={squadMembers}
                squadSkills={squadSkills}
                predecessorTasks={predecessorTasks}
              />
            ))}
            {(step.tasks ?? []).length === 0 && (
              <div className="p-4 text-center text-caption text-muted-foreground rounded-lg border border-dashed border-border">
                No tasks in this step. Click &ldquo;Add task&rdquo; to create one.
              </div>
            )}
          </div>
        </div>
      )}

      {/* Edges Section */}
      <EdgeList
        edges={step.edges ?? []}
        onChangeEdges={(edges) => onChangeStep({ ...step, edges })}
        doc={doc}
        currentStepKey={step.key}
      />
    </div>
  );
}
