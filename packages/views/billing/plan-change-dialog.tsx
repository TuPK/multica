"use client";

import type { WorkspacePlanChangePreview } from "@multica/core/types";
import { AlertCircle, Loader2 } from "@multica/ui/icons";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { useLocale, useT } from "../i18n";
import { formatStripeMinorAmount } from "../settings/components/billing-format";

export interface PlanChangeDialogProps {
  open: boolean;
  onOpenChange(open: boolean): void;
  planName: string;
  preview: WorkspacePlanChangePreview | null;
  submitting: boolean;
  error: string | null;
  onConfirm(): void;
}

function formatDate(value: string, locale: string): string | null {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

/** Confirmation dialog for previewed workspace plan changes. */
export function PlanChangeDialog({
  open,
  onOpenChange,
  planName,
  preview,
  submitting,
  error,
  onConfirm,
}: PlanChangeDialogProps) {
  const { t } = useT("billing");
  const locale = useLocale();

  const isSchedule = preview?.mode === "schedule";
  const formattedAmount =
    preview && !isSchedule
      ? formatStripeMinorAmount(preview.amount, preview.currency, locale)
      : null;
  const effectiveDate =
    preview && isSchedule ? formatDate(preview.effectiveAt, locale) : null;

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && submitting) return;
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t(($) => $.workspace.plan_change.title, { planName })}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {error !== null ? (
            <Alert variant="destructive">
              <AlertCircle />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          {preview ? (
            isSchedule ? (
              effectiveDate ? (
                <p className="text-body text-muted-foreground">
                  {t(($) => $.workspace.plan_change.effective_on, {
                    date: effectiveDate,
                  })}
                </p>
              ) : null
            ) : (
              <div className="flex items-center justify-between gap-4 rounded-lg border border-surface-border p-4 text-body">
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.plan_change.due_now)}
                </span>
                {formattedAmount !== null ? (
                  <span className="font-medium tabular-nums">
                    {formattedAmount}
                  </span>
                ) : null}
              </div>
            )
          ) : null}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={submitting}
            onClick={() => onOpenChange(false)}
          >
            {t(($) => $.workspace.plan_change.cancel)}
          </Button>
          <Button
            type="button"
            variant="default"
            disabled={!preview || submitting}
            aria-busy={submitting}
            onClick={onConfirm}
          >
            {submitting ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : null}
            {isSchedule
              ? t(($) => $.workspace.plan_change.confirm_schedule)
              : t(($) => $.workspace.plan_change.confirm_charge)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
