"use client";

import { AlertCircle } from "@multica/ui/icons";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../i18n";

export interface UnpaidUsageBannerProps {
  unpaidOrderId: string | null | undefined;
  onViewInvoice?: (orderId: string) => void;
}

/** Warning banner shown when a workspace has an unpaid usage invoice blocking paid purchases. */
export function UnpaidUsageBanner({
  unpaidOrderId,
  onViewInvoice,
}: UnpaidUsageBannerProps) {
  const { t } = useT("billing");

  if (!unpaidOrderId) return null;

  return (
    <Alert variant="destructive">
      <AlertCircle />
      <AlertTitle>{t(($) => $.workspace.unpaid_usage.title)}</AlertTitle>
      <AlertDescription>
        <p>{t(($) => $.workspace.unpaid_usage.body)}</p>
        {onViewInvoice ? (
          <Button
            className="mt-3"
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onViewInvoice(unpaidOrderId)}
          >
            {t(($) => $.workspace.unpaid_usage.view_invoice)}
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}
