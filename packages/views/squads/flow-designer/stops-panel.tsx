/* eslint-disable i18next/no-literal-string, no-restricted-syntax */
import { useState } from "react";
import type { FlowDoc, FlowDocStopKind } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AlertCircle, Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface StopsPanelProps {
  doc: FlowDoc;
  onChangeStops: (stops: FlowDocStopKind[]) => void;
}

export function StopsPanel({ doc, onChangeStops }: StopsPanelProps) {
  const { t } = useT("squads");
  const [newCode, setNewCode] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [newClass, setNewClass] = useState<FlowDocStopKind["class"]>("custom");

  const handleAddStop = () => {
    const code = newCode.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    const label = newLabel.trim() || code;
    if (!code || doc.stops?.some((s) => s.code === code)) return;

    const newStop: FlowDocStopKind = {
      code,
      class: newClass,
      label,
      message: `${code}_message`,
      resume_by: "managers",
    };

    onChangeStops([...(doc.stops ?? []), newStop]);
    setNewCode("");
    setNewLabel("");
  };

  const handleDeleteStop = (code: string) => {
    onChangeStops((doc.stops ?? []).filter((s) => s.code !== code));
  };

  const getStopLabel = (stop: FlowDocStopKind) => {
    if (stop.legacy_no === 1) return t(($) => $.flow_designer.stop.stop1);
    if (stop.legacy_no === 2) return t(($) => $.flow_designer.stop.stop2);
    if (stop.legacy_no === 3) return t(($) => $.flow_designer.stop.stop3);
    return stop.label;
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-body font-semibold">{t(($) => $.flow_designer.stops.declared)}</h3>
          <p className="text-caption text-muted-foreground">
            Declared stop reasons when routing cannot proceed or reaches a gate block.
          </p>
        </div>
      </div>

      {/* Built-in stalled */}
      <div className="p-2.5 rounded-lg border border-border bg-muted/20 flex items-center justify-between text-caption">
        <div className="flex items-center gap-2">
          <AlertCircle className="size-4 text-amber-500" aria-hidden />
          <span className="font-mono text-micro font-medium">stalled</span>
          <span className="text-muted-foreground">·</span>
          <span>{t(($) => $.flow_designer.stop.stalled)}</span>
        </div>
        <Badge variant="secondary" className="text-micro font-normal">Built-in</Badge>
      </div>

      {/* Declared stops */}
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">Code</th>
              <th className="p-2 font-medium">Class</th>
              <th className="p-2 font-medium">Label</th>
              <th className="p-2 font-medium">Message Key</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {(doc.stops ?? []).map((stop) => (
              <tr key={stop.code} className="hover:bg-muted/30">
                <td className="p-2 font-mono text-micro text-foreground">{stop.code}</td>
                <td className="p-2">
                  <Badge variant="secondary" className="text-micro font-normal">
                    {stop.class}
                  </Badge>
                </td>
                <td className="p-2">{getStopLabel(stop)}</td>
                <td className="p-2 font-mono text-micro text-muted-foreground">
                  {stop.message}
                </td>
                <td className="p-2 text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    aria-label={`Delete ${stop.code}`}
                    onClick={() => handleDeleteStop(stop.code)}
                    className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 className="size-3.5" aria-hidden />
                  </Button>
                </td>
              </tr>
            ))}
            {(doc.stops ?? []).length === 0 && (
              <tr>
                <td colSpan={5} className="p-4 text-center text-muted-foreground">
                  No declared stops
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Add Stop */}
      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          placeholder="stop_code"
          value={newCode}
          onChange={(e) => setNewCode(e.target.value)}
          className="h-8 text-caption w-32"
        />
        <Input
          placeholder="Label"
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          className="h-8 text-caption w-40"
        />
        <select
          value={newClass}
          onChange={(e) => setNewClass(e.target.value as FlowDocStopKind["class"])}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
        >
          <option value="custom">custom</option>
          <option value="missing_grounds">missing_grounds</option>
          <option value="loop_exhausted">loop_exhausted</option>
        </select>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!newCode.trim()}
          onClick={handleAddStop}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>Add stop</span>
        </Button>
      </div>
    </div>
  );
}
