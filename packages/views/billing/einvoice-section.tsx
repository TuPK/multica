"use client";

import { useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  Loader2,
} from "@multica/ui/icons";
import { ApiError, errorCode } from "@multica/core/api";
import {
  Alert,
  AlertDescription,
} from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../i18n";

export interface OrderEinvoice {
  state: string;
  number: string;
  lookupUrl: string;
  missingFields: string[];
}

export interface OrderBuyerValues {
  name: string;
  taxCode: string;
  address: string;
  invoiceEmail: string;
}

/** Validates an optional Vietnamese tax code (empty, 10 digits, or 13 digits). */
export function validateTaxCode(taxCode: string): boolean {
  const trimmed = taxCode.trim();
  if (trimmed === "") return true;
  const digitsOnly = trimmed.replace(/-/g, "");
  return /^\d{10}$/.test(digitsOnly) || /^\d{13}$/.test(digitsOnly);
}

/** Allows only syntactically valid `https:` URLs for e-invoice lookup links. */
export function isSafeHttpsUrl(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed.startsWith("https://")) return false;
  try {
    return new URL(trimmed).protocol === "https:";
  } catch {
    return false;
  }
}

export function canEditEinvoiceBuyer(
  einvoice: OrderEinvoice | null | undefined,
): boolean {
  return einvoice?.state === "needs_info" || einvoice?.state === "failed";
}

export function normalizeEinvoice(raw: unknown): OrderEinvoice | null {
  if (!raw || typeof raw !== "object") return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.state !== "string" || obj.state === "") return null;
  const rawUrl =
    typeof obj.lookupUrl === "string"
      ? obj.lookupUrl
      : typeof obj.lookup_url === "string"
        ? obj.lookup_url
        : "";
  const rawMissing = Array.isArray(obj.missingFields)
    ? obj.missingFields
    : Array.isArray(obj.missing_fields)
      ? obj.missing_fields
      : [];
  return {
    state: obj.state,
    number: typeof obj.number === "string" ? obj.number : "",
    lookupUrl: isSafeHttpsUrl(rawUrl) ? rawUrl.trim() : "",
    missingFields: rawMissing.filter(
      (field): field is string => typeof field === "string",
    ),
  };
}

function einvoiceBadgeVariant(
  state: string,
): "default" | "secondary" | "destructive" | "outline" {
  switch (state) {
    case "issued":
      return "default";
    case "needs_info":
    case "failed":
      return "destructive";
    case "pending":
      return "outline";
    default:
      return "secondary";
  }
}

export function resolveBuyerSaveErrorMessage(
  err: unknown,
  t: ReturnType<typeof useT<"billing">>["t"],
  preferEinvoiceConflict = false,
): string {
  if (err instanceof ApiError && err.status === 409) {
    const code = errorCode(err);
    if (
      code === "einvoice_already_issued" ||
      (preferEinvoiceConflict && code !== "order_not_pending")
    ) {
      return t(($) => $.order.einvoice.already_issued);
    }
    return t(($) => $.order.order_not_pending);
  }
  return t(($) => $.order.buyer_save_failed);
}

/** Displays e-invoice status, invoice number, missing buyer fields, and a safe https link when issued. */
export function EinvoiceStatusSummary({
  einvoice,
  showHeading = true,
}: {
  einvoice: OrderEinvoice;
  showHeading?: boolean;
}) {
  const { t } = useT("billing");

  const stateLabel = (() => {
    switch (einvoice.state) {
      case "pending":
        return t(($) => $.order.einvoice.status.pending);
      case "issued":
        return t(($) => $.order.einvoice.status.issued);
      case "needs_info":
        return t(($) => $.order.einvoice.status.needs_info);
      case "failed":
        return t(($) => $.order.einvoice.status.failed);
      case "cancelled":
        return t(($) => $.order.einvoice.status.cancelled);
      default:
        return t(($) => $.order.einvoice.status.unknown);
    }
  })();

  const fieldLabel = (field: string): string => {
    switch (field) {
      case "name":
        return t(($) => $.order.buyer_name);
      case "tax_code":
        return t(($) => $.order.buyer_tax_code);
      case "address":
        return t(($) => $.order.buyer_address);
      case "invoice_email":
        return t(($) => $.order.buyer_email);
      default:
        return field;
    }
  };

  const safeLookupUrl =
    einvoice.state === "issued" && isSafeHttpsUrl(einvoice.lookupUrl)
      ? einvoice.lookupUrl.trim()
      : "";

  const missingFieldsText =
    einvoice.missingFields.length > 0
      ? einvoice.missingFields.map(fieldLabel).join(", ")
      : "";

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        {showHeading ? (
          <h3 className="text-body font-semibold">
            {t(($) => $.order.einvoice.title)}
          </h3>
        ) : null}
        <Badge variant={einvoiceBadgeVariant(einvoice.state)}>
          {stateLabel}
        </Badge>
      </div>

      {einvoice.number ? (
        <div className="flex flex-wrap items-center justify-between gap-2 text-body">
          <span className="text-muted-foreground">
            {t(($) => $.order.einvoice.number_label)}
          </span>
          <span className="font-mono font-medium tabular-nums">
            {einvoice.number}
          </span>
        </div>
      ) : null}

      {missingFieldsText ? (
        <p className="text-caption text-destructive">
          {t(($) => $.order.einvoice.missing_fields, {
            fields: missingFieldsText,
          })}
        </p>
      ) : null}

      {safeLookupUrl ? (
        <div>
          <a
            href={safeLookupUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 text-body font-medium text-primary underline underline-offset-3 hover:opacity-80"
          >
            <ExternalLink className="size-4" />
            <span>{t(($) => $.order.einvoice.view_link)}</span>
          </a>
        </div>
      ) : null}
    </div>
  );
}

export interface OrderBuyerFormProps {
  idPrefix: string;
  name: string;
  taxCode: string;
  address: string;
  invoiceEmail: string;
  onNameChange: (value: string) => void;
  onTaxCodeChange: (value: string) => void;
  onAddressChange: (value: string) => void;
  onEmailChange: (value: string) => void;
  disabled: boolean;
  isSubmitting: boolean;
  taxCodeError: string | null;
  formError: string | null;
  formSuccess: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

/** Shared four-field buyer details form used on the order page and expanded paid orders in billing settings. */
export function OrderBuyerForm({
  idPrefix,
  name,
  taxCode,
  address,
  invoiceEmail,
  onNameChange,
  onTaxCodeChange,
  onAddressChange,
  onEmailChange,
  disabled,
  isSubmitting,
  taxCodeError,
  formError,
  formSuccess,
  onSubmit,
}: OrderBuyerFormProps) {
  const { t } = useT("billing");
  const inputsDisabled = disabled || isSubmitting;

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div>
        <label htmlFor={`${idPrefix}-name`} className="text-body font-medium">
          {t(($) => $.order.buyer_name)}
        </label>
        <Input
          id={`${idPrefix}-name`}
          name="name"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          disabled={inputsDisabled}
          className="mt-1"
        />
      </div>

      <div>
        <label
          htmlFor={`${idPrefix}-tax-code`}
          className="text-body font-medium"
        >
          {t(($) => $.order.buyer_tax_code)}
        </label>
        <Input
          id={`${idPrefix}-tax-code`}
          name="tax_code"
          value={taxCode}
          onChange={(e) => onTaxCodeChange(e.target.value)}
          disabled={inputsDisabled}
          className="mt-1"
        />
        {taxCodeError ? (
          <p className="mt-1 text-caption text-destructive" role="alert">
            {taxCodeError}
          </p>
        ) : null}
      </div>

      <div>
        <label
          htmlFor={`${idPrefix}-address`}
          className="text-body font-medium"
        >
          {t(($) => $.order.buyer_address)}
        </label>
        <Input
          id={`${idPrefix}-address`}
          name="address"
          value={address}
          onChange={(e) => onAddressChange(e.target.value)}
          disabled={inputsDisabled}
          className="mt-1"
        />
      </div>

      <div>
        <label htmlFor={`${idPrefix}-email`} className="text-body font-medium">
          {t(($) => $.order.buyer_email)}
        </label>
        <Input
          id={`${idPrefix}-email`}
          name="invoice_email"
          type="email"
          value={invoiceEmail}
          onChange={(e) => onEmailChange(e.target.value)}
          disabled={inputsDisabled}
          className="mt-1"
        />
      </div>

      {formError ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      {formSuccess ? (
        <Alert>
          <CheckCircle2 />
          <AlertDescription>{t(($) => $.order.buyer_saved)}</AlertDescription>
        </Alert>
      ) : null}

      <div className="pt-2">
        <Button
          type="submit"
          variant="default"
          disabled={inputsDisabled}
          aria-busy={isSubmitting}
        >
          {isSubmitting ? (
            <Loader2 className="animate-spin motion-reduce:animate-none" />
          ) : null}
          {t(($) => $.order.buyer_save)}
        </Button>
      </div>
    </form>
  );
}

/** Inline e-invoice buyer editor used inside an expanded paid order row when state is `needs_info` or `failed`. */
export function InlineInvoiceBuyerForm({
  idPrefix,
  initialBuyer,
  saving,
  onSave,
}: {
  idPrefix: string;
  initialBuyer: OrderBuyerValues;
  saving: boolean;
  onSave: (buyer: OrderBuyerValues) => Promise<void>;
}) {
  const { t } = useT("billing");
  const [name, setName] = useState(initialBuyer.name);
  const [taxCode, setTaxCode] = useState(initialBuyer.taxCode);
  const [address, setAddress] = useState(initialBuyer.address);
  const [invoiceEmail, setInvoiceEmail] = useState(initialBuyer.invoiceEmail);
  const [taxCodeError, setTaxCodeError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [formSuccess, setFormSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    setFormSuccess(false);

    if (!validateTaxCode(taxCode)) {
      setTaxCodeError(t(($) => $.order.tax_code_invalid));
      return;
    }
    setTaxCodeError(null);

    try {
      await onSave({
        name: name.trim(),
        taxCode: taxCode.trim(),
        address: address.trim(),
        invoiceEmail: invoiceEmail.trim(),
      });
      setFormSuccess(true);
    } catch (err: unknown) {
      setFormError(resolveBuyerSaveErrorMessage(err, t, true));
    }
  };

  return (
    <OrderBuyerForm
      idPrefix={idPrefix}
      name={name}
      taxCode={taxCode}
      address={address}
      invoiceEmail={invoiceEmail}
      onNameChange={setName}
      onTaxCodeChange={(value) => {
        setTaxCode(value);
        if (taxCodeError) setTaxCodeError(null);
      }}
      onAddressChange={setAddress}
      onEmailChange={setInvoiceEmail}
      disabled={false}
      isSubmitting={saving}
      taxCodeError={taxCodeError}
      formError={formError}
      formSuccess={formSuccess}
      onSubmit={(e) => void handleSubmit(e)}
    />
  );
}
