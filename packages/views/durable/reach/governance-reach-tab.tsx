"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorCode } from "@multica/core/api";
import {
  REACH_STATES,
  reachSurfaceKind,
  reachSurfaceLabel,
  useCloseReachGap,
  useReachMap,
  type ReachState,
  type ReachSurface,
} from "@multica/core/durable/reach";
import { canGovern } from "@multica/core/governance";
import { useWorkspaceId } from "@multica/core/hooks";
import { cachedIssueRouteRef } from "@multica/core/issues/canonical-id";
import { paths, useWorkspaceSlug } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import { Button } from "@multica/ui/components/ui/button";
import { Empty, EmptyHeader, EmptyTitle } from "@multica/ui/components/ui/empty";
import { TabsContent, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Loader2 } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { PAGE_GUTTER } from "../../layout/page-header";
import { PanelDisclosure } from "../../layout/panel-nav";
import { useNavigation } from "../../navigation";
import { ReachSurfaceDetail } from "./reach-surface-detail";
import { useReachEnabled } from "./use-reach-enabled";

const EMPTY_SURFACES: ReachSurface[] = [];

const STATE_TILE_CLASS: Record<ReachState, string> = {
  agent_operable: "bg-success/15 border-success/40 hover:bg-success/20",
  agent_with_confirm: "bg-info/15 border-info/40 hover:bg-info/20",
  declared_only: "bg-warning/15 border-warning/40 hover:bg-warning/20",
  human_only: "bg-destructive/15 border-destructive/40 hover:bg-destructive/20",
  unknown: "bg-muted border-border hover:bg-surface-hover",
};

const STATE_LABEL_CLASS: Record<ReachState, string> = {
  agent_operable: "text-success",
  agent_with_confirm: "text-info",
  declared_only: "text-warning",
  human_only: "text-destructive",
  unknown: "text-muted-foreground",
};

const STATE_SWATCH_CLASS: Record<ReachState, string> = {
  agent_operable: "bg-success",
  agent_with_confirm: "bg-info",
  declared_only: "bg-warning",
  human_only: "bg-destructive",
  unknown: "bg-muted-foreground",
};

function formatPercent(value: number): string {
  const rounded = Math.round(value * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(1)}%`;
}

function formatMinutes(minutes: number): string {
  const rounded = Math.round(minutes * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function tileDimensions(minutes: number, maxMinutes: number): {
  minHeight: string;
  flexBasis: string;
  flexGrow: number;
} {
  const ratio = maxMinutes > 0 ? Math.min(1, Math.max(0, minutes / maxMinutes)) : 0;
  return {
    minHeight: `${Math.round(88 + ratio * 96)}px`,
    flexBasis: `${Math.round(168 + ratio * 152)}px`,
    flexGrow: Math.max(1, Math.round(1 + ratio * 3)),
  };
}

/**
 * Governance → Reach tab hook, matching useDurableGovernanceTabs(). Shows only while
 * durable_open is on.
 */
export function useReachGovernanceTab(): { triggers: ReactNode; contents: ReactNode } {
  const { t } = useT("durable");
  const reachOn = useReachEnabled();
  if (!reachOn) {
    return { triggers: null, contents: null };
  }
  return {
    triggers: <TabsTrigger value="reach">{t(($) => $.reach.tab_title)}</TabsTrigger>,
    contents: (
      <TabsContent value="reach" className="min-h-0 flex-1 overflow-y-auto">
        <GovernanceReachTab />
      </TabsContent>
    ),
  };
}

/**
 * Governance → Reach (durable spec 11.4, F10): headline agent share of effects (30d),
 * surface grid colored by state and sized by human minutes, surface detail disclosure,
 * and gap list ranked by human minutes descending.
 */
export function GovernanceReachTab() {
  const enabled = useReachEnabled();
  if (!enabled) return null;
  return <GovernanceReachTabContent />;
}

function GovernanceReachTabContent() {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const slug = useWorkspaceSlug();
  const qc = useQueryClient();
  const { push } = useNavigation();
  const { role } = useCurrentMember(wsId);
  const editable = canGovern(role);

  const reachQuery = useReachMap(wsId);
  const closeGap = useCloseReachGap(wsId);
  const [selectedSurfaceName, setSelectedSurfaceName] = useState<string | null>(null);
  const [closingSurface, setClosingSurface] = useState<string | null>(null);

  const surfaces = reachQuery.data?.surfaces ?? EMPTY_SURFACES;
  const rankedSurfaces = useMemo(
    () =>
      [...surfaces].sort(
        (a, b) =>
          b.human_minutes_30d - a.human_minutes_30d || a.surface.localeCompare(b.surface),
      ),
    [surfaces],
  );
  const maxMinutes = useMemo(
    () => surfaces.reduce((max, item) => Math.max(max, item.human_minutes_30d), 0),
    [surfaces],
  );
  const selectedSurface: ReachSurface | null = useMemo(
    () => surfaces.find((item) => item.surface === selectedSurfaceName) ?? null,
    [surfaces, selectedSurfaceName],
  );

  const handleCloseGap = (surfaceName: string) => {
    setClosingSurface(surfaceName);
    closeGap
      .mutateAsync({ surface: surfaceName })
      .then(
        (res) => {
          if (!res?.issue_id) {
            toast.error(t(($) => $.reach.close_gap_failed));
            return;
          }
          toast.success(t(($) => $.reach.close_gap_opened));
          if (slug) {
            const ref = cachedIssueRouteRef(qc, wsId, res.issue_id);
            push(paths.workspace(slug).issueDetail(ref));
          }
        },
        (err: unknown) => {
          toast.error(
            errorCode(err) === "reach_no_squad"
              ? t(($) => $.reach.close_gap_no_squad)
              : t(($) => $.reach.close_gap_failed),
          );
        },
      )
      .finally(() => {
        setClosingSurface((current) => (current === surfaceName ? null : current));
      });
  };

  if (reachQuery.isPending) {
    return (
      <div className={cn(PAGE_GUTTER, "flex items-center py-8")}>
        <Loader2 className="size-4 animate-spin text-muted-foreground" aria-hidden="true" />
      </div>
    );
  }

  if (reachQuery.isError) {
    return (
      <div className={cn(PAGE_GUTTER, "py-4")}>
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.reach.load_failed)}
        </p>
      </div>
    );
  }

  const data = reachQuery.data;
  const isEmpty = !data || (!data.snapshot_date && surfaces.length === 0);

  if (isEmpty) {
    return (
      <div className={cn(PAGE_GUTTER, "py-6")}>
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t(($) => $.reach.empty)}</EmptyTitle>
          </EmptyHeader>
        </Empty>
      </div>
    );
  }

  return (
    <div className={cn(PAGE_GUTTER, "flex flex-col gap-8 py-4")}>
      <section aria-label={t(($) => $.reach.title)} className="flex flex-col gap-1">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-title-sm font-semibold">{t(($) => $.reach.agent_share_title)}</h2>
          {data.snapshot_date ? (
            <span className="text-caption tabular-nums text-muted-foreground">
              {t(($) => $.reach.snapshot_date, { date: data.snapshot_date })}
            </span>
          ) : null}
        </div>
        <p className="text-display-sm font-semibold tabular-nums">
          {formatPercent(data.agent_share_30d)}
        </p>
      </section>

      <section aria-label={t(($) => $.reach.grid_heading)} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-title-sm font-medium">{t(($) => $.reach.grid_heading)}</h2>
          <ul className="flex flex-wrap items-center gap-3 text-caption text-muted-foreground">
            {REACH_STATES.map((state) => (
              <li key={state} className="flex items-center gap-1.5">
                <span
                  aria-hidden="true"
                  className={cn("inline-block size-2.5 rounded-xs", STATE_SWATCH_CLASS[state])}
                />
                <span>{t(($) => $.reach.state[state])}</span>
              </li>
            ))}
          </ul>
        </div>

        {surfaces.length === 0 ? (
          <p className="text-body text-muted-foreground">{t(($) => $.reach.gaps_empty)}</p>
        ) : (
          <div className="flex flex-wrap gap-3">
            {rankedSurfaces.map((item) => {
              const isSelected = selectedSurface?.surface === item.surface;
              const kind = reachSurfaceKind(item.surface);
              const label = reachSurfaceLabel(item.surface);
              const dims = tileDimensions(item.human_minutes_30d, maxMinutes);
              return (
                <button
                  key={item.surface}
                  type="button"
                  data-state={item.state}
                  data-surface={item.surface}
                  aria-pressed={isSelected}
                  style={dims}
                  onClick={() =>
                    setSelectedSurfaceName((current) =>
                      current === item.surface ? null : item.surface,
                    )
                  }
                  className={cn(
                    "flex max-w-full flex-col justify-between gap-2 rounded-lg border p-3 text-left transition-colors motion-reduce:transition-none",
                    "outline-none focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-ring",
                    STATE_TILE_CLASS[item.state],
                    isSelected && "ring-2 ring-ring ring-offset-1 ring-offset-background",
                  )}
                >
                  <div className="flex items-start justify-between gap-2">
                    <span className="min-w-0 truncate text-body font-medium text-foreground">
                      {label}
                    </span>
                    <span className="shrink-0 text-caption text-muted-foreground">
                      {t(($) => $.reach.kind[kind])}
                    </span>
                  </div>
                  <div className="flex flex-wrap items-end justify-between gap-2">
                    <span
                      className={cn("text-caption font-medium", STATE_LABEL_CLASS[item.state])}
                    >
                      {t(($) => $.reach.state[item.state])}
                    </span>
                    <span className="text-caption tabular-nums text-muted-foreground">
                      {t(($) => $.reach.minutes_short, {
                        minutes: formatMinutes(item.human_minutes_30d),
                      })}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        )}

        <PanelDisclosure open={Boolean(selectedSurface)}>
          {selectedSurface ? (
            <div className="pt-1">
              <ReachSurfaceDetail
                wsId={wsId}
                surface={selectedSurface}
                canCloseGap={editable}
                closing={closingSurface === selectedSurface.surface}
                onCloseGap={handleCloseGap}
                onDismiss={() => setSelectedSurfaceName(null)}
              />
            </div>
          ) : null}
        </PanelDisclosure>
      </section>

      <section aria-label={t(($) => $.reach.gaps_heading)} className="flex flex-col gap-3">
        <h2 className="text-title-sm font-medium">{t(($) => $.reach.gaps_heading)}</h2>
        {rankedSurfaces.length === 0 ? (
          <p className="text-body text-muted-foreground">{t(($) => $.reach.gaps_empty)}</p>
        ) : (
          <ul className="flex flex-col divide-y rounded-lg border border-surface-border bg-surface">
            {rankedSurfaces.map((item) => {
              const isSelected = selectedSurface?.surface === item.surface;
              const isClosing = closingSurface === item.surface;
              return (
                <li
                  key={item.surface}
                  data-surface={item.surface}
                  className={cn(
                    "flex flex-wrap items-center justify-between gap-3 px-4 py-3 transition-colors motion-reduce:transition-none",
                    isSelected && "bg-surface-selected",
                  )}
                >
                  <button
                    type="button"
                    onClick={() => setSelectedSurfaceName(item.surface)}
                    className="flex min-w-0 flex-1 flex-col gap-0.5 rounded-md text-left outline-none focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-body font-medium text-foreground">
                        {reachSurfaceLabel(item.surface)}
                      </span>
                      <span
                        className={cn("text-caption font-medium", STATE_LABEL_CLASS[item.state])}
                      >
                        {t(($) => $.reach.state[item.state])}
                      </span>
                    </div>
                    <div className="flex flex-wrap items-center gap-3 text-caption tabular-nums text-muted-foreground">
                      <span>
                        {t(($) => $.reach.minutes_short, {
                          minutes: formatMinutes(item.human_minutes_30d),
                        })}
                      </span>
                      <span>
                        {t(($) => $.reach.effects_short, { count: item.effects_30d })}
                      </span>
                      <span>
                        {t(($) => $.reach.gap_asks_short, { count: item.gap_asks_30d })}
                      </span>
                    </div>
                  </button>
                  {editable ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={isClosing}
                      aria-busy={isClosing || undefined}
                      onClick={() => handleCloseGap(item.surface)}
                    >
                      {isClosing ? (
                        <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                      ) : null}
                      {t(($) => $.reach.close_gap)}
                    </Button>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
