"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  DEFAULT_AUTONOMY_LEVEL,
  squadAutonomyOptions,
  useSetSquadAutonomy,
  type AutonomyLevel,
} from "@multica/core/governance";
import type { Squad } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useT } from "../i18n";
import { AUTONOMY_LEVEL_KEYS } from "./autonomy-levels";

const LEVELS: readonly AutonomyLevel[] = [0, 1, 2, 3];

/**
 * One squad's autonomy level: the level, the reason of its last change, and, for
 * the boss (`editable`), a change with a reason. The Governance tab and the
 * company room's panel both show it, so they share the server state and the copy.
 */
export function SquadAutonomyEditor({ wsId, squad, editable }: { wsId: string; squad: Squad; editable: boolean }) {
  const { t } = useT("governance");
  const { data: view } = useQuery(squadAutonomyOptions(wsId, squad.id));
  const level = view?.level ?? squad.autonomy_level ?? DEFAULT_AUTONOMY_LEVEL;
  const reason = view?.reason ?? "";
  const changedAt = view?.changed_at ?? null;
  const change = useSetSquadAutonomy(wsId, squad.id);

  const [draftLevel, setDraftLevel] = useState<number>(level);
  const [reasonText, setReasonText] = useState("");
  useEffect(() => setDraftLevel(level), [level]);

  const levelKey = AUTONOMY_LEVEL_KEYS[level as AutonomyLevel];
  const canSave = editable && !change.isPending && draftLevel !== level && reasonText.trim() !== "";
  return (
    <div className="flex flex-col gap-3">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-caption text-muted-foreground">
          {t(($) => $.autonomy.level_tag, { level })}
          {levelKey ? ` ${t(($) => $.autonomy.levels[levelKey].name)}` : ""}
        </span>
        {reason ? (
          <span className="text-caption text-muted-foreground">
            {reason}
            {changedAt ? ` · ${new Date(changedAt).toLocaleDateString()}` : ""}
          </span>
        ) : (
          <span className="text-caption text-muted-foreground">{t(($) => $.autonomy.default_level)}</span>
        )}
      </div>
      {editable ? (
        <form
          className="flex flex-col gap-2 sm:flex-row sm:items-end"
          onSubmit={(event) => {
            event.preventDefault();
            if (!canSave) return;
            change.mutate(
              { level: draftLevel as AutonomyLevel, reason: reasonText.trim() },
              {
                onSuccess: () => {
                  setReasonText("");
                  toast.success(t(($) => $.autonomy.level_saved));
                },
                onError: () => toast.error(t(($) => $.autonomy.level_save_failed)),
              },
            );
          }}
        >
          <div className="flex flex-col gap-1">
            <Label className="text-caption">{t(($) => $.autonomy.level_label)}</Label>
            <Select
              items={LEVELS.map((value) => ({ value: String(value), label: `L${value}` }))}
              value={String(draftLevel)}
              disabled={change.isPending}
              onValueChange={(value) => {
                if (value) setDraftLevel(Number(value));
              }}
            >
              <SelectTrigger aria-label={t(($) => $.autonomy.level_label)} className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {LEVELS.map((value) => (
                  <SelectItem key={value} value={String(value)}>
                    {`L${value} ${t(($) => $.autonomy.levels[AUTONOMY_LEVEL_KEYS[value]].name)}`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Label htmlFor={`autonomy-reason-${squad.id}`} className="text-caption">
              {t(($) => $.autonomy.reason_label)}
            </Label>
            <Textarea
              id={`autonomy-reason-${squad.id}`}
              value={reasonText}
              maxLength={500}
              rows={2}
              placeholder={t(($) => $.autonomy.reason_placeholder)}
              disabled={change.isPending}
              onChange={(event) => setReasonText(event.target.value)}
            />
          </div>
          <Button type="submit" size="sm" disabled={!canSave}>
            {t(($) => $.autonomy.save_level)}
          </Button>
        </form>
      ) : null}
    </div>
  );
}
