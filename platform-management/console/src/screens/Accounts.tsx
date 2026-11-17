import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Badge } from "@multica/ui/components/ui/badge";
import { Spinner } from "@multica/ui/components/ui/spinner";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { UserPlus } from "@multica/ui/icons";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
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

const STATE_TABS: { value: StateFilter; label: string }[] = [
  { value: "", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "provisioned", label: "Provisioned" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
  { value: "canceled", label: "Canceled" },
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

const SELECT_CLASS =
  "h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

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
    return (
      <Badge
        variant="outline"
        className="border-amber-600/40 bg-amber-50 text-amber-800 dark:bg-amber-950/30 dark:text-amber-300"
      >
        Finalize failed
      </Badge>
    );
  }
  switch (req.state) {
    case "pending":
      return <Badge variant="secondary">Pending</Badge>;
    case "provisioned":
      return <Badge variant="outline">Provisioned</Badge>;
    case "completed":
      return (
        <Badge
          variant="outline"
          className="border-emerald-600/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400"
        >
          Completed
        </Badge>
      );
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    case "canceled":
      return (
        <Badge variant="outline" className="text-muted-foreground">
          Canceled
        </Badge>
      );
    default:
      return <Badge variant="outline">Unknown</Badge>;
  }
}

export function Accounts({ userRole }: AccountsProps) {
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

  if (effectiveRole && effectiveRole !== "admin") {
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

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="font-heading text-title-lg font-bold">Accounts</h1>
        <Button onClick={handleOpenCreate} className="gap-1.5">
          <UserPlus className="size-4" />
          <span>Create account</span>
        </Button>
      </div>

      {actionNotice && (
        <div
          role="status"
          className="rounded-lg border border-emerald-600/30 bg-emerald-50 p-3 text-body text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-300"
        >
          {actionNotice}
        </div>
      )}

      {actionError && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-body text-destructive"
        >
          {actionError}
        </div>
      )}

      <div className="flex border-b border-surface-border">
        <div role="tablist" className="flex gap-2">
          {STATE_TABS.map((tab) => {
            const isActive = stateFilter === tab.value;
            return (
              <button
                key={tab.label}
                role="tab"
                type="button"
                aria-selected={isActive}
                onClick={() => setStateFilter(tab.value)}
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
          title="No account requests"
          description={
            stateFilter
              ? `No account requests with state "${stateFilter}".`
              : "Create an account request to provision a user and workspace."
          }
        />
      )}

      {!isLoading && !isError && requestList.length > 0 && (
        <div className="rounded-lg border border-surface-border bg-surface overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>State</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Plan</TableHead>
                <TableHead>Workspace Name</TableHead>
                <TableHead>Error Code</TableHead>
                <TableHead className="text-right">Attempts</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {requestList.map((req) => {
                const canCancel = req.state === "pending";
                const canResend =
                  req.state === "completed" ||
                  (req.state === "failed" && req.error_code === "finalize_failed");
                const isResending = resendingId === req.id;

                return (
                  <TableRow key={req.id}>
                    <TableCell>
                      <AccountStateBadge req={req} />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium text-foreground">{req.email}</div>
                      {req.display_name && (
                        <div className="text-caption text-muted-foreground">{req.display_name}</div>
                      )}
                    </TableCell>
                    <TableCell className="capitalize">{req.kind}</TableCell>
                    <TableCell>
                      <div className="font-medium">{req.plan_key}</div>
                      <div className="text-caption text-muted-foreground">
                        {req.seats} seats &bull; {req.interval}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{req.workspace_name}</div>
                      {req.workspace_slug && (
                        <div className="font-mono text-caption text-muted-foreground">
                          {req.workspace_slug}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-caption text-muted-foreground">
                      {req.error_code ? (
                        <span className="text-destructive">{formatErrorCode(req.error_code)}</span>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{req.attempts}</TableCell>
                    <TableCell className="text-caption text-muted-foreground whitespace-nowrap">
                      {req.created_at ? new Date(req.created_at).toLocaleString() : "-"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        {canCancel && (
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => {
                              setCancelTarget(req);
                              setCancelError(null);
                            }}
                          >
                            Cancel
                          </Button>
                        )}
                        {canResend && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={isResending}
                            aria-busy={isResending}
                            onClick={() => handleResendWelcome(req)}
                          >
                            {isResending && <Spinner className="size-3.5" />}
                            Resend welcome
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Create Account Request Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-xl">
          <form noValidate onSubmit={handleSubmit} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Create account</DialogTitle>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
                <label htmlFor="account-kind" className="text-caption font-medium text-foreground">
                  Account kind
                </label>
                <select
                  id="account-kind"
                  value={kind}
                  onChange={(e) => handleKindChange(e.target.value as "individual" | "business")}
                  disabled={createAccount.isPending}
                  aria-invalid={Boolean(fieldErrors.kind)}
                  className={SELECT_CLASS}
                >
                  <option value="individual">Individual</option>
                  <option value="business">Business</option>
                </select>
                {fieldErrors.kind && (
                  <p className="text-caption text-destructive">{fieldErrors.kind}</p>
                )}
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="account-email" className="text-caption font-medium text-foreground">
                    Email
                  </label>
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
                    <p className="text-caption text-destructive">{fieldErrors.email}</p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <label
                    htmlFor="account-confirm-email"
                    className="text-caption font-medium text-foreground"
                  >
                    Confirm email
                  </label>
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
                    <p className="text-caption text-destructive">{confirmEmailError}</p>
                  )}
                </div>
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-1.5">
                  <label
                    htmlFor="account-display-name"
                    className="text-caption font-medium text-foreground"
                  >
                    Display name
                  </label>
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
                    <p className="text-caption text-destructive">{fieldErrors.display_name}</p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <label
                    htmlFor="account-workspace-name"
                    className="text-caption font-medium text-foreground"
                  >
                    Workspace name
                  </label>
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
                    <p className="text-caption text-destructive">{fieldErrors.workspace_name}</p>
                  )}
                </div>
              </div>

              <div className="flex flex-col gap-1.5">
                <label
                  htmlFor="account-workspace-slug"
                  className="text-caption font-medium text-foreground"
                >
                  Workspace slug (optional)
                </label>
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
                  <p className="text-caption text-destructive">{fieldErrors.workspace_slug}</p>
                )}
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="account-plan-key" className="text-caption font-medium text-foreground">
                    Plan
                  </label>
                  {activePlans.length > 0 ? (
                    <select
                      id="account-plan-key"
                      value={effectivePlanKey}
                      onChange={(e) => handlePlanChange(e.target.value)}
                      disabled={createAccount.isPending}
                      aria-invalid={Boolean(fieldErrors.plan_key)}
                      className={SELECT_CLASS}
                    >
                      {activePlans.map((p) => (
                        <option key={p.key} value={p.key}>
                          {p.name} ({p.key})
                        </option>
                      ))}
                    </select>
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
                    <p className="text-caption text-destructive">{fieldErrors.plan_key}</p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <label htmlFor="account-seats" className="text-caption font-medium text-foreground">
                    Seats
                  </label>
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
                    <p className="text-caption text-destructive">{fieldErrors.seats}</p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <label htmlFor="account-interval" className="text-caption font-medium text-foreground">
                    Interval
                  </label>
                  <select
                    id="account-interval"
                    value={interval}
                    onChange={(e) => {
                      setInterval(e.target.value as "month" | "year");
                      clearFieldError("interval");
                    }}
                    disabled={createAccount.isPending}
                    aria-invalid={Boolean(fieldErrors.interval)}
                    className={SELECT_CLASS}
                  >
                    <option value="month">Monthly</option>
                    <option value="year">Yearly</option>
                  </select>
                  {fieldErrors.interval && (
                    <p className="text-caption text-destructive">{fieldErrors.interval}</p>
                  )}
                </div>
              </div>

              <div className="space-y-2 rounded-lg border border-surface-border p-3">
                <div className="space-y-1">
                  <label className="flex items-center gap-2 text-body font-medium">
                    <input
                      id="account-trial"
                      type="checkbox"
                      checked={trial}
                      onChange={(e) => {
                        setTrial(e.target.checked);
                        clearFieldError("trial");
                        clearFieldError("activation");
                      }}
                      disabled={createAccount.isPending}
                      className="size-4 rounded border-surface-border text-primary focus:ring-primary"
                    />
                    <span>Trial</span>
                  </label>
                  <p className="text-caption text-muted-foreground">
                    A trial starts now and ends after the plan's trial days.
                  </p>
                  {fieldErrors.trial && (
                    <p className="text-caption text-destructive">{fieldErrors.trial}</p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <label
                    htmlFor="account-comp-reason"
                    className="text-caption font-medium text-foreground"
                  >
                    Complimentary reason
                  </label>
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
                  <p className="text-caption text-muted-foreground">
                    Grants one full billing interval at no charge.
                  </p>
                  {fieldErrors.comp_reason && (
                    <p className="text-caption text-destructive">{fieldErrors.comp_reason}</p>
                  )}
                </div>

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
                    <div className="flex flex-col gap-1.5">
                      <label
                        htmlFor="billing-legal-name"
                        className="text-caption font-medium text-foreground"
                      >
                        Legal name
                      </label>
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
                        <p className="text-caption text-destructive">
                          {fieldErrors["billing_profile.legal_name"]}
                        </p>
                      )}
                    </div>

                    <div className="flex flex-col gap-1.5">
                      <label
                        htmlFor="billing-tax-code"
                        className="text-caption font-medium text-foreground"
                      >
                        Tax code
                      </label>
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
                        <p className="text-caption text-destructive">
                          {fieldErrors["billing_profile.tax_code"]}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex flex-col gap-1.5">
                    <label
                      htmlFor="billing-address"
                      className="text-caption font-medium text-foreground"
                    >
                      Address
                    </label>
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
                      <p className="text-caption text-destructive">
                        {fieldErrors["billing_profile.address"]}
                      </p>
                    )}
                  </div>

                  <div className="flex flex-col gap-1.5">
                    <label
                      htmlFor="billing-invoice-email"
                      className="text-caption font-medium text-foreground"
                    >
                      Invoice email
                    </label>
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
                      <p className="text-caption text-destructive">
                        {fieldErrors["billing_profile.invoice_email"]}
                      </p>
                    )}
                  </div>
                </fieldset>
              )}
            </div>

            {formError && (
              <div
                role="alert"
                className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-caption text-destructive"
              >
                {formError}
              </div>
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
              <div role="alert" className="text-caption text-destructive">
                {cancelError}
              </div>
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
