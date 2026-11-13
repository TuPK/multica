"use client";

import type { FlowFactInfo } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import {
  enumValuesForField,
  fieldsForFacts,
  listValueText,
  operatorsForField,
  ruleWithField,
  ruleWithListValue,
  ruleWithOperator,
  ruleWithScalarValue,
  valueKindOf,
} from "../condition-spec";
import type {
  Condition,
  ConditionOperator,
  ConditionRule,
} from "../types";

export interface ConditionBuilderProps {
  condition: Condition;
  onChange: (condition: Condition) => void;
  depth?: number;
  onRemove?: () => void;
  facts?: FlowFactInfo[];
}

/**
 * Edits one routing condition: a group (AND, OR, NOT) or a field rule. A rule
 * offers only the operators its field takes, and a value of the kind the server
 * accepts (see condition-spec.ts).
 */
export function ConditionBuilder({
  condition,
  onChange,
  depth = 1,
  onRemove,
  facts,
}: ConditionBuilderProps) {
  const { t } = useT("workflow");

  const isGroup = "all" in condition || "any" in condition || "not" in condition;

  if (isGroup) {
    const groupType: "all" | "any" | "not" =
      "all" in condition ? "all" : "any" in condition ? "any" : "not";

    const items =
      "all" in condition
        ? condition.all
        : "any" in condition
          ? condition.any
          : [condition.not];

    const defaultRule: ConditionRule = facts && facts[0]
      ? { field: facts[0].key, op: (facts[0].ops[0] ?? "eq") as ConditionOperator, value: "" }
      : { field: "outcome", op: "eq", value: "approved" };

    const handleTypeChange = (newType: "all" | "any" | "not") => {
      if (newType === "not") {
        onChange({ not: items[0] ?? defaultRule });
      } else if (newType === "all") {
        onChange({ all: items });
      } else {
        onChange({ any: items });
      }
    };

    const handleItemChange = (idx: number, updatedItem: Condition) => {
      if (groupType === "not") {
        onChange({ not: updatedItem });
      } else {
        const nextItems = [...items];
        nextItems[idx] = updatedItem;
        onChange(groupType === "all" ? { all: nextItems } : { any: nextItems });
      }
    };

    const handleItemRemove = (idx: number) => {
      const nextItems = items.filter((_, i) => i !== idx);
      if (nextItems.length === 0) {
        onRemove?.();
      } else if (groupType === "not") {
        onRemove?.();
      } else {
        onChange(groupType === "all" ? { all: nextItems } : { any: nextItems });
      }
    };

    const handleAddRule = () => {
      const newRule: ConditionRule = { ...defaultRule };
      if (groupType === "not" && "not" in condition) {
        onChange({ all: [condition.not, newRule] });
      } else {
        onChange(
          groupType === "all"
            ? { all: [...items, newRule] }
            : { any: [...items, newRule] },
        );
      }
    };

    const handleAddGroup = () => {
      if (depth >= 4) return;
      const newGroup: Condition = {
        all: [{ ...defaultRule }],
      };
      onChange(
        groupType === "all"
          ? { all: [...items, newGroup] }
          : { any: [...items, newGroup] },
      );
    };

    const groupItems = [
      { value: "all", label: t(($) => $.designer.conditions.group_and) },
      { value: "any", label: t(($) => $.designer.conditions.group_or) },
      { value: "not", label: t(($) => $.designer.conditions.group_not) },
    ];

    return (
      <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Select
              items={groupItems}
              value={groupType}
              onValueChange={(v) => handleTypeChange(v as "all" | "any" | "not")}
            >
              <SelectTrigger className="h-8 w-44 text-caption font-medium">
                <SelectValue>
                  {groupItems.find((g) => g.value === groupType)?.label}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t(($) => $.designer.conditions.group_and)}</SelectItem>
                <SelectItem value="any">{t(($) => $.designer.conditions.group_or)}</SelectItem>
                <SelectItem value="not">{t(($) => $.designer.conditions.group_not)}</SelectItem>
              </SelectContent>
            </Select>

            <span className="text-micro text-muted-foreground font-mono">
              ({depth}/4)
            </span>
          </div>

          {onRemove && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={onRemove}
              aria-label={t(($) => $.designer.conditions.remove_group)}
            >
              <Trash2 className="size-3.5 text-muted-foreground hover:text-destructive" />
            </Button>
          )}
        </div>

        {/* Children */}
        <div className="space-y-2 pl-2 border-l-2 border-border/80">
          {items.map((item, idx) => (
            <ConditionBuilder
              key={idx}
              condition={item}
              onChange={(updated) => handleItemChange(idx, updated)}
              depth={depth + 1}
              onRemove={() => handleItemRemove(idx)}
              facts={facts}
            />
          ))}
        </div>

        {/* Actions to add */}
        <div className="flex items-center gap-2 pt-1">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAddRule}
            className="h-7 text-micro gap-1"
          >
            <Plus className="size-3" />
            <span>{t(($) => $.designer.conditions.add_rule)}</span>
          </Button>

          {depth < 4 && groupType !== "not" && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={handleAddGroup}
              className="h-7 text-micro gap-1 text-muted-foreground"
            >
              <Plus className="size-3" />
              <span>{t(($) => $.designer.conditions.add_group)}</span>
            </Button>
          )}
        </div>
      </div>
    );
  }

  // Single rule
  const rule = condition as ConditionRule;
  const field = rule.field;
  const kind = valueKindOf(field, facts);
  const enumValues = enumValuesForField(field, facts);

  const fieldLabels: Record<string, string> = {
    outcome: t(($) => $.designer.conditions.fields.outcome),
    priority: t(($) => $.designer.conditions.fields.priority),
    labels: t(($) => $.designer.conditions.fields.labels),
    project_id: t(($) => $.designer.conditions.fields.project_id),
    rework_count: t(($) => $.designer.conditions.fields.rework_count),
    creator_type: t(($) => $.designer.conditions.fields.creator_type),
    assignee_type: t(($) => $.designer.conditions.fields.assignee_type),
  };

  const fieldItems = fieldsForFacts(facts).map((f) => ({
    value: f,
    label: fieldLabels[f] ?? f,
  }));

  const opItems = operatorsForField(field, facts).map((op) => ({
    value: op,
    label: t(($) => $.designer.conditions.operators[op as ConditionOperator] ?? op),
  }));

  const enumItems = (enumValues ?? []).map((opt) => ({
    value: opt,
    label: opt,
  }));

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card p-2 text-caption">
      {/* Field */}
      <Select
        items={fieldItems}
        value={rule.field}
        onValueChange={(f) => {
          if (f) onChange(ruleWithField(rule, f, facts));
        }}
      >
        <SelectTrigger className="h-7 w-40 text-caption font-mono">
          <SelectValue>
            {fieldItems.find((f) => f.value === rule.field)?.label ?? rule.field}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {fieldItems.map((f) => (
            <SelectItem key={f.value} value={f.value}>
              {f.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Operator */}
      <Select
        items={opItems}
        value={rule.op}
        onValueChange={(op) => {
          if (op) onChange(ruleWithOperator(rule, op as ConditionOperator, facts));
        }}
      >
        <SelectTrigger className="h-7 w-28 text-caption font-mono">
          <SelectValue>
            {opItems.find((o) => o.value === rule.op)?.label ?? rule.op}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {opItems.map((op) => (
            <SelectItem key={op.value} value={op.value}>
              {op.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Value */}
      {rule.op !== "exists" && (
        <div className="flex-1 min-w-[120px]">
          {rule.op === "in" ? (
            <Input
              value={listValueText(rule.value)}
              onChange={(e) => onChange(ruleWithListValue(rule, e.target.value, facts))}
              placeholder={t(($) => $.designer.conditions.value)}
              className="h-7 text-caption"
            />
          ) : enumValues ? (
            <Select
              items={enumItems}
              value={String(rule.value ?? enumValues[0] ?? "")}
              onValueChange={(v) => {
                if (v) onChange(ruleWithScalarValue(rule, v, facts));
              }}
            >
              <SelectTrigger className="h-7 w-full text-caption">
                <SelectValue>{String(rule.value ?? enumValues[0] ?? "")}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {enumItems.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : kind === "number" ? (
            <Input
              type="number"
              min={0}
              step={1}
              value={typeof rule.value === "number" ? rule.value : 0}
              onChange={(e) => onChange(ruleWithScalarValue(rule, e.target.value, facts))}
              className="h-7 w-24 text-caption"
            />
          ) : (
            <Input
              value={String(rule.value ?? "")}
              onChange={(e) => onChange(ruleWithScalarValue(rule, e.target.value, facts))}
              placeholder={t(($) => $.designer.conditions.value)}
              className="h-7 text-caption"
            />
          )}
        </div>
      )}

      {onRemove && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={onRemove}
          aria-label={t(($) => $.designer.conditions.remove_rule)}
        >
          <Trash2 className="size-3.5 text-muted-foreground hover:text-destructive" />
        </Button>
      )}
    </div>
  );
}
