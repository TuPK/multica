"use client";

import { toast } from "sonner";
import {
  DECISIONS,
  useEffectPolicy,
  useSaveEffectPolicyOverride,
  type Decision,
  type PolicyCell,
} from "@multica/core/durable/effects";
import { canGovern } from "@multica/core/governance";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentMember } from "@multica/core/permissions";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@multica/ui/components/ui/table";
import { useT } from "../../i18n";

const LEVELS = [0, 1, 2, 3] as const;

// The reasons the server gives for a locked decision (durable spec 4.3).
const LOCK_REASONS = ["loosen_needs_l3", "never_allow", "no_cell", "invalid_decision"] as const;
type LockReason = (typeof LOCK_REASONS)[number];

function isLockReason(value: string): value is LockReason {
  return (LOCK_REASONS as readonly string[]).includes(value);
}

// One row of the table per effect class, and write split by its reversibility (durable spec 4.3).
type RowKey = "read" | "write_internal" | "write_reversible" | "write_compensable" | "send" | "share" | "delete" | "spend";

function rowKeyOf(cell: Pick<PolicyCell, "effect_class" | "reversibility">): RowKey | null {
  if (cell.effect_class === "write") {
    if (cell.reversibility === "reversible") return "write_reversible";
    if (cell.reversibility === "compensable") return "write_compensable";
    return null;
  }
  return cell.effect_class;
}

const ROW_ORDER: RowKey[] = ["read", "write_internal", "write_reversible", "write_compensable", "send", "share", "delete", "spend"];

/**
 * The effect policy as a grid: a row per effect class, a column per autonomy level (durable
 * spec 4.3). Every choice and every lock comes from the server; a decision the server marks
 * not allowed is disabled and its reason is written under the cell, so nobody can loosen a
 * cell past what the policy allows.
 */
export function EffectPolicyGrid() {
  const { t } = useT("durable");
  const { t: tGov } = useT("governance");
  const wsId = useWorkspaceId();
  const { role } = useCurrentMember(wsId);
  const editable = canGovern(role);
  const policy = useEffectPolicy(wsId);
  const save = useSaveEffectPolicyOverride(wsId);

  const cells = policy.data ?? [];
  const byRow = new Map<RowKey, Map<number, PolicyCell>>();
  for (const cell of cells) {
    const key = rowKeyOf(cell);
    if (!key) continue;
    const levels = byRow.get(key) ?? new Map<number, PolicyCell>();
    levels.set(cell.autonomy_level, cell);
    byRow.set(key, levels);
  }

  const choose = (cell: PolicyCell, decision: Decision) => {
    if (decision === (cell.override ?? cell.default)) return;
    save
      .mutateAsync({
        autonomy_level: cell.autonomy_level,
        effect_class: cell.effect_class,
        reversibility: cell.reversibility,
        decision,
      })
      .then(
        (saved) => {
          if (saved) toast.success(t(($) => $.policy.saved));
          else toast.error(t(($) => $.policy.save_failed));
        },
        () => toast.error(t(($) => $.policy.save_failed)),
      );
  };

  if (policy.isError) {
    return (
      <p role="alert" className="text-caption text-destructive">
        {t(($) => $.policy.load_failed)}
      </p>
    );
  }

  return (
    <section aria-labelledby="effect-policy-heading" className="flex flex-col gap-3">
      <h2 id="effect-policy-heading" className="text-title-sm font-medium">
        {t(($) => $.policy.title)}
      </h2>
      {!editable ? <p className="text-caption text-muted-foreground">{t(($) => $.policy.read_only)}</p> : null}
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead />
              {LEVELS.map((level) => (
                <TableHead key={level}>{tGov(($) => $.autonomy.level_tag, { level })}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {ROW_ORDER.filter((key) => byRow.has(key)).map((key) => {
              const rowLabel = t(($) => $.policy.rows[key]);
              const levels = byRow.get(key)!;
              return (
                <TableRow key={key}>
                  <TableCell className="font-medium">{rowLabel}</TableCell>
                  {LEVELS.map((level) => {
                    const cell = levels.get(level);
                    if (!cell) return <TableCell key={level} />;
                    return (
                      <PolicyCellView
                        key={level}
                        cell={cell}
                        rowLabel={rowLabel}
                        levelLabel={tGov(($) => $.autonomy.level_tag, { level })}
                        editable={editable && !save.isPending}
                        onChoose={(decision) => choose(cell, decision)}
                      />
                    );
                  })}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}

function PolicyCellView({
  cell,
  rowLabel,
  levelLabel,
  editable,
  onChoose,
}: {
  cell: PolicyCell;
  rowLabel: string;
  levelLabel: string;
  editable: boolean;
  onChoose: (decision: Decision) => void;
}) {
  const { t } = useT("durable");
  const value = cell.override ?? cell.default;
  const items = DECISIONS.map((decision) => ({ value: decision, label: t(($) => $.effects.decision[decision]) }));
  const reasons = [...new Set(cell.options.filter((o) => !o.allowed).map((o) => o.reason))].filter(isLockReason);

  return (
    <TableCell className="align-top">
      <div className="flex min-w-36 flex-col gap-1">
        <Select
          items={items}
          value={value}
          onValueChange={(next) => onChoose(next as Decision)}
          disabled={!editable}
        >
          <SelectTrigger aria-label={`${rowLabel}, ${levelLabel}`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {DECISIONS.map((decision) => {
              const option = cell.options.find((o) => o.decision === decision);
              return (
                <SelectItem key={decision} value={decision} disabled={option ? !option.allowed : true}>
                  {t(($) => $.effects.decision[decision])}
                </SelectItem>
              );
            })}
          </SelectContent>
        </Select>
        {cell.override !== null ? (
          <span className="text-caption text-brand">{t(($) => $.policy.changed)}</span>
        ) : null}
        {reasons.map((reason) => (
          <span key={reason} className="text-caption text-muted-foreground">
            {t(($) => $.policy.reason[reason])}
          </span>
        ))}
      </div>
    </TableCell>
  );
}
