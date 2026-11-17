"use client";

import type { WorkspacePendingPlanChange } from "@multica/core/types";
import { AlertCircle, Loader2 } from "@multica/ui/icons";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { useLocale, useT } from "../i18n";

export interface PendingChangeBannerProps {
  pendingChange: WorkspacePendingPlanChange | null;
  planName: string;
  effectiveAt: string | null;
  canCancel: boolean;
  cancelling: boolean;
  onCancel(): void;
}

function formatDate(value: string | null, locale: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

/** Banner describing a plan change scheduled for the next renewal. */
export function PendingChangeBanner({
  pendingChange,
  planName,
  effectiveAt,
  canCancel,
  cancelling,
  onCancel,
}: PendingChangeBannerProps) {
  const { t } = useT("billing");
  const locale = useLocale();

  if (!pendingChange) return null;

  const formattedDate = formatDate(effectiveAt, locale);

  return (
    <Alert>
      <AlertCircle />
      <AlertTitle>{t(($) => $.workspace.pending_change.title)}</AlertTitle>
      <AlertDescription>
        <p>
          {formattedDate
            ? t(($) => $.workspace.pending_change.body, {
                planName,
                date: formattedDate,
              })
            : t(($) => $.workspace.pending_change.body_no_date, {
                planName,
              })}
        </p>
        {canCancel ? (
          <Button
            className="mt-3"
            type="button"
            variant="outline"
            size="sm"
            disabled={cancelling}
            aria-busy={cancelling}
            onClick={onCancel}
          >
            {cancelling ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : null}
            {t(($) => $.workspace.pending_change.cancel)}
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}
