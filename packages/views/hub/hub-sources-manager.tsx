"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { ExternalLink, Globe, Pencil, Plus, Trash2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Switch } from "@multica/ui/components/ui/switch";
import { cn } from "@multica/ui/lib/utils";
import { hubSourcesOptions, useUpdateHubSource, type HubSource } from "@multica/core/hub";
import { useT, useTimeAgo } from "../i18n";
import { CollectionPageState } from "../layout/collection-page";
import { PAGE_GUTTER } from "../layout/page-header";
import { errorMessage } from "./hub-import-request";
import { HubSourceDeleteDialog, HubSourceFormDialog } from "./hub-source-dialogs";
import { isKnownSourceType, safeSourceLink, syncStatusKind } from "./hub-source-meta";
import { SyncSourceButton } from "./hub-source-sync-button";
import { useAutoSyncLabel } from "./hub-source-sync";

/** Sources table on the Hub's Sources tab. Only admins and owners can change anything. */
export function HubSourcesManager({ wsId, canManage }: { wsId: string; canManage: boolean }) {
  const { t } = useT("hub");
  const query = useQuery(hubSourcesOptions(wsId));
  // null: closed; "new": add; a source: edit.
  const [editing, setEditing] = useState<HubSource | "new" | null>(null);
  const [removing, setRemoving] = useState<HubSource | null>(null);

  if (query.isError) {
    return (
      <CollectionPageState
        icon={Globe}
        tone="destructive"
        title={t(($) => $.sources.load_error)}
        actions={
          <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
            {t(($) => $.retry)}
          </Button>
        }
      />
    );
  }

  if (query.isLoading) {
    return (
      <div className={cn("flex flex-col gap-2 py-4", PAGE_GUTTER)}>
        <Skeleton className="h-10 w-full" aria-label={t(($) => $.sources.loading)} />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  const rows = query.data ?? [];

  return (
    <div className={cn("flex min-h-0 flex-col gap-3 py-4", PAGE_GUTTER)}>
      {canManage ? (
        <div className="flex justify-end">
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" />
            {t(($) => $.sources.add)}
          </Button>
        </div>
      ) : (
        <p className="text-caption text-muted-foreground">{t(($) => $.sources.read_only)}</p>
      )}

      {rows.length === 0 ? (
        <CollectionPageState icon={Globe} title={t(($) => $.sources.empty_title)} />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[44rem] table-fixed text-label">
            <thead className="text-left text-caption text-muted-foreground">
              <tr className="border-b">
                <th className="py-2 pr-3 font-normal">{t(($) => $.sources.columns.name)}</th>
                <th className="w-28 py-2 pr-3 font-normal">{t(($) => $.sources.columns.type)}</th>
                <th className="py-2 pr-3 font-normal">{t(($) => $.sources.columns.url)}</th>
                <th className="w-20 py-2 pr-3 font-normal">{t(($) => $.sources.columns.items)}</th>
                <th className="w-44 py-2 pr-3 font-normal">{t(($) => $.sources.columns.last_sync)}</th>
                <th className="w-28 py-2 pr-3 font-normal">{t(($) => $.sources.columns.enabled)}</th>
                {canManage ? <th className="w-28 py-2 font-normal" aria-label={t(($) => $.sources.columns.actions)} /> : null}
              </tr>
            </thead>
            <tbody>
              {rows.map((source) => (
                <SourceRow
                  key={source.id}
                  wsId={wsId}
                  source={source}
                  canManage={canManage}
                  onEdit={() => setEditing(source)}
                  onRemove={() => setRemoving(source)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <HubSourceFormDialog
        wsId={wsId}
        source={editing === "new" || editing === null ? null : editing}
        open={editing !== null}
        onClose={() => setEditing(null)}
      />
      <HubSourceDeleteDialog wsId={wsId} source={removing} onClose={() => setRemoving(null)} />
    </div>
  );
}

function SourceRow({
  wsId,
  source,
  canManage,
  onEdit,
  onRemove,
}: {
  wsId: string;
  source: HubSource;
  canManage: boolean;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const update = useUpdateHubSource();
  const scheduleLabel = useAutoSyncLabel();
  const typeLabel = isKnownSourceType(source.type)
    ? t(($) => $.source_types[source.type])
    : source.type;
  const link = source.url ? safeSourceLink(source.url) : null;
  const status = syncStatusKind(source);
  // The scheduler only runs enabled sources, so a disabled one says it is paused.
  const schedule =
    source.auto_sync_interval_sec === 0 || source.enabled
      ? t(($) => $.sync.auto, { interval: scheduleLabel(source.auto_sync_interval_sec) })
      : t(($) => $.sync.auto_paused, { interval: scheduleLabel(source.auto_sync_interval_sec) });

  const toggle = async (enabled: boolean) => {
    try {
      await update.mutateAsync({ wsId, id: source.id, enabled });
    } catch (err) {
      toast.error(errorMessage(err, t(($) => $.sources.toggle_failed)));
    }
  };

  return (
    <tr className="border-b last:border-b-0 align-middle">
      <td className="min-w-0 py-2 pr-3">
        <span className="block truncate font-medium">{source.name}</span>
      </td>
      <td className="py-2 pr-3">
        <span className="inline-block max-w-full truncate rounded-sm bg-muted px-1.5 py-0.5 text-caption text-muted-foreground">
          {typeLabel}
        </span>
      </td>
      <td className="min-w-0 py-2 pr-3">
        {link ? (
          <a
            href={link}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex max-w-full items-center gap-1 text-muted-foreground hover:text-foreground"
          >
            <span className="truncate">{source.url}</span>
            <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
            <span className="sr-only">{t(($) => $.open_source)}</span>
          </a>
        ) : (
          <span className="block truncate text-muted-foreground">{source.url || t(($) => $.sources.no_url)}</span>
        )}
      </td>
      <td className="py-2 pr-3 tabular-nums">{source.item_count}</td>
      <td className="min-w-0 py-2 pr-3">
        <div className="flex min-w-0 flex-col">
          <span className="text-caption">
            {t(($) => $.sources.sync[status])}
            {source.last_sync_at ? (
              <span className="text-muted-foreground"> · {timeAgo(source.last_sync_at)}</span>
            ) : null}
          </span>
          {status === "error" && source.last_sync_error ? (
            <span className="truncate text-caption text-destructive" title={source.last_sync_error}>
              {source.last_sync_error}
            </span>
          ) : null}
          <span className="truncate text-caption text-muted-foreground">
            {schedule}
          </span>
        </div>
      </td>
      <td className="py-2 pr-3">
        {canManage ? (
          <Switch
            checked={source.enabled}
            disabled={update.isPending}
            onCheckedChange={(checked) => void toggle(checked)}
            aria-label={t(($) => $.sources.toggle_aria, { name: source.name })}
          />
        ) : (
          <span className="text-caption text-muted-foreground">
            {source.enabled ? t(($) => $.sources.enabled) : t(($) => $.sources.disabled)}
          </span>
        )}
      </td>
      {canManage ? (
        <td className="py-2">
          <div className="flex items-center justify-end gap-1">
            <SyncSourceButton wsId={wsId} source={source} />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t(($) => $.sources.edit_aria, { name: source.name })}
              onClick={onEdit}
            >
              <Pencil aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t(($) => $.sources.remove_aria, { name: source.name })}
              onClick={onRemove}
            >
              <Trash2 aria-hidden="true" />
            </Button>
          </div>
        </td>
      ) : null}
    </tr>
  );
}
