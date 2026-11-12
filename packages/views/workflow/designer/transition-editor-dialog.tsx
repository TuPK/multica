"use client";

import { useEffect, useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Switch } from "@multica/ui/components/ui/switch";
import { Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import type { Condition, IssueWorkflowTransition } from "../types";
import { ConditionBuilder } from "./condition-builder";

export interface TransitionEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  transition: IssueWorkflowTransition | null;
  sourceStepKey: string;
  availableStatuses: { key: string; label: string }[];
  onSave: (updated: IssueWorkflowTransition) => void;
  onDelete?: () => void;
}

export function TransitionEditorDialog({
  open,
  onOpenChange,
  transition,
  sourceStepKey,
  availableStatuses,
  onSave,
  onDelete,
}: TransitionEditorDialogProps) {
  const { t } = useT("workflow");

  const [label, setLabel] = useState("");
  const [toStatusKey, setToStatusKey] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [condition, setCondition] = useState<Condition>({
    field: "outcome",
    op: "eq",
    value: "approved",
  });

  useEffect(() => {
    if (transition) {
      setLabel(transition.label || "");
      setToStatusKey(transition.to_status_key || availableStatuses[0]?.key || "");
      setIsDefault(transition.when === null);
      if (transition.when !== null) {
        setCondition(transition.when);
      } else {
        setCondition({ field: "outcome", op: "eq", value: "approved" });
      }
    }
  }, [transition, availableStatuses]);

  if (!transition) return null;

  const handleSave = () => {
    const updated: IssueWorkflowTransition = {
      ...transition,
      label: label.trim(),
      to_status_key: toStatusKey,
      when: isDefault ? null : condition,
    };
    onSave(updated);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t(($) => $.designer.edge.edit_transition)}</DialogTitle>
          <DialogDescription className="text-body-sm text-muted-foreground">
            {t(($) => $.designer.edge.from_step, { step: sourceStepKey })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* Branch Label */}
          <div className="space-y-1.5">
            <Label htmlFor="branch-label">{t(($) => $.designer.edge.label)}</Label>
            <Input
              id="branch-label"
              value={label}
              onChange={(e) => setLabel(e.target.value.slice(0, 64))}
              placeholder={t(($) => $.designer.edge.label_placeholder)}
              maxLength={64}
            />
            <div className="text-right text-micro text-muted-foreground">
              {label.length}/64
            </div>
          </div>

          {/* Target Status */}
          <div className="space-y-1.5">
            <Label>{t(($) => $.designer.edge.target_status)}</Label>
            <Select
              items={availableStatuses.map((s) => ({ value: s.key, label: s.label }))}
              value={toStatusKey}
              onValueChange={(val) => {
                if (val) setToStatusKey(val);
              }}
            >
              <SelectTrigger>
                <SelectValue>
                  {availableStatuses.find((s) => s.key === toStatusKey)?.label || toStatusKey}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {availableStatuses.map((st) => (
                  <SelectItem key={st.key} value={st.key}>
                    {st.label} ({st.key})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Default branch toggle */}
          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div className="space-y-0.5">
              <Label className="text-body font-medium">
                {t(($) => $.designer.edge.is_default)}
              </Label>
              <p className="text-caption text-muted-foreground">
                {t(($) => $.designer.edge.is_default_hint)}
              </p>
            </div>
            <Switch checked={isDefault} onCheckedChange={setIsDefault} />
          </div>

          {/* Routing conditions */}
          {!isDefault && (
            <div className="space-y-2 pt-2">
              <Label className="text-body font-medium">
                {t(($) => $.designer.conditions.builder_title)}
              </Label>
              <ConditionBuilder
                condition={condition}
                onChange={setCondition}
                depth={1}
              />
            </div>
          )}
        </div>

        <DialogFooter className="flex-row items-center justify-between gap-2 sm:gap-0">
          {onDelete ? (
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() => {
                onDelete();
                onOpenChange(false);
              }}
              className="gap-1.5"
            >
              <Trash2 className="size-4" />
              <span>{t(($) => $.designer.edge.delete_transition)}</span>
            </Button>
          ) : (
            <div />
          )}

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onOpenChange(false)}
            >
              {t(($) => $.manage.delete_dialog.cancel)}
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={handleSave}
              disabled={label.trim() === ""}
            >
              {t(($) => $.designer.save)}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
