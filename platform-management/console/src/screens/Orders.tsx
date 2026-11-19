import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Copy, Check, CreditCard, ExternalLink } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { useOrders, useOrderEinvoices, useConfirmOrder, useCancelOrder, useMe } from "../api/hooks";
import { PlatformApiError } from "../api/client";
import {
  isSafeHttpsUrl,
  type Order,
  type OrderKind,
  type OrderStatus,
  type OrderEinvoice,
} from "../api/schemas";
import type { StaffRole } from "../components/types";
import { formatMinorAmount } from "../money";

interface OrdersProps {
  userRole?: StaffRole;
}

const STATUS_TABS: { value: OrderStatus; label: string }[] = [
  { value: "pending", label: "Pending" },
  { value: "paid", label: "Paid" },
  { value: "expired", label: "Expired" },
  { value: "canceled", label: "Canceled" },
  { value: "refunded", label: "Refunded" },
];

const ORDER_KIND_LABELS: Record<OrderKind, string> = {
  subscribe: "Subscribe",
  change_plan: "Change plan",
  add_seats: "Add seats",
  renew: "Renew",
  usage: "Usage",
  unknown: "Unknown",
};

export function Orders({ userRole }: OrdersProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const [activeTab, setActiveTab] = useState<OrderStatus>("pending");
  const { data: orders, isLoading, isError, error, refetch } = useOrders(activeTab);

  const confirmOrder = useConfirmOrder();
  const cancelOrder = useCancelOrder();

  const [copiedId, setCopiedId] = useState<string | null>(null);

  // Confirm dialog state
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [selectedOrderForConfirm, setSelectedOrderForConfirm] = useState<Order | null>(null);
  const [transferReference, setTransferReference] = useState("");
  const [confirmError, setConfirmError] = useState<string | null>(null);

  // Cancel / write-off dialog state
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [selectedOrderForCancel, setSelectedOrderForCancel] = useState<Order | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

  // E-invoice history dialog state
  const [historyOrderId, setHistoryOrderId] = useState<string | null>(null);
  const {
    data: orderEinvoices,
    isLoading: historyLoading,
    isError: historyIsError,
    error: historyError,
    refetch: refetchHistory,
  } = useOrderEinvoices(historyOrderId);

  // Role gating: Only billing and admin roles can view the orders screen
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

  const handleCopyTransferCode = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code);
      setCopiedId(code);
      setTimeout(() => setCopiedId(null), 2000);
    } catch {
      // Ignore clipboard write failures
    }
  };

  const handleOpenConfirmDialog = (order: Order) => {
    setSelectedOrderForConfirm(order);
    setTransferReference("");
    setConfirmError(null);
    setConfirmDialogOpen(true);
  };

  const handleConfirmPayment = async (reason: string) => {
    if (!selectedOrderForConfirm) return;
    setConfirmError(null);
    try {
      await confirmOrder.mutateAsync({
        orderId: selectedOrderForConfirm.id,
        reason,
        transferReference: transferReference.trim() || undefined,
      });
      setConfirmDialogOpen(false);
      setSelectedOrderForConfirm(null);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError || err instanceof Error) {
        setConfirmError(err.message);
      } else {
        setConfirmError("Failed to confirm order payment");
      }
    }
  };

  const handleOpenCancelDialog = (order: Order) => {
    setSelectedOrderForCancel(order);
    setCancelError(null);
    setCancelDialogOpen(true);
  };

  const handleCancelOrder = async (reason: string) => {
    if (!selectedOrderForCancel) return;
    setCancelError(null);
    try {
      await cancelOrder.mutateAsync({
        orderId: selectedOrderForCancel.id,
        reason,
      });
      setCancelDialogOpen(false);
      setSelectedOrderForCancel(null);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError || err instanceof Error) {
        setCancelError(err.message);
      } else {
        setCancelError(
          selectedOrderForCancel.kind === "usage"
            ? "Failed to write off order"
            : "Failed to cancel order",
        );
      }
    }
  };

  const sortedHistory = orderEinvoices
    ? [...orderEinvoices].sort((a, b) => b.revision - a.revision)
    : [];

  const isWriteOffTarget = selectedOrderForCancel?.kind === "usage";

  return (
    <div className="space-y-6">
      <PageHeader title="Orders" />

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (value) setActiveTab(value as OrderStatus);
        }}
      >
        <div className="border-b border-surface-border">
          <TabsList variant="line" aria-label="Order status">
            {STATUS_TABS.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </Tabs>

      {isLoading && <LoadingState message="Loading orders..." />}

      {isError && (
        <ErrorState
          title="Failed to load orders"
          message={error instanceof Error ? error.message : "An error occurred while loading orders."}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!orders || orders.length === 0) && (
        <EmptyState
          icon={<CreditCard className="size-4" />}
          title="No orders"
          description={`No orders found with status "${activeTab}".`}
        />
      )}

      {!isLoading && !isError && orders && orders.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table density="compact" dividers="rows" hasHover>
            <TableHeader>
              <TableRow isHeaderRow>
                <TableHeaderCell>Workspace</TableHeaderCell>
                <TableHeaderCell>Kind</TableHeaderCell>
                <TableHeaderCell>Transfer Code</TableHeaderCell>
                <TableHeaderCell>Plan & Seats</TableHeaderCell>
                <TableHeaderCell>Total (incl. VAT)</TableHeaderCell>
                <TableHeaderCell>Buyer Email</TableHeaderCell>
                <TableHeaderCell>Status</TableHeaderCell>
                <TableHeaderCell>E-invoice</TableHeaderCell>
                <TableHeaderCell>Expiry</TableHeaderCell>
                <TableHeaderCell className="text-right">Actions</TableHeaderCell>
              </TableRow>
            </TableHeader>
            <TableBody>
              {orders.map((order: Order) => {
                const totalAmount = order.amount + order.vat_amount;
                const isCopied = copiedId === order.transfer_code;
                const isUsageOrder = order.kind === "usage";
                const canWriteOffUsage = effectiveRole === "admin";

                return (
                  <TableRow key={order.id}>
                    <TableCell>
                      <div className="max-w-52 truncate font-medium text-foreground" title={order.workspace_name}>
                        {order.workspace_name}
                      </div>
                      <div
                        className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                        title={order.workspace_id}
                      >
                        {order.workspace_id}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant="neutral"
                        label={ORDER_KIND_LABELS[order.kind] ?? order.kind}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5 font-mono text-body font-semibold">
                        <span>{order.transfer_code}</span>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => handleCopyTransferCode(order.transfer_code)}
                          aria-label="Copy transfer code"
                          className="text-muted-foreground hover:text-foreground"
                        >
                          {isCopied ? (
                            <Check className="size-3.5 text-success" aria-hidden="true" />
                          ) : (
                            <Copy className="size-3.5" aria-hidden="true" />
                          )}
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Text type="body" weight="medium" as="div" className="capitalize">
                        {order.plan_key} &bull; {order.interval}
                      </Text>
                      <Text type="supporting" hasTabularNumbers as="div">
                        {order.seats} seats
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="body" weight="medium" hasTabularNumbers as="div">
                        {formatMinorAmount(totalAmount, order.currency)}
                      </Text>
                      <Text type="supporting" hasTabularNumbers as="div">
                        Net: {formatMinorAmount(order.amount, order.currency)} + VAT: {formatMinorAmount(order.vat_amount, order.currency)}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="body" as="div">
                        {order.buyer.invoice_email || "-"}
                      </Text>
                      {order.buyer.tax_code && (
                        <Text type="code" color="secondary" as="div">
                          MST: {order.buyer.tax_code}
                        </Text>
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={order.status} />
                    </TableCell>
                    <TableCell>
                      {order.invoice_state ? (
                        <StatusBadge status={order.invoice_state} />
                      ) : (
                        <Text type="supporting">—</Text>
                      )}
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {order.expires_at ? new Date(order.expires_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        {activeTab === "pending" && (
                          <>
                            <Button
                              size="sm"
                              onClick={() => handleOpenConfirmDialog(order)}
                            >
                              Confirm
                            </Button>
                            {isUsageOrder ? (
                              canWriteOffUsage && (
                                <Button
                                  variant="destructive"
                                  size="sm"
                                  onClick={() => handleOpenCancelDialog(order)}
                                >
                                  Write off
                                </Button>
                              )
                            ) : (
                              <Button
                                variant="destructive"
                                size="sm"
                                onClick={() => handleOpenCancelDialog(order)}
                              >
                                Cancel
                              </Button>
                            )}
                          </>
                        )}
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setHistoryOrderId(order.id)}
                        >
                          History
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Confirm Reason Dialog */}
      <ConfirmReasonDialog
        open={confirmDialogOpen}
        onOpenChange={setConfirmDialogOpen}
        title="Confirm payment"
        description={`Confirm bank transfer payment for order ${selectedOrderForConfirm?.transfer_code || ""}. This will apply the subscription immediately.`}
        confirmLabel="Confirm payment"
        isPending={confirmOrder.isPending}
        error={confirmError}
        onConfirm={handleConfirmPayment}
      >
        <Field>
          <FieldLabel htmlFor="transfer-reference-input" className="text-label">
            Bank transfer reference (optional)
          </FieldLabel>
          <Input
            id="transfer-reference-input"
            value={transferReference}
            onChange={(e) => setTransferReference(e.target.value)}
            placeholder="e.g. FT26111600123"
            disabled={confirmOrder.isPending}
          />
        </Field>
      </ConfirmReasonDialog>

      {/* Cancel / Write-off Reason Dialog */}
      <ConfirmReasonDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        title={isWriteOffTarget ? "Write off usage order" : "Cancel order"}
        description={
          isWriteOffTarget
            ? `Write off pending usage order ${selectedOrderForCancel?.transfer_code || ""}? Its lines will stay excluded from future invoices.`
            : `Are you sure you want to cancel pending order ${selectedOrderForCancel?.transfer_code || ""}? This action cannot be undone.`
        }
        confirmLabel={isWriteOffTarget ? "Write off order" : "Cancel order"}
        confirmVariant="destructive"
        isPending={cancelOrder.isPending}
        error={cancelError}
        onConfirm={handleCancelOrder}
      />

      {/* E-invoice History Dialog */}
      <Dialog
        open={historyOrderId !== null}
        onOpenChange={(open) => {
          if (!open) setHistoryOrderId(null);
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>E-invoice history</DialogTitle>
          </DialogHeader>

          {historyLoading && <LoadingState message="Loading e-invoice history..." />}

          {historyIsError && (
            <ErrorState
              title="Failed to load e-invoice history"
              message={
                historyError instanceof PlatformApiError || historyError instanceof Error
                  ? historyError.message
                  : "An error occurred while loading e-invoice history."
              }
              onRetry={refetchHistory}
            />
          )}

          {!historyLoading && !historyIsError && sortedHistory.length === 0 && (
            <EmptyState title="No e-invoice for this order" isCompact />
          )}

          {!historyLoading && !historyIsError && sortedHistory.length > 0 && (
            <Card padding={0} className="overflow-x-auto">
              <Table density="compact" dividers="rows" hasHover>
                <TableHeader>
                  <TableRow isHeaderRow>
                    <TableHeaderCell>Revision</TableHeaderCell>
                    <TableHeaderCell>State</TableHeaderCell>
                    <TableHeaderCell>Invoice number</TableHeaderCell>
                    <TableHeaderCell>Buyer</TableHeaderCell>
                    <TableHeaderCell>Attempts</TableHeaderCell>
                    <TableHeaderCell>Last error</TableHeaderCell>
                    <TableHeaderCell>Updated</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedHistory.map((item: OrderEinvoice) => {
                    const safeUrl = isSafeHttpsUrl(item.lookup_url) ? item.lookup_url.trim() : "";

                    return (
                      <TableRow key={item.revision}>
                        <TableCell>
                          <Text type="code" color="secondary">
                            rev {item.revision}
                          </Text>
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={item.state} />
                        </TableCell>
                        <TableCell>
                          <Text type="code" as="div">
                            {item.invoice_number || "—"}
                          </Text>
                          {safeUrl && (
                            <a
                              href={safeUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 text-caption text-primary underline underline-offset-3 hover:opacity-80"
                            >
                              <ExternalLink className="size-3.5" aria-hidden="true" />
                              <span>View</span>
                            </a>
                          )}
                        </TableCell>
                        <TableCell>
                          <Text type="body" weight="medium" as="div">
                            {item.buyer.name || "—"}
                          </Text>
                          {item.buyer.tax_code && (
                            <Text type="code" color="secondary" as="div">
                              {item.buyer.tax_code}
                            </Text>
                          )}
                        </TableCell>
                        <TableCell>
                          <Text type="body" hasTabularNumbers>
                            {item.attempts}
                          </Text>
                        </TableCell>
                        <TableCell>
                          {item.last_error_code ? (
                            <Text type="code" color="inherit" className="text-destructive">
                              {item.last_error_code}
                            </Text>
                          ) : (
                            <Text type="supporting">—</Text>
                          )}
                        </TableCell>
                        <TableCell>
                          <Text type="supporting" hasTabularNumbers>
                            {item.updated_at ? new Date(item.updated_at).toLocaleString() : "—"}
                          </Text>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Card>
          )}

          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Close
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
