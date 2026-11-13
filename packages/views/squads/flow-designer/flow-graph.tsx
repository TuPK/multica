/* eslint-disable i18next/no-literal-string */
import { useMemo, useState } from "react";
import type { FlowDoc, FlowIssue } from "@multica/core/types";
import { layoutFlowGraph } from "@multica/core/squads/flow-layout";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  AlertTriangle,
  Maximize2,
  RotateCcw,
  Search,
} from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

export interface FlowGraphProps {
  doc: FlowDoc;
  selectedStepKey: string | null;
  onSelectStep: (stepKey: string) => void;
  issues?: FlowIssue[];
}

export function FlowGraph({
  doc,
  selectedStepKey,
  onSelectStep,
  issues = [],
}: FlowGraphProps) {
  const { t } = useT("squads");
  const [showLoopsOnly, setShowLoopsOnly] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const [zoom, setZoom] = useState(1);

  // Layout calculation
  const layout = useMemo(() => {
    return layoutFlowGraph({
      start: doc.start,
      steps: doc.steps,
      terminals: doc.terminals,
    });
  }, [doc.start, doc.steps, doc.terminals]);

  // Map loop info by header
  const loopMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const loop of doc.loops ?? []) {
      const maxText = typeof loop.max === "number" ? loop.max : (loop.max?.cap ?? "?");
      m.set(loop.header, `↺ ${maxText}`);
    }
    return m;
  }, [doc.loops]);

  // Filtered steps
  const displaySteps = useMemo(() => {
    let steps = doc.steps;
    if (showLoopsOnly) {
      const loopHeaders = new Set((doc.loops ?? []).map((l) => l.header));
      steps = steps.filter((s) => loopHeaders.has(s.key) || s.edges.some((e) => Boolean(e.loop)));
    }
    if (searchTerm.trim()) {
      const q = searchTerm.trim().toLowerCase();
      steps = steps.filter((s) => s.key.toLowerCase().includes(q) || s.title.toLowerCase().includes(q));
    }
    return steps;
  }, [doc.steps, doc.loops, showLoopsOnly, searchTerm]);

  return (
    <div className="flex-1 flex flex-col h-full bg-muted/10 relative overflow-hidden">
      {/* Graph Toolbar */}
      <div className="flex items-center justify-between gap-2 p-2 border-b border-border bg-card z-10 flex-wrap">
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="size-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder={t(($) => $.flow_designer.graph.find)}
              className="h-7 pl-7 text-caption w-40"
            />
          </div>

          <Button
            type="button"
            variant={showLoopsOnly ? "secondary" : "outline"}
            size="xs"
            onClick={() => setShowLoopsOnly(!showLoopsOnly)}
            className="h-7 text-caption gap-1"
          >
            <RotateCcw className="size-3" aria-hidden />
            <span>Show loops only</span>
          </Button>
        </div>

        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={() => setZoom(1)}
            className="h-7 text-caption gap-1"
          >
            <Maximize2 className="size-3" aria-hidden />
            <span>Fit</span>
          </Button>
        </div>
      </div>

      {/* Mobile Vertical List (< 640px) */}
      <div className="block sm:hidden flex-1 overflow-y-auto p-4 space-y-2">
        <div className="text-caption font-semibold text-muted-foreground mb-2">
          Workflow Steps (Vertical Flow)
        </div>
        {displaySteps.map((step, idx) => {
          const isSelected = step.key === selectedStepKey;
          const loopInfo = loopMap.get(step.key);

          return (
            <div
              key={step.key}
              onClick={() => onSelectStep(step.key)}
              className={cn(
                "p-3 rounded-lg border transition-colors cursor-pointer space-y-1.5",
                isSelected
                  ? "bg-accent border-accent-foreground/20 text-accent-foreground"
                  : "bg-card border-border hover:bg-muted/50",
              )}
            >
              <div className="flex items-center justify-between">
                <span className="font-semibold text-caption">
                  #{idx + 1} {step.title || step.key}
                </span>
                <Badge variant="secondary" className="text-micro font-normal">
                  {step.kind}
                </Badge>
              </div>

              <div className="flex items-center gap-2 text-micro text-muted-foreground">
                <span>{step.tasks?.length ?? 0} tasks</span>
                <span>·</span>
                <span>{step.edges?.length ?? 0} exits</span>
                {loopInfo && (
                  <Badge variant="outline" className="text-micro text-blue-600 border-blue-500/30">
                    {loopInfo}
                  </Badge>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Desktop / Tablet Canvas (>= 640px) */}
      <div className="hidden sm:block flex-1 overflow-auto p-8 relative">
        <div
          className="relative min-w-[800px] min-h-[600px] transition-transform"
          style={{ transform: `scale(${zoom})`, transformOrigin: "top left" }}
        >
          {layout.nodes.map((node) => {
            const step = doc.steps.find((s) => s.key === node.id);
            if (!step) return null;
            const isSelected = step.key === selectedStepKey;
            const loopInfo = loopMap.get(step.key);
            const stepHasIssues = issues.some(
              (i) => i.path?.includes(step.key) || i.params?.step === step.key,
            );

            return (
              <div
                key={node.id}
                onClick={() => onSelectStep(step.key)}
                className={cn(
                  "absolute w-64 p-3 rounded-lg border shadow-xs transition-all cursor-pointer flex flex-col gap-1.5",
                  isSelected
                    ? "bg-accent border-primary text-accent-foreground ring-2 ring-primary/20"
                    : "bg-card border-border hover:border-foreground/40",
                )}
                style={{
                  left: node.position.x,
                  top: node.position.y,
                }}
              >
                <div className="flex items-center justify-between gap-1">
                  <span className="text-caption font-semibold truncate flex-1">
                    {step.title || step.key}
                  </span>
                  <Badge variant="secondary" className="text-micro font-normal">
                    {step.kind}
                  </Badge>
                </div>

                <div className="flex items-center justify-between text-micro text-muted-foreground">
                  <span>{step.tasks?.length ?? 0} tasks</span>
                  {loopInfo && (
                    <Badge variant="outline" className="text-micro text-blue-600 border-blue-500/30">
                      {loopInfo}
                    </Badge>
                  )}
                  {stepHasIssues && (
                    <AlertTriangle className="size-3 text-amber-500" aria-hidden />
                  )}
                </div>

                {/* Edges Summary */}
                <div className="pt-1 border-t border-border/50 flex flex-wrap gap-1 text-micro">
                  {step.edges.slice(0, 3).map((edge, eIdx) => (
                    <span
                      key={eIdx}
                      className="inline-flex items-center gap-0.5 px-1 py-0.5 rounded bg-muted/40 font-mono text-[10px]"
                    >
                      {edge.label || edge.to || edge.stop}
                    </span>
                  ))}
                  {step.edges.length > 3 && (
                    <span className="text-muted-foreground text-[10px]">
                      +{step.edges.length - 3}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
