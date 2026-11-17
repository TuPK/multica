"use client";

import { useState } from "react";
import { Loader2 } from "@multica/ui/icons";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Button } from "@multica/ui/components/ui/button";
import { useLocale, useT } from "../i18n";

export interface SubscriptionLifecycleSectionProps {
  canCancel: boolean;
  canResume: boolean;
  periodEnd: string | null;
  cancelling: boolean;
  resuming: boolean;
  onCancel(): void;
  onResume(): void;
}

function formatDate(value: string | null, locale: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

/** Cancel and resume controls driven strictly by server-authorized actions. */
export function SubscriptionLifecycleSection({
  canCancel,
  canResume,
  periodEnd,
  cancelling,
  resuming,
  onCancel,
  onResume,
}: SubscriptionLifecycleSectionProps) {
  const { t } = useT("billing");
  const locale = useLocale();
  const [confirmOpen, setConfirmOpen] = useState(false);

  if (!canCancel && !canResume) return null;

  const formattedPeriodEnd = formatDate(periodEnd, locale);

  return (
    <div className="flex flex-wrap items-center gap-3">
      {canCancel ? (
        <>
          <Button
            type="button"
            variant="outline"
            disabled={cancelling || resuming}
            onClick={() => setConfirmOpen(true)}
          >
            {cancelling ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : null}
            {t(($) => $.workspace.lifecycle.cancel)}
          </Button>
          <AlertDialog
            open={confirmOpen}
            onOpenChange={(nextOpen) => {
              if (!nextOpen && cancelling) return;
              setConfirmOpen(nextOpen);
            }}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {t(($) => $.workspace.lifecycle.cancel_title)}
                </AlertDialogTitle>
                {formattedPeriodEnd ? (
                  <AlertDialogDescription>
                    {t(($) => $.workspace.lifecycle.cancel_body, {
                      date: formattedPeriodEnd,
                    })}
                  </AlertDialogDescription>
                ) : null}
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={cancelling}>
                  {t(($) => $.workspace.lifecycle.keep)}
                </AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={cancelling}
                  aria-busy={cancelling}
                  onClick={() => {
                    setConfirmOpen(false);
                    onCancel();
                  }}
                >
                  {t(($) => $.workspace.lifecycle.cancel_confirm)}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      ) : null}

      {canResume ? (
        <Button
          type="button"
          variant="default"
          disabled={resuming || cancelling}
          aria-busy={resuming}
          onClick={onResume}
        >
          {resuming ? (
            <Loader2 className="animate-spin motion-reduce:animate-none" />
          ) : null}
          {t(($) => $.workspace.lifecycle.resume)}
        </Button>
      ) : null}
    </div>
  );
}
