"use client";

import { useMemo, useState } from "react";
import { clientErrorMessage } from "@multica/core/api";
import { useSimulateFlowDraft } from "@multica/core/flow-drafts";
import type { FlowDoc, FlowDocTask, FlowTraceResponse } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { ArrowRight, Loader2, Play, RotateCcw } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface SimulateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
  draftId: string;
  doc: FlowDoc;
}

// The most steps a simulation follows by default (spec 7.6); the server caps it at 120.
const DEFAULT_MAX_HOPS = 60;
// A script answers one entry into a step at a time. A task result chosen here holds for every entry a
// loop can make, so it is repeated as often as a loop may run (limits.loop_max) and once more.
const SCRIPT_ENTRIES = 21;

const BUILTIN_VERDICTS: Record<string, string[]> = {
  work: ["done"],
  gate: ["approved", "rejected"],
};

/** The verdicts a task can report: its own set, or the built-in one of its kind. */
export function verdictKeysOf(doc: FlowDoc, task: FlowDocTask): string[] {
  const setKey = task.verdicts || (task.kind === "gate" ? "gate" : "work");
  const declared = doc.verdicts?.find((set) => set.key === setKey);
  if (declared) return declared.verdicts.map((v) => v.key);
  return BUILTIN_VERDICTS[setKey] ?? [];
}

/** The simulate request of a document: its variables, and what each task reports when it closes. */
export function buildSimulateRequest(
  doc: FlowDoc,
  vars: Record<string, string>,
  results: Record<string, string>,
) {
  const typed: Record<string, unknown> = {};
  for (const v of doc.vars ?? []) {
    const raw = vars[v.key];
    if (raw === undefined || raw === "") continue;
    if (v.type === "bool") typed[v.key] = raw === "true";
    else if (v.type === "int") typed[v.key] = Number(raw);
    else if (v.type === "list_string") typed[v.key] = raw.split(",").map((s) => s.trim()).filter(Boolean);
    else typed[v.key] = raw;
  }
  const script: Record<string, Array<Record<string, string>>> = {};
  for (const step of doc.steps ?? []) {
    const entry: Record<string, string> = {};
    for (const task of step.tasks ?? []) {
      const chosen = results[`${step.key}.${task.key}`];
      if (chosen) entry[task.key] = chosen;
    }
    if (Object.keys(entry).length > 0) script[step.key] = Array.from({ length: SCRIPT_ENTRIES }, () => ({ ...entry }));
  }
  return { doc, vars: typed, script, max_hops: DEFAULT_MAX_HOPS };
}

export function SimulateDialog({ open, onOpenChange, squadId, draftId, doc }: SimulateDialogProps) {
  const { t } = useT("squads");
  const [trace, setTrace] = useState<FlowTraceResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [vars, setVars] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, string>>({});
  const simulateMut = useSimulateFlowDraft();

  const tasks = useMemo(
    () => (doc.steps ?? []).flatMap((step) => (step.tasks ?? []).map((task) => ({ step: step.key, task }))),
    [doc.steps],
  );

  const handleRun = async () => {
    setError(null);
    try {
      setTrace(await simulateMut.mutateAsync({ squadId, draftId, data: buildSimulateRequest(doc, vars, results) }));
    } catch (err) {
      setError(clientErrorMessage(err) ?? t(($) => $.flow_designer.simulate.failed));
    }
  };

  const lastHop = trace?.hops[trace.hops.length - 1];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-xl flex-col p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle className="flex items-center gap-2">
            <Play className="size-4 text-emerald-500" aria-hidden />
            <span>{t(($) => $.flow_designer.actions.simulate)}</span>
          </DialogTitle>
          <DialogDescription className="text-body-sm text-muted-foreground">
            {t(($) => $.flow_designer.simulate.note)}
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 space-y-4 overflow-y-auto py-2 text-caption">
          {(doc.vars?.length ?? 0) > 0 && (
            <fieldset className="space-y-1.5">
              <legend className="text-micro font-medium text-muted-foreground">
                {t(($) => $.flow_designer.simulate.vars)}
              </legend>
              {(doc.vars ?? []).map((v) => (
                <label key={v.key} className="flex items-center justify-between gap-2">
                  <span className="font-mono">{v.key}</span>
                  {v.type === "bool" || v.type === "enum" ? (
                    <select
                      className="h-8 rounded-md border border-input bg-background px-2"
                      value={vars[v.key] ?? ""}
                      onChange={(e) => setVars((prev) => ({ ...prev, [v.key]: e.target.value }))}
                    >
                      <option value="">{t(($) => $.flow_designer.simulate.default)}</option>
                      {(v.type === "bool" ? ["true", "false"] : (v.values ?? [])).map((value) => (
                        <option key={value} value={value}>
                          {value}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      className="h-8 w-40 rounded-md border border-input bg-background px-2"
                      value={vars[v.key] ?? ""}
                      onChange={(e) => setVars((prev) => ({ ...prev, [v.key]: e.target.value }))}
                    />
                  )}
                </label>
              ))}
            </fieldset>
          )}

          {tasks.length > 0 && (
            <fieldset className="space-y-1.5">
              <legend className="text-micro font-medium text-muted-foreground">
                {t(($) => $.flow_designer.simulate.results)}
              </legend>
              {tasks.map(({ step, task }) => (
                <label key={`${step}.${task.key}`} className="flex items-center justify-between gap-2">
                  <span className="min-w-0 truncate">
                    <span className="font-mono">{step}</span>
                    <span className="text-muted-foreground"> / </span>
                    <span>{task.title || task.key}</span>
                  </span>
                  <select
                    className="h-8 rounded-md border border-input bg-background px-2"
                    value={results[`${step}.${task.key}`] ?? ""}
                    onChange={(e) => setResults((prev) => ({ ...prev, [`${step}.${task.key}`]: e.target.value }))}
                  >
                    <option value="">{t(($) => $.flow_designer.simulate.default)}</option>
                    {verdictKeysOf(doc, task).map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
            </fieldset>
          )}

          {error && (
            <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-destructive">
              {error}
            </div>
          )}

          {trace && (
            <div className="space-y-3">
              <div className="flex items-center justify-between rounded-lg bg-muted/40 p-2">
                <span className="font-semibold text-foreground">{t(($) => $.flow_designer.simulate.end)}</span>
                <Badge variant={trace.end.kind === "terminal" ? "secondary" : "outline"} className="font-mono text-micro">
                  {trace.end.code ? `${trace.end.kind}: ${trace.end.code}` : trace.end.kind}
                </Badge>
              </div>

              <div className="space-y-1.5">
                <span className="text-micro font-medium text-muted-foreground">
                  {t(($) => $.flow_designer.simulate.path)} ({trace.hops.length})
                </span>
                <ul className="divide-y divide-border/60 overflow-hidden rounded-lg border border-border">
                  {trace.hops.map((hop, idx) => (
                    <li key={idx} className="flex items-center justify-between gap-2 p-2.5 hover:bg-muted/20">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-micro tabular-nums text-muted-foreground">#{idx + 1}</span>
                        <span className="font-medium text-foreground">{hop.step}</span>
                        {hop.activation > 1 && (
                          <Badge variant="outline" className="gap-0.5 text-micro font-normal text-blue-600">
                            <RotateCcw className="size-2.5" aria-hidden />
                            <span>{t(($) => $.flow_designer.simulate.visit, { n: hop.activation })}</span>
                          </Badge>
                        )}
                      </div>
                      {hop.edge && (
                        <div className="flex items-center gap-1 text-micro text-muted-foreground">
                          <ArrowRight className="size-3" aria-hidden />
                          <span className="font-mono">{hop.edge}</span>
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
                {trace.truncated && (
                  <p className="text-micro text-muted-foreground">{t(($) => $.flow_designer.simulate.truncated)}</p>
                )}
              </div>

              {lastHop?.loops && Object.keys(lastHop.loops).length > 0 && (
                <div className="flex flex-wrap items-center gap-2 text-micro text-muted-foreground">
                  <span>{t(($) => $.flow_designer.simulate.loops)}</span>
                  {Object.entries(lastHop.loops).map(([loopKey, count]) => (
                    <Badge key={loopKey} variant="secondary" className="font-mono text-micro">
                      {loopKey}: {count}
                    </Badge>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter className="gap-2 border-t border-border pt-4">
          <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t(($) => $.flow_designer.simulate.close)}
          </Button>
          <Button type="button" size="sm" disabled={simulateMut.isPending} onClick={() => void handleRun()} className="gap-1.5">
            {simulateMut.isPending ? (
              <Loader2 className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Play className="size-3.5" aria-hidden />
            )}
            <span>{t(($) => $.flow_designer.simulate.run)}</span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
