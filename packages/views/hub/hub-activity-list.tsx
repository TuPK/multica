"use client";

import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Activity, ArrowUpCircle, FileText, GitBranch, RefreshCw } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { hubActivityInfiniteOptions, type HubActivityEntry, type HubActivityFilter } from "@multica/core/hub";
import { useT, useTimeAgo } from "../i18n";
import { CollectionPageState } from "../layout/collection-page";
import { hubActivityActionKey, hubActivityFacts, type HubActivityFact } from "./hub-activity-format";

/**
 * Hub activity, newest first. The item page passes a filter; the Hub's Activity tab
 * passes none and shows which item each entry belongs to.
 */
export function HubActivityList({
  wsId,
  filter,
  showItem = false,
}: {
  wsId: string;
  filter?: HubActivityFilter;
  showItem?: boolean;
}) {
  const { t } = useT("hub");
  const query = useInfiniteQuery(hubActivityInfiniteOptions(wsId, filter));
  const entries = useMemo(
    () => query.data?.pages.flatMap((page) => page.entries) ?? [],
    [query.data],
  );

  if (query.isPending) {
    return (
      <div className="flex flex-col gap-3 py-2" aria-busy="true" aria-label={t(($) => $.activity.loading)}>
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-4 w-3/5" />
      </div>
    );
  }
  if (query.isError) {
    return (
      <CollectionPageState
        icon={Activity}
        tone="destructive"
        title={t(($) => $.activity.load_error)}
        actions={
          <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
            {t(($) => $.retry)}
          </Button>
        }
      />
    );
  }
  if (entries.length === 0) {
    return <CollectionPageState icon={Activity} title={t(($) => $.activity.empty)} />;
  }

  return (
    <div className="flex flex-col">
      <ul aria-label={t(($) => $.activity.list_label)} className="divide-y">
        {entries.map((entry) => (
          <HubActivityRow key={entry.id} entry={entry} showItem={showItem} />
        ))}
      </ul>
      {query.hasNextPage ? (
        <div className="flex justify-center py-4">
          <Button
            variant="outline"
            size="sm"
            disabled={query.isFetchingNextPage}
            aria-busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            {t(($) => $.activity.load_more)}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function ActivityIcon({ actionKey }: { actionKey: string }) {
  const className = "size-3.5";
  switch (actionKey) {
    case "file_saved":
      return <FileText className={className} aria-hidden="true" />;
    case "source_synced":
      return <RefreshCw className={className} aria-hidden="true" />;
    case "forked":
      return <GitBranch className={className} aria-hidden="true" />;
    case "update_applied":
      return <ArrowUpCircle className={className} aria-hidden="true" />;
    default:
      return <Activity className={className} aria-hidden="true" />;
  }
}

function HubActivityRow({ entry, showItem }: { entry: HubActivityEntry; showItem: boolean }) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const factLabel = useFactLabel();
  const actionKey = hubActivityActionKey(entry.action);
  const facts = hubActivityFacts(entry).map(factLabel);
  const name = entry.actor_name.trim();
  const actor = name !== "" ? name : entry.actor_type === "system" ? t(($) => $.activity.actor.system) : t(($) => $.activity.actor.unknown);
  const item = entry.item_name.trim();

  return (
    <li className="flex min-w-0 items-start gap-3 py-3">
      <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground mt-0.5">
        <ActivityIcon actionKey={actionKey} />
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 text-label">
          <span className="font-medium">{t(($) => $.activity.actions[actionKey])}</span>
          {showItem && item !== "" ? <span className="min-w-0 truncate text-muted-foreground">{item}</span> : null}
        </div>
        {facts.length > 0 ? (
          <p className="min-w-0 truncate text-caption text-muted-foreground">{facts.join(" · ")}</p>
        ) : null}
        <p className="text-caption text-muted-foreground">
          <span>{actor}</span>
          <span aria-hidden="true"> · </span>
          <time dateTime={entry.created_at} title={new Date(entry.created_at).toLocaleString()}>
            {timeAgo(entry.created_at)}
          </time>
        </p>
      </div>
    </li>
  );
}

/** Localized text for one fact. Each fact has its own key, so plurals follow the locale's rules. */
function useFactLabel() {
  const { t } = useT("hub");
  return (fact: HubActivityFact): string => {
    switch (fact.kind) {
      case "version":
        return t(($) => $.activity.detail.version, { version: fact.version });
      case "items":
        return t(($) => $.activity.detail.items, { count: fact.count });
      case "files":
        return t(($) => $.activity.detail.files, { count: fact.count });
      case "checked":
        return t(($) => $.activity.detail.checked, { count: fact.count });
      case "updates":
        return t(($) => $.activity.detail.updates, { count: fact.count });
      case "path":
        return t(($) => $.activity.detail.path, { path: fact.path });
      case "size":
        return t(($) => $.activity.detail.size, { size: fact.size });
    }
  };
}
