"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  OUTCOME_VERDICTS,
  useOutcomeSummary,
  useOutcomes,
  type OutcomeSummaryGroup,
  type OutcomeVerdict,
} from "@multica/core/durable/outcomes";
import { useWorkspaceId } from "@multica/core/hooks";
import { agentListOptions, squadListOptions } from "@multica/core/workspace/queries";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { cn } from "@multica/ui/lib/utils";
import { PAGE_GUTTER } from "../../layout/page-header";
import { useT } from "../../i18n";
import { VerdictChip } from "./verdict-chip";

// The list reads the newest outcomes in one page. The note says so when more exist, rather than
// paging silently.
const PAGE_SIZE = 100;
const ALL = "all";

type SelectItemValue = { value: string; label: string };

/**
 * Governance → Outcomes (durable spec 2.9): every settled and pending outcome, filtered by agent,
 * squad and verdict, with the verified coverage of the last week above the list.
 */
export function GovernanceOutcomesTab() {
  const { t } = useT("durable");
  const { t: tGov } = useT("governance");
  const wsId = useWorkspaceId();
  const [agentId, setAgentId] = useState(ALL);
  const [squadId, setSquadId] = useState(ALL);
  const [verdict, setVerdict] = useState(ALL);

  const agents = useQuery(agentListOptions(wsId));
  const squads = useQuery(squadListOptions(wsId));
  const outcomes = useOutcomes(wsId, {
    agent_id: agentId === ALL ? undefined : agentId,
    squad_id: squadId === ALL ? undefined : squadId,
    verdict: verdict === ALL ? undefined : (verdict as OutcomeVerdict),
    limit: PAGE_SIZE,
  });
  const summary = useOutcomeSummary(wsId);

  const agentNames = useMemo(
    () => new Map((agents.data ?? []).map((agent) => [agent.id, agent.name])),
    [agents.data],
  );
  const rows = outcomes.data?.outcomes ?? [];
  const percent = verifiedCoverage(summary.data?.groups ?? []);

  const agentItems: SelectItemValue[] = [
    { value: ALL, label: t(($) => $.outcomes.tab.all) },
    ...(agents.data ?? []).map((agent) => ({ value: agent.id, label: agent.name })),
  ];
  const squadItems: SelectItemValue[] = [
    { value: ALL, label: t(($) => $.outcomes.tab.all) },
    ...(squads.data ?? []).map((squad) => ({ value: squad.id, label: squad.name })),
  ];
  const verdictItems: SelectItemValue[] = [
    { value: ALL, label: t(($) => $.outcomes.tab.all) },
    ...OUTCOME_VERDICTS.map((value) => ({ value, label: t(($) => $.outcomes.verdict[value]) })),
  ];

  return (
    <div className={cn(PAGE_GUTTER, "flex flex-col gap-8 py-4")}>
      <section aria-label={t(($) => $.outcomes.tab.coverage)} className="flex max-w-prose flex-col gap-1">
        <h2 className="text-heading">{t(($) => $.outcomes.tab.coverage)}</h2>
        <p className="text-title font-semibold tabular-nums">{percent === null ? "—" : `${percent}%`}</p>
        <p className="text-caption text-muted-foreground">{t(($) => $.outcomes.tab.coverage_hint)}</p>
      </section>

      <section aria-label={tGov(($) => $.tabs.outcomes)} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <FilterSelect label={t(($) => $.outcomes.tab.agent)} value={agentId} items={agentItems} onChange={setAgentId} />
          <FilterSelect label={t(($) => $.outcomes.tab.squad)} value={squadId} items={squadItems} onChange={setSquadId} />
          <FilterSelect label={t(($) => $.outcomes.tab.verdict)} value={verdict} items={verdictItems} onChange={setVerdict} />
        </div>

        {outcomes.isError ? (
          <p role="alert" className="text-caption text-destructive">
            {t(($) => $.outcomes.tab.load_failed)}
          </p>
        ) : null}
        {!outcomes.isPending && !outcomes.isError && rows.length === 0 ? (
          <p className="text-body text-muted-foreground">{t(($) => $.outcomes.tab.empty)}</p>
        ) : null}
        {rows.length > 0 ? (
          <ul className="flex flex-col divide-y">
            {rows.map((outcome) => (
              <li key={outcome.id} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-0.5 py-2.5">
                <span className="min-w-0 truncate text-body">{outcome.work_class}</span>
                <VerdictChip verdict={outcome.verdict} />
                <span className="min-w-0 truncate text-caption text-muted-foreground">
                  {outcome.agent_id ? (agentNames.get(outcome.agent_id) ?? "") : ""}
                </span>
                <span className="text-caption tabular-nums text-muted-foreground">
                  {new Date(outcome.created_at).toLocaleDateString()}
                </span>
              </li>
            ))}
          </ul>
        ) : null}
        {outcomes.data?.next_cursor ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.outcomes.tab.limit_note)}</p>
        ) : null}
      </section>
    </div>
  );
}

function FilterSelect({
  label,
  value,
  items,
  onChange,
}: {
  label: string;
  value: string;
  items: SelectItemValue[];
  onChange: (value: string) => void;
}) {
  return (
    <Select
      items={items}
      value={value}
      onValueChange={(next) => {
        if (next) onChange(next);
      }}
    >
      <SelectTrigger aria-label={label} className="w-44">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Share of outcomes with a verified check: pass, partial and fail over all of them, pending and
 * unverified included. Null when there is nothing to count.
 */
export function verifiedCoverage(groups: OutcomeSummaryGroup[]): number | null {
  const total = groups.reduce((sum, group) => sum + group.total, 0);
  if (total === 0) return null;
  const verified = groups.reduce((sum, group) => sum + group.pass + group.partial + group.fail, 0);
  return Math.round((verified * 100) / total);
}
