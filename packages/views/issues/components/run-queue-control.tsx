"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { issueStatusCategory } from "@multica/core/issues";
import { runQueueWaitingOn, runQueueWaitsOptions } from "@multica/core/issues/queries";
import type { Issue, RunQueueSetting } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { cn } from "@multica/ui/lib/utils";
import { Check, ListOrdered, Search } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { useRunQueueApply } from "./use-run-queue-apply";

const PICKER_LIMIT = 50;

/** Open issues the queued issues can wait on. Terminal issues are not offered. */
function useWaitCandidates(query: string, excludeIds: readonly string[]) {
  const wsId = useWorkspaceId();
  const { data, isFetching } = useQuery({
    queryKey: [...["issues", wsId], "run-queue-candidates", query.trim()] as const,
    queryFn: () => api.listIssues({ limit: PICKER_LIMIT, q: query.trim() || undefined }),
    enabled: !!wsId,
    staleTime: 10_000,
  });
  const excluded = useMemo(() => new Set(excludeIds), [excludeIds]);
  const issues = useMemo(
    () =>
      (data?.issues ?? []).filter(
        (issue) => !excluded.has(issue.id) && issueStatusCategory(issue) !== "done" && issueStatusCategory(issue) !== "closed",
      ),
    [data, excluded],
  );
  return { issues, isFetching };
}

/**
 * Search field and checkable list of open issues, each shown as `ISSUE-ID: Title`.
 * `waits` is the selection; an empty selection means the queued issues start right away.
 * Used by the queue popover and by the new-issue form, so it owns no footer or submit.
 */
export function RunQueueWaitPicker({
  excludeIds,
  waits,
  onChange,
  autoFocus = false,
}: {
  /** Issues that cannot be chosen, such as the queued issue itself. */
  excludeIds: string[];
  waits: string[];
  onChange: (waits: string[]) => void;
  autoFocus?: boolean;
}) {
  const { t } = useT("issues");
  const [query, setQuery] = useState("");
  const { issues, isFetching } = useWaitCandidates(query, excludeIds);

  const toggle = (id: string) =>
    onChange(waits.includes(id) ? waits.filter((w) => w !== id) : [...waits, id]);

  return (
    <div className="flex flex-col">
      <div className="p-2 pb-1.5">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            autoFocus={autoFocus}
            autoComplete="off"
            aria-label={t(($) => $.run_queue.search_label)}
            placeholder={t(($) => $.run_queue.search_placeholder)}
            className="h-7 pl-7 text-caption"
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.stopPropagation()}
          />
        </div>
      </div>

      <div role="listbox" aria-multiselectable="true" className="flex max-h-72 flex-col overflow-y-auto px-1 pb-1">
        {issues.length === 0 && (
          <div className="py-4 text-center text-caption text-muted-foreground">
            {isFetching ? t(($) => $.run_queue.searching) : t(($) => $.run_queue.no_results)}
          </div>
        )}
        {issues.map((issue: Issue) => {
          const selected = waits.includes(issue.id);
          return (
            <button
              key={issue.id}
              type="button"
              role="option"
              aria-selected={selected}
              onClick={() => toggle(issue.id)}
              className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-left text-caption hover:bg-accent"
            >
              {/* Visual check box only: the row itself is the button. */}
              <span
                aria-hidden="true"
                className={cn(
                  "flex size-4 shrink-0 items-center justify-center rounded-xs border",
                  selected ? "border-primary bg-primary text-primary-foreground" : "border-input bg-background",
                )}
              >
                {selected && <Check className="size-3" />}
              </span>
              <span className="min-w-0 truncate">
                <span className="font-medium tabular-nums">{issue.identifier}</span>: {issue.title}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** The popover body: the wait picker plus the apply footer. */
function RunQueueForm({
  issueIds,
  initialWaits,
  initialQueued,
  onSubmit,
  onTakeOut,
  disabled,
}: {
  issueIds: string[];
  initialWaits: string[];
  initialQueued: boolean;
  onSubmit: (queue: RunQueueSetting) => void;
  onTakeOut?: () => void;
  disabled?: boolean;
}) {
  const { t } = useT("issues");
  const [waits, setWaits] = useState<string[]>(initialWaits);

  return (
    <div className="flex w-80 flex-col">
      <RunQueueWaitPicker excludeIds={issueIds} waits={waits} onChange={setWaits} autoFocus />

      <div className="flex items-center justify-between gap-2 border-t px-2.5 py-2">
        <span className="text-caption text-muted-foreground">
          {waits.length === 0
            ? t(($) => $.run_queue.none_selected)
            : t(($) => $.run_queue.selected_count, { count: waits.length })}
        </span>
        <div className="flex gap-1.5">
          {initialQueued && onTakeOut && (
            <Button variant="ghost" size="sm" disabled={disabled} onClick={onTakeOut}>
              {t(($) => $.run_queue.take_out)}
            </Button>
          )}
          <Button size="sm" disabled={disabled} onClick={() => onSubmit({ queued: true, afterIssueIds: waits })}>
            {t(($) => $.run_queue.apply)}
          </Button>
        </div>
      </div>
    </div>
  );
}

/** Run-queue state of one backlog issue: a button that opens the wait picker. */
export function RunQueueControl({
  issue,
  className,
}: {
  issue: Pick<Issue, "id" | "run_queued" | "run_after_issue_ids">;
  className?: string;
}) {
  const { t } = useT("issues");
  const { apply, dialog, isPending } = useRunQueueApply();
  const [open, setOpen] = useState(false);
  const queued = issue.run_queued ?? false;
  const waits = issue.run_after_issue_ids ?? [];

  const submit = (queue: RunQueueSetting) => {
    setOpen(false);
    void apply([issue.id], queue);
  };

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              variant={queued ? "brandSubtle" : "outline"}
              size="sm"
              disabled={isPending}
              className={className}
              // Board cards wrap this control in a link; the browser's own anchor
              // activation would still open the card, so the click is cancelled here.
              onClick={(e) => e.preventDefault()}
            />
          }
        >
          <ListOrdered />
          {queued ? t(($) => $.run_queue.queued) : t(($) => $.run_queue.queue)}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 gap-0 p-0">
          {open && (
            <RunQueueForm
              issueIds={[issue.id]}
              initialWaits={waits}
              initialQueued={queued}
              disabled={isPending}
              onSubmit={submit}
              onTakeOut={() => submit({ queued: false, afterIssueIds: [] })}
            />
          )}
        </PopoverContent>
      </Popover>
      {dialog}
    </>
  );
}

/** Puts several backlog issues in the run queue behind the same issues. */
export function BatchRunQueueControl({
  issueIds,
  disabled,
  onApplied,
}: {
  issueIds: string[];
  disabled?: boolean;
  onApplied?: () => void;
}) {
  const { t } = useT("issues");
  const { apply, dialog, isPending } = useRunQueueApply();
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (issueIds.length === 0) setOpen(false);
  }, [issueIds.length]);

  if (issueIds.length === 0) return dialog;

  const submit = (queue: RunQueueSetting) => {
    setOpen(false);
    void apply(issueIds, queue, { onDone: onApplied });
  };

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger render={<Button variant="ghost" size="sm" disabled={disabled || isPending} />}>
          {t(($) => $.run_queue.label)}
        </PopoverTrigger>
        <PopoverContent align="center" className="w-80 gap-0 p-0">
          {open && (
            <RunQueueForm
              issueIds={issueIds}
              initialWaits={[]}
              initialQueued={false}
              disabled={isPending}
              onSubmit={submit}
            />
          )}
        </PopoverContent>
      </Popover>
      {dialog}
    </>
  );
}

/** "Waiting on N" while the queued issue still waits on unfinished work. */
export function RunQueueWaitingLabel({
  issue,
  className,
}: {
  issue: Pick<Issue, "id" | "status" | "run_queued">;
  className?: string;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const { data } = useQuery({
    ...runQueueWaitsOptions(wsId),
    enabled: issue.status === "backlog" && issue.run_queued === true,
  });
  const n = runQueueWaitingOn(issue.id, data).length;
  if (n === 0) return null;
  return <span className={className}>{t(($) => $.run_queue.waiting, { count: n })}</span>;
}
