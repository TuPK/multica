/* eslint-disable i18next/no-literal-string, no-restricted-syntax */
import { useMemo, useState } from "react";
import type { FlowDoc, FlowDocLoop, FlowDocVar } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Plus, RotateCcw, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface LoopsPanelProps {
  doc: FlowDoc;
  onChangeLoops: (loops: FlowDocLoop[]) => void;
  loopLimit?: number;
}

export function LoopsPanel({
  doc,
  onChangeLoops,
  loopLimit = 8,
}: LoopsPanelProps) {
  const { t } = useT("squads");
  const [newKey, setNewKey] = useState("");
  const [newHeader, setNewHeader] = useState("");
  const [maxMode, setMaxMode] = useState<"fixed" | "var">("fixed");
  const [fixedMax, setFixedMax] = useState(3);
  const [varKey, setVarKey] = useState("");
  const [varCap, setVarCap] = useState(5);

  const atLimit = (doc.loops?.length ?? 0) >= loopLimit;

  // Variables valid for loop max: type === "int" and has a defined max property
  const eligibleIntVars = useMemo(() => {
    return (doc.vars ?? []).filter(
      (v): v is FlowDocVar & { max: number } => v.type === "int" && typeof v.max === "number",
    );
  }, [doc.vars]);

  const handleAddLoop = () => {
    if (atLimit) return;
    const key = newKey.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    const header = newHeader || doc.steps[0]?.key || "";
    if (!key || !header || doc.loops?.some((l) => l.key === key)) return;

    let max: number | { var: string; cap: number } = fixedMax;
    if (maxMode === "var") {
      if (!varKey) return;
      max = { var: varKey, cap: varCap };
    }

    const newLoop: FlowDocLoop = {
      key,
      header,
      max,
      reset: "on_entry",
    };

    onChangeLoops([...(doc.loops ?? []), newLoop]);
    setNewKey("");
  };

  const handleDeleteLoop = (key: string) => {
    onChangeLoops((doc.loops ?? []).filter((l) => l.key !== key));
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <h3 className="text-body font-semibold">{t(($) => $.flow_designer.loop.title)}</h3>
        <span className="text-caption text-muted-foreground tabular-nums">
          {doc.loops?.length ?? 0} / {loopLimit}
        </span>
      </div>

      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">Key</th>
              <th className="p-2 font-medium">Header Step</th>
              <th className="p-2 font-medium">Max Limit</th>
              <th className="p-2 font-medium">Reset</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {(doc.loops ?? []).map((loop) => {
              const maxDesc =
                typeof loop.max === "number"
                  ? t(($) => $.flow_designer.loop.max, { count: loop.max })
                  : `var(${loop.max.var}, cap ${loop.max.cap})`;

              return (
                <tr key={loop.key} className="hover:bg-muted/30">
                  <td className="p-2 font-mono text-micro text-foreground flex items-center gap-1.5">
                    <RotateCcw className="size-3 text-blue-500" aria-hidden />
                    <span>{loop.key}</span>
                  </td>
                  <td className="p-2 font-mono text-micro">{loop.header}</td>
                  <td className="p-2">
                    <Badge variant="outline" className="text-micro font-normal">
                      {maxDesc}
                    </Badge>
                  </td>
                  <td className="p-2 text-micro text-muted-foreground">{loop.reset}</td>
                  <td className="p-2 text-right">
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      aria-label={`Delete ${loop.key}`}
                      onClick={() => handleDeleteLoop(loop.key)}
                      className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-3.5" aria-hidden />
                    </Button>
                  </td>
                </tr>
              );
            })}
            {(doc.loops ?? []).length === 0 && (
              <tr>
                <td colSpan={5} className="p-4 text-center text-muted-foreground">
                  No loops declared
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Add Loop */}
      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          placeholder="loop_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-32"
          disabled={atLimit}
        />
        <select
          value={newHeader}
          onChange={(e) => setNewHeader(e.target.value)}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
          disabled={atLimit}
        >
          <option value="">Header step...</option>
          {doc.steps.map((s) => (
            <option key={s.key} value={s.key}>
              {s.title || s.key}
            </option>
          ))}
        </select>
        <select
          value={maxMode}
          onChange={(e) => setMaxMode(e.target.value as "fixed" | "var")}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
          disabled={atLimit}
        >
          <option value="fixed">Fixed count</option>
          <option value="var">From variable</option>
        </select>

        {maxMode === "fixed" ? (
          <Input
            type="number"
            min={1}
            max={20}
            value={fixedMax}
            onChange={(e) => setFixedMax(Number(e.target.value))}
            className="h-8 text-caption w-20"
            disabled={atLimit}
          />
        ) : (
          <div className="flex items-center gap-1.5">
            <select
              value={varKey}
              onChange={(e) => setVarKey(e.target.value)}
              className="h-8 rounded-md border border-input bg-background px-2 text-caption"
              disabled={atLimit}
              aria-label="Eligible integer variable"
            >
              <option value="">Select variable...</option>
              {eligibleIntVars.map((v) => (
                <option key={v.key} value={v.key}>
                  {v.key} (max: {v.max})
                </option>
              ))}
            </select>
            <Input
              type="number"
              min={1}
              max={20}
              placeholder="Cap"
              value={varCap}
              onChange={(e) => setVarCap(Number(e.target.value))}
              className="h-8 text-caption w-16"
              disabled={atLimit}
            />
          </div>
        )}

        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={atLimit || !newKey.trim() || !newHeader || (maxMode === "var" && !varKey)}
          onClick={handleAddLoop}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>Add loop</span>
        </Button>
      </div>
    </div>
  );
}
