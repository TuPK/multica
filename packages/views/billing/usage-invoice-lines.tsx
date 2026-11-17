"use client";

import { useQuery } from "@tanstack/react-query";
import { workspaceOrderUsageOptions } from "@multica/core/billing";
import { AlertCircle, RefreshCw } from "@multica/ui/icons";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@multica/ui/components/ui/table";
import { useLocale, useT } from "../i18n";
import {
  formatUsageMoney,
  formatUsageQuantity,
  formatUsageUnitPer,
  formatUsageUtcDate,
  resolveMeterLabel,
} from "./usage-card";

export interface UsageInvoiceLinesProps {
  wsId: string;
  orderId: string;
  currency: string;
}

/** Loads and renders the per-day rated lines belonging to a usage invoice. */
export function UsageInvoiceLines({
  wsId,
  orderId,
  currency,
}: UsageInvoiceLinesProps) {
  const { t } = useT("billing");
  const locale = useLocale();
  const query = useQuery(workspaceOrderUsageOptions(wsId, orderId));

  if (query.isPending) {
    return (
      <div
        className="space-y-2 py-2 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
        role="status"
        aria-label={t(($) => $.workspace.invoices.lines_loading)}
      >
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-3/4" />
      </div>
    );
  }

  if (query.isError || !query.data) {
    return (
      <Alert variant="destructive">
        <AlertCircle />
        <AlertDescription>
          <p>{t(($) => $.workspace.invoices.lines_unavailable)}</p>
          <Button
            className="mt-2"
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
          >
            <RefreshCw />
            {t(($) => $.workspace.actions.retry)}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  if (query.data.lines.length === 0) {
    return (
      <p className="py-2 text-caption text-muted-foreground">
        {t(($) => $.workspace.invoices.lines_empty)}
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t(($) => $.workspace.invoices.col_day)}</TableHead>
          <TableHead>{t(($) => $.workspace.invoices.col_meter)}</TableHead>
          <TableHead className="text-right">
            {t(($) => $.workspace.invoices.col_quantity)}
          </TableHead>
          <TableHead className="text-right">
            {t(($) => $.workspace.invoices.col_rate)}
          </TableHead>
          <TableHead className="text-right">
            {t(($) => $.workspace.invoices.col_discount)}
          </TableHead>
          <TableHead className="text-right">
            {t(($) => $.workspace.invoices.col_amount)}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {query.data.lines.map((line) => {
          const formattedDay = formatUsageUtcDate(line.day, locale) ?? line.day;
          const noteLabel =
            line.note === "workspace_suspended"
              ? t(($) => $.workspace.invoices.note_suspended)
              : line.note;

          return (
            <TableRow key={`${line.day}:${line.meter}`}>
              <TableCell className="tabular-nums text-muted-foreground">
                {formattedDay}
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap items-center gap-1.5">
                  <span>{resolveMeterLabel(line.meter, t)}</span>
                  {noteLabel ? (
                    <Badge variant="secondary">{noteLabel}</Badge>
                  ) : null}
                </div>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatUsageQuantity(line.meter, line.quantity, locale)}
              </TableCell>
              <TableCell className="text-right tabular-nums text-muted-foreground">
                {t(($) => $.workspace.usage.rate, {
                  price: formatUsageMoney(line.unitPrice, currency, locale),
                  per: formatUsageUnitPer(line.meter, line.per, locale),
                })}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {line.discountPct > 0
                  ? t(($) => $.workspace.usage.discount_badge, {
                      pct: line.discountPct,
                    })
                  : t(($) => $.shared.date_dash)}
              </TableCell>
              <TableCell className="text-right font-medium tabular-nums">
                {formatUsageMoney(line.netAmount, currency, locale)}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
