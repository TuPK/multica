"use client";

import { useMemo } from "react";
import type { FlowDoc, FlowDocEdge, FlowDocLoop } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { ArrowDown, ArrowUp, Plus, RotateCcw, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface EdgeListProps {
  edges: FlowDocEdge[];
  onChangeEdges: (edges: FlowDocEdge[]) => void;
  doc: FlowDoc;
  currentStepKey: string;
}

export function EdgeList({
  edges,
  onChangeEdges,
  doc,
  currentStepKey: _currentStepKey,
}: EdgeListProps) {
  const { t } = useT("squads");

  // Step keys and terminal keys for "to" picker
  const targetOptions = useMemo(() => {
    const steps = doc.steps.map((s) => ({ value: s.key, label: t(($) => $.flow_designer.edges.step_option, { name: s.title || s.key }), kind: "step" as const }));
    const terminals = (doc.terminals ?? []).map((term) => ({
      value: term.key,
      label: t(($) => $.flow_designer.edges.terminal_option, { key: term.key }),
      kind: "terminal" as const,
    }));
    return [...steps, ...terminals];
  }, [doc.steps, doc.terminals, t]);

  const handleMove = (index: number, direction: "up" | "down") => {
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= edges.length) return;
    const next = [...edges];
    const temp = next[index]!;
    next[index] = next[targetIndex]!;
    next[targetIndex] = temp;
    onChangeEdges(next);
  };

  const handleDelete = (index: number) => {
    onChangeEdges(edges.filter((_, i) => i !== index));
  };

  const handleAddEdge = () => {
    const id = `edge_${Date.now()}`;
    const newEdge: FlowDocEdge = {
      id,
      label: t(($) => $.flow_designer.edges.default_label),
      to: doc.steps[0]?.key,
      trigger: "auto",
    };
    onChangeEdges([...edges, newEdge]);
  };

  const handleUpdateEdge = (index: number, patch: Partial<FlowDocEdge>) => {
    const next = [...edges];
    const updated = { ...next[index]!, ...patch };
    // If loop is set, target must be loop.header
    if (patch.loop) {
      const loop = doc.loops?.find((l) => l.key === patch.loop);
      if (loop) {
        updated.to = loop.header;
        delete updated.stop;
      }
    }
    next[index] = updated;
    onChangeEdges(next);
  };

  // Only loops whose header is eligible
  const loopsForTarget = (toKey?: string): FlowDocLoop[] => {
    if (!toKey) return doc.loops ?? [];
    return (doc.loops ?? []).filter((l) => l.header === toKey);
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-caption font-semibold">{t(($) => $.flow_designer.edges.title)}</span>
        <Button
          type="button"
          variant="outline"
          size="xs"
          onClick={handleAddEdge}
          className="h-7 gap-1 text-caption"
        >
          <Plus className="size-3" aria-hidden />
          <span>{t(($) => $.flow_designer.edges.add)}</span>
        </Button>
      </div>

      <div className="space-y-2">
        {edges.map((edge, index) => {
          const isStop = Boolean(edge.stop);
          const isLoop = Boolean(edge.loop);
          const loops = loopsForTarget(edge.to);

          return (
            <div
              key={edge.id || index}
              className="p-2.5 rounded-lg border border-border bg-card space-y-2 text-caption"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-1.5 flex-1">
                  <span className="text-micro font-mono text-muted-foreground tabular-nums">
                    #{index + 1}
                  </span>
                  <Input
                    value={edge.label}
                    onChange={(e) => handleUpdateEdge(index, { label: e.target.value })}
                    placeholder={t(($) => $.flow_designer.edges.label_placeholder)}
                    className="h-7 text-caption font-medium max-w-44"
                  />
                  {edge.trigger === "manual" && (
                    <Badge variant="outline" className="text-micro font-normal">{t(($) => $.flow_designer.edges.manual)}</Badge>
                  )}
                  {isLoop && (
                    <Badge variant="outline" className="text-micro text-blue-600 border-blue-500/30 gap-1 font-normal">
                      <RotateCcw className="size-2.5" aria-hidden />
                      <span>{edge.loop}</span>
                    </Badge>
                  )}
                </div>

                <div className="flex items-center gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    disabled={index === 0}
                    aria-label={t(($) => $.flow_designer.edges.move_up, { n: index + 1 })}
                    onClick={() => handleMove(index, "up")}
                    className="h-6 w-6 p-0"
                  >
                    <ArrowUp className="size-3" aria-hidden />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    disabled={index === edges.length - 1}
                    aria-label={t(($) => $.flow_designer.edges.move_down, { n: index + 1 })}
                    onClick={() => handleMove(index, "down")}
                    className="h-6 w-6 p-0"
                  >
                    <ArrowDown className="size-3" aria-hidden />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    aria-label={t(($) => $.flow_designer.edges.delete, { n: index + 1 })}
                    onClick={() => handleDelete(index)}
                    className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 className="size-3" aria-hidden />
                  </Button>
                </div>
              </div>

              {/* Destination picker: Step/Terminal vs Stop */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-1">
                <div>
                  <label className="text-micro text-muted-foreground block mb-0.5">{t(($) => $.flow_designer.edges.destination)}</label>
                  <select
                    value={isStop ? `stop:${edge.stop}` : `to:${edge.to}`}
                    onChange={(e) => {
                      const val = e.target.value;
                      if (val.startsWith("stop:")) {
                        handleUpdateEdge(index, { stop: val.slice(5), to: undefined, loop: undefined });
                      } else {
                        handleUpdateEdge(index, { to: val.slice(3), stop: undefined });
                      }
                    }}
                    className="w-full h-7 rounded border border-input bg-background px-2 text-micro"
                  >
                    <optgroup label={t(($) => $.flow_designer.edges.group_steps)}>
                      {targetOptions.map((opt) => (
                        <option key={`to:${opt.value}`} value={`to:${opt.value}`}>
                          {opt.label}
                        </option>
                      ))}
                    </optgroup>
                    <optgroup label={t(($) => $.flow_designer.edges.group_stops)}>
                      <option value="stop:stalled">{t(($) => $.flow_designer.edges.stop_stalled)}</option>
                      {(doc.stops ?? []).map((s) => (
                        <option key={`stop:${s.code}`} value={`stop:${s.code}`}>
                          {t(($) => $.flow_designer.edges.stop_option, { name: s.label || s.code })}
                        </option>
                      ))}
                    </optgroup>
                  </select>
                </div>

                {!isStop && (
                  <div>
                    <label className="text-micro text-muted-foreground block mb-0.5">{t(($) => $.flow_designer.edges.loop)}</label>
                    <select
                      value={edge.loop || ""}
                      onChange={(e) => handleUpdateEdge(index, { loop: e.target.value || undefined })}
                      className="w-full h-7 rounded border border-input bg-background px-2 text-micro"
                    >
                      <option value="">{t(($) => $.flow_designer.edges.no_loop)}</option>
                      {loops.map((l) => (
                        <option key={l.key} value={l.key}>
                          {t(($) => $.flow_designer.edges.loop_option, { key: l.key, header: l.header })}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            </div>
          );
        })}

        {edges.length === 0 && (
          <div className="p-3 text-center text-muted-foreground text-caption rounded-lg border border-dashed border-border">
            {t(($) => $.flow_designer.edges.empty)}
          </div>
        )}
      </div>
    </div>
  );
}
