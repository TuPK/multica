"use client";

import { useQuery } from "@tanstack/react-query";
import { effectListOptions } from "@multica/core/durable/effects";
import { useWorkspaceId } from "@multica/core/hooks";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { IconButton } from "@multica/ui/components/common/icon-button";
import { ShieldCheck } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { EffectRow } from "./effect-row";
import { useEffectsEnabled } from "./use-effects-enabled";

/**
 * The effects one run recorded (durable spec 4.7), in the run's transcript header. The list is
 * fetched only when the popover opens; nothing renders while durable effects is off.
 */
export function RunEffectsButton({ taskId, open }: { taskId: string; open: boolean }) {
  const enabled = useEffectsEnabled();
  if (!enabled) return null;
  return <RunEffectsPopover taskId={taskId} open={open} />;
}

function RunEffectsPopover({ taskId, open }: { taskId: string; open: boolean }) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const effects = useQuery({ ...effectListOptions(wsId, { task_id: taskId, limit: 50 }), enabled: open });
  const rows = effects.data ?? [];

  return (
    <Popover>
      <PopoverTrigger
        render={
          <IconButton
            variant="ghost"
            size="icon-sm"
            label={t(($) => $.run.label)}
            tooltipSide="bottom"
            className="text-muted-foreground"
          />
        }
      >
        <ShieldCheck className="h-3.5 w-3.5" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 max-w-[calc(100vw-2rem)] p-3">
        <div className="mb-2 text-caption font-medium text-foreground">{t(($) => $.run.title)}</div>
        {rows.length === 0 ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.run.empty)}</p>
        ) : (
          <ul className="flex max-h-80 flex-col divide-y overflow-y-auto">
            {rows.map((effect) => (
              <EffectRow key={effect.id} effect={effect} />
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
