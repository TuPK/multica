import { useState } from "react";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@multica/ui/components/ui/table";
import { ExternalLink } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PlatformApiError } from "../api/client";
import { useEinvoices, useMe, type EinvoiceFilterState } from "../api/hooks";
import { isSafeHttpsUrl, type Einvoice, type EinvoiceState } from "../api/schemas";
import type { StaffRole } from "../components/types";

interface EinvoiceFailuresProps {
  userRole?: StaffRole;
}

const STATE_TABS: { value: EinvoiceFilterState; label: string }[] = [
  { value: "", label: "Needs attention" },
  { value: "failed", label: "Failed" },
  { value: "needs_info", label: "Needs info" },
  { value: "pending", label: "Pending" },
  { value: "issued", label: "Issued" },
];

function formatCurrency(amount: number, currency: string): string {
  return currency
    ? `${amount.toLocaleString()} ${currency.toUpperCase()}`
    : amount.toLocaleString();
}

function formatDateTime(value: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof PlatformApiError || err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function EinvoiceStateBadge({ state }: { state: EinvoiceState }) {
  switch (state) {
    case "issued":
      return <Badge variant="default">Issued</Badge>;
    case "needs_info":
      return <Badge variant="destructive">Needs buyer details</Badge>;
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    case "pending":
      return <Badge variant="outline">Pending</Badge>;
    case "cancelled":
      return <Badge variant="secondary">Cancelled</Badge>;
    default:
      return <Badge variant="outline">Unknown</Badge>;
  }
}

/** Staff view of e-invoice issuance records (defaulting to failed and needs_info). */
export function EinvoiceFailures({ userRole }: EinvoiceFailuresProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const [activeTab, setActiveTab] = useState<EinvoiceFilterState>("");
  const { data: einvoices, isLoading, isError, error, refetch } = useEinvoices(activeTab);

  // Role gating: Only billing and admin roles can view e-invoice issuance records
  if (effectiveRole && effectiveRole !== "billing" && effectiveRole !== "admin") {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-title-lg font-bold">E-invoices</h1>
      </div>

      <div className="flex border-b border-surface-border">
        <div role="tablist" className="flex gap-2">
          {STATE_TABS.map((tab) => {
            const isActive = activeTab === tab.value;
            return (
              <button
                key={tab.value || "default"}
                role="tab"
                type="button"
                aria-selected={isActive}
                onClick={() => setActiveTab(tab.value)}
                className={`border-b-2 px-4 py-2 text-body font-medium transition-colors ${
                  isActive
                    ? "border-primary text-foreground"
                    : "border-transparent text-muted-foreground hover:border-surface-border hover:text-foreground"
                }`}
              >
                {tab.label}
              </button>
            );
          })}
        </div>
      </div>

      {isLoading && <LoadingState message="Loading e-invoices..." />}

      {isError && (
        <ErrorState
          title="Failed to load e-invoices"
          message={getErrorMessage(error, "An error occurred while loading e-invoices.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!einvoices || einvoices.length === 0) && (
        <EmptyState title="No e-invoices found" />
      )}

      {!isLoading && !isError && einvoices && einvoices.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Order</TableHead>
                <TableHead>Workspace</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Total (incl. VAT)</TableHead>
                <TableHead>Attempts</TableHead>
                <TableHead>Details</TableHead>
                <TableHead>Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {einvoices.map((item: Einvoice) => {
                const totalAmount = item.amount + item.vat_amount;
                const safeUrl =
                  item.state === "issued" && isSafeHttpsUrl(item.lookup_url)
                    ? item.lookup_url.trim()
                    : "";

                return (
                  <TableRow key={`${item.order_id}-${item.revision}`}>
                    <TableCell>
                      <div className="font-mono text-caption">{item.order_id}</div>
                      <div className="text-caption text-muted-foreground">
                        rev {item.revision}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-caption text-muted-foreground">
                      {item.workspace_id || "-"}
                    </TableCell>
                    <TableCell>
                      <EinvoiceStateBadge state={item.state} />
                    </TableCell>
                    <TableCell className="text-body">{item.provider || "-"}</TableCell>
                    <TableCell>
                      <div className="font-medium tabular-nums">
                        {formatCurrency(totalAmount, item.currency)}
                      </div>
                      <div className="text-caption text-muted-foreground tabular-nums">
                        Net: {formatCurrency(item.amount, item.currency)} + VAT:{" "}
                        {formatCurrency(item.vat_amount, item.currency)}
                      </div>
                    </TableCell>
                    <TableCell className="tabular-nums text-body">
                      <div>{item.attempts}</div>
                      {item.state === "pending" && item.next_attempt_at ? (
                        <div className="text-caption text-muted-foreground">
                          {formatDateTime(item.next_attempt_at)}
                        </div>
                      ) : null}
                    </TableCell>
                    <TableCell className="text-caption">
                      {item.last_error_code ? (
                        <div className="font-mono text-destructive">
                          {item.last_error_code}
                        </div>
                      ) : null}
                      {item.missing_fields.length > 0 ? (
                        <div className="text-muted-foreground">
                          {item.missing_fields.join(", ")}
                        </div>
                      ) : null}
                      {item.invoice_number ? (
                        <div className="font-mono font-medium">
                          {item.invoice_number}
                        </div>
                      ) : null}
                      {safeUrl ? (
                        <a
                          href={safeUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-primary underline underline-offset-3 hover:opacity-80"
                        >
                          <ExternalLink className="size-3.5" />
                          <span>View</span>
                        </a>
                      ) : null}
                      {!item.last_error_code &&
                      item.missing_fields.length === 0 &&
                      !item.invoice_number &&
                      !safeUrl
                        ? "-"
                        : null}
                    </TableCell>
                    <TableCell className="text-caption text-muted-foreground">
                      {formatDateTime(item.updated_at)}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
