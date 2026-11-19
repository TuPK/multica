"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertCircle,
  Building2,
  Calendar,
  Check,
  Copy,
  CreditCard,
  FileText,
} from "@multica/ui/icons";
import { api } from "@multica/core/api";
import { useCurrentWorkspace } from "@multica/core/paths";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@multica/ui/components/ui/card";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { useLocale, useT } from "../i18n";
import { useNavigation } from "../navigation";
import { formatStripeMinorAmount } from "../settings/components/billing-format";
import {
  EinvoiceStatusSummary,
  OrderBuyerForm,
  canEditEinvoiceBuyer,
  normalizeEinvoice,
  resolveBuyerSaveErrorMessage,
  validateTaxCode,
  type OrderEinvoice,
} from "./einvoice-section";
import { UsageInvoiceLines } from "./usage-invoice-lines";

export interface BuyerInfo {
  name: string;
  taxCode: string;
  address: string;
  invoiceEmail: string;
}

export interface BankTransferDetails {
  bankName: string;
  accountName: string;
  accountNumber: string;
}

export interface BillingOrder {
  id: string;
  workspaceId: string;
  kind: "subscribe" | "renew" | "add_seats" | "change_plan" | "usage" | string;
  planKey: string;
  interval: "month" | "year" | string;
  seats: number;
  amount: number;
  vatAmount: number;
  currency: string;
  status: "pending" | "paid" | "expired" | "canceled" | "refunded" | string;
  provider?: string;
  transferCode: string;
  expiresAt: string | null;
  createdAt?: string;
  bankDetails?: BankTransferDetails;
  bankName?: string;
  accountName?: string;
  accountNumber?: string;
  buyer?: Partial<BuyerInfo>;
  einvoice?: OrderEinvoice | null;
}

export interface BillingOrderPageProps {
  orderId?: string;
  initialOrder?: BillingOrder;
}

export { validateTaxCode };

export function normalizeOrder(raw: any): BillingOrder {
  return {
    id: raw.id ?? "",
    workspaceId: raw.workspace_id ?? raw.workspaceId ?? "",
    kind: raw.kind ?? "subscribe",
    planKey: raw.plan_key ?? raw.planKey ?? "",
    interval: raw.interval ?? "month",
    seats: raw.seats ?? 1,
    amount: raw.amount ?? 0,
    vatAmount: raw.vat_amount ?? raw.vatAmount ?? 0,
    currency: raw.currency ?? "",
    status: raw.status ?? "pending",
    provider: raw.provider ?? "manual",
    transferCode: raw.transfer_code ?? raw.transferCode ?? "",
    expiresAt: raw.expires_at ?? raw.expiresAt ?? "",
    createdAt: raw.created_at ?? raw.createdAt ?? "",
    bankDetails: {
      bankName:
        raw.bank_details?.bank_name ??
        raw.bank_details?.bankName ??
        raw.bank_name ??
        raw.bankName ??
        "",
      accountName:
        raw.bank_details?.account_name ??
        raw.bank_details?.accountName ??
        raw.account_name ??
        raw.accountName ??
        "",
      accountNumber:
        raw.bank_details?.account_number ??
        raw.bank_details?.accountNumber ??
        raw.account_number ??
        raw.accountNumber ??
        "",
    },
    buyer: {
      name: raw.buyer?.name ?? "",
      taxCode: raw.buyer?.tax_code ?? raw.buyer?.taxCode ?? "",
      address: raw.buyer?.address ?? "",
      invoiceEmail: raw.buyer?.invoice_email ?? raw.buyer?.invoiceEmail ?? "",
    },
    einvoice: normalizeEinvoice(raw.einvoice),
  };
}

export function BillingOrderPage({
  orderId: propOrderId,
  initialOrder,
}: BillingOrderPageProps) {
  const { t } = useT("billing");
  const locale = useLocale();
  const navigation = useNavigation();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";

  const orderId =
    propOrderId ||
    navigation.searchParams.get("order_id") ||
    navigation.searchParams.get("orderId") ||
    navigation.searchParams.get("id") ||
    "";

  const [copied, setCopied] = useState(false);
  const [buyerName, setBuyerName] = useState(initialOrder?.buyer?.name ?? "");
  const [buyerTaxCode, setBuyerTaxCode] = useState(
    initialOrder?.buyer?.taxCode ?? "",
  );
  const [buyerAddress, setBuyerAddress] = useState(
    initialOrder?.buyer?.address ?? "",
  );
  const [buyerEmail, setBuyerEmail] = useState(
    initialOrder?.buyer?.invoiceEmail ?? "",
  );
  const [taxCodeError, setTaxCodeError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [formSuccess, setFormSuccess] = useState(false);

  const queryClient = useQueryClient();

  const orderQuery = useQuery({
    queryKey: ["workspace-orders", wsId, "detail", orderId],
    queryFn: async (): Promise<BillingOrder> => {
      if ((api as any).getWorkspaceOrder) {
        const res = await (api as any).getWorkspaceOrder(wsId, orderId);
        if (res) return normalizeOrder(res);
      }
      if ((api as any).getWorkspaceOrders) {
        const list = await (api as any).getWorkspaceOrders(wsId);
        const orders = Array.isArray(list) ? list : (list?.orders ?? []);
        const found = orders.find((o: any) => o.id === orderId);
        if (found) return normalizeOrder(found);
      }
      if (initialOrder) return initialOrder;
      throw new Error("Order not found");
    },
    enabled: !!orderId && !initialOrder,
    initialData: initialOrder,
  });

  const order = orderQuery.data;

  useEffect(() => {
    if (order?.buyer) {
      if (order.buyer.name !== undefined) setBuyerName(order.buyer.name);
      if (order.buyer.taxCode !== undefined)
        setBuyerTaxCode(order.buyer.taxCode);
      if (order.buyer.address !== undefined)
        setBuyerAddress(order.buyer.address);
      if (order.buyer.invoiceEmail !== undefined)
        setBuyerEmail(order.buyer.invoiceEmail);
    }
  }, [order?.buyer]);

  const updateBuyerMutation = useMutation({
    mutationFn: async (data: {
      name: string;
      tax_code: string;
      address: string;
      invoice_email: string;
      mode: "order" | "einvoice";
    }) => {
      const buyerPayload = {
        name: data.name,
        taxCode: data.tax_code,
        address: data.address,
        invoiceEmail: data.invoice_email,
      };
      if (data.mode === "einvoice") {
        return (api as any).updateWorkspaceOrderInvoiceBuyer(
          wsId,
          orderId,
          buyerPayload,
        );
      }
      if (typeof (api as any).updateWorkspaceOrderBuyer === "function") {
        return (api as any).updateWorkspaceOrderBuyer(
          wsId,
          orderId,
          buyerPayload,
        );
      }
      if (typeof (api as any).updateOrderBuyer === "function") {
        return (api as any).updateOrderBuyer(wsId, orderId, data);
      }
      return data;
    },
    onSuccess: (updated) => {
      setFormSuccess(true);
      setFormError(null);
      if (
        updated &&
        typeof updated === "object" &&
        "id" in updated &&
        typeof (updated as { id?: unknown }).id === "string" &&
        (updated as { id: string }).id !== ""
      ) {
        queryClient.setQueryData(
          ["workspace-orders", wsId, "detail", orderId],
          normalizeOrder(updated),
        );
      }
      void queryClient.invalidateQueries({
        queryKey: ["workspace-orders", wsId, "list"],
      });
    },
    onError: (err: unknown, variables) => {
      setFormSuccess(false);
      setFormError(
        resolveBuyerSaveErrorMessage(err, t, variables.mode === "einvoice"),
      );
    },
  });

  const handleCopyCode = async () => {
    const code = order?.transferCode;
    if (!code) return;
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(code);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // ignore
    }
  };

  const handleBuyerSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    setFormSuccess(false);

    const isOrderPending = order?.status === "pending";
    const isInvoiceEditable = canEditEinvoiceBuyer(order?.einvoice);
    if (order && !isOrderPending && !isInvoiceEditable) {
      setFormError(t(($) => $.order.order_not_pending));
      return;
    }

    if (!validateTaxCode(buyerTaxCode)) {
      setTaxCodeError(t(($) => $.order.tax_code_invalid));
      return;
    }
    setTaxCodeError(null);

    updateBuyerMutation.mutate({
      name: buyerName.trim(),
      tax_code: buyerTaxCode.trim(),
      address: buyerAddress.trim(),
      invoice_email: buyerEmail.trim(),
      mode: !isOrderPending && isInvoiceEditable ? "einvoice" : "order",
    });
  };

  if (orderQuery.isLoading && !order) {
    return (
      <div className="space-y-6 p-6" role="status" aria-label={t(($) => $.order.loading)}>
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-60 w-full" />
      </div>
    );
  }

  if ((orderQuery.isError || !order) && !initialOrder) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>{t(($) => $.order.load_failed)}</AlertTitle>
          <AlertDescription>{t(($) => $.order.not_found)}</AlertDescription>
        </Alert>
      </div>
    );
  }

  if (!order) return null;

  const subtotalFormatted =
    formatStripeMinorAmount(order.amount, order.currency, locale) ??
    String(order.amount);
  const vatFormatted =
    formatStripeMinorAmount(order.vatAmount, order.currency, locale) ??
    String(order.vatAmount);
  const totalFormatted =
    formatStripeMinorAmount(
      order.amount + order.vatAmount,
      order.currency,
      locale,
    ) ?? String(order.amount + order.vatAmount);

  const formattedExpiry = order.expiresAt
    ? new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(order.expiresAt))
    : "";

  const isPending = order.status === "pending";
  const canEditBuyer = isPending || canEditEinvoiceBuyer(order.einvoice);
  const bank = order.bankDetails ?? {
    bankName: order.bankName ?? "",
    accountName: order.accountName ?? "",
    accountNumber: order.accountNumber ?? "",
  };

  const statusVariant =
    order.status === "paid"
      ? "default"
      : order.status === "pending"
        ? "outline"
        : "secondary";

  const statusText =
    order.status === "pending"
      ? t(($) => $.order.status.pending)
      : order.status === "paid"
        ? t(($) => $.order.status.paid)
        : order.status === "expired"
          ? t(($) => $.order.status.expired)
          : order.status === "canceled"
            ? t(($) => $.order.status.canceled)
            : order.status === "refunded"
              ? t(($) => $.order.status.refunded)
              : order.status;

  return (
    <div className="mx-auto max-w-4xl space-y-6 p-4 sm:p-6">
      <header className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-title-lg font-semibold">
            {t(($) => $.order.title)}
          </h1>
          {formattedExpiry ? (
            <p className="mt-1 text-caption text-muted-foreground flex items-center gap-1.5">
              <Calendar className="h-3.5 w-3.5" />
              <span>
                {t(($) => $.order.expires_at)}: {formattedExpiry}
              </span>
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <span className="text-caption font-medium text-muted-foreground">
            {t(($) => $.order.status_label)}:
          </span>
          <Badge variant={statusVariant}>{statusText}</Badge>
        </div>
      </header>

      {/* Instructions Alert */}
      <Alert>
        <FileText />
        <AlertTitle>{t(($) => $.order.bank_details_title)}</AlertTitle>
        <AlertDescription>
          {t(($) => $.order.transfer_instructions)}
        </AlertDescription>
      </Alert>

      {/* Transfer Details Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-title-md flex items-center gap-2">
            <Building2 className="h-5 w-5" />
            {t(($) => $.order.bank_details_title)}
          </CardTitle>
          <CardDescription>
            {t(($) => $.order.transfer_instructions)}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg border border-surface-border p-3">
              <span className="text-caption text-muted-foreground">
                {t(($) => $.order.bank_name)}
              </span>
              <p className="text-body font-medium">{bank.bankName || "—"}</p>
            </div>
            <div className="rounded-lg border border-surface-border p-3">
              <span className="text-caption text-muted-foreground">
                {t(($) => $.order.account_name)}
              </span>
              <p className="text-body font-medium">{bank.accountName || "—"}</p>
            </div>
            <div className="rounded-lg border border-surface-border p-3 sm:col-span-2">
              <span className="text-caption text-muted-foreground">
                {t(($) => $.order.account_number)}
              </span>
              <p className="text-body font-semibold tabular-nums tracking-wide">
                {bank.accountNumber || "—"}
              </p>
            </div>
          </div>

          {/* Transfer Code Box */}
          <div className="flex flex-col gap-2 rounded-lg bg-surface-muted p-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <span className="text-caption font-medium text-muted-foreground">
                {t(($) => $.order.transfer_code)}
              </span>
              <p className="text-title-md font-bold tracking-wider text-foreground">
                {order.transferCode}
              </p>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleCopyCode}
              aria-label={t(($) => $.order.copy_code)}
              className="gap-1.5 self-start sm:self-auto"
            >
              {copied ? (
                <Check className="h-4 w-4 text-success" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
              <span>
                {copied ? t(($) => $.order.copied) : t(($) => $.order.copy_code)}
              </span>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Amounts Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-title-md flex items-center gap-2">
            <CreditCard className="h-5 w-5" />
            {t(($) => $.order.total)}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex justify-between border-b border-surface-border pb-2 text-body">
            <span className="text-muted-foreground">
              {t(($) => $.order.amount)}
            </span>
            <span className="tabular-nums font-medium">{subtotalFormatted}</span>
          </div>
          <div className="flex justify-between border-b border-surface-border pb-2 text-body">
            <span className="text-muted-foreground">
              {t(($) => $.order.vat)}
            </span>
            <span className="tabular-nums font-medium">{vatFormatted}</span>
          </div>
          <div className="flex justify-between pt-1 text-title-md font-bold">
            <span>{t(($) => $.order.total)}</span>
            <span className="tabular-nums text-foreground">{totalFormatted}</span>
          </div>
        </CardContent>
      </Card>

      {order.kind === "usage" ? (
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} className="text-title-md">
              {t(($) => $.order.usage_lines_title)}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <UsageInvoiceLines
              wsId={order.workspaceId || wsId}
              orderId={order.id}
              currency={order.currency}
            />
          </CardContent>
        </Card>
      ) : null}

      {order.einvoice ? (
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} className="text-title-md">
              {t(($) => $.order.einvoice.title)}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <EinvoiceStatusSummary
              einvoice={order.einvoice}
              showHeading={false}
            />
          </CardContent>
        </Card>
      ) : null}

      {/* Buyer Details Form Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-title-md">
            {t(($) => $.order.buyer_title)}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <OrderBuyerForm
            idPrefix="buyer"
            name={buyerName}
            taxCode={buyerTaxCode}
            address={buyerAddress}
            invoiceEmail={buyerEmail}
            onNameChange={setBuyerName}
            onTaxCodeChange={(value) => {
              setBuyerTaxCode(value);
              if (taxCodeError) setTaxCodeError(null);
            }}
            onAddressChange={setBuyerAddress}
            onEmailChange={setBuyerEmail}
            disabled={!canEditBuyer}
            isSubmitting={updateBuyerMutation.isPending}
            taxCodeError={taxCodeError}
            formError={formError}
            formSuccess={formSuccess}
            onSubmit={handleBuyerSubmit}
          />
        </CardContent>
      </Card>
    </div>
  );
}
