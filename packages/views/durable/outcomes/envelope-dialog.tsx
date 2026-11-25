"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useSetSquadEnvelope, useSquadEnvelope } from "@multica/core/durable/track-records";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useT } from "../../i18n";
import { AUTONOMY_LEVEL_KEYS } from "../../governance/autonomy-levels";
import { useOutcomesEnabled } from "./use-outcomes-enabled";

const LEVELS = [0, 1, 2, 3] as const;

/**
 * A squad's autonomy range in the Autonomy tab (durable spec 3.5): the current floor and ceiling,
 * the 90-day reputation the range reads, and for an owner or admin the button that sets it.
 */
export function SquadEnvelopeControl({ wsId, squadId, editable }: { wsId: string; squadId: string; editable: boolean }) {
  const enabled = useOutcomesEnabled();
  if (!enabled) return null;
  return <EnvelopeSummary wsId={wsId} squadId={squadId} editable={editable} />;
}

function EnvelopeSummary({ wsId, squadId, editable }: { wsId: string; squadId: string; editable: boolean }) {
  const { t } = useT("durable");
  const view = useSquadEnvelope(wsId, squadId);
  const [open, setOpen] = useState(false);
  const envelope = view.data?.envelope ?? null;
  const reputation = view.data?.reputation;
  const score = reputation ? Math.round(reputation.score_lower * 100) : 0;

  return (
    <div className="flex flex-col items-start gap-1 text-caption text-muted-foreground">
      <span>
        {t(($) => $.outcomes.envelope.column)}:{" "}
        {envelope
          ? t(($) => $.outcomes.envelope.range, { floor: envelope.floor_level, ceiling: envelope.ceiling_level })
          : t(($) => $.outcomes.envelope.none)}
      </span>
      <span>
        {t(($) => $.outcomes.envelope.reputation_column)}:{" "}
        {reputation && reputation.n > 0
          ? t(($) => $.outcomes.envelope.reputation, { n: reputation.n, score })
          : t(($) => $.outcomes.envelope.reputation_none)}
        {reputation?.high_fail_90 ? ` · ${t(($) => $.outcomes.envelope.high_fail)}` : ""}
      </span>
      {editable ? (
        <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
          {t(($) => $.outcomes.envelope.set)}
        </Button>
      ) : null}
      <EnvelopeDialog open={open} onOpenChange={setOpen} wsId={wsId} squadId={squadId} />
    </div>
  );
}

/**
 * Sets a squad's floor and ceiling with the reason. The range is refused while the floor is above
 * the ceiling, and the dialog closes only once the server keeps the range.
 */
export function EnvelopeDialog({
  open,
  onOpenChange,
  wsId,
  squadId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
  squadId: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <EnvelopeForm wsId={wsId} squadId={squadId} onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

// The form mounts with the dialog, so a reopened dialog starts from the defaults. A refused save
// keeps what the person typed.
function EnvelopeForm({ wsId, squadId, onDone }: { wsId: string; squadId: string; onDone: () => void }) {
  const { t } = useT("durable");
  const [floor, setFloor] = useState(0);
  const [ceiling, setCeiling] = useState(3);
  const [reason, setReason] = useState("");
  const save = useSetSquadEnvelope(wsId, squadId);

  const floorAboveCeiling = floor > ceiling;
  const canSave = !floorAboveCeiling && reason.trim() !== "" && !save.isPending;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!canSave) return;
    save
      .mutateAsync({ floor_level: floor, ceiling_level: ceiling, reason: reason.trim() })
      .then(
        () => {
          toast.success(t(($) => $.outcomes.envelope.saved));
          onDone();
        },
        () => toast.error(t(($) => $.outcomes.envelope.save_failed)),
      );
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.outcomes.envelope.dialog_title)}</DialogTitle>
        <DialogDescription>{t(($) => $.outcomes.envelope.dialog_hint)}</DialogDescription>
      </DialogHeader>
      <div className="grid grid-cols-2 gap-3">
        <LevelField label={t(($) => $.outcomes.envelope.floor)} value={floor} onChange={setFloor} />
        <LevelField label={t(($) => $.outcomes.envelope.ceiling)} value={ceiling} onChange={setCeiling} />
      </div>
      {floorAboveCeiling ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.outcomes.envelope.floor_above_ceiling)}
        </p>
      ) : null}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="envelope-reason">{t(($) => $.outcomes.envelope.reason)}</Label>
        <Textarea
          id="envelope-reason"
          value={reason}
          rows={2}
          maxLength={500}
          placeholder={t(($) => $.outcomes.envelope.reason_placeholder)}
          disabled={save.isPending}
          onChange={(event) => setReason(event.target.value)}
        />
      </div>
      <DialogFooter>
        <DialogClose render={<Button variant="outline" />}>{t(($) => $.outcomes.cancel)}</DialogClose>
        <Button type="submit" disabled={!canSave} aria-busy={save.isPending}>
          {t(($) => $.outcomes.envelope.save)}
        </Button>
      </DialogFooter>
    </form>
  );
}

function LevelField({ label, value, onChange }: { label: string; value: number; onChange: (value: number) => void }) {
  const { t } = useT("governance");
  const items = LEVELS.map((level) => ({
    value: String(level),
    label: `L${level} ${t(($) => $.autonomy.levels[AUTONOMY_LEVEL_KEYS[level]].name)}`,
  }));
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-caption text-muted-foreground">{label}</span>
      <Select
        items={items}
        value={String(value)}
        onValueChange={(next) => {
          if (next) onChange(Number(next));
        }}
      >
        <SelectTrigger aria-label={label} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
