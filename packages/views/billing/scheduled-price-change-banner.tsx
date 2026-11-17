"use client";

import type { ScheduledPriceChange } from "@multica/core/billing";
import { AlertCircle } from "@multica/ui/icons";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import { useLocale, useT } from "../i18n";
import { formatUsageMoney, formatUsageUtcDate } from "./usage-card";

export interface ScheduledPriceChangeBannerProps {
  scheduledPriceChange: ScheduledPriceChange | null;
  planName: string;
  currency: string;
}

/** Notice banner shown when the workspace's current plan has an announced upcoming price change. */
export function ScheduledPriceChangeBanner({
  scheduledPriceChange,
  planName,
  currency,
}: ScheduledPriceChangeBannerProps) {
  const { t } = useT("billing");
  const locale = useLocale();

  if (!scheduledPriceChange) return null;

  const formattedDate = formatUsageUtcDate(
    scheduledPriceChange.effectiveFrom,
    locale,
  );
  const monthlyPrice =
    scheduledPriceChange.seatPriceMonth !== null
      ? formatUsageMoney(scheduledPriceChange.seatPriceMonth, currency, locale)
      : null;
  const yearlyPrice =
    scheduledPriceChange.seatPriceYear !== null
      ? formatUsageMoney(scheduledPriceChange.seatPriceYear, currency, locale)
      : null;

  return (
    <Alert>
      <AlertCircle />
      <AlertTitle>{t(($) => $.workspace.scheduled_price_change.title)}</AlertTitle>
      <AlertDescription>
        <p>
          {formattedDate
            ? t(($) => $.workspace.scheduled_price_change.body, {
                planName,
                date: formattedDate,
              })
            : t(($) => $.workspace.scheduled_price_change.body_no_date, {
                planName,
              })}
        </p>
        {monthlyPrice || yearlyPrice ? (
          <ul className="mt-1 space-y-0.5 tabular-nums">
            {monthlyPrice ? (
              <li>
                {t(($) => $.workspace.scheduled_price_change.monthly_price, {
                  price: monthlyPrice,
                })}
              </li>
            ) : null}
            {yearlyPrice ? (
              <li>
                {t(($) => $.workspace.scheduled_price_change.yearly_price, {
                  price: yearlyPrice,
                })}
              </li>
            ) : null}
          </ul>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}
