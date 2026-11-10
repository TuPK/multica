"use client";

import { useId, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  hubItemMetaListOptions,
  hubUpdateCheckOptions,
  type HubItem,
  type HubItemMeta,
} from "@multica/core/hub";
import { useT, useTimeAgo } from "../i18n";
import { safeSourceLink } from "./hub-source-meta";
import { formatHubSize } from "./hub-files";
import { HubForkDialog } from "./hub-fork-dialog";
import { UpdateAvailableBadge } from "./hub-update-banner";
import { HubUpdateDialog } from "./hub-update-dialog";

/**
 * The item's provenance and tracking state. Checking, updating and forking follow
 * who may edit the item; everyone else reads the state.
 */
export function HubItemSourceTab({
  wsId,
  item,
  meta,
  metaLoading,
  canEdit,
}: {
  wsId: string;
  item: HubItem;
  meta: HubItemMeta | undefined;
  metaLoading: boolean;
  canEdit: boolean;
}) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const qc = useQueryClient();
  const headingId = useId();
  // Checks run on demand. The query is never enabled automatically.
  const check = useQuery({ ...hubUpdateCheckOptions(wsId, item.kind, item.refId), enabled: false });
  const [reviewing, setReviewing] = useState(false);
  const [forking, setForking] = useState(false);

  if (metaLoading && !meta) {
    return <Skeleton className="mx-auto h-24 w-full max-w-[760px]" aria-busy="true" />;
  }

  const sourceLabel = t(($) => $.source_types[item.source]);
  const link = item.sourceUrl ? safeSourceLink(item.sourceUrl) : null;
  const tracking = meta?.tracking ?? null;
  // A check run from this tab can find an update before the stored meta is refreshed.
  const updateAvailable = meta?.update_available === true || check.data?.update_available === true;

  const runCheck = async () => {
    await check.refetch();
    void qc.invalidateQueries({ queryKey: hubItemMetaListOptions(wsId).queryKey });
  };

  return (
    <div className="mx-auto flex w-full max-w-[760px] flex-col gap-8 px-4">
      <section aria-labelledby={headingId} className="flex flex-col gap-3">
        <h2 id={headingId} className="text-label font-medium">
          {t(($) => $.source_tab.provenance)}
        </h2>
        <dl className="grid grid-cols-[minmax(7rem,auto)_minmax(0,1fr)] gap-x-6 gap-y-2.5 text-label">
          <dt className="text-muted-foreground">{t(($) => $.source_tab.source)}</dt>
          <dd className="min-w-0">
            {link ? (
              <a
                href={link}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex max-w-full items-center gap-1 hover:underline"
              >
                <span className="truncate">{sourceLabel}</span>
                <span className="sr-only">{t(($) => $.open_source)}</span>
              </a>
            ) : (
              <span>{sourceLabel}</span>
            )}
          </dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.source_ref)}</dt>
          <dd className="min-w-0 truncate font-mono text-caption">
            {meta?.source_ref || t(($) => $.source_tab.no_source_ref)}
          </dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.version)}</dt>
          <dd className="font-mono text-caption">{meta?.latest_version || "—"}</dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.digest)}</dt>
          <dd className="min-w-0">
            {meta?.bundle_digest ? (
              <code className="font-mono text-caption" title={meta.bundle_digest}>
                {meta.bundle_digest.slice(0, 12)}
              </code>
            ) : (
              "—"
            )}
          </dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.size)}</dt>
          <dd className="tabular-nums">{meta ? formatHubSize(meta.size_bytes) : "—"}</dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.files)}</dt>
          <dd className="tabular-nums">{meta ? meta.file_count : "—"}</dd>

          <dt className="text-muted-foreground">{t(($) => $.source_tab.last_checked)}</dt>
          <dd>
            {meta?.checked_at ? timeAgo(meta.checked_at) : t(($) => $.source_tab.never_checked)}
          </dd>
        </dl>
      </section>

      <section aria-labelledby={`${headingId}-updates`} className="flex flex-col gap-3">
        <h2 id={`${headingId}-updates`} className="text-label font-medium">
          {t(($) => $.source_tab.tracking)}
        </h2>
        {tracking ? (
          <p className="text-label">
            <span className="font-medium">{t(($) => $.source_tab.tracking_state[tracking])}</span>
            <span className="text-muted-foreground">
              {" "}
              {t(($) => $.source_tab.tracking_help[tracking], { source: sourceLabel })}
            </span>
          </p>
        ) : null}

        {tracking === "tracking" && meta ? (
          <>
            {meta.local_modified ? (
              <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-caption text-muted-foreground">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden="true" />
                <span className="min-w-0 flex-1">{t(($) => $.source_tab.local_edits_warning)}</span>
              </div>
            ) : null}

            <div className="flex flex-wrap items-center gap-2">
              {updateAvailable ? (
                <UpdateAvailableBadge />
              ) : (
                <span className="text-label text-muted-foreground">
                  {check.data || meta.checked_at
                    ? t(($) => $.source_tab.up_to_date)
                    : t(($) => $.source_tab.never_checked)}
                </span>
              )}
              {canEdit ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={check.isFetching}
                    aria-busy={check.isFetching}
                    onClick={() => void runCheck()}
                  >
                    {check.isFetching ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
                    {check.isFetching ? t(($) => $.source_tab.checking) : t(($) => $.source_tab.check)}
                  </Button>
                  {updateAvailable ? (
                    <Button type="button" size="sm" onClick={() => setReviewing(true)}>
                      {t(($) => $.update.review)}
                    </Button>
                  ) : null}
                  <Button type="button" variant="outline" size="sm" onClick={() => setForking(true)}>
                    {t(($) => $.fork.action)}
                  </Button>
                </>
              ) : null}
            </div>

            {/* The badge or the "up to date" text carries the result; this line only reports a failed check. */}
            <p role="status" className="text-caption text-destructive">
              {check.isError ? t(($) => $.update.check_failed) : null}
            </p>

            {canEdit ? null : <p className="text-caption text-muted-foreground">{t(($) => $.source_tab.read_only)}</p>}

            {canEdit ? (
              <>
                <HubUpdateDialog
                  wsId={wsId}
                  kind={item.kind}
                  refId={item.refId}
                  name={item.name}
                  open={reviewing}
                  onClose={() => setReviewing(false)}
                />
                <HubForkDialog
                  wsId={wsId}
                  kind={item.kind}
                  refId={item.refId}
                  name={item.name}
                  sourceName={sourceLabel}
                  open={forking}
                  onClose={() => setForking(false)}
                />
              </>
            ) : null}
          </>
        ) : null}
      </section>
    </div>
  );
}
