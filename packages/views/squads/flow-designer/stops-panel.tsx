"use client";

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
  /** Saves the texts of the document: a stop names the text a request is told when it stops there. */
  onChangeMessages?: (messages: Record<string, string>) => void;
}

export function StopsPanel({ doc, onChangeStops, onChangeMessages }: StopsPanelProps) {
  const { t } = useT("squads");
  const [newCode, setNewCode] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [newClass, setNewClass] = useState<FlowDocStopKind["class"]>("custom");

  const handleAddStop = () => {
    const code = newCode.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    const label = newLabel.trim() || code;
    if (!code || doc.stops?.some((s) => s.code === code)) return;

    const messageKey = `${code}_message`;
    const newStop: FlowDocStopKind = {
      code,
      class: newClass,
      label,
      message: messageKey,
      resume_by: "managers",
    };

    onChangeStops([...(doc.stops ?? []), newStop]);
    // A stop whose text is missing does not publish: it starts with a text the person can edit.
    if (!doc.messages?.[messageKey]) {
      onChangeMessages?.({ ...doc.messages, [messageKey]: t(($) => $.flow_designer.stops.default_message, { label }) });
    }
    setNewCode("");
    setNewLabel("");
  };

  const handleDeleteStop = (code: string) => {
    onChangeStops((doc.stops ?? []).filter((s) => s.code !== code));
  };

  // The classes the server lists; one this build does not know is shown as it is sent.
  const stopClassLabel = (cls: string) => {
    switch (cls) {
      case "custom":
        return t(($) => $.flow_designer.stops.class.custom);
      case "missing_grounds":
        return t(($) => $.flow_designer.stops.class.missing_grounds);
      case "loop_exhausted":
        return t(($) => $.flow_designer.stops.class.loop_exhausted);
      default:
        return cls;
    }
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
        </div>
      </div>

      {/* Built-in stalled */}
      <div className="p-2.5 rounded-lg border border-border bg-muted/20 flex items-center justify-between text-caption">
        <div className="flex items-center gap-2">
          <AlertCircle className="size-4 text-amber-500" aria-hidden />
          {/* The code of the built-in stop is a token: the same in every language. */}
          {/* eslint-disable-next-line i18next/no-literal-string */}
          <span className="font-mono text-micro font-medium">stalled</span>
          <span className="text-muted-foreground">·</span>
          <span>{t(($) => $.flow_designer.stop.stalled)}</span>
        </div>
        <Badge variant="secondary" className="text-micro font-normal">{t(($) => $.flow_designer.stops.built_in)}</Badge>
      </div>

      {/* Declared stops */}
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.code)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.class)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.label)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.stops.message)}</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {(doc.stops ?? []).map((stop) => (
              <tr key={stop.code} className="hover:bg-muted/30">
                <td className="p-2 font-mono text-micro text-foreground">{stop.code}</td>
                <td className="p-2">
                  <Badge variant="secondary" className="text-micro font-normal">
                    {stopClassLabel(stop.class)}
                  </Badge>
                </td>
                <td className="p-2">{getStopLabel(stop)}</td>
                <td className="p-2">
                  <textarea
                    aria-label={`${t(($) => $.flow_designer.stops.message)}: ${stop.code}`}
                    value={doc.messages?.[stop.message ?? ""] ?? ""}
                    disabled={!stop.message || !onChangeMessages}
                    onChange={(e) => stop.message && onChangeMessages?.({ ...doc.messages, [stop.message]: e.target.value })}
                    rows={3}
                    className="w-full min-w-48 rounded-md border border-input bg-background px-2 py-1 font-mono text-micro"
                  />
                </td>
                <td className="p-2 text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    aria-label={t(($) => $.flow_designer.stops.delete, { code: stop.code })}
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
                  {t(($) => $.flow_designer.stops.empty)}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Add Stop */}
      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          // A code is a token: the same in every language.
          // eslint-disable-next-line no-restricted-syntax
          placeholder="stop_code"
          value={newCode}
          onChange={(e) => setNewCode(e.target.value)}
          className="h-8 text-caption w-32"
        />
        <Input
          placeholder={t(($) => $.flow_designer.common.label)}
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          className="h-8 text-caption w-40"
        />
        <select
          value={newClass}
          onChange={(e) => setNewClass(e.target.value as FlowDocStopKind["class"])}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
        >
          <option value="custom">{stopClassLabel("custom")}</option>
          <option value="missing_grounds">{stopClassLabel("missing_grounds")}</option>
          <option value="loop_exhausted">{stopClassLabel("loop_exhausted")}</option>
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
          <span>{t(($) => $.flow_designer.stops.add)}</span>
        </Button>
      </div>
    </div>
  );
}
