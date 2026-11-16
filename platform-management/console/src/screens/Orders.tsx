import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { Copy, Check } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { useOrders, useConfirmOrder, useCancelOrder, useMe } from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type { StaffRole } from "../components/types";
import type { Order, OrderStatus } from "../api/schemas";

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

  // Cancel dialog state
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [selectedOrderForCancel, setSelectedOrderForCancel] = useState<Order | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

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
      if (err instanceof PlatformApiError) {
        setConfirmError(err.message);
      } else if (err instanceof Error) {
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
      if (err instanceof PlatformApiError) {
        setCancelError(err.message);
      } else if (err instanceof Error) {
        setCancelError(err.message);
      } else {
        setCancelError("Failed to cancel order");
      }
    }
  };

  const formatCurrency = (amount: number, currency: string) => {
    return `${amount.toLocaleString()} ${currency.toUpperCase()}`;
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-heading text-title-lg font-bold">Orders</h1>
          <p className="text-body text-muted-foreground">
            Review and confirm bank transfer payments, view recent orders across workspaces.
          </p>
        </div>
      </div>

      <div className="flex border-b border-surface-border">
        <div role="tablist" className="flex gap-2">
          {STATUS_TABS.map((tab) => {
            const isActive = activeTab === tab.value;
            return (
              <button
                key={tab.value}
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
          title="No orders"
          description={`No orders found with status "${activeTab}".`}
        />
      )}

      {!isLoading && !isError && orders && orders.length > 0 && (
        <div className="rounded-lg border border-surface-border bg-surface overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Workspace</TableHead>
                <TableHead>Transfer Code</TableHead>
                <TableHead>Plan & Seats</TableHead>
                <TableHead>Total (incl. VAT)</TableHead>
                <TableHead>Buyer Email</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Expiry</TableHead>
                {activeTab === "pending" && <TableHead className="text-right">Actions</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {orders.map((order: Order) => {
                const totalAmount = order.amount + order.vat_amount;
                const isCopied = copiedId === order.transfer_code;

                return (
                  <TableRow key={order.id}>
                    <TableCell>
                      <div className="font-medium text-foreground">{order.workspace_name}</div>
                      <div className="font-mono text-caption text-muted-foreground">{order.workspace_id}</div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5 font-mono text-body font-semibold">
                        <span>{order.transfer_code}</span>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleCopyTransferCode(order.transfer_code)}
                          aria-label="Copy transfer code"
                          className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
                        >
                          {isCopied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-body font-medium capitalize">
                        {order.plan_key} &bull; {order.interval}
                      </div>
                      <div className="text-caption text-muted-foreground">{order.seats} seats</div>
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{formatCurrency(totalAmount, order.currency)}</div>
                      <div className="text-caption text-muted-foreground">
                        Net: {formatCurrency(order.amount, order.currency)} + VAT: {formatCurrency(order.vat_amount, order.currency)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-body">{order.buyer.invoice_email || "-"}</div>
                      {order.buyer.tax_code && (
                        <div className="text-caption text-muted-foreground">MST: {order.buyer.tax_code}</div>
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={order.status} />
                    </TableCell>
                    <TableCell className="text-caption text-muted-foreground">
                      {order.expires_at ? new Date(order.expires_at).toLocaleString() : "-"}
                    </TableCell>
                    {activeTab === "pending" && (
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            size="sm"
                            onClick={() => handleOpenConfirmDialog(order)}
                          >
                            Confirm
                          </Button>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => handleOpenCancelDialog(order)}
                          >
                            Cancel
                          </Button>
                        </div>
                      </TableCell>
                    )}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
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
        <div className="flex flex-col gap-1.5">
          <label htmlFor="transfer-reference-input" className="text-caption font-medium text-foreground">
            Bank transfer reference (optional)
          </label>
          <Input
            id="transfer-reference-input"
            value={transferReference}
            onChange={(e) => setTransferReference(e.target.value)}
            placeholder="e.g. FT26111600123"
            disabled={confirmOrder.isPending}
          />
        </div>
      </ConfirmReasonDialog>

      {/* Cancel Reason Dialog */}
      <ConfirmReasonDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        title="Cancel order"
        description={`Are you sure you want to cancel pending order ${selectedOrderForCancel?.transfer_code || ""}? This action cannot be undone.`}
        confirmLabel="Cancel order"
        confirmVariant="destructive"
        isPending={cancelOrder.isPending}
        error={cancelError}
        onConfirm={handleCancelOrder}
      />
    </div>
  );
}
