"use client";

import { useId, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { cn } from "@multica/ui/lib/utils";
import {
  HubLocalChangesError,
  HubQuotaExceededError,
  hubUpdateCheckOptions,
  hubUpdateDiffOptions,
  useApplyHubUpdate,
  type HubItemKind,
  type HubUpdateCheck,
} from "@multica/core/hub";
import { useT } from "../i18n";
import { errorMessage } from "./hub-import-request";
import { formatHubSize } from "./hub-files";
import {
  collapseHubDiff,
  diffHubLines,
  pairHubDiffRows,
  type HubDiffLine,
  type HubDiffRow,
  type HubSideBySideRow,
} from "./hub-update-diff";

export type LocalChangesChoice = "overwrite" | "fork";

/**
 * Review and apply an update to a tracked item. Opens with the file-level change list;
 * selecting a file loads its text diff. Local edits must be resolved before applying.
 */
export function HubUpdateDialog({
  wsId,
  kind,
  refId,
  name,
  open,
  onClose,
}: {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  name: string;
  open: boolean;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-3xl">
        {open ? (
          <UpdateBody wsId={wsId} kind={kind} refId={refId} name={name} onClose={onClose} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function UpdateBody({
  wsId,
  kind,
  refId,
  name,
  onClose,
}: {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  name: string;
  onClose: () => void;
}) {
  const { t } = useT("hub");
  const check = useQuery(hubUpdateCheckOptions(wsId, kind, refId));
  const apply = useApplyHubUpdate();
  const radioName = useId();
  const [selected, setSelected] = useState<string | null>(null);
  const [choice, setChoice] = useState<LocalChangesChoice | null>(null);
  const [error, setError] = useState("");

  const data = check.data;
  const changed = useMemo(
    () => (data?.files ?? []).filter((file: UpdateFile) => file.status !== "unchanged"),
    [data],
  );
  // The first changed file is shown until the user picks another.
  const activePath = selected ?? changed[0]?.path ?? null;
  const diff = useQuery({
    ...hubUpdateDiffOptions(wsId, kind, refId, activePath ?? ""),
    enabled: activePath !== null && data !== undefined,
  });

  const localModified = data?.local_modified ?? false;
  const canApply = data?.update_available === true && (!localModified || choice !== null) && !apply.isPending;

  const submit = async () => {
    if (!data) return;
    setError("");
    const onLocalChanges: LocalChangesChoice | null = localModified ? choice : "overwrite";
    if (onLocalChanges === null) {
      setError(t(($) => $.update.choose_local));
      return;
    }
    try {
      await apply.mutateAsync({ wsId, kind, refId, onLocalChanges });
      toast.success(t(($) => $.update.toast.updated, { name }));
      onClose();
    } catch (err) {
      if (err instanceof HubLocalChangesError) {
        setError(t(($) => $.update.choose_local));
      } else if (err instanceof HubQuotaExceededError) {
        setError(t(($) => $.update.errors.quota));
      } else {
        setError(errorMessage(err, t(($) => $.update.errors.failed)));
      }
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t(($) => $.update.dialog_title, { name })}</DialogTitle>
      </DialogHeader>

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto">
        {check.isLoading ? (
          <div className="flex flex-col gap-2" aria-busy="true">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : check.isError ? (
          <div className="flex flex-col items-start gap-3 text-caption">
            <p role="alert" className="text-destructive">
              {t(($) => $.update.check_failed)}
            </p>
            <Button type="button" variant="outline" size="sm" onClick={() => void check.refetch()}>
              {t(($) => $.retry)}
            </Button>
          </div>
        ) : data && !data.update_available ? (
          <p className="text-label text-muted-foreground">{t(($) => $.source_tab.up_to_date)}</p>
        ) : data ? (
          <div className="grid min-h-0 gap-4 md:grid-cols-[16rem_minmax(0,1fr)]">
            <FileList
              changed={changed}
              activePath={activePath}
              onSelect={setSelected}
            />
            <DiffPane
              path={activePath}
              diff={diff.data}
              isLoading={diff.isLoading && activePath !== null}
              isError={diff.isError}
              onRetry={() => void diff.refetch()}
            />
          </div>
        ) : null}

        {data?.update_available && localModified ? (
          <fieldset className="flex flex-col gap-2 rounded-md border p-3">
            <legend className="px-1 text-caption text-muted-foreground">{t(($) => $.update.local.legend)}</legend>
            <p className="text-caption text-muted-foreground">{t(($) => $.update.local.description)}</p>
            <div className="grid gap-2 pt-1 sm:grid-cols-2">
              <label className="flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-label transition-colors hover:bg-muted/40">
                <input
                  type="radio"
                  name={radioName}
                  value="overwrite"
                  checked={choice === "overwrite"}
                  onChange={() => setChoice("overwrite")}
                  className="mt-0.5 accent-primary"
                />
                <span className="font-medium">{t(($) => $.update.local.overwrite)}</span>
              </label>
              <label className="flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-label transition-colors hover:bg-muted/40">
                <input
                  type="radio"
                  name={radioName}
                  value="fork"
                  checked={choice === "fork"}
                  onChange={() => setChoice("fork")}
                  className="mt-0.5 accent-primary"
                />
                <span className="font-medium">{t(($) => $.update.local.fork)}</span>
              </label>
            </div>
            {choice === null ? (
              <p className="text-caption text-destructive">{t(($) => $.update.choose_local)}</p>
            ) : null}
          </fieldset>
        ) : null}

        {error ? (
          <p role="alert" className="text-caption text-destructive">
            {error}
          </p>
        ) : null}
      </div>

      <DialogFooter>
        <DialogClose render={<Button variant="outline" disabled={apply.isPending} />}>
          {t(($) => $.update.cancel)}
        </DialogClose>
        {data?.update_available ? (
          <Button
            type="button"
            disabled={!canApply}
            aria-busy={apply.isPending}
            onClick={() => void submit()}
          >
            {apply.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
            {apply.isPending ? t(($) => $.update.applying) : t(($) => $.update.apply)}
          </Button>
        ) : null}
      </DialogFooter>
    </>
  );
}

type UpdateFile = HubUpdateCheck["files"][number];

function FileList({
  changed,
  activePath,
  onSelect,
}: {
  changed: UpdateFile[];
  activePath: string | null;
  onSelect: (path: string) => void;
}) {
  const { t } = useT("hub");
  const counts = {
    added: changed.filter((file) => file.status === "added").length,
    modified: changed.filter((file) => file.status === "modified").length,
    removed: changed.filter((file) => file.status === "removed").length,
  };

  return (
    <div className="flex min-h-0 flex-col gap-2">
      <p className="text-caption text-muted-foreground">
        {t(($) => $.update.changes, counts)}
      </p>
      <ul aria-label={t(($) => $.update.files_label)} className="flex max-h-64 flex-col gap-0.5 overflow-y-auto md:max-h-none">
        {changed.map((file) => {
          const selected = file.path === activePath;
          const statusTone =
            file.status === "added"
              ? "bg-success/15 text-success border-success/30"
              : file.status === "modified"
                ? "bg-warning/15 text-warning border-warning/30"
                : "bg-destructive/15 text-destructive border-destructive/30";
          return (
            <li key={file.path}>
              <button
                type="button"
                aria-pressed={selected}
                onClick={() => onSelect(file.path)}
                className={cn(
                  "flex w-full min-w-0 flex-col items-start gap-1 rounded-md px-2.5 py-2 text-left transition-colors focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring",
                  selected ? "bg-muted font-medium" : "hover:bg-muted/60",
                )}
              >
                <span className="flex w-full min-w-0 items-center justify-between gap-2 text-caption">
                  <span className="truncate font-mono">{file.path}</span>
                  <span className={cn("shrink-0 rounded-xs border px-1.5 py-0.5 text-micro font-medium", statusTone)}>
                    {t(($) => $.update.status[file.status])}
                  </span>
                </span>
                <span className="text-caption tabular-nums text-muted-foreground">
                  {t(($) => $.update.size_change, {
                    from: file.old_size === null ? t(($) => $.update.size_none) : formatHubSize(file.old_size),
                    to: file.new_size === null ? t(($) => $.update.size_none) : formatHubSize(file.new_size),
                  })}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** One file's text before and after the update. Null on a side the file does not exist on. */
type DiffResponse = { path: string; old: string | null; new: string | null; binary: boolean };

type DiffLayout = "unified" | "side_by_side";

function DiffPane({
  path,
  diff,
  isLoading,
  isError,
  onRetry,
}: {
  path: string | null;
  diff: DiffResponse | undefined;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
}) {
  const { t } = useT("hub");
  const [layout, setLayout] = useState<DiffLayout>("unified");
  const rows = useMemo(() => {
    if (!diff || diff.binary) return null;
    const result = diffHubLines(diff.old, diff.new);
    return { approximate: result.approximate, rows: collapseHubDiff(result.lines) };
  }, [diff]);

  if (path === null) {
    return <p className="text-label text-muted-foreground">{t(($) => $.update.diff.select_file)}</p>;
  }
  if (isLoading) {
    return (
      <div className="flex flex-col gap-2" aria-busy="true">
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3 text-caption">
        <p role="alert" className="text-destructive">
          {t(($) => $.update.diff.load_error)}
        </p>
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          {t(($) => $.retry)}
        </Button>
      </div>
    );
  }
  if (diff?.binary) {
    return <p className="text-label text-muted-foreground">{t(($) => $.update.diff.binary)}</p>;
  }
  if (!rows) return null;

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <DiffLayoutToggle layout={layout} onChange={setLayout} />
      <section
        role="region"
        aria-label={t(($) => $.update.diff.region, { path })}
        tabIndex={0}
        className="flex max-h-[50vh] min-w-0 flex-col overflow-auto rounded-md border font-mono text-caption leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {rows.approximate ? (
          <p className="border-b bg-muted/40 px-3 py-2 font-sans text-muted-foreground">
            {t(($) => $.update.diff.approximate)}
          </p>
        ) : null}
        {rows.rows.length === 0 ? (
          <p className="px-3 py-2 font-sans text-muted-foreground">{t(($) => $.update.diff.no_changes)}</p>
        ) : layout === "unified" ? (
          rows.rows.map((row, index) => <DiffRowView key={index} row={row} />)
        ) : (
          <SideBySideView rows={pairHubDiffRows(rows.rows)} />
        )}
      </section>
    </div>
  );
}

function DiffLayoutToggle({
  layout,
  onChange,
}: {
  layout: DiffLayout;
  onChange: (layout: DiffLayout) => void;
}) {
  const { t } = useT("hub");
  return (
    <div
      role="group"
      aria-label={t(($) => $.update.diff.layout.aria)}
      className="flex w-fit items-center gap-0.5 rounded-md bg-muted p-0.5"
    >
      {(["unified", "side_by_side"] as const).map((value) => (
        <button
          key={value}
          type="button"
          aria-pressed={layout === value}
          onClick={() => onChange(value)}
          className={cn(
            "h-6 rounded-xs px-2 text-caption font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            layout === value ? "bg-surface text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {value === "unified" ? t(($) => $.update.diff.layout.unified) : t(($) => $.update.diff.layout.side_by_side)}
        </button>
      ))}
    </div>
  );
}

/**
 * Before and after as a table, so each line sits opposite its counterpart. Cells do
 * not wrap: a long line scrolls the enclosing region, and both columns scroll together.
 */
function SideBySideView({ rows }: { rows: HubSideBySideRow[] }) {
  const { t } = useT("hub");
  return (
    <table className="w-max min-w-full border-separate border-spacing-0">
      <thead className="sticky top-0 bg-surface text-left font-sans text-caption text-muted-foreground">
        <tr>
          <th scope="col" className="border-b px-3 py-1.5 font-medium">
            {t(($) => $.update.diff.before)}
          </th>
          <th scope="col" className="border-b border-l px-3 py-1.5 font-medium">
            {t(($) => $.update.diff.after)}
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row, index) =>
          row.type === "gap" ? (
            <tr key={index}>
              <td colSpan={2} className="bg-muted/40 px-3 py-1 font-sans text-muted-foreground">
                {t(($) => $.update.diff.gap, { count: row.count })}
              </td>
            </tr>
          ) : (
            <tr key={index}>
              <SideBySideCell line={row.left} column="before" />
              <SideBySideCell line={row.right} column="after" />
            </tr>
          ),
        )}
      </tbody>
    </table>
  );
}

function SideBySideCell({ line, column }: { line: HubDiffLine | null; column: "before" | "after" }) {
  const { t } = useT("hub");
  // The change is marked by color and by a screen-reader word, not by a sign alone.
  const kind = line?.kind ?? null;
  return (
    <td
      className={cn(
        "min-w-[20rem] whitespace-pre px-3 align-top",
        column === "after" && "border-l",
        kind === "removed" && "bg-destructive/10",
        kind === "added" && "bg-success/10",
        line === null && "bg-muted/30",
      )}
    >
      {kind === "added" || kind === "removed" ? (
        <span className="sr-only">
          {kind === "added" ? t(($) => $.update.diff.added_line) : t(($) => $.update.diff.removed_line)}
        </span>
      ) : null}
      {line ? line.text || " " : null}
    </td>
  );
}

function DiffRowView({ row }: { row: HubDiffRow }) {
  const { t } = useT("hub");
  if (row.type === "gap") {
    return (
      <div className="bg-muted/40 px-3 py-1 font-sans text-muted-foreground">
        {t(($) => $.update.diff.gap, { count: row.count })}
      </div>
    );
  }
  const { line } = row;
  const added = line.kind === "added";
  const removed = line.kind === "removed";
  return (
    <div
      className={cn(
        "flex min-w-0 px-3",
        added && "bg-success/10",
        removed && "bg-destructive/10",
      )}
    >
      <span aria-hidden="true" className="w-4 shrink-0 select-none text-muted-foreground">
        {added ? "+" : removed ? "-" : " "}
      </span>
      {added || removed ? (
        <span className="sr-only">
          {added ? t(($) => $.update.diff.added_line) : t(($) => $.update.diff.removed_line)}
        </span>
      ) : null}
      <span className="min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{line.text || " "}</span>
    </div>
  );
}
