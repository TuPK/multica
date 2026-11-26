"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ATTENTION_SPLITS,
  attentionBudgetOptions,
  useSetAttentionBudget,
  type AttentionBudget,
  type AttentionSplit,
} from "@multica/core/governance";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { useT } from "../i18n";

const MAX_MINUTES_PER_WEEK = 7 * 24 * 60;

/**
 * The weekly attention budget (durable spec 7.3): the minutes people spend on
 * questions each week, the split between squads, and what this week has used.
 * Shown only while durable_principal is on.
 */
export function AttentionBudgetSection({ wsId, editable }: { wsId: string; editable: boolean }) {
  const { data } = useQuery(attentionBudgetOptions(wsId));
  if (!data?.enabled) return null;
  return (
    <AttentionBudgetForm
      key={`${data.minutes_per_week}:${data.split}`}
      wsId={wsId}
      initial={data}
      editable={editable}
    />
  );
}

/**
 * The budget as a draft. It is keyed by the saved value, so a saved change starts
 * the form again from the server's answer.
 */
function AttentionBudgetForm({
  wsId,
  initial,
  editable,
}: {
  wsId: string;
  initial: AttentionBudget;
  editable: boolean;
}) {
  const { t } = useT("governance");
  const save = useSetAttentionBudget(wsId);
  const [minutes, setMinutes] = useState(String(initial.minutes_per_week));
  const [split, setSplit] = useState<AttentionSplit>(initial.split);
  const parsed = Number(minutes);
  const valid = minutes.trim() !== "" && Number.isInteger(parsed) && parsed >= 0 && parsed <= MAX_MINUTES_PER_WEEK;
  const dirty = valid && (parsed !== initial.minutes_per_week || split !== initial.split);
  const busy = save.isPending;

  return (
    <section className="flex flex-col gap-3" aria-labelledby="governance-attention-heading">
      <h2 id="governance-attention-heading" className="text-title-sm font-medium">
        {t(($) => $.controls.attention_heading)}
      </h2>
      <form
        className="flex max-w-xl flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (!editable || !dirty || busy) return;
          save.mutate(
            { minutes_per_week: parsed, split },
            {
              onSuccess: () => toast.success(t(($) => $.controls.attention_saved)),
              onError: () => toast.error(t(($) => $.controls.attention_save_failed)),
            },
          );
        }}
      >
        <div className="flex flex-wrap items-end gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="governance-attention-minutes" className="text-caption font-medium">
              {t(($) => $.controls.attention_minutes)}
            </Label>
            <Input
              id="governance-attention-minutes"
              type="number"
              inputMode="numeric"
              min={0}
              max={MAX_MINUTES_PER_WEEK}
              step={1}
              value={minutes}
              disabled={!editable || busy}
              aria-invalid={!valid}
              onChange={(event) => setMinutes(event.target.value)}
              className="w-32"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-caption font-medium">{t(($) => $.controls.attention_split)}</span>
            <Select
              items={ATTENTION_SPLITS.map((option) => ({
                value: option,
                label: t(($) => $.controls.attention_split_options[option]),
              }))}
              value={split}
              disabled={!editable || busy}
              onValueChange={(next) => {
                if (next) setSplit(next as AttentionSplit);
              }}
            >
              <SelectTrigger aria-label={t(($) => $.controls.attention_split)} className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ATTENTION_SPLITS.map((option) => (
                  <SelectItem key={option} value={option}>
                    {t(($) => $.controls.attention_split_options[option])}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {!valid ? (
          <p role="alert" className="text-caption text-destructive">
            {t(($) => $.controls.attention_invalid, { max: MAX_MINUTES_PER_WEEK })}
          </p>
        ) : null}

        <p className="text-caption text-muted-foreground">
          {t(($) => $.controls.attention_spent, {
            spent: Math.round(initial.spent_minutes),
            budget: initial.minutes_per_week,
          })}
        </p>
        <p className="text-caption text-muted-foreground">{t(($) => $.controls.attention_hint)}</p>

        <div>
          <Button type="submit" size="sm" disabled={!editable || !dirty || busy} aria-busy={busy}>
            {t(($) => $.controls.attention_save)}
          </Button>
        </div>
      </form>
    </section>
  );
}
