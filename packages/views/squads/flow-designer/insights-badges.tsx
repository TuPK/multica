"use client";

import type { FlowInsights } from "@multica/core/types";
import { useT } from "../../i18n";

/** True when the report has requests to speak of. A version nobody has run has nothing to show. */
export function insightsHaveData(insights: FlowInsights | null | undefined): insights is FlowInsights {
  return Boolean(insights && insights.requests.total > 0);
}

/** How often a step was entered and how long a visit took, small enough to sit in the node of the step. */
export function StepInsightBadge({ insights, stepKey }: { insights: FlowInsights | null | undefined; stepKey: string }) {
  const { t } = useT("squads");
  if (!insightsHaveData(insights)) return null;
  const step = insights.steps.find((s) => s.key === stepKey);
  if (!step) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-2 text-micro text-muted-foreground">
      <span>{t(($) => $.flow_designer.insights.entered, { count: step.entered })}</span>
      {step.median_minutes !== null && step.p90_minutes !== null && (
        <span>{t(($) => $.flow_designer.insights.duration, { median: step.median_minutes, p90: step.p90_minutes })}</span>
      )}
    </div>
  );
}

/** How often an edge was taken. An edge no request took shows nothing. */
export function EdgeInsightBadge({ insights, stepKey, edgeId }: { insights: FlowInsights | null | undefined; stepKey: string; edgeId: string }) {
  const { t } = useT("squads");
  if (!insightsHaveData(insights)) return null;
  const edge = insights.edges.find((e) => e.step === stepKey && e.edge === edgeId);
  if (!edge || edge.taken === 0) return null;
  return <span className="text-micro text-muted-foreground">{t(($) => $.flow_designer.insights.taken, { count: edge.taken })}</span>;
}

export interface InsightsBarProps {
  insights: FlowInsights | null | undefined;
  includeTrial: boolean;
  onIncludeTrial: (include: boolean) => void;
}

/**
 * What the numbers are made of, and the switch for trial runs. With no data it is not there, unless trial
 * runs are included: then the person can still switch them off.
 */
export function InsightsBar({ insights, includeTrial, onIncludeTrial }: InsightsBarProps) {
  const { t } = useT("squads");
  if (!insightsHaveData(insights) && !includeTrial) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-micro text-muted-foreground">
      {insightsHaveData(insights) && <span>{t(($) => $.flow_designer.insights.requests, { count: insights.requests.total })}</span>}
      {insights?.truncated && <span>{t(($) => $.flow_designer.insights.truncated)}</span>}
      <label className="flex cursor-pointer items-center gap-1.5">
        <input type="checkbox" checked={includeTrial} onChange={(e) => onIncludeTrial(e.target.checked)} className="size-3.5" />
        <span>{t(($) => $.flow_designer.insights.include_trial)}</span>
      </label>
    </div>
  );
}
