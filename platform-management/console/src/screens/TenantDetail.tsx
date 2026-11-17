import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@multica/ui/components/ui/card";
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
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Badge } from "@multica/ui/components/ui/badge";
import { ArrowLeft, Plus } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import {
  useMe,
  useTenant,
  useTenantUsage,
  useTenantBillingProfile,
  useTenantNotes,
  useTenantAudit,
  useChangeTenantStatus,
  useAddTenantNote,
  useUpdateSubscription,
  useSetBillingProfileVerified,
  useUpdateTenantBillingProfile,
  useCancelOrder,
} from "../api/hooks";
import { PlatformApiError } from "../api/client";
import { formatBytes } from "./Tenants";
import type { StaffRole } from "../components/types";
import type {
  Note,
  AuditEvent,
  Order,
  BillingProfile,
  TenantUsage,
  TenantUsageRow,
  TenantUsageUnpaidOrder,
} from "../api/schemas";

interface TenantDetailProps {
  tenantId: string;
  onBack: () => void;
  userRole?: StaffRole;
}

function formatUsageQuantity(row: TenantUsageRow): string {
  if (row.meter === "storage_bytes") {
    return formatBytes(row.quantity);
  }
  return row.quantity.toLocaleString();
}

interface MonthlyUsageCardProps {
  usageData: TenantUsage | undefined;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  onRetry: () => void;
  canWriteOff: boolean;
  onWriteOff: (order: TenantUsageUnpaidOrder) => void;
}

/** Monthly rated usage charges, unrated missing days, and unpaid usage invoices. */
function MonthlyUsageCard({
  usageData,
  isLoading,
  isError,
  error,
  onRetry,
  canWriteOff,
  onWriteOff,
}: MonthlyUsageCardProps) {
  return (
    <Card data-testid="usage-month">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle role="heading" aria-level={2}>Monthly Usage</CardTitle>
        {usageData && (
          <span className="font-mono text-caption text-muted-foreground">
            {usageData.period_start.slice(0, 10)} to {usageData.period_end.slice(0, 10)} (UTC)
          </span>
        )}
      </CardHeader>
      <CardContent className="space-y-6">
        {isLoading && <LoadingState message="Loading monthly usage..." />}
        {isError && (
          <ErrorState
            title="Failed to load monthly usage"
            message={error instanceof Error ? error.message : "Unable to load monthly usage."}
            onRetry={onRetry}
          />
        )}
        {!isLoading && !isError && usageData && (
          <>
            <div data-testid="usage-rows" className="space-y-2">
              <h3 className="text-caption font-medium text-foreground">Daily charges</h3>
              {usageData.rows.length === 0 ? (
                <p className="text-caption text-muted-foreground">
                  No rated usage rows for this month.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Day</TableHead>
                        <TableHead>Meter</TableHead>
                        <TableHead className="text-right">Quantity</TableHead>
                        <TableHead className="text-right">Unit price</TableHead>
                        <TableHead className="text-right">Discount</TableHead>
                        <TableHead className="text-right">Net amount</TableHead>
                        <TableHead>Note</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {usageData.rows.map((row) => (
                        <TableRow key={`${row.day}-${row.meter}`}>
                          <TableCell className="font-mono text-caption">{row.day}</TableCell>
                          <TableCell className="font-mono text-caption">{row.meter}</TableCell>
                          <TableCell
                            className="text-right tabular-nums"
                            title={
                              row.meter === "storage_bytes"
                                ? `${row.quantity.toLocaleString()} bytes`
                                : undefined
                            }
                          >
                            {formatUsageQuantity(row)}
                          </TableCell>
                          <TableCell className="text-right text-muted-foreground tabular-nums">
                            {row.unit_price.toLocaleString()} / {row.per.toLocaleString()}
                          </TableCell>
                          <TableCell className="text-right text-muted-foreground tabular-nums">
                            {row.discount_pct > 0 ? `${row.discount_pct}%` : "-"}
                          </TableCell>
                          <TableCell className="text-right font-medium tabular-nums">
                            {row.net_amount.toLocaleString()}
                          </TableCell>
                          <TableCell className="text-caption text-muted-foreground">
                            {row.note || "-"}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>

            {usageData.missing_days.length > 0 && (
              <div data-testid="usage-missing-days" className="space-y-2">
                <h3 className="text-caption font-medium text-foreground">Missing days</h3>
                <ul className="flex flex-wrap gap-2">
                  {usageData.missing_days.map((day) => (
                    <li
                      key={day}
                      className="inline-flex items-center gap-1.5 rounded-md border border-surface-border bg-muted/40 px-2.5 py-1 text-caption"
                    >
                      <span className="font-mono">{day}</span>
                      <Badge variant="outline">missing</Badge>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <div data-testid="usage-unpaid-orders" className="space-y-2">
              <h3 className="text-caption font-medium text-foreground">Unpaid usage orders</h3>
              {usageData.unpaid_orders.length === 0 ? (
                <p className="text-caption text-muted-foreground">
                  No unpaid usage orders.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Order</TableHead>
                        <TableHead>Period</TableHead>
                        <TableHead>Created</TableHead>
                        <TableHead className="text-right">Amount</TableHead>
                        <TableHead className="text-right">VAT</TableHead>
                        <TableHead>Status</TableHead>
                        {canWriteOff && <TableHead className="text-right">Action</TableHead>}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {usageData.unpaid_orders.map((order) => (
                        <TableRow key={order.id}>
                          <TableCell className="font-mono text-caption">{order.id}</TableCell>
                          <TableCell className="font-mono text-caption text-muted-foreground">
                            {order.period_start && order.period_end
                              ? `${order.period_start.slice(0, 10)} – ${order.period_end.slice(0, 10)}`
                              : "-"}
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                          </TableCell>
                          <TableCell className="text-right font-medium tabular-nums">
                            {order.amount.toLocaleString()}
                          </TableCell>
                          <TableCell className="text-right text-muted-foreground tabular-nums">
                            {order.vat_amount.toLocaleString()}
                          </TableCell>
                          <TableCell>
                            <StatusBadge status={order.status} />
                          </TableCell>
                          {canWriteOff && (
                            <TableCell className="text-right">
                              <Button
                                variant="destructive"
                                size="sm"
                                onClick={() => onWriteOff(order)}
                              >
                                Write off
                              </Button>
                            </TableCell>
                          )}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return "-";
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? value : new Date(ms).toLocaleString();
}

function UsageValue({ label, value }: { label: string; value: number | string }) {
  return (
    <div>
      <span className="block text-caption text-muted-foreground">{label}</span>
      <span className="mt-1 block font-medium tabular-nums">{value}</span>
    </div>
  );
}

const BILLING_PROFILE_FIELDS = [
  "account_kind",
  "legal_name",
  "tax_code",
  "address",
  "invoice_email",
] as const;

type BillingProfileField = (typeof BILLING_PROFILE_FIELDS)[number];

const BILLING_PROFILE_INCOMPLETE_MESSAGES: Record<BillingProfileField, string> = {
  account_kind: "Account kind is required.",
  legal_name: "Legal name is required.",
  tax_code: "Tax code is required.",
  address: "Address is required.",
  invoice_email: "Invoice email is required.",
};

const BILLING_PROFILE_INVALID_MESSAGES: Record<BillingProfileField, string> = {
  account_kind: "Select a valid account kind.",
  legal_name: "Legal name is invalid or exceeds 255 characters.",
  tax_code: "Enter a valid 10-digit (or 10-3) tax code.",
  address: "Address is invalid or exceeds 255 characters.",
  invoice_email: "Enter a valid invoice email address.",
};

function isBillingProfileField(value: string): value is BillingProfileField {
  return (BILLING_PROFILE_FIELDS as readonly string[]).includes(value);
}

export function TenantDetail({ tenantId, onBack, userRole }: TenantDetailProps) {
  const { data: me } = useMe();
  const {
    data: tenant,
    isLoading: tenantLoading,
    isError: tenantError,
    error: tenantErrorObj,
    refetch: refetchTenant,
  } = useTenant(tenantId);

  const {
    data: usageData,
    isLoading: usageLoading,
    isError: usageError,
    error: usageErrorObj,
    refetch: refetchUsage,
  } = useTenantUsage(tenantId);

  const {
    data: billingProfile,
  } = useTenantBillingProfile(tenantId);

  const {
    data: notesData,
    isLoading: notesLoading,
  } = useTenantNotes(tenantId);

  const {
    data: auditData,
    isLoading: auditLoading,
  } = useTenantAudit(tenantId);

  const changeStatus = useChangeTenantStatus();
  const addNote = useAddTenantNote();
  const updateSubscription = useUpdateSubscription();
  const setBillingProfileVerified = useSetBillingProfileVerified();
  const updateBillingProfile = useUpdateTenantBillingProfile();
  const cancelOrder = useCancelOrder();

  const [profileOverride, setProfileOverride] = useState<{
    tenantId: string;
    profile: BillingProfile;
  } | null>(null);
  const [profileError, setProfileError] = useState<{
    tenantId: string;
    message: string;
  } | null>(null);

  // Billing profile dialog state
  const [profileDialogOpen, setProfileDialogOpen] = useState(false);
  const [profileKind, setProfileKind] = useState<"individual" | "business">("individual");
  const [profileLegalNameInput, setProfileLegalNameInput] = useState("");
  const [profileTaxCodeInput, setProfileTaxCodeInput] = useState("");
  const [profileAddressInput, setProfileAddressInput] = useState("");
  const [profileInvoiceEmailInput, setProfileInvoiceEmailInput] = useState("");
  const [profileFieldErrors, setProfileFieldErrors] = useState<
    Partial<Record<BillingProfileField, string>>
  >({});
  const [profileFormError, setProfileFormError] = useState<string | null>(null);

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [targetStatus, setTargetStatus] = useState("active");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [noteBody, setNoteBody] = useState("");
  const [noteError, setNoteError] = useState<string | null>(null);

  // Subscription dialog state
  const [subDialogOpen, setSubDialogOpen] = useState(false);
  const [subPlanKey, setSubPlanKey] = useState("pro");
  const [subSeats, setSubSeats] = useState(1);
  const [subInterval, setSubInterval] = useState("month");
  const [subTrialEnd, setSubTrialEnd] = useState("");
  const [subReason, setSubReason] = useState("");
  const [subError, setSubError] = useState<string | null>(null);

  // Usage debt write-off dialog state
  const [writeOffOrder, setWriteOffOrder] = useState<TenantUsageUnpaidOrder | null>(null);
  const [writeOffError, setWriteOffError] = useState<string | null>(null);

  const effectiveRole = userRole ?? me?.role;
  const canEditSubscription = effectiveRole === "billing" || effectiveRole === "admin";
  const canWriteOff = me === null ? false : (me?.role ?? userRole) === "admin";

  if (tenantLoading) {
    return <LoadingState message="Loading tenant details..." />;
  }

  if (tenantError || !tenant) {
    return (
      <ErrorState
        title="Failed to load tenant"
        message={
          tenantErrorObj instanceof Error
            ? tenantErrorObj.message
            : "The tenant could not be found."
        }
        onRetry={refetchTenant}
      />
    );
  }

  const handleOpenStatusDialog = () => {
    setTargetStatus(tenant.state?.status || tenant.status || "active");
    setStatusError(null);
    setStatusDialogOpen(true);
  };

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await changeStatus.mutateAsync({
        tenantId,
        status: targetStatus as any,
        reason,
      });
      setStatusDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : "Failed to change workspace status";
      setStatusError(msg);
    }
  };

  const handleOpenSubDialog = () => {
    setSubPlanKey(tenant.subscription?.plan_key || "pro");
    setSubSeats(tenant.subscription?.seats ?? 1);
    setSubInterval(tenant.subscription?.interval || "month");
    setSubTrialEnd(tenant.subscription?.trial_end || "");
    setSubReason("");
    setSubError(null);
    setSubDialogOpen(true);
  };

  const handleSaveSubscription = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!subReason.trim() || updateSubscription.isPending) return;
    setSubError(null);
    try {
      await updateSubscription.mutateAsync({
        tenantId,
        planKey: subPlanKey,
        seats: Number(subSeats),
        interval: subInterval,
        trialEnd: subTrialEnd.trim() || null,
        reason: subReason.trim(),
      });
      setSubDialogOpen(false);
    } catch (err: unknown) {
      setSubError(err instanceof Error ? err.message : "Failed to update subscription");
    }
  };

  const handleAddNote = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!noteBody.trim() || addNote.isPending) return;
    setNoteError(null);
    try {
      await addNote.mutateAsync({
        tenantId,
        body: noteBody.trim(),
      });
      setNoteBody("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to add note";
      setNoteError(msg);
    }
  };

  const handleOpenWriteOffDialog = (order: TenantUsageUnpaidOrder) => {
    setWriteOffOrder(order);
    setWriteOffError(null);
  };

  const handleConfirmWriteOff = async (reason: string) => {
    if (!writeOffOrder) return;
    setWriteOffError(null);
    try {
      await cancelOrder.mutateAsync({
        orderId: writeOffOrder.id,
        reason,
      });
      setWriteOffOrder(null);
    } catch (err: unknown) {
      setWriteOffError(err instanceof Error ? err.message : "Failed to write off order");
    }
  };

  const fullProfile: BillingProfile | null =
    (profileOverride?.tenantId === tenantId ? profileOverride.profile : null) ??
    billingProfile ??
    null;
  const activeProfile = fullProfile ?? tenant.billing_profile ?? null;
  const accountKind = activeProfile?.account_kind ?? "individual";
  const profileLegalName = fullProfile?.legal_name ?? "";
  const profileTaxCode = fullProfile?.tax_code ?? "";
  const profileAddress = fullProfile?.address ?? "";
  const profileInvoiceEmail = fullProfile?.invoice_email ?? "";
  const isProfileVerified = activeProfile?.verified ?? false;
  const profileVerifiedAt = activeProfile?.verified_at ?? null;

  const activeProfileError =
    profileError?.tenantId === tenantId ? profileError.message : null;

  const clearProfileFieldError = (field: BillingProfileField) => {
    setProfileFieldErrors((prev) => {
      if (!(field in prev)) return prev;
      const next = { ...prev };
      delete next[field];
      return next;
    });
  };

  const handleOpenProfileDialog = () => {
    setProfileKind(accountKind === "business" ? "business" : "individual");
    setProfileLegalNameInput(profileLegalName);
    setProfileTaxCodeInput(profileTaxCode);
    setProfileAddressInput(profileAddress);
    setProfileInvoiceEmailInput(profileInvoiceEmail);
    setProfileFieldErrors({});
    setProfileFormError(null);
    setProfileDialogOpen(true);
  };

  const handleSaveBillingProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (updateBillingProfile.isPending) return;
    setProfileFieldErrors({});
    setProfileFormError(null);
    try {
      const res = await updateBillingProfile.mutateAsync({
        tenantId,
        account_kind: profileKind,
        legal_name: profileLegalNameInput.trim(),
        tax_code: profileTaxCodeInput.trim(),
        address: profileAddressInput.trim(),
        invoice_email: profileInvoiceEmailInput.trim(),
      });
      const { changed: _changed, ...savedProfile } = res;
      setProfileOverride({ tenantId, profile: savedProfile });
      setProfileError(null);
      setProfileDialogOpen(false);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError) {
        const fields = Array.isArray(err.details.fields)
          ? err.details.fields.filter((f): f is string => typeof f === "string" && f.length > 0)
          : [];
        if (fields.length > 0) {
          const messages =
            err.code === "billing_profile_incomplete"
              ? BILLING_PROFILE_INCOMPLETE_MESSAGES
              : BILLING_PROFILE_INVALID_MESSAGES;
          const nextFieldErrors: Partial<Record<BillingProfileField, string>> = {};
          const unmapped: string[] = [];
          for (const f of fields) {
            if (isBillingProfileField(f)) {
              nextFieldErrors[f] = messages[f];
            } else {
              unmapped.push(f);
            }
          }
          setProfileFieldErrors(nextFieldErrors);
          if (unmapped.length > 0 || Object.keys(nextFieldErrors).length === 0) {
            setProfileFormError(
              unmapped.length > 0 ? `${err.message}: ${unmapped.join(", ")}` : err.message,
            );
          }
          return;
        }
        setProfileFormError(err.message);
        return;
      }
      setProfileFormError(
        err instanceof Error ? err.message : "Failed to update billing profile",
      );
    }
  };

  const handleToggleProfileVerified = async () => {
    if (setBillingProfileVerified.isPending) return;
    setProfileError(null);
    try {
      const res = await setBillingProfileVerified.mutateAsync({
        tenantId,
        verified: !isProfileVerified,
      });
      if (res) {
        setProfileOverride({
          tenantId,
          profile: {
            legal_name: profileLegalName,
            tax_code: profileTaxCode,
            address: profileAddress,
            invoice_email: profileInvoiceEmail,
            ...res,
          },
        });
      }
    } catch (err: unknown) {
      if (err instanceof PlatformApiError && err.code === "billing_profile_incomplete") {
        const fields = Array.isArray(err.details.fields)
          ? err.details.fields.filter((f): f is string => typeof f === "string" && f.length > 0)
          : [];
        setProfileError({
          tenantId,
          message:
            fields.length > 0
              ? `Missing billing profile fields: ${fields.join(", ")}`
              : err.message,
        });
      } else {
        setProfileError({
          tenantId,
          message:
            err instanceof Error
              ? err.message
              : "Failed to update billing profile verification",
        });
      }
    }
  };

  const availableStatuses = [
    { value: "active", label: "Active" },
    { value: "read_only", label: "Read-only" },
    { value: "suspended", label: "Suspended" },
    ...(effectiveRole === "admin"
      ? [{ value: "pending_deletion", label: "Pending deletion" }]
      : []),
  ];

  const notes: Note[] = Array.isArray(notesData)
    ? notesData
    : (notesData as unknown as { notes?: Note[] })?.notes || [];
  const auditEvents: AuditEvent[] = Array.isArray(auditData)
    ? auditData
    : (auditData as unknown as { events?: AuditEvent[] })?.events || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={onBack}
            className="gap-1 text-muted-foreground"
          >
            <ArrowLeft className="size-4" />
            <span>Back to tenants</span>
          </Button>
          <div>
            <h1 className="font-heading text-title-lg font-bold">{tenant.name}</h1>
            <p className="font-mono text-caption text-muted-foreground">
              {tenant.slug} &bull; ID: {tenant.id}
            </p>
          </div>
        </div>
        <Button variant="outline" onClick={handleOpenStatusDialog}>
          Change status
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle>Workspace State</CardTitle>
            <CardDescription>
              Current status, state rationale, and tenant metadata.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-body sm:grid-cols-3">
              <div>
                <span className="block text-caption text-muted-foreground">Status</span>
                <div className="mt-1">
                  <StatusBadge status={tenant.state?.status || tenant.status} />
                </div>
              </div>
              <div>
                <span className="block text-caption text-muted-foreground">Members</span>
                <span className="mt-1 block font-medium tabular-nums">
                  {tenant.member_count}
                </span>
              </div>
              <div>
                <span className="block text-caption text-muted-foreground">Account groups</span>
                <span className="mt-1 block font-medium tabular-nums">
                  {tenant.group_count}
                </span>
              </div>
              <div>
                <span className="block text-caption text-muted-foreground">Owner</span>
                <span className="mt-1 block font-medium">
                  {tenant.owner?.email || "Unknown"}
                </span>
              </div>
              <div>
                <span className="block text-caption text-muted-foreground">Created</span>
                <span className="mt-1 block text-muted-foreground">
                  {tenant.created_at ? new Date(tenant.created_at).toLocaleString() : "-"}
                </span>
              </div>
              <div>
                <span className="block text-caption text-muted-foreground">Last Activity</span>
                <span className="mt-1 block text-muted-foreground">
                  {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
                </span>
              </div>
              {tenant.state?.changed_at && (
                <div>
                  <span className="block text-caption text-muted-foreground">Status Changed At</span>
                  <span className="mt-1 block text-muted-foreground">
                    {new Date(tenant.state.changed_at).toLocaleString()}
                  </span>
                </div>
              )}
            </div>

            {tenant.state?.reason && (
              <div className="rounded-lg border border-surface-border bg-muted/40 p-3">
                <span className="block text-caption font-medium text-foreground">
                  State Reason:
                </span>
                <p className="mt-0.5 text-body text-muted-foreground">
                  {tenant.state.reason}
                </p>
              </div>
            )}

            {tenant.state?.deletion_scheduled_at && (
              <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-destructive">
                <span className="block text-caption font-medium">
                  Deletion Scheduled At:
                </span>
                <p className="mt-0.5 text-body">
                  {new Date(tenant.state.deletion_scheduled_at).toLocaleString()}
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Internal Notes</CardTitle>
            <CardDescription>
              Staff-only notes. Never visible to tenant members.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <form onSubmit={handleAddNote} className="space-y-2">
              <Textarea
                value={noteBody}
                onChange={(e) => setNoteBody(e.target.value)}
                placeholder="Write an internal note..."
                rows={3}
                disabled={addNote.isPending}
              />
              {noteError && (
                <p role="alert" className="text-caption text-destructive">
                  {noteError}
                </p>
              )}
              <Button
                type="submit"
                size="sm"
                className="w-full gap-1"
                disabled={!noteBody.trim() || addNote.isPending}
                aria-busy={addNote.isPending}
              >
                {addNote.isPending ? (
                  <Spinner className="size-3.5" />
                ) : (
                  <Plus className="size-3.5" />
                )}
                Add note
              </Button>
            </form>

            <div className="space-y-2.5 pt-2">
              {notesLoading && <LoadingState message="Loading notes..." />}
              {!notesLoading && notes.length === 0 && (
                <p className="text-center text-caption text-muted-foreground py-4">
                  No internal notes yet.
                </p>
              )}
              {!notesLoading &&
                notes.map((note: Note) => (
                  <div
                    key={note.id}
                    className="rounded-lg border border-surface-border bg-surface p-3 text-caption"
                  >
                    <p className="text-body whitespace-pre-wrap">{note.body}</p>
                    <div className="mt-2 flex items-center justify-between text-muted-foreground">
                      <span>{note.author_email || "Staff"}</span>
                      <span>{note.created_at ? new Date(note.created_at).toLocaleDateString() : ""}</span>
                    </div>
                  </div>
                ))}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Subscription Panel */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle role="heading" aria-level={2}>Subscription</CardTitle>
            <CardDescription>
              Workspace plan, seats, billing interval, and trial period.
            </CardDescription>
          </div>
          {canEditSubscription && (
            <Button variant="outline" size="sm" onClick={handleOpenSubDialog}>
              Edit subscription
            </Button>
          )}
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 text-body sm:grid-cols-4">
            <div>
              <span className="block text-caption text-muted-foreground">Plan</span>
              <span className="mt-1 block font-medium capitalize">
                {tenant.subscription?.plan_key || "free"}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Seats</span>
              <span className="mt-1 block font-medium tabular-nums">
                {tenant.subscription?.seats ?? 0}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Interval</span>
              <span className="mt-1 block font-medium capitalize">
                {tenant.subscription?.interval || "month"}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Trial End</span>
              <span className="mt-1 block text-muted-foreground">
                {tenant.subscription?.trial_end
                  ? new Date(tenant.subscription.trial_end).toLocaleDateString()
                  : "-"}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Billing Profile */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle role="heading" aria-level={2}>Billing profile</CardTitle>
          {canEditSubscription && (
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" onClick={handleOpenProfileDialog}>
                Edit
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleToggleProfileVerified}
                disabled={setBillingProfileVerified.isPending}
                aria-busy={setBillingProfileVerified.isPending}
              >
                {setBillingProfileVerified.isPending && <Spinner className="size-3.5" />}
                {isProfileVerified ? "Unverify" : "Verify"}
              </Button>
            </div>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4 text-body sm:grid-cols-3">
            <div>
              <span className="block text-caption text-muted-foreground">Account kind</span>
              <span className="mt-1 block font-medium capitalize">
                {accountKind}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Status</span>
              <div className="mt-1">
                {isProfileVerified ? (
                  <Badge
                    variant="outline"
                    className="border-emerald-600/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400"
                  >
                    Verified
                  </Badge>
                ) : (
                  <Badge variant="outline">Unverified</Badge>
                )}
              </div>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Verified at</span>
              <span className="mt-1 block text-muted-foreground">
                {formatTimestamp(profileVerifiedAt)}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Legal name</span>
              <span className="mt-1 block font-medium">
                {profileLegalName || "-"}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Tax code</span>
              <span className="mt-1 block font-mono">
                {profileTaxCode || "-"}
              </span>
            </div>
            <div>
              <span className="block text-caption text-muted-foreground">Address</span>
              <span className="mt-1 block">
                {profileAddress || "-"}
              </span>
            </div>
            <div className="sm:col-span-2">
              <span className="block text-caption text-muted-foreground">Invoice email</span>
              <span className="mt-1 block">
                {profileInvoiceEmail || "-"}
              </span>
            </div>
          </div>

          {activeProfileError && (
            <p role="alert" className="text-caption text-destructive">
              {activeProfileError}
            </p>
          )}
        </CardContent>
      </Card>

      {/* Usage */}
      <Card>
        <CardHeader>
          <CardTitle role="heading" aria-level={2}>Usage</CardTitle>
          <CardDescription>
            Counts only. Current counts are read now; the daily snapshot is the last closed UTC day.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div
            data-testid="usage-current"
            className="grid grid-cols-2 gap-4 text-body sm:grid-cols-5"
          >
            <UsageValue label="Members now" value={tenant.usage_current?.members ?? 0} />
            <UsageValue label="Agents now" value={tenant.usage_current?.agents ?? 0} />
            <UsageValue label="Runtimes now" value={tenant.usage_current?.runtimes ?? 0} />
            <UsageValue label="Issues now" value={tenant.usage_current?.issues_total ?? 0} />
            <UsageValue
              label="Storage now"
              value={formatBytes(tenant.usage_current?.storage_bytes ?? 0)}
            />
          </div>

          {tenant.usage_day ? (
            <div data-testid="usage-daily" className="space-y-2">
              <span className="block text-caption text-muted-foreground">
                Daily snapshot for {tenant.usage_day} (UTC)
              </span>
              <div className="grid grid-cols-2 gap-4 text-body sm:grid-cols-4">
                <UsageValue label="Members" value={tenant.usage_members ?? 0} />
                <UsageValue label="Agents" value={tenant.usage_agents ?? 0} />
                <UsageValue label="Runtimes" value={tenant.usage_runtimes ?? 0} />
                <UsageValue label="Issues" value={tenant.usage_issues_total ?? 0} />
                <UsageValue label="Issues created" value={tenant.usage_issues_created ?? 0} />
                <UsageValue label="Tasks run" value={tenant.usage_tasks_run ?? 0} />
                <UsageValue label="Storage" value={formatBytes(tenant.usage_storage_bytes ?? 0)} />
              </div>
            </div>
          ) : (
            <p data-testid="usage-daily" className="text-caption text-muted-foreground">
              No daily snapshot yet.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Monthly Usage */}
      <MonthlyUsageCard
        usageData={usageData}
        isLoading={usageLoading}
        isError={usageError}
        error={usageErrorObj}
        onRetry={refetchUsage}
        canWriteOff={canWriteOff}
        onWriteOff={handleOpenWriteOffDialog}
      />

      {/* Recent Orders */}
      <Card>
        <CardHeader>
          <CardTitle role="heading" aria-level={2}>Recent Orders</CardTitle>
          <CardDescription>
            Recent payment orders for this workspace (up to 20, newest first).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!tenant.recent_orders || tenant.recent_orders.length === 0 ? (
            <p className="text-center text-caption text-muted-foreground py-4">
              No recent orders for this workspace.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Time</TableHead>
                    <TableHead>Transfer Code</TableHead>
                    <TableHead>Plan & Seats</TableHead>
                    <TableHead>Total (incl. VAT)</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tenant.recent_orders.map((order: Order) => (
                    <TableRow key={order.id}>
                      <TableCell className="text-muted-foreground">
                        {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                      </TableCell>
                      <TableCell className="font-mono text-body font-semibold">
                        {order.transfer_code}
                      </TableCell>
                      <TableCell className="capitalize">
                        {order.plan_key} &bull; {order.seats} seats ({order.interval})
                      </TableCell>
                      <TableCell className="font-medium">
                        {(order.amount + order.vat_amount).toLocaleString()} {order.currency.toUpperCase()}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={order.status} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Workspace Audit Log</CardTitle>
          <CardDescription>
            Audit events recorded for this workspace.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {auditLoading && <LoadingState message="Loading audit events..." />}
          {!auditLoading && auditEvents.length === 0 && (
            <EmptyState
              title="No audit events"
              description="No events found for this workspace."
            />
          )}
          {!auditLoading && auditEvents.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Target</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {auditEvents.map((event: AuditEvent) => (
                  <TableRow key={event.id}>
                    <TableCell className="text-muted-foreground">
                      {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                    </TableCell>
                    <TableCell className="font-mono text-caption">
                      {event.action}
                    </TableCell>
                    <TableCell>{event.actor_label}</TableCell>
                    <TableCell>{event.target_label || event.target_id}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <ConfirmReasonDialog
        open={statusDialogOpen}
        onOpenChange={setStatusDialogOpen}
        title="Change workspace status"
        description="Select the new status and provide a justification."
        confirmLabel="Update status"
        isPending={changeStatus.isPending}
        error={statusError}
        onConfirm={handleConfirmStatusChange}
      >
        <div className="flex flex-col gap-1.5">
          <label htmlFor="target-status-select" className="text-caption font-medium text-foreground">
            Target status
          </label>
          <select
            id="target-status-select"
            value={targetStatus}
            onChange={(e) => setTargetStatus(e.target.value)}
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            disabled={changeStatus.isPending}
          >
            {availableStatuses.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      </ConfirmReasonDialog>

      {/* Edit Subscription Dialog */}
      <Dialog open={subDialogOpen} onOpenChange={setSubDialogOpen}>
        <DialogContent>
          <form onSubmit={handleSaveSubscription} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Edit Subscription</DialogTitle>
              <DialogDescription>
                Update subscription plan, seats, billing interval, and trial period.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
                <label htmlFor="sub-plan-key-input" className="text-caption font-medium text-foreground">
                  Plan <span className="text-destructive">*</span>
                </label>
                <Input
                  id="sub-plan-key-input"
                  value={subPlanKey}
                  onChange={(e) => setSubPlanKey(e.target.value)}
                  placeholder="e.g. pro, business, enterprise"
                  required
                  disabled={updateSubscription.isPending}
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="sub-seats-input" className="text-caption font-medium text-foreground">
                    Seats <span className="text-destructive">*</span>
                  </label>
                  <Input
                    id="sub-seats-input"
                    type="number"
                    min={0}
                    value={subSeats}
                    onChange={(e) => setSubSeats(Number(e.target.value))}
                    required
                    disabled={updateSubscription.isPending}
                  />
                </div>

                <div className="flex flex-col gap-1.5">
                  <label htmlFor="sub-interval-select" className="text-caption font-medium text-foreground">
                    Billing Interval <span className="text-destructive">*</span>
                  </label>
                  <select
                    id="sub-interval-select"
                    value={subInterval}
                    onChange={(e) => setSubInterval(e.target.value)}
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    disabled={updateSubscription.isPending}
                  >
                    <option value="month">Month</option>
                    <option value="year">Year</option>
                  </select>
                </div>
              </div>

              <div className="flex flex-col gap-1.5">
                <label htmlFor="sub-trial-end-input" className="text-caption font-medium text-foreground">
                  Trial End (RFC3339 or blank)
                </label>
                <Input
                  id="sub-trial-end-input"
                  value={subTrialEnd}
                  onChange={(e) => setSubTrialEnd(e.target.value)}
                  placeholder="e.g. 2026-12-01T00:00:00Z"
                  disabled={updateSubscription.isPending}
                />
              </div>

              <div className="flex flex-col gap-1.5">
                <label htmlFor="sub-reason-input" className="text-caption font-medium text-foreground">
                  Reason <span className="text-destructive">*</span>
                </label>
                <Textarea
                  id="sub-reason-input"
                  value={subReason}
                  onChange={(e) => setSubReason(e.target.value)}
                  placeholder="Provide a reason for this subscription change..."
                  rows={3}
                  required
                  disabled={updateSubscription.isPending}
                />
              </div>
            </div>

            {subError && (
              <div role="alert" className="text-caption text-destructive">
                {subError}
              </div>
            )}

            <DialogFooter>
              <DialogClose render={<Button variant="outline" type="button" disabled={updateSubscription.isPending} />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={!subReason.trim() || updateSubscription.isPending}>
                {updateSubscription.isPending && <Spinner className="size-3.5" />}
                Save subscription
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Billing Profile Dialog */}
      <Dialog open={profileDialogOpen} onOpenChange={setProfileDialogOpen}>
        <DialogContent>
          <form noValidate onSubmit={handleSaveBillingProfile} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Edit Billing Profile</DialogTitle>
              <DialogDescription>
                Update workspace account kind and billing contact details.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
                <label htmlFor="bp-account-kind" className="text-caption font-medium text-foreground">
                  Account kind
                </label>
                <select
                  id="bp-account-kind"
                  value={profileKind}
                  onChange={(e) => {
                    setProfileKind(e.target.value as "individual" | "business");
                    clearProfileFieldError("account_kind");
                  }}
                  aria-invalid={Boolean(profileFieldErrors.account_kind)}
                  disabled={updateBillingProfile.isPending}
                  className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20"
                >
                  <option value="individual">Individual</option>
                  <option value="business">Business</option>
                </select>
                {profileFieldErrors.account_kind && (
                  <p className="text-caption text-destructive">{profileFieldErrors.account_kind}</p>
                )}
              </div>

              {[
                {
                  field: "legal_name" as const,
                  id: "bp-legal-name",
                  label: "Legal name",
                  type: "text",
                  value: profileLegalNameInput,
                  setValue: setProfileLegalNameInput,
                },
                {
                  field: "tax_code" as const,
                  id: "bp-tax-code",
                  label: "Tax code",
                  type: "text",
                  value: profileTaxCodeInput,
                  setValue: setProfileTaxCodeInput,
                },
                {
                  field: "address" as const,
                  id: "bp-address",
                  label: "Address",
                  type: "text",
                  value: profileAddressInput,
                  setValue: setProfileAddressInput,
                },
                {
                  field: "invoice_email" as const,
                  id: "bp-invoice-email",
                  label: "Invoice email",
                  type: "email",
                  value: profileInvoiceEmailInput,
                  setValue: setProfileInvoiceEmailInput,
                },
              ].map(({ field, id, label, type, value, setValue }) => (
                <div key={field} className="flex flex-col gap-1.5">
                  <label htmlFor={id} className="text-caption font-medium text-foreground">
                    {label}
                  </label>
                  <Input
                    id={id}
                    type={type}
                    value={value}
                    onChange={(e) => {
                      setValue(e.target.value);
                      clearProfileFieldError(field);
                    }}
                    aria-invalid={Boolean(profileFieldErrors[field])}
                    disabled={updateBillingProfile.isPending}
                  />
                  {profileFieldErrors[field] && (
                    <p className="text-caption text-destructive">{profileFieldErrors[field]}</p>
                  )}
                </div>
              ))}
            </div>

            {profileFormError && (
              <div role="alert" className="text-caption text-destructive">
                {profileFormError}
              </div>
            )}

            <DialogFooter>
              <DialogClose
                render={
                  <Button variant="outline" type="button" disabled={updateBillingProfile.isPending} />
                }
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={updateBillingProfile.isPending}
                aria-busy={updateBillingProfile.isPending}
              >
                {updateBillingProfile.isPending && (
                  <Spinner className="size-3.5" aria-hidden="true" />
                )}
                Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmReasonDialog
        open={writeOffOrder !== null}
        onOpenChange={(open) => {
          if (!open) {
            setWriteOffOrder(null);
            setWriteOffError(null);
          }
        }}
        title="Write off usage order"
        description={
          writeOffOrder
            ? `Write off pending usage order ${writeOffOrder.id} (${(writeOffOrder.amount + writeOffOrder.vat_amount).toLocaleString()} total)? Its lines will stay excluded from future invoices.`
            : ""
        }
        confirmLabel="Write off order"
        confirmVariant="destructive"
        isPending={cancelOrder.isPending}
        error={writeOffError}
        onConfirm={handleConfirmWriteOff}
      />
    </div>
  );
}
