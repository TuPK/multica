"use client";

import { useQuery } from "@tanstack/react-query";
import { queuedAttentionOptions } from "@multica/core/governance";
import { Loader2 } from "@multica/ui/icons";
import { useT, useTimeAgo } from "../i18n";

/**
 * Questions queued for the caller while a squad has spent its weekly attention share
 * (durable spec 7.5). Mounted by the Controls tab only when `durable_principal` is on.
 */
export function AttentionQueueSection({ wsId }: { wsId: string }) {
  const { t } = useT("governance");
  const timeAgo = useTimeAgo();
  const query = useQuery(queuedAttentionOptions(wsId));

  const asks = query.data?.asks ?? [];

  return (
    <section className="flex flex-col gap-3" aria-labelledby="governance-attention-queue-heading">
      <h2 id="governance-attention-queue-heading" className="text-title-sm font-medium">
        {t(($) => $.controls.attention_queue_heading)}
      </h2>

      {query.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.controls.attention_queue_load_failed)}
        </p>
      ) : query.isPending ? (
        <div className="flex items-center py-2">
          <Loader2 className="size-4 animate-spin text-muted-foreground" />
        </div>
      ) : asks.length === 0 ? (
        <p className="text-caption text-muted-foreground">
          {t(($) => $.controls.attention_queue_empty)}
        </p>
      ) : (
        <ul className="flex max-w-2xl flex-col divide-y divide-surface-border rounded-md border border-surface-border">
          {asks.map((ask) => {
            const age = formatCreatedAge(ask.created_at, timeAgo);
            return (
              <li
                key={`${ask.subject_type}:${ask.subject_id}`}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5 text-body"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
                  <span className="font-medium text-foreground">
                    {t(($) => $.controls.attention_subject_type[ask.subject_type])}
                  </span>
                  <span aria-hidden className="text-muted-foreground">·</span>
                  <span className="text-caption text-muted-foreground">
                    {t(($) => $.controls.attention_reason[ask.reason])}
                  </span>
                </div>
                <div className="flex shrink-0 items-center gap-x-2 text-caption tabular-nums text-muted-foreground">
                  <span>
                    {t(($) => $.controls.attention_queue_minutes, {
                      minutes: formatMinutes(ask.estimated_minutes),
                    })}
                  </span>
                  {age ? (
                    <>
                      <span aria-hidden>·</span>
                      <time dateTime={ask.created_at}>{age}</time>
                    </>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function formatMinutes(minutes: number): string {
  const rounded = Math.round(minutes * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function formatCreatedAge(createdAt: string, timeAgo: (iso: string) => string): string {
  if (!createdAt || Number.isNaN(Date.parse(createdAt))) return "";
  return timeAgo(createdAt);
}
