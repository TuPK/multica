"use client";

import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { ApiError, errorCode } from "@multica/core/api";
import {
  isValidShadowEstimateParams,
  SHADOW_DEFAULT_SAMPLE_RATE,
  SHADOW_DEFAULT_TARGET_PAIRS,
  SHADOW_MAX_TARGET_PAIRS,
  SHADOW_MIN_TARGET_PAIRS,
  shadowNeedsCostConfirmation,
  useShadowEstimate,
  useStartShadowRun,
} from "@multica/core/durable/shadow";
import { runtimeDisplayLabel, runtimeListOptions } from "@multica/core/runtimes";
import { AlertTriangle } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

function formatMicrosUsd(micros: number): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(micros / 1_000_000);
}

/**
 * Starts a shadow trial on a seat (durable spec 9.4 and 9.5). Shows the isolation warning and
 * the cost estimate before starting. Never exposes auto-promotion; always sends auto_promote=false.
 */
export function ShadowTrialDialog({
  open,
  onOpenChange,
  wsId,
  agentId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
  agentId: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85%] overflow-y-auto">
        {open ? (
          <ShadowTrialForm
            wsId={wsId}
            agentId={agentId}
            onDone={() => onOpenChange(false)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

// Mounted only while the dialog is open so reopening starts from the spec defaults,
// while a failed submit keeps what the person typed.
function ShadowTrialForm({
  wsId,
  agentId,
  onDone,
}: {
  wsId: string;
  agentId: string;
  onDone: () => void;
}) {
  const { t } = useT("durable");
  const [candidateRuntimeId, setCandidateRuntimeId] = useState("");
  const [candidateModel, setCandidateModel] = useState("");
  const [workClassPrefix, setWorkClassPrefix] = useState("");
  const [sampleRateText, setSampleRateText] = useState(String(SHADOW_DEFAULT_SAMPLE_RATE));
  const [targetPairsText, setTargetPairsText] = useState(String(SHADOW_DEFAULT_TARGET_PAIRS));
  const [confirmCost, setConfirmCost] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const runtimesQuery = useQuery(runtimeListOptions(wsId));
  const runtimes = runtimesQuery.data ?? [];
  const runtimeItems = runtimes.map((rt) => ({
    value: rt.id,
    label: runtimeDisplayLabel(rt),
  }));

  const parsedRate = sampleRateText.trim() === "" ? Number.NaN : Number(sampleRateText);
  const parsedPairs = targetPairsText.trim() === "" ? Number.NaN : Number(targetPairsText);
  const paramsValid = isValidShadowEstimateParams({
    sample_rate: parsedRate,
    target_pairs: parsedPairs,
  });

  const estimate = useShadowEstimate(
    wsId,
    agentId,
    { sample_rate: parsedRate, target_pairs: parsedPairs },
    paramsValid,
  );
  const start = useStartShadowRun(wsId, agentId);

  const needsConfirm = shadowNeedsCostConfirmation(estimate.data);
  const estimateReady = paramsValid && !estimate.isPending && !estimate.isError && !!estimate.data;
  const canStart =
    candidateRuntimeId !== "" &&
    estimateReady &&
    (!needsConfirm || confirmCost) &&
    !start.isPending;

  const mapError = (err: unknown): string => {
    const code = errorCode(err);
    if (code === "shadow_already_running") return t(($) => $.seats.shadow.error_already_running);
    if (code === "shadow_cost_confirmation_required") return t(($) => $.seats.shadow.error_cost_confirm);
    if (code === "auto_promote_owner_only") return t(($) => $.seats.shadow.error_owner_only);
    if (err instanceof ApiError && err.status === 422) return t(($) => $.seats.shadow.error_invalid);
    return t(($) => $.seats.shadow.error_failed);
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canStart) return;
    setServerError(null);
    start
      .mutateAsync({
        candidate_runtime_id: candidateRuntimeId,
        candidate_model: candidateModel.trim(),
        sample_rate: parsedRate,
        target_pairs: parsedPairs,
        work_class_prefix: workClassPrefix.trim(),
        auto_promote: false,
        confirm_cost: needsConfirm && confirmCost,
      })
      .then(
        (run) => {
          if (run) {
            toast.success(t(($) => $.seats.shadow.started));
            onDone();
          } else {
            const msg = t(($) => $.seats.shadow.error_failed);
            setServerError(msg);
            toast.error(msg);
          }
        },
        (err: unknown) => {
          const msg = mapError(err);
          setServerError(msg);
          toast.error(msg);
        },
      );
  };

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.seats.shadow.title)}</DialogTitle>
      </DialogHeader>

      <div
        role="note"
        className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-caption text-foreground"
      >
        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
        <p className="break-words">{t(($) => $.seats.shadow.isolation_warning)}</p>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t(($) => $.seats.shadow.runtime_label)}</Label>
        {runtimeItems.length === 0 && !runtimesQuery.isPending ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.seats.shadow.runtime_empty)}</p>
        ) : (
          <Select
            items={runtimeItems}
            value={candidateRuntimeId}
            disabled={start.isPending || runtimeItems.length === 0}
            onValueChange={(next) => {
              if (next) {
                setCandidateRuntimeId(next);
                setServerError(null);
              }
            }}
          >
            <SelectTrigger aria-label={t(($) => $.seats.shadow.runtime_label)} className="w-full">
              <SelectValue placeholder={t(($) => $.seats.shadow.runtime_placeholder)} />
            </SelectTrigger>
            <SelectContent>
              {runtimeItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="shadow-candidate-model">{t(($) => $.seats.shadow.model_label)}</Label>
        <Input
          id="shadow-candidate-model"
          value={candidateModel}
          placeholder={t(($) => $.seats.shadow.model_placeholder)}
          disabled={start.isPending}
          onChange={(event) => {
            setCandidateModel(event.target.value);
            setServerError(null);
          }}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="shadow-work-class-prefix">{t(($) => $.seats.shadow.work_class_prefix_label)}</Label>
        <Input
          id="shadow-work-class-prefix"
          value={workClassPrefix}
          placeholder={t(($) => $.seats.shadow.work_class_prefix_placeholder)}
          disabled={start.isPending}
          onChange={(event) => {
            setWorkClassPrefix(event.target.value);
            setServerError(null);
          }}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label htmlFor="shadow-sample-rate">{t(($) => $.seats.shadow.sample_rate_label)}</Label>
          <Input
            id="shadow-sample-rate"
            type="number"
            min={0.01}
            max={1}
            step="any"
            value={sampleRateText}
            disabled={start.isPending}
            onChange={(event) => {
              setSampleRateText(event.target.value);
              setConfirmCost(false);
              setServerError(null);
            }}
          />
          <span className="text-caption text-muted-foreground">{t(($) => $.seats.shadow.sample_rate_hint)}</span>
        </div>

        <div className="flex min-w-0 flex-col gap-1.5">
          <Label htmlFor="shadow-target-pairs">{t(($) => $.seats.shadow.target_pairs_label)}</Label>
          <Input
            id="shadow-target-pairs"
            type="number"
            min={SHADOW_MIN_TARGET_PAIRS}
            max={SHADOW_MAX_TARGET_PAIRS}
            step={1}
            value={targetPairsText}
            disabled={start.isPending}
            onChange={(event) => {
              setTargetPairsText(event.target.value);
              setConfirmCost(false);
              setServerError(null);
            }}
          />
          <span className="text-caption text-muted-foreground">{t(($) => $.seats.shadow.target_pairs_hint)}</span>
        </div>
      </div>

      <div className="flex flex-col gap-1.5 rounded-md border bg-muted/40 px-3 py-2.5 text-caption">
        <span className="font-medium text-foreground">{t(($) => $.seats.shadow.estimate_title)}</span>
        {!paramsValid ? (
          <p className="text-destructive">{t(($) => $.seats.shadow.error_invalid)}</p>
        ) : estimate.isPending ? (
          <p className="text-muted-foreground">{t(($) => $.seats.shadow.estimate_loading)}</p>
        ) : estimate.isError ? (
          <p role="alert" className="text-destructive">
            {t(($) => $.seats.shadow.estimate_failed)}
          </p>
        ) : estimate.data ? (
          <div className="flex flex-col gap-1 tabular-nums">
            <p className="text-body text-foreground">
              {t(($) => $.seats.shadow.estimate_amount, {
                amount: formatMicrosUsd(estimate.data.estimate_micros),
              })}
            </p>
            <p className="text-muted-foreground">
              {estimate.data.remaining_micros >= 0
                ? t(($) => $.seats.shadow.budget_remaining, {
                    amount: formatMicrosUsd(estimate.data.remaining_micros),
                  })
                : t(($) => $.seats.shadow.budget_none)}
            </p>
          </div>
        ) : null}
      </div>

      {needsConfirm ? (
        <div className="flex flex-col gap-2 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2.5">
          <p className="text-caption text-destructive">{t(($) => $.seats.shadow.budget_exceeded)}</p>
          <div className="flex items-start gap-2">
            <Checkbox
              id="shadow-confirm-cost"
              checked={confirmCost}
              disabled={start.isPending}
              onCheckedChange={(checked) => {
                setConfirmCost(checked === true);
                setServerError(null);
              }}
            />
            <Label htmlFor="shadow-confirm-cost" className="text-caption leading-snug text-foreground">
              {t(($) => $.seats.shadow.confirm_cost)}
            </Label>
          </div>
        </div>
      ) : null}

      {serverError ? (
        <p role="alert" className="text-caption text-destructive">
          {serverError}
        </p>
      ) : null}

      <DialogFooter>
        <DialogClose render={<Button type="button" variant="outline" />}>
          {t(($) => $.seats.shadow.cancel)}
        </DialogClose>
        <Button type="submit" disabled={!canStart} aria-busy={start.isPending}>
          {t(($) => $.seats.shadow.start)}
        </Button>
      </DialogFooter>
    </form>
  );
}
