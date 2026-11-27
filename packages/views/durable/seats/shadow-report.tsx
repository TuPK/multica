"use client";

import { useSeatShadowRuns, type ShadowState } from "@multica/core/durable/shadow";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

function formatMicrosUsd(micros: number): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(micros / 1_000_000);
}

function formatPercent(fraction: number): string {
  return `${Math.round(fraction * 100)}%`;
}

const STATE_TONE: Record<ShadowState, string> = {
  running: "text-info",
  completed: "text-success",
  stopped: "text-muted-foreground",
};

/**
 * Lists recent shadow runs and their reports on a seat (durable spec 9.4): candidate model,
 * state, pairs progress, comparison counts, Wilson lower bound of not-losing, and average cost
 * per side. Shows raw numbers only without inventing verdicts outside the spec.
 */
export function ShadowReport({ wsId, agentId }: { wsId: string; agentId: string }) {
  const { t } = useT("durable");
  const runsQuery = useSeatShadowRuns(wsId, agentId);
  const runs = runsQuery.data ?? [];

  return (
    <div className="flex flex-col gap-3 pt-2" aria-label={t(($) => $.seats.shadow_runs.title)}>
      <h3 className="text-title-sm font-medium">{t(($) => $.seats.shadow_runs.title)}</h3>

      {runsQuery.isPending ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.seats.shadow_runs.loading)}</p>
      ) : null}

      {runsQuery.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.seats.shadow_runs.load_failed)}
        </p>
      ) : null}

      {!runsQuery.isPending && !runsQuery.isError && runs.length === 0 ? (
        <p className="text-body text-muted-foreground">{t(($) => $.seats.shadow_runs.empty)}</p>
      ) : null}

      {runs.length > 0 ? (
        <ul className="flex flex-col divide-y rounded-lg border bg-card">
          {runs.map((run) => {
            const model = run.candidate_model || t(($) => $.seats.shadow.model_placeholder);
            return (
              <li key={run.id} className="flex flex-col gap-2 px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <span className="break-all text-body font-medium text-foreground">{model}</span>
                    <span
                      className={cn(
                        "inline-flex w-fit items-center rounded-md bg-muted px-1.5 py-0.5 text-caption font-medium",
                        STATE_TONE[run.state],
                      )}
                    >
                      {t(($) => $.seats.shadow_runs.state[run.state])}
                    </span>
                  </div>
                  <span className="tabular-nums text-caption text-muted-foreground">
                    {t(($) => $.seats.shadow_runs.pairs_progress, {
                      pairs: run.report.pairs,
                      target: run.target_pairs,
                    })}
                  </span>
                </div>

                <div className="flex flex-wrap gap-x-4 gap-y-1 tabular-nums text-caption text-muted-foreground">
                  <span>{t(($) => $.seats.shadow_runs.shadow_count, { count: run.report.shadow })}</span>
                  <span>{t(($) => $.seats.shadow_runs.tie_count, { count: run.report.tie })}</span>
                  <span>{t(($) => $.seats.shadow_runs.primary_count, { count: run.report.primary })}</span>
                  <span>{t(($) => $.seats.shadow_runs.unable_count, { count: run.report.unable })}</span>
                  <span className="text-foreground">
                    {t(($) => $.seats.shadow_runs.not_losing_lower, {
                      percent: formatPercent(run.report.not_losing_lower),
                    })}
                  </span>
                </div>

                <div className="flex flex-wrap gap-x-4 gap-y-1 tabular-nums text-caption text-muted-foreground">
                  <span>
                    {t(($) => $.seats.shadow_runs.primary_cost_avg, {
                      amount: formatMicrosUsd(run.report.primary_cost_avg_micros),
                    })}
                  </span>
                  <span>
                    {t(($) => $.seats.shadow_runs.shadow_cost_avg, {
                      amount: formatMicrosUsd(run.report.shadow_cost_avg_micros),
                    })}
                  </span>
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}
