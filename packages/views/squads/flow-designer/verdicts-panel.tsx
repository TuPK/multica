"use client";

import { useState } from "react";
import type { FlowDoc, FlowDocVerdictSet } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface VerdictsPanelProps {
  doc: FlowDoc;
  onChangeVerdicts: (verdicts: FlowDocVerdictSet[]) => void;
  verdictLimit?: number;
}

export function VerdictsPanel({
  doc,
  onChangeVerdicts,
  verdictLimit = 8,
}: VerdictsPanelProps) {
  const { t } = useT("squads");
  const [newKey, setNewKey] = useState("");

  const atLimit = (doc.verdicts?.length ?? 0) >= verdictLimit;

  const handleAddVerdictSet = () => {
    if (atLimit) return;
    const key = newKey.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    if (!key || doc.verdicts?.some((v) => v.key === key)) return;

    const newSet: FlowDocVerdictSet = {
      key,
      verdicts: [
        { key: "pass", class: "pass", label: t(($) => $.flow_designer.verdicts.pass_label) },
        { key: "fail", class: "fail", label: t(($) => $.flow_designer.verdicts.fail_label) },
      ],
    };

    onChangeVerdicts([...(doc.verdicts ?? []), newSet]);
    setNewKey("");
  };

  // A built-in verdict and how it counts, as the engine names them.
  const verdictItem = (key: string, cls: "pass" | "fail") =>
    t(($) => $.flow_designer.verdicts.item, {
      key,
      class: cls === "pass" ? t(($) => $.flow_designer.verdicts.class.pass) : t(($) => $.flow_designer.verdicts.class.fail),
    });

  const handleDeleteVerdictSet = (key: string) => {
    onChangeVerdicts((doc.verdicts ?? []).filter((v) => v.key !== key));
  };

  const getClassBadge = (cls: string) => {
    switch (cls) {
      case "pass":
        return (
          <Badge variant="secondary" className="text-micro text-emerald-600 bg-emerald-500/10 border-emerald-500/20 font-normal">
            {t(($) => $.flow_designer.verdict.pass)}
          </Badge>
        );
      case "fail":
        return (
          <Badge variant="destructive" className="text-micro font-normal">
            {t(($) => $.flow_designer.verdict.fail)}
          </Badge>
        );
      case "neutral":
        return (
          <Badge variant="outline" className="text-micro font-normal">
            {t(($) => $.flow_designer.verdict.neutral)}
          </Badge>
        );
      default:
        return (
          <Badge variant="outline" className="text-micro font-normal">
            {cls}
          </Badge>
        );
    }
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <h3 className="text-body font-semibold">{t(($) => $.flow_designer.verdicts.title)}</h3>
        <span className="text-caption text-muted-foreground tabular-nums">
          {doc.verdicts?.length ?? 0} / {verdictLimit}
        </span>
      </div>

      {/* Built-in sets */}
      <div className="space-y-2">
        <span className="text-caption font-medium text-muted-foreground">{t(($) => $.flow_designer.verdicts.built_in)}</span>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
          <div className="p-2.5 rounded-lg border border-border bg-muted/20 space-y-1 text-caption">
            {/* The names of the built-in sets are code tokens: the same in every language. */}
            {/* eslint-disable-next-line i18next/no-literal-string */}
            <span className="font-mono text-micro font-medium">work</span>
            <div className="flex gap-1 flex-wrap">
              <Badge variant="secondary" className="text-micro">{verdictItem("done", "pass")}</Badge>
            </div>
          </div>
          <div className="p-2.5 rounded-lg border border-border bg-muted/20 space-y-1 text-caption">
            {/* eslint-disable-next-line i18next/no-literal-string */}
            <span className="font-mono text-micro font-medium">gate</span>
            <div className="flex gap-1 flex-wrap">
              <Badge variant="secondary" className="text-micro">{verdictItem("approved", "pass")}</Badge>
              <Badge variant="destructive" className="text-micro">{verdictItem("rejected", "fail")}</Badge>
            </div>
          </div>
          <div className="p-2.5 rounded-lg border border-border bg-muted/20 space-y-1 text-caption">
            {/* eslint-disable-next-line i18next/no-literal-string */}
            <span className="font-mono text-micro font-medium">plan</span>
            <div className="flex gap-1 flex-wrap">
              <Badge variant="secondary" className="text-micro">{verdictItem("planned", "pass")}</Badge>
            </div>
          </div>
        </div>
      </div>

      {/* Custom sets */}
      <div className="space-y-2">
        <span className="text-caption font-medium text-muted-foreground">{t(($) => $.flow_designer.verdicts.custom)}</span>
        <div className="rounded-lg border border-border overflow-hidden">
          <table className="w-full text-caption border-collapse">
            <thead>
              <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
                <th className="p-2 font-medium">{t(($) => $.flow_designer.common.key)}</th>
                <th className="p-2 font-medium">{t(($) => $.flow_designer.verdicts.col_verdicts)}</th>
                <th className="p-2 font-medium w-10"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/60">
              {(doc.verdicts ?? []).map((set) => (
                <tr key={set.key} className="hover:bg-muted/30">
                  <td className="p-2 font-mono text-micro text-foreground">{set.key}</td>
                  <td className="p-2">
                    <div className="flex gap-1 flex-wrap items-center">
                      {set.verdicts.map((v) => (
                        <span key={v.key} className="inline-flex items-center gap-1">
                          <span className="text-micro font-medium">{v.label || v.key}</span>
                          {getClassBadge(v.class)}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="p-2 text-right">
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      aria-label={t(($) => $.flow_designer.verdicts.delete, { key: set.key })}
                      onClick={() => handleDeleteVerdictSet(set.key)}
                      className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-3.5" aria-hidden />
                    </Button>
                  </td>
                </tr>
              ))}
              {(doc.verdicts ?? []).length === 0 && (
                <tr>
                  <td colSpan={3} className="p-4 text-center text-muted-foreground">
                    {t(($) => $.flow_designer.verdicts.empty)}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Add verdict set */}
      <div className="flex items-center gap-2 pt-2">
        <Input
          // A key is a code token: the same in every language.
          // eslint-disable-next-line no-restricted-syntax
          placeholder="set_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-36"
          disabled={atLimit}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={atLimit || !newKey.trim()}
          onClick={handleAddVerdictSet}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>{t(($) => $.flow_designer.verdicts.add)}</span>
        </Button>
      </div>
    </div>
  );
}
