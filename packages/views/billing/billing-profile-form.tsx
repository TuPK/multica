"use client";

import { useEffect, useId, useState, type FormEvent } from "react";
import type {
  UpdateWorkspaceBillingProfileRequest,
  WorkspaceBillingAccountKind,
  WorkspaceBillingProfile,
} from "@multica/core/types";
import { AlertCircle, Loader2 } from "@multica/ui/icons";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  SettingsCard,
  SettingsRow,
} from "../settings/components/settings-layout";
import { useT } from "../i18n";

export interface BillingProfileFormProps {
  profile: WorkspaceBillingProfile | null;
  canEdit: boolean;
  saving: boolean;
  error: string | null;
  onSave(data: UpdateWorkspaceBillingProfileRequest): void;
}

/** Billing profile form (or read-only summary when editing is disallowed). */
export function BillingProfileForm({
  profile,
  canEdit,
  saving,
  error,
  onSave,
}: BillingProfileFormProps) {
  const { t } = useT("billing");
  const baseId = useId();

  const storedKind: WorkspaceBillingAccountKind =
    profile?.accountKind === "business" ? "business" : "individual";
  const storedLegalName = profile?.legalName ?? "";
  const storedTaxCode = profile?.taxCode ?? "";
  const storedAddress = profile?.address ?? "";
  const storedInvoiceEmail = profile?.invoiceEmail ?? "";

  const [accountKind, setAccountKind] =
    useState<WorkspaceBillingAccountKind>(storedKind);
  const [legalName, setLegalName] = useState(storedLegalName);
  const [taxCode, setTaxCode] = useState(storedTaxCode);
  const [address, setAddress] = useState(storedAddress);
  const [invoiceEmail, setInvoiceEmail] = useState(storedInvoiceEmail);

  useEffect(() => {
    setAccountKind(storedKind);
    setLegalName(storedLegalName);
    setTaxCode(storedTaxCode);
    setAddress(storedAddress);
    setInvoiceEmail(storedInvoiceEmail);
  }, [
    storedKind,
    storedLegalName,
    storedTaxCode,
    storedAddress,
    storedInvoiceEmail,
  ]);

  if (!canEdit) {
    const isBusiness = storedKind === "business";
    const dash = t(($) => $.shared.date_dash);
    return (
      <SettingsCard>
        <SettingsRow label={t(($) => $.workspace.profile.account_kind)}>
          <span>
            {isBusiness
              ? t(($) => $.workspace.profile.business)
              : t(($) => $.workspace.profile.individual)}
          </span>
        </SettingsRow>
        {isBusiness ? (
          <>
            <SettingsRow label={t(($) => $.workspace.profile.legal_name)}>
              <span>{storedLegalName || dash}</span>
            </SettingsRow>
            <SettingsRow label={t(($) => $.workspace.profile.tax_code)}>
              <span className="tabular-nums">{storedTaxCode || dash}</span>
            </SettingsRow>
            <SettingsRow label={t(($) => $.workspace.profile.address)}>
              <span>{storedAddress || dash}</span>
            </SettingsRow>
            <SettingsRow label={t(($) => $.workspace.profile.invoice_email)}>
              <span>{storedInvoiceEmail || dash}</span>
            </SettingsRow>
          </>
        ) : null}
      </SettingsCard>
    );
  }

  const switchId = `${baseId}-business`;
  const legalNameId = `${baseId}-legal-name`;
  const taxCodeId = `${baseId}-tax-code`;
  const addressId = `${baseId}-address`;
  const invoiceEmailId = `${baseId}-invoice-email`;
  const isBusiness = accountKind === "business";

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isBusiness) {
      onSave({
        accountKind: "business",
        legalName,
        taxCode,
        address,
        invoiceEmail,
      });
      return;
    }
    onSave({
      accountKind: "individual",
      legalName: null,
      taxCode: null,
      address: null,
      invoiceEmail: null,
    });
  };

  return (
    <form noValidate onSubmit={handleSubmit}>
      <SettingsCard>
        <SettingsRow
          label={
            <Label htmlFor={switchId}>
              {t(($) => $.workspace.profile.business_switch)}
            </Label>
          }
        >
          <Switch
            id={switchId}
            checked={isBusiness}
            disabled={saving}
            aria-label={t(($) => $.workspace.profile.business_switch)}
            onCheckedChange={(checked) =>
              setAccountKind(checked ? "business" : "individual")
            }
          />
        </SettingsRow>

        {isBusiness ? (
          <>
            <SettingsRow
              label={
                <Label htmlFor={legalNameId}>
                  {t(($) => $.workspace.profile.legal_name)}
                </Label>
              }
              size="text"
            >
              <Input
                id={legalNameId}
                value={legalName}
                disabled={saving}
                onChange={(event) => setLegalName(event.currentTarget.value)}
              />
            </SettingsRow>
            <SettingsRow
              label={
                <Label htmlFor={taxCodeId}>
                  {t(($) => $.workspace.profile.tax_code)}
                </Label>
              }
              size="text"
            >
              <Input
                id={taxCodeId}
                value={taxCode}
                disabled={saving}
                onChange={(event) => setTaxCode(event.currentTarget.value)}
              />
            </SettingsRow>
            <SettingsRow
              label={
                <Label htmlFor={addressId}>
                  {t(($) => $.workspace.profile.address)}
                </Label>
              }
              size="text"
            >
              <Input
                id={addressId}
                value={address}
                disabled={saving}
                onChange={(event) => setAddress(event.currentTarget.value)}
              />
            </SettingsRow>
            <SettingsRow
              label={
                <Label htmlFor={invoiceEmailId}>
                  {t(($) => $.workspace.profile.invoice_email)}
                </Label>
              }
              size="text"
            >
              <Input
                id={invoiceEmailId}
                type="email"
                value={invoiceEmail}
                disabled={saving}
                onChange={(event) => setInvoiceEmail(event.currentTarget.value)}
              />
            </SettingsRow>
          </>
        ) : null}

        {error !== null ? (
          <div className="px-4 py-3">
            <Alert variant="destructive">
              <AlertCircle />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        ) : null}

        <div className="flex justify-end px-4 py-3.5">
          <Button
            type="submit"
            variant="default"
            disabled={saving}
            aria-busy={saving}
          >
            {saving ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : null}
            {t(($) => $.workspace.profile.save)}
          </Button>
        </div>
      </SettingsCard>
    </form>
  );
}
