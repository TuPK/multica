"use client";

import { useState } from "react";
import type { FlowDoc, FlowDocPolicy, FlowDocTerminal } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AlertTriangle, Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface TerminalsPanelProps {
  doc: FlowDoc;
  onChangeTerminals: (terminals: FlowDocTerminal[]) => void;
  onChangePolicy?: (policy: FlowDocPolicy) => void;
  isAdmin?: boolean;
}

export function TerminalsPanel({
  doc,
  onChangeTerminals,
  onChangePolicy,
  isAdmin = false,
}: TerminalsPanelProps) {
  const { t } = useT("squads");
  const [newKey, setNewKey] = useState("");
  const [newStatus, setNewStatus] = useState("done");
  const [leavesWorkspace, setLeavesWorkspace] = useState(false);

  const hasLeavesWorkspace = (doc.terminals ?? []).some((term) => term.leaves_workspace);

  const handleAddTerminal = () => {
    const key = newKey.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    if (!key || doc.terminals?.some((t) => t.key === key)) return;

    const newTerm: FlowDocTerminal = {
      key,
      status: newStatus,
      leaves_workspace: leavesWorkspace,
    };

    onChangeTerminals([...(doc.terminals ?? []), newTerm]);
    setNewKey("");
    setLeavesWorkspace(false);
  };

  const handleDeleteTerminal = (key: string) => {
    if ((doc.terminals?.length ?? 0) <= 1) return; // Must have at least 1 terminal
    onChangeTerminals((doc.terminals ?? []).filter((t) => t.key !== key));
  };

  const handleToggleAck = (checked: boolean) => {
    if (!isAdmin || !onChangePolicy) return;
    const policy = doc.policy ?? {
      task_idle_minutes: 120,
      task_hard_cap_minutes: 240,
      task_max_attempts: 2,
      human_remind_minutes: 1440,
      human_stall_minutes: 10080,
      max_activations: 100,
      max_task_issues: 300,
    };
    onChangePolicy({
      ...policy,
      allow_unattended_release: checked,
    });
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-body font-semibold">{t(($) => $.flow_designer.editor.tab_terminals)}</h3>
        </div>
      </div>

      {/* Release warning if any terminal leaves the workspace */}
      {hasLeavesWorkspace && (
        <div className="p-3 rounded-lg border border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400 space-y-2 text-caption">
          <div className="flex items-center gap-2 font-medium">
            <AlertTriangle className="size-4 shrink-0 text-amber-600 dark:text-amber-400" aria-hidden />
            <span>{t(($) => $.flow_designer.release.warning)}</span>
          </div>
          <div className="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="ack_no_human_exit"
              disabled={!isAdmin}
              checked={doc.policy?.allow_unattended_release ?? false}
              onChange={(e) => handleToggleAck(e.target.checked)}
              className="size-4 rounded border-amber-500 text-amber-600 focus:ring-amber-500"
            />
            <label
              htmlFor="ack_no_human_exit"
              className={isAdmin ? "cursor-pointer select-none" : "cursor-not-allowed select-none opacity-70"}
            >
              {t(($) => $.flow_designer.ack.no_human_exit)}
              {!isAdmin && <span className="ml-1 text-micro text-muted-foreground">({t(($) => $.flow_designer.publish.admin_only)})</span>}
            </label>
          </div>
        </div>
      )}

      {/* Terminals table */}
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.key)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.terminals.col_status)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.terminals.col_scope)}</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {(doc.terminals ?? []).map((term) => (
              <tr key={term.key} className="hover:bg-muted/30">
                <td className="p-2 font-mono text-micro text-foreground">{term.key}</td>
                <td className="p-2">
                  <Badge variant="secondary" className="text-micro font-normal">
                    {term.status}
                  </Badge>
                </td>
                <td className="p-2">
                  {term.leaves_workspace ? (
                    <Badge variant="outline" className="text-micro text-amber-600 border-amber-500/30 font-normal">
                      {t(($) => $.flow_designer.terminal.leaves)}
                    </Badge>
                  ) : (
                    <span className="text-micro text-muted-foreground">{t(($) => $.flow_designer.terminals.internal)}</span>
                  )}
                </td>
                <td className="p-2 text-right">
                  {(doc.terminals?.length ?? 0) > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      aria-label={t(($) => $.flow_designer.terminals.delete, { key: term.key })}
                      onClick={() => handleDeleteTerminal(term.key)}
                      className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-3.5" aria-hidden />
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Add Terminal */}
      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          // A key is a code token: the same in every language.
          // eslint-disable-next-line no-restricted-syntax
          placeholder="terminal_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-32"
        />
        <Input
          placeholder={t(($) => $.flow_designer.terminals.status_placeholder)}
          value={newStatus}
          onChange={(e) => setNewStatus(e.target.value)}
          className="h-8 text-caption w-32"
        />
        <label className="flex items-center gap-1.5 text-caption cursor-pointer">
          <input
            type="checkbox"
            checked={leavesWorkspace}
            onChange={(e) => setLeavesWorkspace(e.target.checked)}
            className="size-3.5 rounded border-input"
          />
          <span>{t(($) => $.flow_designer.terminal.leaves)}</span>
        </label>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!newKey.trim()}
          onClick={handleAddTerminal}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>{t(($) => $.flow_designer.terminals.add)}</span>
        </Button>
      </div>
    </div>
  );
}
