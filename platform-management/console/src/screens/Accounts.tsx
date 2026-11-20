import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { Field, FieldDescription, FieldError, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { AlertCircle, CheckCircle2, UserPlus } from "@multica/ui/icons";
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
  type CreateAccountInput,
} from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type { StaffRole } from "../components/types";
import type { AccountRequest, AccountRequestState, Plan } from "../api/schemas";

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
  internal: "Internal error",
};

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
  "billing_profile.invoice_email": "Enter a valid invoice email address.",
};

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
    ? `${prefix} Try again in ${wait}.`
    : `${prefix} Please wait before trying again.`;
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
      retryAfter: err.retryAfter,
    };
  }
  if (err instanceof PlatformApiError) {
    return {
      status: err.status,
      code: err.code,
      message: err.message,
      fields: [],
      retryAfter: null,
    };
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
  return ERROR_CODE_LABELS[code] ?? code;
}

function AccountStateBadge({ req }: { req: AccountRequest }) {
  if (req.state === "failed" && req.error_code === "finalize_failed") {
    return <StatusBadge status="needs_review" label="Finalize failed" />;
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
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
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
    fieldErrors.confirm_email ?? (showMismatchLive ? "Email addresses do not match." : undefined);

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
      errs.email = "Email is required.";
    }
    if (!trimmedConfirm) {
      errs.confirm_email = "Please confirm the email address.";
    } else if (trimmedEmail.toLowerCase() !== trimmedConfirm.toLowerCase()) {
      errs.confirm_email = "Email addresses do not match.";
    }

    const trimmedDisplay = displayName.trim();
    if (!trimmedDisplay || trimmedDisplay.length > 100) {
      errs.display_name = "Display name must be between 1 and 100 characters.";
    }

    const trimmedWorkspace = workspaceName.trim();
    if (!trimmedWorkspace || trimmedWorkspace.length > 100) {
      errs.workspace_name = "Workspace name must be between 1 and 100 characters.";
    }

    const parsedSeats = Number(seats);
    if (!Number.isInteger(parsedSeats) || parsedSeats < 1) {
      errs.seats = "Seats must be a whole number of at least 1.";
    }

    const paid = isPaidPlan(effectivePlanKey, selectedPlan);
    const hasReason = compReason.trim() !== "";
    if (trial && hasReason) {
      errs.activation = "Choose either a trial or a complimentary reason, not both.";
    } else if (paid && !trial && !hasReason) {
      errs.activation = "A paid plan requires either a trial or a complimentary reason.";
    } else if (!paid && (trial || hasReason)) {
      errs.activation = "Free plans activate without a trial or complimentary reason.";
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
              invoice_email: invoiceEmail.trim().toLowerCase(),
            },
          }
        : {}),
    };

    try {
      await createAccount.mutateAsync({ input, idempotencyKey });
      setCreateOpen(false);
      setActionError(null);
      setActionNotice(`Account request submitted for ${input.email}.`);
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      switch (details.code) {
        case "account_request_invalid": {
          const nextFields: Record<string, string> = {};
          for (const f of details.fields) {
            nextFields[f] = FIELD_ERROR_MESSAGES[f] ?? "Invalid value.";
          }
          setFieldErrors(nextFields);
          setFormError(details.message || "Check the highlighted fields and try again.");
          break;
        }
        case "account_activation_required":
          setFieldErrors({
            activation: "Trial or complimentary reason required.",
          });
          setFormError(details.message || "A paid plan requires either a trial or a complimentary reason.");
          break;
        case "account_email_exists":
          setFieldErrors({ email: "Email already registered." });
          setFormError(details.message || "An account with this email already exists.");
          break;
        case "plan_audience_mismatch":
          setFieldErrors({ plan_key: "Not available for this account kind." });
          setFormError(details.message || "This plan is not sold for this account kind.");
          break;
        case "account_create_rate_limited":
          setFormError(formatRateLimitMessage("Account creation rate limit reached.", details.retryAfter));
          break;
        default:
          if (details.status === 429) {
            setFormError(formatRateLimitMessage("Account creation rate limit reached.", details.retryAfter));
          } else {
            setFormError("Unable to create account request. Please try again.");
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
      setActionNotice(`Canceled account request for ${cancelTarget.email}.`);
      setCancelTarget(null);
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      setCancelError(details.message || "Failed to cancel account request.");
    }
  };

  const handleResendWelcome = async (req: AccountRequest) => {
    if (resendingId !== null) return;
    setResendingId(req.id);
    setActionError(null);
    setActionNotice(null);
    try {
      await resendWelcome.mutateAsync({ id: req.id });
      setActionNotice(`Welcome email resent to ${req.email}.`);
    } catch (err: unknown) {
      const details = extractErrorDetails(err);
      if (details.status === 429) {
        setActionError(formatRateLimitMessage("Welcome email rate limit reached.", details.retryAfter));
      } else if (details.status === 502) {
        setActionError("Failed to send welcome email. Please try again later.");
      } else {
        setActionError(
          details.status !== null && details.status < 500 && details.message
            ? details.message
            : "Failed to resend welcome email.",
        );
      }
    } finally {
      setResendingId(null);
    }
  };

  const requestList = requests ?? [];
  const planItems = activePlans.map((p) => ({
    value: p.key,
    label: `${p.name} (${p.key})`,
  }));

  return (
    <div className="space-y-6">
      <PageHeader
        title="Accounts"
        actions={
          <Button size="sm" onClick={handleOpenCreate} className="gap-1.5">
            <UserPlus className="size-4" aria-hidden="true" />
            <span>Create account</span>
          </Button>
        }
      />

      {actionNotice && (
        <Alert
          role="status"
          className="border-success/30 bg-success/10 text-success"
        >
          <CheckCircle2 className="size-4 shrink-0 text-success" aria-hidden="true" />
          <AlertDescription className="text-body text-success">
            {actionNotice}
          </AlertDescription>
        </Alert>
      )}

      {actionError && (
        <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          <AlertDescription className="text-body">{actionError}</AlertDescription>
        </Alert>
      )}

      <Tabs
        value={stateFilter || "all"}
        onValueChange={(value) => {
          const found = STATE_TABS.find((t) => t.value === value);
          setStateFilter(found ? found.filter : "");
        }}
      >
        <div className="border-b border-surface-border">
          <TabsList variant="line" aria-label="Account request state">
            {STATE_TABS.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </Tabs>

      {isLoading && <LoadingState message="Loading account requests..." />}

      {isError && (
        <ErrorState
          title="Failed to load account requests"
          message={error instanceof Error ? error.message : "An error occurred while loading account requests."}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && requestList.length === 0 && (
        <EmptyState
          icon={<UserPlus className="size-4" />}
          title="No account requests"
          description={
            stateFilter
              ? `No account requests with state "${stateFilter}".`
              : "Create an account request to provision a user and workspace."
          }
          actions={
            stateFilter ? (
              <Button variant="outline" size="sm" onClick={() => setStateFilter("")}>
                Show all
              </Button>
            ) : (
              <Button size="sm" onClick={handleOpenCreate}>
                Create account
              </Button>
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
                header: "State",
                width: proportional(1, { minWidth: 100 }),
                renderCell: (req) => (
                  <div className="flex flex-wrap items-center gap-1.5">
                    <AccountStateBadge req={req} />
                    {req.logged_in === false && <Badge variant="neutral" label="Never logged in" />}
                  </div>
                ),
              },
              {
                key: "email",
                header: "Email",
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
                ),
              },
              {
                key: "kind",
                header: "Kind",
                width: proportional(0.6, { minWidth: 56 }),
                renderCell: (req) => <Text type="body" className="capitalize">{req.kind}</Text>,
              },
              {
                key: "plan_key",
                header: "Plan",
                width: proportional(1, { minWidth: 90 }),
                renderCell: (req) => (
                  <>
                    <Text type="body" weight="medium" as="div" className="block">
                      {req.plan_key}
                    </Text>
                    <Text type="supporting" hasTabularNumbers as="div" className="block">
                      {req.seats} seats &bull; {req.interval}
                    </Text>
                  </>
                ),
              },
              {
                key: "workspace_name",
                header: "Workspace Name",
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
                ),
              },
              {
                key: "error_code",
                header: "Error Code",
                width: proportional(1, { minWidth: 84 }),
                renderCell: (req) =>
                  req.error_code ? (
                    <Text type="supporting" color="inherit" className="text-destructive">
                      {formatErrorCode(req.error_code)}
                    </Text>
                  ) : (
                    <Text type="supporting">-</Text>
                  ),
              },
              {
                key: "attempts",
                header: "Attempts",
                width: pixel(56),
                align: "end",
                renderCell: (req) => (
                  <Text type="body" hasTabularNumbers>
                    {req.attempts}
                  </Text>
                ),
              },
              {
                key: "created_at",
                header: "Created",
                width: proportional(1.2, { minWidth: 116 }),
                renderCell: (req) => (
                  <Text type="supporting" hasTabularNumbers>
                    {req.created_at ? new Date(req.created_at).toLocaleString() : "-"}
                  </Text>
                ),
              },
              {
                key: "actions",
                header: "Actions",
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
                      label: "Cancel",
                      destructive: true,
                      onSelect: () => {
                        setCancelTarget(req);
                        setCancelError(null);
                      },
                    });
                  }
                  if (canResend) {
                    actions.push({
                      label: "Resend welcome",
                      busy: isResending,
                      onSelect: () => handleResendWelcome(req),
                    });
                  }
                  return <RowActions label={`account request ${req.email}`} actions={actions} />;
                },
              },
            ]}
          />
        </Card>
      )}

      {/* Create Account Request Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-xl">
          <form noValidate onSubmit={handleSubmit} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Create account</DialogTitle>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <Field data-invalid={Boolean(fieldErrors.kind) || undefined}>
                <FieldLabel htmlFor="account-kind" className="text-label">
                  Account kind
                </FieldLabel>
                <Select
                  items={ACCOUNT_KIND_ITEMS}
                  value={kind}
                  onValueChange={(val) => {
                    if (val === "individual" || val === "business") {
                      handleKindChange(val);
                    }
                  }}
                  disabled={createAccount.isPending}
                >
                  <SelectTrigger
                    id="account-kind"
                    aria-label="Account kind"
                    aria-invalid={Boolean(fieldErrors.kind)}
                    className="w-full"
                  >
                    <SelectValue>
                      {ACCOUNT_KIND_ITEMS.find((i) => i.value === kind)?.label ?? kind}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {ACCOUNT_KIND_ITEMS.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {fieldErrors.kind && (
                  <FieldError role={undefined} className="text-caption">{fieldErrors.kind}</FieldError>
                )}
              </Field>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Field data-invalid={Boolean(fieldErrors.email) || undefined}>
                  <FieldLabel htmlFor="account-email" className="text-label">
                    Email
                  </FieldLabel>
                  <Input
                    id="account-email"
                    type="email"
                    value={email}
                    onChange={(e) => {
                      setEmail(e.target.value);
                      clearFieldError("email");
                      clearFieldError("confirm_email");
                    }}
                    placeholder="user@example.com"
                    aria-invalid={Boolean(fieldErrors.email)}
                    disabled={createAccount.isPending}
                  />
                  {fieldErrors.email && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.email}</FieldError>
                  )}
                </Field>

                <Field data-invalid={Boolean(confirmEmailError) || undefined}>
                  <FieldLabel htmlFor="account-confirm-email" className="text-label">
                    Confirm email
                  </FieldLabel>
                  <Input
                    id="account-confirm-email"
                    type="email"
                    value={confirmEmail}
                    onChange={(e) => {
                      setConfirmEmail(e.target.value);
                      clearFieldError("confirm_email");
                    }}
                    placeholder="user@example.com"
                    aria-invalid={Boolean(confirmEmailError)}
                    disabled={createAccount.isPending}
                  />
                  {confirmEmailError && (
                    <FieldError role={undefined} className="text-caption">{confirmEmailError}</FieldError>
                  )}
                </Field>
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Field data-invalid={Boolean(fieldErrors.display_name) || undefined}>
                  <FieldLabel htmlFor="account-display-name" className="text-label">
                    Display name
                  </FieldLabel>
                  <Input
                    id="account-display-name"
                    value={displayName}
                    onChange={(e) => {
                      setDisplayName(e.target.value);
                      clearFieldError("display_name");
                    }}
                    maxLength={100}
                    aria-invalid={Boolean(fieldErrors.display_name)}
                    disabled={createAccount.isPending}
                  />
                  {fieldErrors.display_name && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.display_name}</FieldError>
                  )}
                </Field>

                <Field data-invalid={Boolean(fieldErrors.workspace_name) || undefined}>
                  <FieldLabel htmlFor="account-workspace-name" className="text-label">
                    Workspace name
                  </FieldLabel>
                  <Input
                    id="account-workspace-name"
                    value={workspaceName}
                    onChange={(e) => {
                      setWorkspaceName(e.target.value);
                      clearFieldError("workspace_name");
                    }}
                    maxLength={100}
                    aria-invalid={Boolean(fieldErrors.workspace_name)}
                    disabled={createAccount.isPending}
                  />
                  {fieldErrors.workspace_name && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.workspace_name}</FieldError>
                  )}
                </Field>
              </div>

              <Field data-invalid={Boolean(fieldErrors.workspace_slug) || undefined}>
                <FieldLabel htmlFor="account-workspace-slug" className="text-label">
                  Workspace slug (optional)
                </FieldLabel>
                <Input
                  id="account-workspace-slug"
                  value={workspaceSlug}
                  onChange={(e) => {
                    setWorkspaceSlug(e.target.value);
                    clearFieldError("workspace_slug");
                  }}
                  placeholder="auto-generated if blank"
                  aria-invalid={Boolean(fieldErrors.workspace_slug)}
                  disabled={createAccount.isPending}
                />
                {fieldErrors.workspace_slug && (
                  <FieldError role={undefined} className="text-caption">{fieldErrors.workspace_slug}</FieldError>
                )}
              </Field>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <Field data-invalid={Boolean(fieldErrors.plan_key) || undefined}>
                  <FieldLabel htmlFor="account-plan-key" className="text-label">
                    Plan
                  </FieldLabel>
                  {activePlans.length > 0 ? (
                    <Select
                      items={planItems}
                      value={effectivePlanKey}
                      onValueChange={(val) => {
                        if (val) handlePlanChange(val);
                      }}
                      disabled={createAccount.isPending}
                    >
                      <SelectTrigger
                        id="account-plan-key"
                        aria-label="Plan"
                        aria-invalid={Boolean(fieldErrors.plan_key)}
                        className="w-full"
                      >
                        <SelectValue>
                          {planItems.find((i) => i.value === effectivePlanKey)?.label ??
                            effectivePlanKey}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {planItems.map((item) => (
                          <SelectItem key={item.value} value={item.value}>
                            {item.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      id="account-plan-key"
                      value={effectivePlanKey}
                      onChange={(e) => handlePlanChange(e.target.value)}
                      aria-invalid={Boolean(fieldErrors.plan_key)}
                      disabled={createAccount.isPending}
                    />
                  )}
                  {fieldErrors.plan_key && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.plan_key}</FieldError>
                  )}
                </Field>

                <Field data-invalid={Boolean(fieldErrors.seats) || undefined}>
                  <FieldLabel htmlFor="account-seats" className="text-label">
                    Seats
                  </FieldLabel>
                  <Input
                    id="account-seats"
                    type="number"
                    min={1}
                    value={seats}
                    onChange={(e) => {
                      setSeats(e.target.value);
                      clearFieldError("seats");
                    }}
                    aria-invalid={Boolean(fieldErrors.seats)}
                    disabled={createAccount.isPending}
                  />
                  {fieldErrors.seats && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.seats}</FieldError>
                  )}
                </Field>

                <Field data-invalid={Boolean(fieldErrors.interval) || undefined}>
                  <FieldLabel htmlFor="account-interval" className="text-label">
                    Interval
                  </FieldLabel>
                  <Select
                    items={INTERVAL_ITEMS}
                    value={interval}
                    onValueChange={(val) => {
                      if (val === "month" || val === "year") {
                        setInterval(val);
                        clearFieldError("interval");
                      }
                    }}
                    disabled={createAccount.isPending}
                  >
                    <SelectTrigger
                      id="account-interval"
                      aria-label="Interval"
                      aria-invalid={Boolean(fieldErrors.interval)}
                      className="w-full"
                    >
                      <SelectValue>
                        {INTERVAL_ITEMS.find((i) => i.value === interval)?.label ?? interval}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {INTERVAL_ITEMS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldErrors.interval && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.interval}</FieldError>
                  )}
                </Field>
              </div>

              <div className="space-y-3 rounded-lg border border-surface-border p-3">
                <div className="space-y-1">
                  <Field orientation="horizontal">
                    <Checkbox
                      id="account-trial"
                      checked={trial}
                      onCheckedChange={(checked) => {
                        setTrial(Boolean(checked));
                        clearFieldError("trial");
                        clearFieldError("activation");
                      }}
                      disabled={createAccount.isPending}
                    />
                    <FieldLabel htmlFor="account-trial" className="text-body font-medium">
                      Trial
                    </FieldLabel>
                  </Field>
                  <p className="text-caption text-muted-foreground">
                    A trial starts now and ends after the plan's trial days.
                  </p>
                  {fieldErrors.trial && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.trial}</FieldError>
                  )}
                </div>

                <Field data-invalid={Boolean(fieldErrors.comp_reason) || undefined}>
                  <FieldLabel htmlFor="account-comp-reason" className="text-label">
                    Complimentary reason
                  </FieldLabel>
                  <Input
                    id="account-comp-reason"
                    value={compReason}
                    onChange={(e) => {
                      setCompReason(e.target.value);
                      clearFieldError("comp_reason");
                      clearFieldError("activation");
                    }}
                    maxLength={500}
                    aria-invalid={Boolean(fieldErrors.comp_reason)}
                    disabled={createAccount.isPending}
                  />
                  <FieldDescription className="text-caption">
                    Grants one full billing interval at no charge.
                  </FieldDescription>
                  {fieldErrors.comp_reason && (
                    <FieldError role={undefined} className="text-caption">{fieldErrors.comp_reason}</FieldError>
                  )}
                </Field>

                {fieldErrors.activation && (
                  <p className="text-caption text-destructive">{fieldErrors.activation}</p>
                )}
              </div>

              {kind === "business" && (
                <fieldset className="space-y-3 rounded-lg border border-surface-border p-3">
                  <legend className="px-1 text-caption font-semibold text-foreground">
                    Billing profile
                  </legend>
                  {fieldErrors.billing_profile && (
                    <p className="text-caption text-destructive">{fieldErrors.billing_profile}</p>
                  )}

                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <Field data-invalid={Boolean(fieldErrors["billing_profile.legal_name"]) || undefined}>
                      <FieldLabel htmlFor="billing-legal-name" className="text-label">
                        Legal name
                      </FieldLabel>
                      <Input
                        id="billing-legal-name"
                        value={legalName}
                        onChange={(e) => {
                          setLegalName(e.target.value);
                          clearFieldError("billing_profile.legal_name");
                        }}
                        aria-invalid={Boolean(fieldErrors["billing_profile.legal_name"])}
                        disabled={createAccount.isPending}
                      />
                      {fieldErrors["billing_profile.legal_name"] && (
                        <FieldError role={undefined} className="text-caption">
                          {fieldErrors["billing_profile.legal_name"]}
                        </FieldError>
                      )}
                    </Field>

                    <Field data-invalid={Boolean(fieldErrors["billing_profile.tax_code"]) || undefined}>
                      <FieldLabel htmlFor="billing-tax-code" className="text-label">
                        Tax code
                      </FieldLabel>
                      <Input
                        id="billing-tax-code"
                        value={taxCode}
                        onChange={(e) => {
                          setTaxCode(e.target.value);
                          clearFieldError("billing_profile.tax_code");
                        }}
                        aria-invalid={Boolean(fieldErrors["billing_profile.tax_code"])}
                        disabled={createAccount.isPending}
                      />
                      {fieldErrors["billing_profile.tax_code"] && (
                        <FieldError role={undefined} className="text-caption">
                          {fieldErrors["billing_profile.tax_code"]}
                        </FieldError>
                      )}
                    </Field>
                  </div>

                  <Field data-invalid={Boolean(fieldErrors["billing_profile.address"]) || undefined}>
                    <FieldLabel htmlFor="billing-address" className="text-label">
                      Address
                    </FieldLabel>
                    <Input
                      id="billing-address"
                      value={address}
                      onChange={(e) => {
                        setAddress(e.target.value);
                        clearFieldError("billing_profile.address");
                      }}
                      aria-invalid={Boolean(fieldErrors["billing_profile.address"])}
                      disabled={createAccount.isPending}
                    />
                    {fieldErrors["billing_profile.address"] && (
                      <FieldError role={undefined} className="text-caption">
                        {fieldErrors["billing_profile.address"]}
                      </FieldError>
                    )}
                  </Field>

                  <Field data-invalid={Boolean(fieldErrors["billing_profile.invoice_email"]) || undefined}>
                    <FieldLabel htmlFor="billing-invoice-email" className="text-label">
                      Invoice email
                    </FieldLabel>
                    <Input
                      id="billing-invoice-email"
                      type="email"
                      value={invoiceEmail}
                      onChange={(e) => {
                        setInvoiceEmail(e.target.value);
                        clearFieldError("billing_profile.invoice_email");
                      }}
                      aria-invalid={Boolean(fieldErrors["billing_profile.invoice_email"])}
                      disabled={createAccount.isPending}
                    />
                    {fieldErrors["billing_profile.invoice_email"] && (
                      <FieldError role={undefined} className="text-caption">
                        {fieldErrors["billing_profile.invoice_email"]}
                      </FieldError>
                    )}
                  </Field>
                </fieldset>
              )}
            </div>

            {formError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{formError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={createAccount.isPending} />}
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={createAccount.isPending}
                aria-busy={createAccount.isPending}
              >
                {createAccount.isPending && <Spinner className="size-3.5" />}
                Submit request
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Cancel Request Confirmation Dialog */}
      <Dialog
        open={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCancelTarget(null);
            setCancelError(null);
          }
        }}
      >
        <DialogContent>
          <div className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Cancel account request</DialogTitle>
              <DialogDescription>
                Cancel the pending account request for{" "}
                <strong className="text-foreground">{cancelTarget?.email}</strong>? This cannot be
                undone.
              </DialogDescription>
            </DialogHeader>

            {cancelError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{cancelError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={cancelAccount.isPending} />}
              >
                Keep request
              </DialogClose>
              <Button
                variant="destructive"
                onClick={handleConfirmCancel}
                disabled={cancelAccount.isPending}
                aria-busy={cancelAccount.isPending}
              >
                {cancelAccount.isPending && <Spinner className="size-3.5" />}
                Confirm cancel
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
