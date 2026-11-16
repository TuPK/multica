"use client";

import { useMemo } from "react";
import type { FlowDoc, FlowDocTask, SquadMember } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { AlertCircle, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface TaskCardProps {
  task: FlowDocTask;
  onChangeTask: (updated: FlowDocTask) => void;
  onDeleteTask: () => void;
  doc: FlowDoc;
  currentStepKey: string;
  squadMembers?: SquadMember[];
  /** The skills the role of this task has: the ones it can be asked to use. */
  roleSkills?: string[];
  predecessorTasks?: { stepKey: string; taskKey: string; label: string; slots?: string[] }[];
}

export function TaskCard({
  task,
  onChangeTask,
  onDeleteTask,
  doc,
  currentStepKey: _currentStepKey,
  squadMembers: _squadMembers = [],
  roleSkills = [],
  predecessorTasks = [],
}: TaskCardProps) {
  const { t } = useT("squads");

  // Roles available
  const availableRoles = useMemo(() => {
    return (doc.roles ?? []).map((r) => ({ key: r.key, label: r.label || r.key }));
  }, [doc.roles]);

  // Verdict sets available
  const availableVerdictSets = useMemo(() => {
    const builtIns = ["work", "gate", "plan"];
    const custom = (doc.verdicts ?? []).map((v) => v.key);
    return Array.from(new Set([...builtIns, ...custom]));
  }, [doc.verdicts]);

  // The variables the role of this task may set (`Var.set_by`, T08). One the task already sets stays listed, so it can be unticked.
  const declaredVars = useMemo(() => {
    return (doc.vars ?? [])
      .filter((v) => v.set_by?.includes(task.role) || task.sets?.includes(v.key))
      .map((v) => v.key);
  }, [doc.vars, task.role, task.sets]);

  // Check R09: any skill in task.skills that the role does not have
  const missingSkills = useMemo(() => {
    if (!task.skills?.length) return [];
    return task.skills.filter((s) => !roleSkills.includes(s));
  }, [task.skills, roleSkills]);

  const handleToggleSetVar = (varKey: string) => {
    const currentSets = task.sets ?? [];
    const nextSets = currentSets.includes(varKey)
      ? currentSets.filter((k) => k !== varKey)
      : [...currentSets, varKey];
    // Also clean up requires.vars if removed from sets
    const currentReq = task.requires?.vars ?? [];
    const nextReq = currentReq.filter((k) => nextSets.includes(k));
    onChangeTask({
      ...task,
      sets: nextSets,
      requires: {
        ...task.requires,
        vars: nextReq,
      },
    });
  };

  const handleToggleRequiredVar = (varKey: string) => {
    const currentReq = task.requires?.vars ?? [];
    const nextReq = currentReq.includes(varKey)
      ? currentReq.filter((k) => k !== varKey)
      : [...currentReq, varKey];
    onChangeTask({
      ...task,
      requires: {
        ...task.requires,
        vars: nextReq,
      },
    });
  };

  return (
    <div className="p-3.5 rounded-lg border border-border bg-card space-y-3 text-caption">
      {/* Header: title, role, kind, delete */}
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 space-y-1">
          <Input
            value={task.title}
            onChange={(e) => onChangeTask({ ...task, title: e.target.value })}
            placeholder={t(($) => $.flow_designer.task_card.title_placeholder)}
            className="h-7 text-caption font-medium max-w-sm"
          />
          <span className="font-mono text-micro text-muted-foreground block">
            {t(($) => $.flow_designer.task_card.key, { key: task.key })}
          </span>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <Badge variant={task.kind === "gate" ? "outline" : "secondary"} className="text-micro font-normal">
            {task.kind === "gate"
              ? t(($) => $.flow_designer.task_card.kind_gate)
              : task.kind === "work"
                ? t(($) => $.flow_designer.task_card.kind_work)
                : task.kind}
          </Badge>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            aria-label={t(($) => $.flow_designer.task_card.delete, { key: task.key })}
            onClick={onDeleteTask}
            className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="size-3.5" aria-hidden />
          </Button>
        </div>
      </div>

      {/* Role & Verdict set */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div>
          <label className="text-micro text-muted-foreground block mb-0.5">{t(($) => $.flow_designer.task_card.role)}</label>
          <select
            value={task.role}
            onChange={(e) => onChangeTask({ ...task, role: e.target.value })}
            className="w-full h-7 rounded border border-input bg-background px-2 text-micro"
          >
            {availableRoles.map((r) => (
              <option key={r.key} value={r.key}>
                {r.label} ({r.key})
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="text-micro text-muted-foreground block mb-0.5">
            {t(($) => $.flow_designer.verdicts.title)}
          </label>
          <select
            value={task.verdicts || (task.kind === "gate" ? "gate" : "work")}
            onChange={(e) => onChangeTask({ ...task, verdicts: e.target.value })}
            className="w-full h-7 rounded border border-input bg-background px-2 text-micro"
          >
            {availableVerdictSets.map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Inputs (Spec 11.9): Only tasks in other steps with a path to this step */}
      <div className="space-y-1">
        <label className="text-micro font-medium text-muted-foreground block">
          {t(($) => $.flow_designer.task.inputs)}
        </label>
        {predecessorTasks.length > 0 ? (
          <div className="space-y-1 max-h-28 overflow-y-auto rounded border border-border p-1.5">
            {predecessorTasks.map((pt) => {
              const inputRef = `${pt.stepKey}.${pt.taskKey}`;
              const isSelected = task.inputs?.some((inp) => inp.from === inputRef);

              return (
                <div key={inputRef} className="flex items-center gap-1.5 text-micro hover:bg-muted/30 p-1 rounded">
                  <label className="flex flex-1 items-center gap-1.5 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={isSelected ?? false}
                      onChange={(e) => {
                        const cur = task.inputs ?? [];
                        const next = e.target.checked
                          ? [...cur, { from: inputRef, required: false }]
                          : cur.filter((i) => i.from !== inputRef);
                        onChangeTask({ ...task, inputs: next });
                      }}
                      className="size-3 rounded border-input"
                    />
                    <span className="font-mono">{inputRef}</span>
                    <span className="text-muted-foreground">({pt.label})</span>
                  </label>
                  {isSelected && (pt.slots?.length ?? 0) > 0 && (
                    <select
                      aria-label={t(($) => $.flow_designer.task_card.input_slot, { from: inputRef })}
                      value={task.inputs?.find((i) => i.from === inputRef)?.slot ?? ""}
                      onChange={(e) => {
                        const slot = e.target.value || undefined;
                        onChangeTask({
                          ...task,
                          inputs: (task.inputs ?? []).map((i) => {
                            if (i.from !== inputRef) return i;
                            const { slot: _old, ...rest } = i;
                            return slot ? { ...rest, slot } : rest;
                          }),
                        });
                      }}
                      className="h-6 rounded border border-input bg-background px-1 text-micro"
                    >
                      <option value="">{t(($) => $.flow_designer.task_card.input_any_slot)}</option>
                      {pt.slots!.map((slot) => (
                        <option key={slot} value={slot}>
                          {slot}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-micro text-muted-foreground italic">
            {t(($) => $.flow_designer.task_card.no_inputs)}
          </p>
        )}
      </div>

      {/* Skills (Spec 11.9, D18) */}
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <label className="text-micro font-medium text-muted-foreground">
            {t(($) => $.flow_designer.task.skills)}
          </label>
          {missingSkills.length > 0 && (
            <span className="flex items-center gap-1 text-micro text-destructive font-medium">
              <AlertCircle className="size-3" aria-hidden />
              <span>{t(($) => $.flow_designer.task_card.missing_skills, { skills: missingSkills.join(", ") })}</span>
            </span>
          )}
        </div>
        <div className="flex gap-1.5 flex-wrap">
          {roleSkills.map((skillName) => {
            const isSelected = task.skills?.includes(skillName);
            return (
              <Badge
                key={skillName}
                variant={isSelected ? "secondary" : "outline"}
                className="cursor-pointer text-micro"
                onClick={() => {
                  const cur = task.skills ?? [];
                  const next = isSelected
                    ? cur.filter((s) => s !== skillName)
                    : [...cur, skillName];
                  onChangeTask({ ...task, skills: next });
                }}
              >
                {skillName}
              </Badge>
            );
          })}
          {roleSkills.length === 0 && (
            <span className="text-micro text-muted-foreground italic">{t(($) => $.flow_designer.task_card.no_skills)}</span>
          )}
        </div>
      </div>

      {/* Sets variables & Must set before closing (Spec 11.9, D17) */}
      {declaredVars.length > 0 && (
        <div className="space-y-1.5 pt-1 border-t border-border/50">
          <label className="text-micro font-medium text-muted-foreground block">
            {t(($) => $.flow_designer.task.sets)}
          </label>
          <div className="flex gap-2 flex-wrap">
            {declaredVars.map((vKey) => {
              const isSet = task.sets?.includes(vKey);
              const isRequired = task.requires?.vars?.includes(vKey);

              return (
                <div key={vKey} className="flex items-center gap-1.5 p-1 rounded bg-muted/20 border border-border text-micro">
                  <label className="flex items-center gap-1 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={isSet ?? false}
                      onChange={() => handleToggleSetVar(vKey)}
                      className="size-3 rounded border-input"
                    />
                    <span className="font-mono">{vKey}</span>
                  </label>
                  {isSet && (
                    <label className="flex items-center gap-1 cursor-pointer text-muted-foreground pl-1 border-l border-border">
                      <input
                        type="checkbox"
                        checked={isRequired ?? false}
                        onChange={() => handleToggleRequiredVar(vKey)}
                        className="size-2.5 rounded border-input"
                      />
                      <span className="text-micro">{t(($) => $.flow_designer.task.requires_vars)}</span>
                    </label>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Instructions */}
      <div>
        <label className="text-micro text-muted-foreground block mb-0.5">{t(($) => $.flow_designer.task_card.instructions)}</label>
        <Textarea
          value={task.instructions || ""}
          onChange={(e) => onChangeTask({ ...task, instructions: e.target.value })}
          placeholder={t(($) => $.flow_designer.task_card.instructions_placeholder)}
          className="text-caption min-h-16 resize-y"
        />
      </div>
    </div>
  );
}
