"use client";

import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { HANDOFF_MODES, squadHandoffRuleOptions, useSetSquadHandoffRule, type HandoffMode } from "@multica/core/squads";
import { squadListOptions } from "@multica/core/workspace/queries";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

const NONE = "__none__";

/**
 * Where this squad hands the requests it finishes, and how: the boss's switch for
 * automatic handoff between rooms. Anyone can read it; owners and admins change it.
 */
export function CompanySquadHandoff({ squadId, canEdit }: { squadId: string; canEdit: boolean }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const { data: rule } = useQuery(squadHandoffRuleOptions(wsId, squadId));
  const { data: squads = [] } = useQuery(squadListOptions(wsId));
  const save = useSetSquadHandoffRule(wsId, squadId);

  const targets = squads.filter((squad) => squad.id !== squadId && !squad.archived_at);
  const targetId = rule?.target_squad_id ?? NONE;
  const mode: HandoffMode = rule?.mode ?? "off";
  const targetItems = [
    { value: NONE, label: t(($) => $.handoff.panel_none) },
    ...targets.map((squad) => ({ value: squad.id, label: squad.name })),
  ];
  const modeItems = HANDOFF_MODES.map((value) => ({ value, label: t(($) => $.handoff.modes[value]) }));

  const update = (next: { target: string; mode: HandoffMode }) =>
    save.mutate(
      { target_squad_id: next.target === NONE ? null : next.target, mode: next.mode },
      { onError: (err) => toast.error(clientErrorMessage(err) ?? t(($) => $.handoff.save_failed)) },
    );

  // A squad that nothing hands to and that the viewer cannot change has nothing to show.
  if (!canEdit && !rule?.target_squad_id) return null;

  return (
    <section aria-label={t(($) => $.handoff.panel_heading)} className="flex flex-col gap-2">
      <h3 className="text-caption font-medium text-muted-foreground">{t(($) => $.handoff.panel_heading)}</h3>
      {canEdit ? (
        <div className="flex flex-col gap-2">
          <Select
            items={targetItems}
            value={targetId}
            disabled={save.isPending}
            onValueChange={(v) => update({ target: v ?? NONE, mode: v && v !== NONE && mode === "off" ? "off" : mode })}
          >
            <SelectTrigger aria-label={t(($) => $.handoff.panel_target)} className="w-full">
              <span className="truncate">{targetItems.find((item) => item.value === targetId)?.label}</span>
            </SelectTrigger>
            <SelectContent>
              {targetItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {targetId !== NONE ? (
            <>
              <Select
                items={modeItems}
                value={mode}
                disabled={save.isPending}
                onValueChange={(v) => v && update({ target: targetId, mode: v as HandoffMode })}
              >
                <SelectTrigger aria-label={t(($) => $.handoff.panel_mode)} className="w-full">
                  <span className="truncate">{modeItems.find((item) => item.value === mode)?.label}</span>
                </SelectTrigger>
                <SelectContent>
                  {modeItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {mode === "auto" ? (
                <p className="text-caption text-muted-foreground">{t(($) => $.handoff.auto_hint)}</p>
              ) : null}
            </>
          ) : null}
        </div>
      ) : (
        <p className="text-body">
          {t(($) => $.handoff.panel_summary, {
            squad: rule?.target_squad_name ?? "",
            mode: t(($) => $.handoff.modes[mode]),
          })}
        </p>
      )}
    </section>
  );
}
