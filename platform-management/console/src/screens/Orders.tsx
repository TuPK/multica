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
import { RowActions } from "../components/RowActions";
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
import { t } from "../i18n";

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

  // Role gating: billing, admin (read-only), and system_admin can view the orders screen
  if (effectiveRole && effectiveRole !== "billing" && effectiveRole !== "admin" && effectiveRole !== "system_admin") {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
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
        setConfirmError(t("Failed to confirm order payment"));
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
            ? t("Failed to write off order")
            : t("Failed to cancel order"),
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
      <PageHeader title={t("Orders")} />

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (value) setActiveTab(value as OrderStatus);
        }}
      >
        <div className="border-b border-surface-border">
          <TabsList variant="line" aria-label={t("Order status")}>
            {STATUS_TABS.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {t(tab.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </Tabs>

      {isLoading && <LoadingState message={t("Loading orders...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load orders")}
          message={error instanceof Error ? error.message : t("An error occurred while loading orders.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!orders || orders.length === 0) && (
        <EmptyState
          icon={<CreditCard className="size-4" />}
          title={t("No orders")}
          description={t("No orders found with status \"{0}\".", activeTab)}
        />
      )}

      {!isLoading && !isError && orders && orders.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table density="compact" dividers="rows" hasHover>
            <TableHeader>
              <TableRow isHeaderRow>
                <TableHeaderCell>{t("Workspace")}</TableHeaderCell>
                <TableHeaderCell>{t("Kind")}</TableHeaderCell>
                <TableHeaderCell>{t("Transfer Code")}</TableHeaderCell>
                <TableHeaderCell>{t("Plan & Seats")}</TableHeaderCell>
                <TableHeaderCell>{t("Total (incl. VAT)")}</TableHeaderCell>
                <TableHeaderCell>{t("Buyer Email")}</TableHeaderCell>
                <TableHeaderCell>{t("Status")}</TableHeaderCell>
                <TableHeaderCell>{t("E-invoice")}</TableHeaderCell>
                <TableHeaderCell>{t("Expiry")}</TableHeaderCell>
                <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
              </TableRow>
            </TableHeader>
            <TableBody>
              {orders.map((order: Order) => {
                const totalAmount = order.amount + order.vat_amount;
                const isCopied = copiedId === order.transfer_code;
                const isUsageOrder = order.kind === "usage";
                const canManageOrders = effectiveRole === "billing" || effectiveRole === "system_admin";
                const canWriteOffUsage = effectiveRole === "system_admin";

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
                        label={t(ORDER_KIND_LABELS[order.kind] ?? order.kind)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5 font-mono text-body font-semibold">
                        <span>{order.transfer_code}</span>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => handleCopyTransferCode(order.transfer_code)}
                          aria-label={t("Copy transfer code")}
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
                        {order.seats} {t("seats")}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="body" weight="medium" hasTabularNumbers as="div">
                        {formatMinorAmount(totalAmount, order.currency)}
                      </Text>
                      <Text type="supporting" hasTabularNumbers as="div">
                        {t("Net:")} {formatMinorAmount(order.amount, order.currency)} {t("+ VAT:")} {formatMinorAmount(order.vat_amount, order.currency)}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="body" as="div">
                        {order.buyer.invoice_email || "-"}
                      </Text>
                      {order.buyer.tax_code && (
                        <Text type="code" color="secondary" as="div">
                          {t("MST:")} {order.buyer.tax_code}
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
                      <RowActions
                        label={t("order {0}", order.transfer_code)}
                        actions={[
                          ...(activeTab === "pending" && canManageOrders
                            ? [
                                { label: t("Confirm"), primary: true, onSelect: () => handleOpenConfirmDialog(order) },
                                ...(isUsageOrder
                                  ? canWriteOffUsage
                                    ? [{ label: t("Write off"), destructive: true, onSelect: () => handleOpenCancelDialog(order) }]
                                    : []
                                  : [{ label: t("Cancel"), destructive: true, onSelect: () => handleOpenCancelDialog(order) }]),
                              ]
                            : []),
                          { label: t("History"), onSelect: () => setHistoryOrderId(order.id) },
                        ]}
                      />
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
        title={t("Confirm payment")}
        description={t("Confirm bank transfer payment for order {0}. This will apply the subscription immediately.", selectedOrderForConfirm?.transfer_code || "")}
        confirmLabel={t("Confirm payment")}
        isPending={confirmOrder.isPending}
        error={confirmError}
        onConfirm={handleConfirmPayment}
      >
        <Field>
          <FieldLabel htmlFor="transfer-reference-input" className="text-label">
            {t("Bank transfer reference (optional)")}
          </FieldLabel>
          <Input
            id="transfer-reference-input"
            value={transferReference}
            onChange={(e) => setTransferReference(e.target.value)}
            placeholder={t("e.g. FT26111600123")}
            disabled={confirmOrder.isPending}
          />
        </Field>
      </ConfirmReasonDialog>

      {/* Cancel / Write-off Reason Dialog */}
      <ConfirmReasonDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        title={isWriteOffTarget ? t("Write off usage order") : t("Cancel order")}
        description={
          isWriteOffTarget
            ? t("Write off pending usage order {0}? Its lines will stay excluded from future invoices.", selectedOrderForCancel?.transfer_code || "")
            : t("Are you sure you want to cancel pending order {0}? This action cannot be undone.", selectedOrderForCancel?.transfer_code || "")
        }
        confirmLabel={isWriteOffTarget ? t("Write off order") : t("Cancel order")}
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
            <DialogTitle>{t("E-invoice history")}</DialogTitle>
          </DialogHeader>

          {historyLoading && <LoadingState message={t("Loading e-invoice history...")} />}

          {historyIsError && (
            <ErrorState
              title={t("Failed to load e-invoice history")}
              message={
                historyError instanceof PlatformApiError || historyError instanceof Error
                  ? historyError.message
                  : t("An error occurred while loading e-invoice history.")
              }
              onRetry={refetchHistory}
            />
          )}

          {!historyLoading && !historyIsError && sortedHistory.length === 0 && (
            <EmptyState title={t("No e-invoice for this order")} isCompact />
          )}

          {!historyLoading && !historyIsError && sortedHistory.length > 0 && (
            <Card padding={0} className="overflow-x-auto">
              <Table density="compact" dividers="rows" hasHover>
                <TableHeader>
                  <TableRow isHeaderRow>
                    <TableHeaderCell>{t("Revision")}</TableHeaderCell>
                    <TableHeaderCell>{t("State")}</TableHeaderCell>
                    <TableHeaderCell>{t("Invoice number")}</TableHeaderCell>
                    <TableHeaderCell>{t("Buyer")}</TableHeaderCell>
                    <TableHeaderCell>{t("Attempts")}</TableHeaderCell>
                    <TableHeaderCell>{t("Last error")}</TableHeaderCell>
                    <TableHeaderCell>{t("Updated")}</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedHistory.map((item: OrderEinvoice) => {
                    const safeUrl = isSafeHttpsUrl(item.lookup_url) ? item.lookup_url.trim() : "";

                    return (
                      <TableRow key={item.revision}>
                        <TableCell>
                          <Text type="code" color="secondary">
                            {t("rev")} {item.revision}
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
                              <span>{t("View")}</span>
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
              {t("Close")}
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
