import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { StatusDot } from "@astryxdesign/core/StatusDot";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@multica/ui/components/ui/button";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { ExternalLink, FileText, RefreshCw, X } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { PlatformApiError } from "../api/client";
import { useEinvoices, useMe, type EinvoiceFilterState } from "../api/hooks";
import { isSafeHttpsUrl, type Einvoice } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { formatMinorAmount } from "../money";

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

function formatDateTime(value: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof PlatformApiError || err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function einvoiceRowKey(item: Einvoice): string {
  return `${item.order_id}-${item.revision}`;
}

/**
 * Staff triage queue of e-invoice issuance records (defaulting to failed and
 * needs_info). Uses Astryx alert-rail queue summary and on-demand inspector.
 */
export function EinvoiceFailures({ userRole }: EinvoiceFailuresProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const [activeTab, setActiveTab] = useState<EinvoiceFilterState>("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const {
    data: einvoices,
    isLoading,
    isFetching,
    isError,
    error,
    refetch,
  } = useEinvoices(activeTab);

  // Role gating: Only billing and system_admin roles can view e-invoice issuance records
  if (effectiveRole && effectiveRole !== "billing" && effectiveRole !== "system_admin") {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
        />
      </div>
    );
  }

  const items = einvoices ?? [];
  const failedCount = items.filter((item) => item.state === "failed").length;
  const needsInfoCount = items.filter((item) => item.state === "needs_info").length;
  const selectedItem = selectedKey
    ? items.find((item) => einvoiceRowKey(item) === selectedKey) ?? null
    : null;

  return (
    <div className="space-y-6">
      <PageHeader
        title="E-invoices"
        meta={
          !isLoading && !isError ? (
            <span className="tabular-nums">
              {items.length} {items.length === 1 ? "record" : "records"} in current view
            </span>
          ) : undefined
        }
        actions={
          items.length > 0 ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => refetch()}
              disabled={Boolean(isFetching)}
              aria-busy={Boolean(isFetching)}
              className="gap-1.5"
            >
              {isFetching ? (
                <Spinner className="size-3.5" />
              ) : (
                <RefreshCw className="size-3.5" aria-hidden="true" />
              )}
              <span>Refresh</span>
            </Button>
          ) : undefined
        }
      />

      <Tabs
        value={activeTab || "default"}
        onValueChange={(value) => {
          setActiveTab((value === "default" ? "" : value) as EinvoiceFilterState);
          setSelectedKey(null);
        }}
      >
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-surface-border">
          <TabsList variant="line" aria-label="E-invoice state">
            {STATE_TABS.map((tab) => (
              <TabsTrigger key={tab.value || "default"} value={tab.value || "default"}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>

          {activeTab === "" && !isLoading && !isError && items.length > 0 && (
            <div
              aria-label="Attention breakdown"
              className="flex flex-wrap items-center gap-4 pb-2"
            >
              <span className="inline-flex items-center gap-1.5">
                <StatusDot variant="error" label="Failed" aria-hidden="true" />
                <Text type="supporting" hasTabularNumbers>
                  {failedCount} failed
                </Text>
              </span>
              <span className="inline-flex items-center gap-1.5">
                <StatusDot variant="warning" label="Needs buyer details" aria-hidden="true" />
                <Text type="supporting" hasTabularNumbers>
                  {needsInfoCount} missing buyer details
                </Text>
              </span>
            </div>
          )}
        </div>
      </Tabs>

      {isLoading && <LoadingState message="Loading e-invoices..." />}

      {isError && (
        <ErrorState
          title="Failed to load e-invoices"
          message={getErrorMessage(error, "An error occurred while loading e-invoices.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && items.length === 0 && (
        <EmptyState
          icon={<FileText className="size-4" />}
          title="No e-invoices found"
          actions={
            activeTab ? (
              <Button variant="outline" size="sm" onClick={() => setActiveTab("")}>
                View needing attention
              </Button>
            ) : (
              <Button variant="outline" size="sm" onClick={() => refetch()}>
                Refresh
              </Button>
            )
          }
        />
      )}

      {!isLoading && !isError && items.length > 0 && (
        <div
          className={
            selectedItem
              ? "grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]"
              : "grid grid-cols-1"
          }
        >
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>Order</TableHeaderCell>
                  <TableHeaderCell>Workspace</TableHeaderCell>
                  <TableHeaderCell>State</TableHeaderCell>
                  <TableHeaderCell>Provider</TableHeaderCell>
                  <TableHeaderCell className="text-right">Total (incl. VAT)</TableHeaderCell>
                  <TableHeaderCell className="text-right">Attempts</TableHeaderCell>
                  <TableHeaderCell>Details</TableHeaderCell>
                  <TableHeaderCell>Updated</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item: Einvoice) => {
                  const rowKey = einvoiceRowKey(item);
                  const isSelected = selectedKey === rowKey;
                  const totalAmount = item.amount + item.vat_amount;
                  const safeUrl =
                    item.state === "issued" && isSafeHttpsUrl(item.lookup_url)
                      ? item.lookup_url.trim()
                      : "";

                  return (
                    <TableRow
                      key={rowKey}
                      data-state={isSelected ? "selected" : undefined}
                      onClick={() => setSelectedKey(isSelected ? null : rowKey)}
                      className="cursor-pointer"
                    >
                      <TableCell>
                        <div
                          className="max-w-44 truncate font-mono text-caption text-foreground"
                          title={item.order_id}
                        >
                          {item.order_id}
                        </div>
                        <Text type="supporting" hasTabularNumbers as="div">
                          rev {item.revision}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <div
                          className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                          title={item.workspace_id || undefined}
                        >
                          {item.workspace_id || "-"}
                        </div>
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          status={item.state}
                          label={item.state === "needs_info" ? "Needs buyer details" : undefined}
                        />
                      </TableCell>
                      <TableCell>
                        <Text type="body">{item.provider || "-"}</Text>
                      </TableCell>
                      <TableCell className="text-right">
                        <Text type="body" weight="medium" hasTabularNumbers as="div">
                          {formatMinorAmount(totalAmount, item.currency)}
                        </Text>
                        <Text type="supporting" hasTabularNumbers as="div">
                          Net: {formatMinorAmount(item.amount, item.currency)} + VAT:{" "}
                          {formatMinorAmount(item.vat_amount, item.currency)}
                        </Text>
                      </TableCell>
                      <TableCell className="text-right">
                        <Text type="body" hasTabularNumbers as="div">
                          {item.attempts}
                        </Text>
                        {item.state === "pending" && item.next_attempt_at ? (
                          <Text type="supporting" hasTabularNumbers as="div">
                            {formatDateTime(item.next_attempt_at)}
                          </Text>
                        ) : null}
                      </TableCell>
                      <TableCell className="text-caption">
                        {item.last_error_code ? (
                          <Text type="code" color="inherit" as="div" className="text-destructive">
                            {item.last_error_code}
                          </Text>
                        ) : null}
                        {item.missing_fields.length > 0 ? (
                          <Text type="supporting" as="div">
                            {item.missing_fields.join(", ")}
                          </Text>
                        ) : null}
                        {item.invoice_number ? (
                          <Text type="code" weight="medium" as="div">
                            {item.invoice_number}
                          </Text>
                        ) : null}
                        {safeUrl ? (
                          <a
                            href={safeUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            onClick={(e) => e.stopPropagation()}
                            className="inline-flex items-center gap-1 text-brand underline underline-offset-3 hover:opacity-80"
                          >
                            <ExternalLink className="size-3.5" aria-hidden="true" />
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
                      <TableCell>
                        <Text type="supporting" hasTabularNumbers>
                          {formatDateTime(item.updated_at)}
                        </Text>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </Card>

          {selectedItem && (
            <aside aria-label="E-invoice details">
              <Card padding={5} className="space-y-4">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <Heading level={2}>E-invoice details</Heading>
                      <StatusBadge
                        status={selectedItem.state}
                        label={
                          selectedItem.state === "needs_info"
                            ? "Needs buyer details"
                            : undefined
                        }
                      />
                    </div>
                    <Text type="code" color="secondary" hasTabularNumbers as="p">
                      rev {selectedItem.revision}
                    </Text>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Close details"
                    onClick={() => setSelectedKey(null)}
                  >
                    <X className="size-4" aria-hidden="true" />
                  </Button>
                </div>

                <Divider />

                <MetadataList columns={1}>
                  <MetadataListItem label="Order ID" mono className="break-all">
                    {selectedItem.order_id}
                  </MetadataListItem>
                  <MetadataListItem label="Workspace ID" mono className="break-all">
                    {selectedItem.workspace_id || "-"}
                  </MetadataListItem>
                  <MetadataListItem label="Provider">
                    {selectedItem.provider || "-"}
                  </MetadataListItem>
                  <MetadataListItem label="Total (incl. VAT)" tabular>
                    {formatMinorAmount(
                      selectedItem.amount + selectedItem.vat_amount,
                      selectedItem.currency,
                    )}
                  </MetadataListItem>
                  <MetadataListItem label="Attempts" tabular>
                    {selectedItem.attempts}
                  </MetadataListItem>
                  {selectedItem.last_error_code && (
                    <MetadataListItem label="Last error code" mono>
                      <Text type="code" color="inherit" className="text-destructive">
                        {selectedItem.last_error_code}
                      </Text>
                    </MetadataListItem>
                  )}
                  {selectedItem.missing_fields.length > 0 && (
                    <MetadataListItem label="Missing fields">
                      {selectedItem.missing_fields.join(", ")}
                    </MetadataListItem>
                  )}
                  {selectedItem.invoice_number && (
                    <MetadataListItem label="Invoice number" mono>
                      {selectedItem.invoice_number}
                    </MetadataListItem>
                  )}
                  {selectedItem.next_attempt_at && (
                    <MetadataListItem label="Next attempt" tabular>
                      {formatDateTime(selectedItem.next_attempt_at)}
                    </MetadataListItem>
                  )}
                  <MetadataListItem label="Updated" tabular>
                    {formatDateTime(selectedItem.updated_at)}
                  </MetadataListItem>
                </MetadataList>
              </Card>
            </aside>
          )}
        </div>
      )}
    </div>
  );
}
