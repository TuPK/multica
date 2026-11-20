import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Selector } from "@astryxdesign/core/Selector";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { TabList, Tab } from "@astryxdesign/core/TabList";
import { CheckboxInput } from "@astryxdesign/core/CheckboxInput";
import { UserPlus } from "@multica/ui/icons";
import { RowActions, useRowActionsWidth, type RowAction } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import {
  useMe,
  usePlans,
  useAccountRequests,
  useCreateAccountRequest,
  useCancelAccountRequest,
  useResendWelcome,
  AccountApiError,
  type CreateAccountInput } from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type { StaffRole } from "../components/types";
import type { AccountRequest, AccountRequestState, Plan } from "../api/schemas";
import { t } from "../i18n";

interface AccountsProps {
  userRole?: StaffRole;
}

type StateFilter = Exclude<AccountRequestState, "unknown"> | "";

const STATE_TABS: { value: string; filter: StateFilter; label: string }[] = [
  { value: "all", filter: "", label: "All" },
  { value: "pending", filter: "pending", label: "Pending" },
  { value: "provisioned", filter: "provisioned", label: "Provisioned" },
  { value: "completed", filter: "completed", label: "Completed" },
  { value: "failed", filter: "failed", label: "Failed" },
  { value: "canceled", filter: "canceled", label: "Canceled" },
];

const ACCOUNT_KIND_ITEMS = [
  { value: "individual", label: "Individual" },
  { value: "business", label: "Business" },
];

const INTERVAL_ITEMS = [
  { value: "month", label: "Monthly" },
  { value: "year", label: "Yearly" },
];

const ERROR_CODE_LABELS: Record<string, string> = {
  email_exists: "Email already exists",
  email_domain_blocked: "Email domain blocked",
  slug_unavailable: "Workspace slug unavailable",
  finalize_failed: "Subscription finalization failed",
  internal: "Internal error" };

const FIELD_ERROR_MESSAGES: Record<string, string> = {
  kind: "Select a valid account kind.",
  email: "Enter a valid email address (max 254 characters).",
  display_name: "Display name must be between 1 and 100 characters.",
  workspace_name: "Workspace name must be between 1 and 100 characters.",
  workspace_slug: "Use lowercase letters, numbers, and hyphens only (max 63 characters).",
  plan_key: "Select a valid, active plan.",
  seats: "Seats do not meet the limits for this plan and account kind.",
  interval: "Select a valid billing interval.",
  trial: "Trial is not available for this plan.",
  comp_reason: "Complimentary reason is invalid or exceeds 500 characters.",
  billing_profile: "Billing profile is required for business accounts only.",
  "billing_profile.legal_name": "Legal name is required.",
  "billing_profile.tax_code": "Enter a valid 10-digit (or 10-3) tax code.",
  "billing_profile.address": "Address is required.",
  "billing_profile.invoice_email": "Enter a valid invoice email address." };

function createIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `acct-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function formatRetryAfter(retryAfter: string | null): string | null {
  if (!retryAfter) return null;
  const trimmed = retryAfter.trim();
  if (/^\d+$/.test(trimmed)) {
    const seconds = Number.parseInt(trimmed, 10);
    if (seconds <= 0) return null;
    if (seconds < 60 || seconds % 60 !== 0) {
      return `${seconds} ${seconds === 1 ? "second" : "seconds"}`;
    }
    const minutes = seconds / 60;
    return `${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
  }
  const targetMs = Date.parse(trimmed);
  if (!Number.isNaN(targetMs)) {
    const seconds = Math.max(1, Math.ceil((targetMs - Date.now()) / 1000));
    return `${seconds} ${seconds === 1 ? "second" : "seconds"}`;
  }
  return trimmed;
}

function formatRateLimitMessage(prefix: string, retryAfter: string | null): string {
  const wait = formatRetryAfter(retryAfter);
  return wait
    ? t("{0} Try again in {1}.", prefix, wait)
    : t("{0} Please wait before trying again.", prefix);
}

function extractErrorDetails(err: unknown): {
  status: number | null;
  code: string | null;
  message: string | null;
  fields: string[];
  retryAfter: string | null;
} {
  if (err instanceof AccountApiError) {
    return {
      status: err.status,
      code: err.code,
      message: err.message,
      fields: err.fields,
      retryAfter: err.retryAfter };
  }
  if (err instanceof PlatformApiError) {
    return {
      status: err.status,
      code: err.code,
      message: err.message,
      fields: [],
      retryAfter: null };
  }
  if (err instanceof Error) {
    return { status: null, code: null, message: err.message, fields: [], retryAfter: null };
  }
  return { status: null, code: null, message: null, fields: [], retryAfter: null };
}

function isPaidPlan(planKey: string, selectedPlan: Plan | undefined): boolean {
  if (selectedPlan) {
    if (selectedPlan.key === "free") return false;
    const monthZero = selectedPlan.seat_price_month === 0;
    const yearZero = selectedPlan.seat_price_year === null || selectedPlan.seat_price_year === 0;
    return !(monthZero && yearZero);
  }
  return planKey.trim().toLowerCase() !== "free";
}

function formatErrorCode(code: string): string {
  if (!code) return "-";
  return t(ERROR_CODE_LABELS[code] ?? code);
}

function AccountStateBadge({ req }: { req: AccountRequest }) {
  if (req.state === "failed" && req.error_code === "finalize_failed") {
    return <StatusBadge status="needs_review" label={t("Finalize failed")} />;
  }
  return <StatusBadge status={req.state} />;
}

export function Accounts({ userRole }: AccountsProps) {
  const actionsWidth = useRowActionsWidth(200);
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const [stateFilter, setStateFilter] = useState<StateFilter>("");
  const { data: requests, isLoading, isError, error, refetch } = useAccountRequests(stateFilter);
  const { data: plans } = usePlans();
  const activePlans = (plans ?? []).filter((p) => !p.archived_at);

  const createAccount = useCreateAccountRequest();
  const cancelAccount = useCancelAccountRequest();
  const resendWelcome = useResendWelcome();

  // Create form dialog state
  const [createOpen, setCreateOpen] = useState(false);
  const [idempotencyKey, setIdempotencyKey] = useState("");
  const [kind, setKind] = useState<"individual" | "business">("individual");
  const [email, setEmail] = useState("");
  const [confirmEmail, setConfirmEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceSlug, setWorkspaceSlug] = useState("");
  const [planKey, setPlanKey] = useState("free");
  const [seats, setSeats] = useState("1");
  const [interval, setInterval] = useState<"month" | "year">("month");
  const [trial, setTrial] = useState(false);
  const [compReason, setCompReason] = useState("");
  const [legalName, setLegalName] = useState("");
  const [taxCode, setTaxCode] = useState("");
  const [address, setAddress] = useState("");
  const [invoiceEmail, setInvoiceEmail] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  // List row action feedback state
  const [actionNotice, setActionNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [resendingId, setResendingId] = useState<string | null>(null);

  // Cancel confirmation dialog state
  const [cancelTarget, setCancelTarget] = useState<AccountRequest | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

  if (effectiveRole && effectiveRole !== "system_admin") {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  const effectivePlanKey =
    activePlans.length === 0 || activePlans.some((p) => p.key === planKey)
      ? planKey
      : activePlans[0]!.key;
  const selectedPlan = activePlans.find((p) => p.key === effectivePlanKey);
  const showMismatchLive =
    confirmEmail !== "" && email.trim().toLowerCase() !== confirmEmail.trim().toLowerCase();
  const confirmEmailError =
    fieldErrors.confirm_email ?? (showMismatchLive ? t("Email addresses do not match.") : undefined);

  const clearFieldError = (name: string) => {
    setFieldErrors((prev) => {
      if (!(name in prev)) return prev;
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const resetForm = (nextKind: "individual" | "business" = "individual") => {
    const defaultPlanKey = nextKind === "business" ? "business" : "free";
    const defaultPlan = activePlans.find((p) => p.key === defaultPlanKey);
    setKind(nextKind);
    setEmail("");
    setConfirmEmail("");
    setDisplayName("");
    setWorkspaceName("");
    setWorkspaceSlug("");
    setPlanKey(defaultPlanKey);
    setSeats(String(defaultPlan?.min_seats ?? 1));
    setInterval("month");
    setTrial(false);
    setCompReason("");
    setLegalName("");
    setTaxCode("");
    setAddress("");
    setInvoiceEmail("");
    setFieldErrors({});
    setFormError(null);
  };

  const handleOpenCreate = () => {
    resetForm("individual");
    setIdempotencyKey(createIdempotencyKey());
    setCreateOpen(true);
  };

  const handleKindChange = (nextKind: "individual" | "business") => {
    setKind(nextKind);
    clearFieldError("kind");
    if (nextKind === "business" && planKey === "free") {
      const bizPlan = activePlans.find((p) => p.key === "business");
      setPlanKey("business");
      if (bizPlan?.min_seats) setSeats(String(bizPlan.min_seats));
    } else if (nextKind === "individual" && planKey === "business") {
      setPlanKey("free");
      setSeats("1");
      setTrial(false);
      setCompReason("");
    }
  };

  const handlePlanChange = (nextKey: string) => {
    setPlanKey(nextKey);
    clearFieldError("plan_key");
    clearFieldError("activation");
    const plan = activePlans.find((p) => p.key === nextKey);
    if (plan?.min_seats && Number(seats) < plan.min_seats) {
      setSeats(String(plan.min_seats));
    }
  };

  const validateClient = (): Record<string, string> => {
    const errs: Record<string, string> = {};
    const trimmedEmail = email.trim();
    const trimmedConfirm = confirmEmail.trim();

    if (!trimmedEmail) {
      errs.email = t("Email is required.");
    }
    if (!trimmedConfirm) {
      errs.confirm_email = t("Please confirm the email address.");
    } else if (trimmedEmail.toLowerCase() !== trimmedConfirm.toLowerCase()) {
      errs.confirm_email = t("Email addresses do not match.");
    }

    const trimmedDisplay = displayName.trim();
    if (!trimmedDisplay || trimmedDisplay.length > 100) {
      errs.display_name = t("Display name must be between 1 and 100 characters.");
    }

    const trimmedWorkspace = workspaceName.trim();
    if (!trimmedWorkspace || trimmedWorkspace.length > 100) {
      errs.workspace_name = t("Workspace name must be between 1 and 100 characters.");
    }

    const parsedSeats = Number(seats);
    if (!Number.isInteger(parsedSeats) || parsedSeats < 1) {
      errs.seats = t("Seats must be a whole number of at least 1.");
    }

    const paid = isPaidPlan(effectivePlanKey, selectedPlan);
    const hasReason = compReason.trim() !== "";
    if (trial && hasReason) {
      errs.activation = t("Choose either a trial or a complimentary reason, not both.");
    } else if (paid && !trial && !hasReason) {
      errs.activation = t("A paid plan requires either a trial or a complimentary reason.");
    } else if (!paid && (trial || hasReason)) {
      errs.activation = t("Free plans activate without a trial or complimentary reason.");
    }

    if (kind === "business") {
      if (!legalName.trim()) errs["billing_profile.legal_name"] = "Legal name is required.";
      if (!taxCode.trim()) errs["billing_profile.tax_code"] = "Tax code is required.";
      if (!address.trim()) errs["billing_profile.address"] = "Address is required.";
      if (!invoiceEmail.trim()) errs["billing_profile.invoice_email"] = "Invoice email is required.";
    }

    return errs;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (createAccount.isPending) return;

    setFormError(null);
    const clientErrors = validateClient();
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors);
      return;
    }
    setFieldErrors({});

    const input: CreateAccountInput = {
      kind,
      email: email.trim().toLowerCase(),
      display_name: displayName.trim(),
      workspace_name: workspaceName.trim(),
      workspace_slug: workspaceSlug.trim(),
      plan_key: effectivePlanKey.trim(),
      seats: Number(seats),
      interval,
      trial,
      comp_reason: compReason.trim(),
      ...(kind === "business"
        ? {
            billing_profile: {
              legal_name: legalName.trim(),
              tax_code: taxCode.trim(),
              address: address.trim(),
              invoice_email: invoiceEmail.trim().toLowerCase() } }
        : {}) };

    try {
      await createAccount.mutateAsync({ input, idempotencyKey });
      setCreateOpen(false);
      setActionError(null);
      setActionNotice(t("Account request submitted for {0}.", input.email));
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      switch (details.code) {
        case "account_request_invalid": {
          const nextFields: Record<string, string> = {};
          for (const f of details.fields) {
            nextFields[f] = t(FIELD_ERROR_MESSAGES[f] ?? "Invalid value.");
          }
          setFieldErrors(nextFields);
          setFormError(details.message || t("Check the highlighted fields and try again."));
          break;
        }
        case "account_activation_required":
          setFieldErrors({
            activation: t("Trial or complimentary reason required.") });
          setFormError(details.message || t("A paid plan requires either a trial or a complimentary reason."));
          break;
        case "account_email_exists":
          setFieldErrors({ email: t("Email already registered.") });
          setFormError(details.message || t("An account with this email already exists."));
          break;
        case "plan_audience_mismatch":
          setFieldErrors({ plan_key: t("Not available for this account kind.") });
          setFormError(details.message || t("This plan is not sold for this account kind."));
          break;
        case "account_create_rate_limited":
          setFormError(formatRateLimitMessage(t("Account creation rate limit reached."), details.retryAfter));
          break;
        default:
          if (details.status === 429) {
            setFormError(formatRateLimitMessage(t("Account creation rate limit reached."), details.retryAfter));
          } else {
            setFormError(t("Unable to create account request. Please try again."));
          }
      }
    }
  };

  const handleConfirmCancel = async () => {
    if (!cancelTarget || cancelAccount.isPending) return;
    setCancelError(null);
    try {
      await cancelAccount.mutateAsync({ id: cancelTarget.id });
      setActionError(null);
      setActionNotice(t("Canceled account request for {0}.", cancelTarget.email));
      setCancelTarget(null);
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      setCancelError(details.message || t("Failed to cancel account request."));
    }
  };

  const handleResendWelcome = async (req: AccountRequest) => {
    if (resendingId !== null) return;
    setResendingId(req.id);
    setActionError(null);
    setActionNotice(null);
    try {
      await resendWelcome.mutateAsync({ id: req.id });
      setActionNotice(t("Welcome email resent to {0}.", req.email));
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      if (details.status === 429) {
        setActionError(formatRateLimitMessage(t("Welcome email rate limit reached."), details.retryAfter));
      } else if (details.status === 502) {
        setActionError(t("Failed to send welcome email. Please try again later."));
      } else {
        setActionError(
          details.status !== null && details.status < 500 && details.message
            ? details.message
            : t("Failed to resend welcome email."),
        );
      }
    } finally {
      setResendingId(null);
    }
  };

  const requestList = requests ?? [];
  const planItems = activePlans.map((p) => ({
    value: p.key,
    label: `${p.name} (${p.key})` }));

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Account requests")}
        actions={
          <Button
            label={t("Create account")}
            variant="primary"
            size="sm"
            icon={<UserPlus className="size-4" aria-hidden="true" />}
            onClick={handleOpenCreate}
            className="gap-1.5"
          />
        }
      />

      {actionNotice && (
        <Banner
          status="info"
          role="status"
          title={actionNotice}
        />
      )}

      {actionError && (
        <Banner
          status="error"
          role="alert"
          title={actionError}
        />
      )}

      <div className="border-b border-surface-border">
        <TabList
          role="tablist"
          value={stateFilter || "all"}
          onChange={(value) => {
            const found = STATE_TABS.find((t) => t.value === value);
            setStateFilter(found ? found.filter : "");
          }}
          aria-label={t("Account request state")}
        >
          {STATE_TABS.map((tab) => (
            <Tab key={tab.value} value={tab.value} label={t(tab.label)} />
          ))}
        </TabList>
      </div>

      {isLoading && <LoadingState message={t("Loading account requests...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load account requests")}
          message={error instanceof Error ? error.message : t("An error occurred while loading account requests.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && requestList.length === 0 && (
        <EmptyState
          icon={<UserPlus className="size-4" />}
          title={t("No account requests")}
          description={
            stateFilter
              ? t("No account requests with state \"{0}\".", stateFilter)
              : t("Create an account request to provision a user and workspace.")
          }
          actions={
            stateFilter ? (
              <Button
                label={t("Show all")}
                variant="secondary"
                size="sm"
                onClick={() => setStateFilter("")}
              />
            ) : (
              <Button
                label={t("Create account")}
                variant="primary"
                size="sm"
                onClick={handleOpenCreate}
              />
            )
          }
        />
      )}

      {!isLoading && !isError && requestList.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table
            data={requestList}
            idKey="id"
            density="compact"
            dividers="rows"
            hasHover
            columns={[
              {
                key: "state",
                header: t("State"),
                width: proportional(1, { minWidth: 100 }),
                renderCell: (req) => (
                  <div className="flex flex-wrap items-center gap-1.5">
                    <AccountStateBadge req={req} />
                    {req.logged_in === false && <Badge variant="neutral" label={t("Never logged in")} />}
                  </div>
                ) },
              {
                key: "email",
                header: t("Email"),
                width: proportional(1.4, { minWidth: 120 }),
                renderCell: (req) => (
                  <>
                    <Text type="body" weight="medium" as="div" className="block">
                      {req.email}
                    </Text>
                    {req.display_name && (
                      <Text type="supporting" as="div" className="block">
                        {req.display_name}
                      </Text>
                    )}
                  </>
                ) },
              {
                key: "kind",
                header: t("Kind"),
                width: proportional(0.6, { minWidth: 56 }),
                renderCell: (req) => <Text type="body" className="capitalize">{req.kind}</Text> },
              {
                key: "plan_key",
                header: t("Plan"),
                width: proportional(1, { minWidth: 90 }),
                renderCell: (req) => (
                  <>
                    <Text type="body" weight="medium" as="div" className="block">
                      {req.plan_key}
                    </Text>
                    <Text type="supporting" hasTabularNumbers as="div" className="block">
                      {req.seats} {t("seats •")} {req.interval}
                    </Text>
                  </>
                ) },
              {
                key: "workspace_name",
                header: t("Workspace Name"),
                width: proportional(1, { minWidth: 90 }),
                renderCell: (req) => (
                  <>
                    <div className="truncate font-medium" title={req.workspace_name}>
                      {req.workspace_name}
                    </div>
                    {req.workspace_slug && (
                      <div
                        className="truncate font-mono text-caption text-muted-foreground"
                        title={req.workspace_slug}
                      >
                        {req.workspace_slug}
                      </div>
                    )}
                  </>
                ) },
              {
                key: "error_code",
                header: t("Error Code"),
                width: proportional(1, { minWidth: 84 }),
                renderCell: (req) =>
                  req.error_code ? (
                    <Text type="supporting" color="inherit" className="text-destructive">
                      {formatErrorCode(req.error_code)}
                    </Text>
                  ) : (
                    <Text type="supporting">-</Text>
                  ) },
              {
                key: "attempts",
                header: t("Attempts"),
                width: pixel(56),
                align: "end",
                renderCell: (req) => (
                  <Text type="body" hasTabularNumbers>
                    {req.attempts}
                  </Text>
                ) },
              {
                key: "created_at",
                header: t("Created"),
                width: proportional(1.2, { minWidth: 116 }),
                renderCell: (req) => (
                  <Text type="supporting" hasTabularNumbers>
                    {req.created_at ? new Date(req.created_at).toLocaleString() : "-"}
                  </Text>
                ) },
              {
                key: "actions",
                header: t("Actions"),
                width: pixel(actionsWidth),
                align: "end",
                renderCell: (req) => {
                  const canCancel = req.state === "pending";
                  const canResend =
                    req.state === "completed" ||
                    (req.state === "failed" && req.error_code === "finalize_failed");
                  const isResending = resendingId === req.id;
                  const actions: RowAction[] = [];
                  if (canCancel) {
                    actions.push({
                      label: t("Cancel"),
                      destructive: true,
                      onSelect: () => {
                        setCancelTarget(req);
                        setCancelError(null);
                      } });
                  }
                  if (canResend) {
                    actions.push({
                      label: t("Resend welcome"),
                      busy: isResending,
                      onSelect: () => handleResendWelcome(req) });
                  }
                  return <RowActions label={t("account request {0}", req.email)} actions={actions} />;
                } },
            ]}
          />
        </Card>
      )}

      {/* Create Account Request Dialog */}
      <Dialog
        isOpen={createOpen}
        onOpenChange={setCreateOpen}
        purpose="form"
        width={576}
      >

          <form noValidate onSubmit={handleSubmit} className="flex flex-col gap-4">
            <DialogHeader title={t("Create account")} />

            <div className="flex flex-col gap-3">
              <Selector
                label={t("Account kind")}
                id="account-kind"
                width="100%"
                options={ACCOUNT_KIND_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={kind}
                onChange={(val) => {
                    if (val === "individual" || val === "business") {
                      handleKindChange(val);
                    }
                  }}
                status={fieldErrors.kind ? { type: "error", message: fieldErrors.kind } : undefined}
              />

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <TextInput
                  label={t("Email")}
                  id="account-email"
                  type="email"
                  value={email}
                  onChange={(value) => {
                      setEmail(value);
                      clearFieldError("email");
                      clearFieldError("confirm_email");
                    }}
                  placeholder={t("user@example.com")}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={fieldErrors.email ? { type: "error", message: fieldErrors.email } : undefined}
                />

                <TextInput
                  label={t("Confirm email")}
                  id="account-confirm-email"
                  type="email"
                  value={confirmEmail}
                  onChange={(value) => {
                      setConfirmEmail(value);
                      clearFieldError("confirm_email");
                    }}
                  placeholder={t("user@example.com")}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={confirmEmailError ? { type: "error", message: confirmEmailError } : undefined}
                />
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <TextInput
                  label={t("Display name")}
                  id="account-display-name"
                  value={displayName}
                  onChange={(value) => {
                      setDisplayName(value.slice(0, 100));
                      clearFieldError("display_name");
                    }}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={fieldErrors.display_name ? { type: "error", message: fieldErrors.display_name } : undefined}
                />

                <TextInput
                  label={t("Workspace name")}
                  id="account-workspace-name"
                  value={workspaceName}
                  onChange={(value) => {
                      setWorkspaceName(value.slice(0, 100));
                      clearFieldError("workspace_name");
                    }}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={fieldErrors.workspace_name ? { type: "error", message: fieldErrors.workspace_name } : undefined}
                />
              </div>

              <TextInput
                label={t("Workspace slug (optional)")}
                id="account-workspace-slug"
                value={workspaceSlug}
                onChange={(value) => {
                    setWorkspaceSlug(value);
                    clearFieldError("workspace_slug");
                  }}
                placeholder={t("auto-generated if blank")}
                isDisabled={createAccount.isPending}
                width="100%"
                status={fieldErrors.workspace_slug ? { type: "error", message: fieldErrors.workspace_slug } : undefined}
              />

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                {activePlans.length > 0 ? (
                  <Selector
                    label={t("Plan")}
                    width="100%"
                    options={planItems.map((item) => ({ value: item.value, label: t(item.label) }))}
                    value={effectivePlanKey}
                    onChange={(val) => {
                      if (val) handlePlanChange(val);
                    }}
                    isDisabled={createAccount.isPending}
                    status={fieldErrors.plan_key ? { type: "error", message: fieldErrors.plan_key } : undefined}
                  />
                ) : (
                  <TextInput
                    id="account-plan-key"
                    label={t("Plan")}
                    width="100%"
                    value={effectivePlanKey}
                    onChange={(value) => handlePlanChange(value)}
                    isDisabled={createAccount.isPending}
                    status={fieldErrors.plan_key ? { type: "error", message: fieldErrors.plan_key } : undefined}
                  />
                )}

                <NumberInput
                  label={t("Seats")}
                  id="account-seats"
                  min={1}
                  isIntegerOnly
                  value={seats.trim() === "" ? null : Number(seats)}
                  onChange={(value) => {
                    setSeats(value === null ? "" : String(value));
                    clearFieldError("seats");
                  }}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={fieldErrors.seats ? { type: "error", message: fieldErrors.seats } : undefined}
                />

                <Selector
                  label={t("Interval")}
                  id="account-interval"
                  width="100%"
                  options={INTERVAL_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                  value={interval}
                  onChange={(val) => {
                      if (val === "month" || val === "year") {
                        setInterval(val);
                        clearFieldError("interval");
                      }
                    }}
                  status={fieldErrors.interval ? { type: "error", message: fieldErrors.interval } : undefined}
                />
              </div>

              <Card padding={3} elevation="none" className="space-y-3">
                <div className="space-y-1">
                  <CheckboxInput
                    id="account-trial"
                    label={t("Trial")}
                    description={t("A trial starts now and ends after the plan's trial days.")}
                    value={trial}
                    onChange={(checked) => {
                      setTrial(checked);
                      clearFieldError("trial");
                      clearFieldError("activation");
                    }}
                    isDisabled={createAccount.isPending}
                    status={fieldErrors.trial ? { type: "error", message: fieldErrors.trial } : undefined}
                  />
                </div>

                <TextInput
                  label={t("Complimentary reason")}
                  id="account-comp-reason"
                  value={compReason}
                  onChange={(value) => {
                      setCompReason(value.slice(0, 500));
                      clearFieldError("comp_reason");
                      clearFieldError("activation");
                    }}
                  isDisabled={createAccount.isPending}
                  width="100%"
                  status={fieldErrors.comp_reason ? { type: "error", message: fieldErrors.comp_reason } : undefined}
                  description={t("Grants one full billing interval at no charge.")}
                />

                {fieldErrors.activation && (
                  <Text type="supporting" className="text-destructive">{fieldErrors.activation}</Text>
                )}
              </Card>

              {kind === "business" && (
                <Card padding={3} elevation="none" className="space-y-3"><fieldset className="space-y-3">
                  <legend className="px-1 text-caption font-semibold text-foreground">
                    {t("Billing profile")}
                  </legend>
                  {fieldErrors.billing_profile && (
                    <Text type="supporting" className="text-destructive">{fieldErrors.billing_profile}</Text>
                  )}

                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <TextInput
                      label={t("Legal name")}
                      id="billing-legal-name"
                      value={legalName}
                      onChange={(value) => {
                          setLegalName(value);
                          clearFieldError("billing_profile.legal_name");
                        }}
                      isDisabled={createAccount.isPending}
                      width="100%"
                      status={fieldErrors["billing_profile.legal_name"] ? { type: "error", message: fieldErrors["billing_profile.legal_name"] } : undefined}
                    />

                    <TextInput
                      label={t("Tax code")}
                      id="billing-tax-code"
                      value={taxCode}
                      onChange={(value) => {
                          setTaxCode(value);
                          clearFieldError("billing_profile.tax_code");
                        }}
                      isDisabled={createAccount.isPending}
                      width="100%"
                      status={fieldErrors["billing_profile.tax_code"] ? { type: "error", message: fieldErrors["billing_profile.tax_code"] } : undefined}
                    />
                  </div>

                  <TextInput
                    label={t("Address")}
                    id="billing-address"
                    value={address}
                    onChange={(value) => {
                        setAddress(value);
                        clearFieldError("billing_profile.address");
                      }}
                    isDisabled={createAccount.isPending}
                    width="100%"
                    status={fieldErrors["billing_profile.address"] ? { type: "error", message: fieldErrors["billing_profile.address"] } : undefined}
                  />

                  <TextInput
                    label={t("Invoice email")}
                    id="billing-invoice-email"
                    type="email"
                    value={invoiceEmail}
                    onChange={(value) => {
                        setInvoiceEmail(value);
                        clearFieldError("billing_profile.invoice_email");
                      }}
                    isDisabled={createAccount.isPending}
                    width="100%"
                    status={fieldErrors["billing_profile.invoice_email"] ? { type: "error", message: fieldErrors["billing_profile.invoice_email"] } : undefined}
                  />
                </fieldset></Card>
              )}
            </div>

            {formError && (
              <Banner
                status="error"
                role="alert"
                title={formError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
        type="button"
        isDisabled={createAccount.isPending}
        label={t("Cancel")}
        variant="secondary"
        onClick={() => setCreateOpen(false)}
      />
              <Button
                type="submit"
                label={t("Submit request")}
                variant="primary"
                isLoading={createAccount.isPending}
                aria-busy={createAccount.isPending}
                isDisabled={createAccount.isPending}
              />
            </div>
          </form>

      </Dialog>

      {/* Cancel Request Confirmation Dialog */}
      <Dialog
        isOpen={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCancelTarget(null);
            setCancelError(null);
          }
        }}
        purpose="info"
      >

          <div className="flex flex-col gap-4">
            <DialogHeader title={t("Cancel account request")} subtitle={<>{t("Cancel the pending account request for")}{" "}
                <strong className="text-foreground">{cancelTarget?.email}</strong>{t("? This cannot be undone.")}</>} />

            {cancelError && (
              <Banner
                status="error"
                role="alert"
                title={cancelError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
        type="button"
        isDisabled={cancelAccount.isPending}
        label={t("Keep request")}
        variant="secondary"
        onClick={() => ((open) => {
          if (!open) {
            setCancelTarget(null);
            setCancelError(null);
          }
        })(false)}
      />
              <Button
                label={t("Confirm cancel")}
                variant="destructive"
                isLoading={cancelAccount.isPending}
                onClick={handleConfirmCancel}
                aria-busy={cancelAccount.isPending}
                isDisabled={cancelAccount.isPending}
              />
            </div>
          </div>

      </Dialog>
    </div>
  );
}
