"use client";

import {
  reachSurfaceKind,
  reachSurfaceLabel,
  type ReachState,
  type ReachSurface,
} from "@multica/core/durable/reach";
import { useIssueRouteRef } from "@multica/core/issues/canonical-id";
import { paths, useWorkspaceSlug } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { ExternalLink, Loader2, X } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";

const STATE_BADGE_TONE: Record<ReachState, string> = {
  agent_operable: "bg-success/15 text-success",
  agent_with_confirm: "bg-info/15 text-info",
  declared_only: "bg-warning/15 text-warning",
  human_only: "bg-destructive/15 text-destructive",
  unknown: "bg-muted text-muted-foreground",
};

function formatMinutes(minutes: number): string {
  const rounded = Math.round(minutes * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function GapIssueLink({ wsId, issueId }: { wsId: string; issueId: string }) {
  const { t } = useT("durable");
  const slug = useWorkspaceSlug();
  const issueRef = useIssueRouteRef(wsId, issueId);
  if (!slug) {
    return <span className="font-mono text-caption text-muted-foreground">{issueId}</span>;
  }
  return (
    <AppLink
      href={paths.workspace(slug).issueDetail(issueRef)}
      className="inline-flex items-center gap-1 text-label font-medium text-brand hover:underline"
    >
      {t(($) => $.reach.detail.view_issue)}
      <ExternalLink className="size-3.5" aria-hidden="true" />
    </AppLink>
  );
}

/**
 * Detail panel for one surface of the latest Reach Map snapshot (durable spec 11.4).
 * Shows snapshot counters and any linked gap issue; omits time series and ask bodies
 * because the server currently exposes only the newest snapshot.
 */
export function ReachSurfaceDetail({
  wsId,
  surface,
  canCloseGap,
  closing,
  onCloseGap,
  onDismiss,
}: {
  wsId: string;
  surface: ReachSurface;
  canCloseGap: boolean;
  closing: boolean;
  onCloseGap: (surfaceName: string) => void;
  onDismiss: () => void;
}) {
  const { t } = useT("durable");
  const kind = reachSurfaceKind(surface.surface);
  const label = reachSurfaceLabel(surface.surface);

  return (
    <section
      aria-label={t(($) => $.reach.detail.title)}
      className="flex flex-col gap-4 rounded-xl border border-surface-border bg-surface p-4"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="truncate text-title-sm font-semibold">{label}</h3>
            <span className="rounded-md bg-muted px-1.5 py-0.5 text-caption text-muted-foreground">
              {t(($) => $.reach.kind[kind])}
            </span>
            <span
              className={cn(
                "rounded-md px-1.5 py-0.5 text-caption font-medium",
                STATE_BADGE_TONE[surface.state],
              )}
            >
              {t(($) => $.reach.state[surface.state])}
            </span>
          </div>
          <p className="mt-0.5 truncate font-mono text-caption text-muted-foreground">
            {surface.surface}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={t(($) => $.reach.detail.close)}
          onClick={onDismiss}
        >
          <X aria-hidden="true" />
        </Button>
      </div>

      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <MetricCell
          label={t(($) => $.reach.detail.human_minutes_30d)}
          value={t(($) => $.reach.minutes_short, {
            minutes: formatMinutes(surface.human_minutes_30d),
          })}
        />
        <MetricCell
          label={t(($) => $.reach.detail.effects_30d)}
          value={String(surface.effects_30d)}
        />
        <MetricCell
          label={t(($) => $.reach.detail.agent_effects_30d)}
          value={String(surface.agent_effects_30d)}
        />
        <MetricCell
          label={t(($) => $.reach.detail.gap_asks_30d)}
          value={String(surface.gap_asks_30d)}
        />
        <MetricCell
          label={t(($) => $.reach.detail.top_asks_count)}
          value={String(surface.top_ask_ids.length)}
        />
      </dl>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3">
        <div className="flex items-center gap-2 text-label">
          {surface.gap_issue_id ? (
            <>
              <span className="text-muted-foreground">{t(($) => $.reach.detail.gap_issue)}</span>
              <GapIssueLink wsId={wsId} issueId={surface.gap_issue_id} />
            </>
          ) : null}
        </div>
        {canCloseGap ? (
          <Button
            type="button"
            size="sm"
            disabled={closing}
            aria-busy={closing || undefined}
            onClick={() => onCloseGap(surface.surface)}
          >
            {closing ? <Loader2 className="size-3.5 animate-spin" aria-hidden="true" /> : null}
            {t(($) => $.reach.close_gap)}
          </Button>
        ) : null}
      </div>
    </section>
  );
}

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border border-border bg-background px-3 py-2">
      <dt className="text-caption text-muted-foreground">{label}</dt>
      <dd className="text-title-sm font-semibold tabular-nums">{value}</dd>
    </div>
  );
}
