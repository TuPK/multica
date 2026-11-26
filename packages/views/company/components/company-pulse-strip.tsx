"use client";

import { useQuery } from "@tanstack/react-query";
import {
  attentionBudgetOptions,
  companyPulseOptions,
  type AttentionBudget,
  type CompanyPulse,
} from "@multica/core/governance";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths, type GovernanceTab } from "@multica/core/paths";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";

interface PulseChip {
  key: string;
  text: string;
  /** The Governance tab the chip opens. Undefined when no tab answers it. */
  tab?: GovernanceTab;
}

/**
 * The company pulse strip (spec 4.8): what needs the boss, in one line. Each
 * chip that has a Governance tab opens it. Zero counts are left out, so a quiet
 * company shows a single line that says so. With durable_principal on, the
 * week's attention spend (spec 7.5) closes the strip and opens the Controls tab.
 */
export function CompanyPulseStrip() {
  const { t } = useT("governance");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const pulse = useQuery(companyPulseOptions(wsId));
  const budget = useQuery(attentionBudgetOptions(wsId));

  if (!pulse.data) return null;
  const chips = pulseChips(pulse.data, t);
  const attention = budget.data?.enabled
    ? { key: "attention", text: attentionText(budget.data, t), tab: "controls" as const }
    : null;

  if (chips.length === 0) {
    return (
      <div className="flex min-w-0 items-center gap-2">
        <span data-pulse="quiet" className="shrink-0 text-caption text-muted-foreground">
          {t(($) => $.pulse.quiet)}
        </span>
        {attention ? <PulseChipLink chip={attention} href={paths.governance("controls")} /> : null}
      </div>
    );
  }
  return (
    <div role="group" aria-label={t(($) => $.pulse.label)} data-pulse="strip" className="flex min-w-0 items-center gap-1">
      {chips.map((chip) =>
        chip.tab ? (
          <PulseChipLink key={chip.key} chip={chip} href={paths.governance(chip.tab)} />
        ) : (
          <span key={chip.key} className={chipClass} data-pulse-chip={chip.key}>
            {chip.text}
          </span>
        ),
      )}
      {attention ? <PulseChipLink chip={attention} href={paths.governance("controls")} /> : null}
    </div>
  );
}

function PulseChipLink({ chip, href }: { chip: { key: string; text: string }; href: string }) {
  return (
    <AppLink href={href} className={chipClass} data-pulse-chip={chip.key}>
      {chip.text}
    </AppLink>
  );
}

type Translate = ReturnType<typeof useT<"governance">>["t"];

/** The week's attention reading: minutes spent of the budget (spec 7.5). */
function attentionText(b: AttentionBudget, t: Translate): string {
  return t(($) => $.pulse.attention, { spent: Math.round(b.spent_minutes), budget: b.minutes_per_week });
}

const chipClass = cn(
  "inline-flex h-7 shrink-0 items-center rounded-md border px-2 text-caption text-muted-foreground",
  "hover:bg-accent hover:text-foreground",
);

/** The chips for the non-zero readings, in the order the boss reads them. */
export function pulseChips(p: CompanyPulse, t: Translate): PulseChip[] {
  const chips: PulseChip[] = [];
  if (p.decisions_waiting > 0) {
    chips.push({ key: "decisions", text: t(($) => $.pulse.decisions, { count: p.decisions_waiting }) });
  }
  if (p.goals_at_risk > 0) {
    chips.push({ key: "goals", text: t(($) => $.pulse.goals_at_risk, { count: p.goals_at_risk }), tab: "goals" });
  }
  if (p.budgets_above_soft_line > 0) {
    chips.push({
      key: "budgets",
      text: t(($) => $.pulse.budgets, { count: p.budgets_above_soft_line }),
      tab: "budgets",
    });
  }
  if (p.budget_holds > 0) {
    chips.push({ key: "holds", text: t(($) => $.pulse.holds, { count: p.budget_holds }), tab: "budgets" });
  }
  if (p.stalled_initiatives > 0) {
    chips.push({ key: "stalled", text: t(($) => $.pulse.stalled, { count: p.stalled_initiatives }) });
  }
  if (p.routines_late > 0) {
    chips.push({ key: "routines", text: t(($) => $.pulse.routines_late, { count: p.routines_late }), tab: "reviews" });
  }
  if (p.runtimes_offline > 0) {
    chips.push({
      key: "runtimes",
      text: t(($) => $.pulse.runtimes_offline, { count: p.runtimes_offline }),
      tab: "controls",
    });
  }
  return chips;
}
