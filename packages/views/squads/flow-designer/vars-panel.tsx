/* eslint-disable i18next/no-literal-string, no-restricted-syntax */
import { useState } from "react";
import type { FlowDoc, FlowDocVar } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface VarsPanelProps {
  doc: FlowDoc;
  onChangeVars: (vars: FlowDocVar[]) => void;
  varLimit?: number;
}

export function VarsPanel({
  doc,
  onChangeVars,
  varLimit = 16,
}: VarsPanelProps) {
  const { t } = useT("squads");
  const [newKey, setNewKey] = useState("");
  const [newType, setNewType] = useState<FlowDocVar["type"]>("string");
  const [newDefault, setNewDefault] = useState("");

  const atLimit = (doc.vars?.length ?? 0) >= varLimit;

  const handleAddVar = () => {
    if (atLimit) return;
    const key = newKey.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    if (!key || doc.vars?.some((v) => v.key === key)) return;

    let defVal: unknown = newDefault;
    if (newType === "int") defVal = Number(newDefault) || 0;
    else if (newType === "bool") defVal = newDefault === "true";
    else if (newType === "list_string") defVal = newDefault ? newDefault.split(",").map((s) => s.trim()) : [];

    const newVar: FlowDocVar = {
      key,
      type: newType,
      default: defVal,
      set_by: ["lead", "managers"],
      set_at: ["*"],
    };

    onChangeVars([...(doc.vars ?? []), newVar]);
    setNewKey("");
    setNewDefault("");
  };

  const handleDeleteVar = (key: string) => {
    onChangeVars((doc.vars ?? []).filter((v) => v.key !== key));
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <h3 className="text-body font-semibold">{t(($) => $.flow_designer.vars.title)}</h3>
        <span className="text-caption text-muted-foreground tabular-nums">
          {doc.vars?.length ?? 0} / {varLimit}
        </span>
      </div>

      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">Key</th>
              <th className="p-2 font-medium">Type</th>
              <th className="p-2 font-medium">Default</th>
              <th className="p-2 font-medium">Set By</th>
              <th className="p-2 font-medium">Set At</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {(doc.vars ?? []).map((v) => (
              <tr key={v.key} className="hover:bg-muted/30">
                <td className="p-2 font-mono text-micro text-foreground">{v.key}</td>
                <td className="p-2">
                  <Badge variant="secondary" className="text-micro font-normal">
                    {v.type}
                  </Badge>
                </td>
                <td className="p-2 font-mono text-micro text-muted-foreground">
                  {JSON.stringify(v.default)}
                </td>
                <td className="p-2 text-micro text-muted-foreground">
                  {v.set_by?.join(", ") || "—"}
                </td>
                <td className="p-2 text-micro text-muted-foreground">
                  {v.set_at?.join(", ") || "—"}
                </td>
                <td className="p-2 text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    aria-label={`Delete ${v.key}`}
                    onClick={() => handleDeleteVar(v.key)}
                    className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 className="size-3.5" aria-hidden />
                  </Button>
                </td>
              </tr>
            ))}
            {(doc.vars ?? []).length === 0 && (
              <tr>
                <td colSpan={6} className="p-4 text-center text-muted-foreground">
                  No variables declared
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          placeholder="var_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-32"
          disabled={atLimit}
        />
        <select
          value={newType}
          onChange={(e) => setNewType(e.target.value as FlowDocVar["type"])}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
          disabled={atLimit}
        >
          <option value="string">string</option>
          <option value="int">int</option>
          <option value="bool">bool</option>
          <option value="enum">enum</option>
          <option value="list_string">list_string</option>
        </select>
        <Input
          placeholder="Default value"
          value={newDefault}
          onChange={(e) => setNewDefault(e.target.value)}
          className="h-8 text-caption w-36"
          disabled={atLimit}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={atLimit || !newKey.trim()}
          onClick={handleAddVar}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>Add variable</span>
        </Button>
        {atLimit && (
          <span className="text-micro text-muted-foreground">
            Variable limit of {varLimit} reached
          </span>
        )}
      </div>
    </div>
  );
}
