import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { Section } from "@astryxdesign/core/Section";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription, AlertTitle } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { Field, FieldError, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Activity, AlertCircle, AlertTriangle, CreditCard, Info, Plus } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import {
  useMe,
  useWorkspace,
  useWorkspaceUsage,
  useWorkspaceBillingProfile,
  useWorkspaceNotes,
  useWorkspaceAudit,
  useChangeWorkspaceStatus,
  useAddWorkspaceNote,
  useUpdateSubscription,
  useSetBillingProfileVerified,
  useUpdateWorkspaceBillingProfile,
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
  WorkspaceUsage,
  WorkspaceUsageRow,
  WorkspaceUsageUnpaidOrder,
} from "../api/schemas";
import { formatMinorAmount } from "../money";

interface WorkspaceDetailProps {
  workspaceId: string;
  onBack: () => void;
  /** The label of the back button: the tenant the workspace was opened from, or the tenant list. */
  backLabel?: string;
  userRole?: StaffRole;
}

function formatUsageQuantity(row: WorkspaceUsageRow): string {
  if (row.meter === "storage_bytes") {
    return formatBytes(row.quantity);
  }
  return row.quantity.toLocaleString();
}

interface MonthlyUsageSectionProps {
  usageData: WorkspaceUsage | undefined;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  onRetry: () => void;
  canWriteOff: boolean;
  onWriteOff: (order: WorkspaceUsageUnpaidOrder) => void;
}

/** Monthly rated usage charges, unrated missing days, and unpaid usage invoices. */
function MonthlyUsageSection({
  usageData,
  isLoading,
  isError,
  error,
  onRetry,
  canWriteOff,
  onWriteOff,
}: MonthlyUsageSectionProps) {
  return (
    <Section variant="transparent" padding={0} data-testid="usage-month" className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <Heading level={2}>Monthly Usage</Heading>
        {usageData && (
          <Text type="code" color="secondary">
            {usageData.period_start.slice(0, 10)} to {usageData.period_end.slice(0, 10)} (UTC)
          </Text>
        )}
      </div>

      {isLoading && <LoadingState message="Loading monthly usage..." />}
      {isError && (
        <ErrorState
          title="Failed to load monthly usage"
          message={error instanceof Error ? error.message : "Unable to load monthly usage."}
          onRetry={onRetry}
        />
      )}
      {!isLoading && !isError && usageData && (
        <div className="space-y-6">
          <div data-testid="usage-rows" className="space-y-2">
            <Heading level={3}>Daily charges</Heading>
            {usageData.rows.length === 0 ? (
              <Text type="supporting" as="p">
                No rated usage rows for this month.
              </Text>
            ) : (
              <Card padding={0} className="overflow-x-auto">
                <Table
                  data={usageData.rows}
                  idKey={(row) => `${row.day}-${row.meter}`}
                  density="compact"
                  dividers="rows"
                  hasHover
                  columns={[
                    {
                      key: "day",
                      header: "Day",
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (row) => (
                        <Text type="code" hasTabularNumbers>
                          {row.day}
                        </Text>
                      ),
                    },
                    {
                      key: "meter",
                      header: "Meter",
                      width: proportional(1.2, { minWidth: 130 }),
                      renderCell: (row) => <Text type="code">{row.meter}</Text>,
                    },
                    {
                      key: "quantity",
                      header: "Quantity",
                      width: proportional(1, { minWidth: 100 }),
                      align: "end",
                      renderCell: (row) => (
                        <span
                          title={row.meter === "storage_bytes" ? `${row.quantity.toLocaleString()} bytes` : undefined}
                        >
                          <Text type="body" hasTabularNumbers>
                            {formatUsageQuantity(row)}
                          </Text>
                        </span>
                      ),
                    },
                    {
                      key: "unit_price",
                      header: "Unit price",
                      width: proportional(1, { minWidth: 120 }),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="supporting" hasTabularNumbers>
                          {row.unit_price.toLocaleString()} / {row.per.toLocaleString()}
                        </Text>
                      ),
                    },
                    {
                      key: "discount_pct",
                      header: "Discount",
                      width: pixel(90),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="supporting" hasTabularNumbers>
                          {row.discount_pct > 0 ? `${row.discount_pct}%` : "-"}
                        </Text>
                      ),
                    },
                    {
                      key: "net_amount",
                      header: "Net amount",
                      width: proportional(1, { minWidth: 110 }),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="body" weight="medium" hasTabularNumbers>
                          {row.net_amount.toLocaleString()}
                        </Text>
                      ),
                    },
                    {
                      key: "note",
                      header: "Note",
                      width: proportional(1.5, { minWidth: 120 }),
                      renderCell: (row) => <Text type="supporting">{row.note || "-"}</Text>,
                    },
                  ]}
                />
              </Card>
            )}
          </div>

          {usageData.missing_days.length > 0 && (
            <div data-testid="usage-missing-days" className="space-y-2">
              <Heading level={3}>Missing days</Heading>
              <ul className="flex flex-wrap gap-2">
                {usageData.missing_days.map((day) => (
                  <li
                    key={day}
                    className="inline-flex items-center gap-1.5 rounded-md border border-surface-border bg-muted/40 px-2.5 py-1 text-caption"
                  >
                    <Text type="code">{day}</Text>
                    <Badge variant="neutral" label="missing" />
                  </li>
                ))}
              </ul>
            </div>
          )}

          <div data-testid="usage-unpaid-orders" className="space-y-2">
            <Heading level={3}>Unpaid usage orders</Heading>
            {usageData.unpaid_orders.length === 0 ? (
              <Text type="supporting" as="p">
                No unpaid usage orders.
              </Text>
            ) : (
              <Card padding={0} className="overflow-x-auto">
                <Table
                  data={usageData.unpaid_orders}
                  idKey="id"
                  density="compact"
                  dividers="rows"
                  hasHover
                  columns={[
                    {
                      key: "id",
                      header: "Order",
                      width: proportional(1.5, { minWidth: 140 }),
                      renderCell: (order) => <Text type="code">{order.id}</Text>,
                    },
                    {
                      key: "period_start",
                      header: "Period",
                      width: proportional(1.8, { minWidth: 170 }),
                      renderCell: (order) => (
                        <Text type="code" color="secondary" hasTabularNumbers>
                          {order.period_start && order.period_end
                            ? `${order.period_start.slice(0, 10)} – ${order.period_end.slice(0, 10)}`
                            : "-"}
                        </Text>
                      ),
                    },
                    {
                      key: "created_at",
                      header: "Created",
                      width: proportional(1.5, { minWidth: 150 }),
                      renderCell: (order) => (
                        <Text type="supporting" hasTabularNumbers>
                          {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                        </Text>
                      ),
                    },
                    {
                      key: "amount",
                      header: "Amount",
                      width: proportional(1, { minWidth: 100 }),
                      align: "end",
                      renderCell: (order) => (
                        <Text type="body" weight="medium" hasTabularNumbers>
                          {order.amount.toLocaleString()}
                        </Text>
                      ),
                    },
                    {
                      key: "vat_amount",
                      header: "VAT",
                      width: proportional(1, { minWidth: 90 }),
                      align: "end",
                      renderCell: (order) => (
                        <Text type="supporting" hasTabularNumbers>
                          {order.vat_amount.toLocaleString()}
                        </Text>
                      ),
                    },
                    {
                      key: "status",
                      header: "Status",
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (order) => <StatusBadge status={order.status} />,
                    },
                    ...(canWriteOff
                      ? [
                          {
                            key: "actions",
                            header: "Action",
                            width: pixel(120),
                            align: "end" as const,
                            renderCell: (order: WorkspaceUsageUnpaidOrder) => (
                              <Button variant="destructive" size="sm" onClick={() => onWriteOff(order)}>
                                Write off
                              </Button>
                            ),
                          },
                        ]
                      : []),
                  ]}
                />
              </Card>
            )}
          </div>
        </div>
      )}
    </Section>
  );
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return "-";
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? value : new Date(ms).toLocaleString();
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

const BILLING_INTERVAL_ITEMS = [
  { value: "month", label: "Month" },
  { value: "year", label: "Year" },
];

const ACCOUNT_KIND_ITEMS = [
  { value: "individual", label: "Individual" },
  { value: "business", label: "Business" },
];

export function WorkspaceDetail({ workspaceId, onBack, backLabel = "Back to tenants", userRole }: WorkspaceDetailProps) {
  const { data: me } = useMe();
  const {
    data: workspace,
    isLoading: workspaceLoading,
    isError: workspaceError,
    error: workspaceErrorObj,
    refetch: refetchWorkspace,
  } = useWorkspace(workspaceId);

  const {
    data: usageData,
    isLoading: usageLoading,
    isError: usageError,
    error: usageErrorObj,
    refetch: refetchUsage,
  } = useWorkspaceUsage(workspaceId);

  const {
    data: billingProfile,
  } = useWorkspaceBillingProfile(workspaceId);

  const {
    data: notesData,
    isLoading: notesLoading,
  } = useWorkspaceNotes(workspaceId);

  const {
    data: auditData,
    isLoading: auditLoading,
  } = useWorkspaceAudit(workspaceId);

  const changeStatus = useChangeWorkspaceStatus();
  const addNote = useAddWorkspaceNote();
  const updateSubscription = useUpdateSubscription();
  const setBillingProfileVerified = useSetBillingProfileVerified();
  const updateBillingProfile = useUpdateWorkspaceBillingProfile();
  const cancelOrder = useCancelOrder();

  const [profileOverride, setProfileOverride] = useState<{
    workspaceId: string;
    profile: BillingProfile;
  } | null>(null);
  const [profileError, setProfileError] = useState<{
    workspaceId: string;
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
  const [writeOffOrder, setWriteOffOrder] = useState<WorkspaceUsageUnpaidOrder | null>(null);
  const [writeOffError, setWriteOffError] = useState<string | null>(null);

  const effectiveRole = userRole ?? me?.role;
  const canEditSubscription = effectiveRole === "billing" || effectiveRole === "admin";
  const canWriteOff = me === null ? false : (me?.role ?? userRole) === "admin";

  if (workspaceLoading) {
    return <LoadingState message="Loading workspace details..." />;
  }

  if (workspaceError || !workspace) {
    return (
      <ErrorState
        title="Failed to load workspace"
        message={
          workspaceErrorObj instanceof Error
            ? workspaceErrorObj.message
            : "The workspace could not be found."
        }
        onRetry={refetchWorkspace}
      />
    );
  }

  const handleOpenStatusDialog = () => {
    setTargetStatus(workspace.state?.status || workspace.status || "active");
    setStatusError(null);
    setStatusDialogOpen(true);
  };

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await changeStatus.mutateAsync({
        workspaceId,
        status: targetStatus as "active" | "read_only" | "suspended" | "pending_deletion",
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
    setSubPlanKey(workspace.subscription?.plan_key || "pro");
    setSubSeats(workspace.subscription?.seats ?? 1);
    setSubInterval(workspace.subscription?.interval || "month");
    setSubTrialEnd(workspace.subscription?.trial_end || "");
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
        workspaceId,
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
        workspaceId,
        body: noteBody.trim(),
      });
      setNoteBody("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to add note";
      setNoteError(msg);
    }
  };

  const handleOpenWriteOffDialog = (order: WorkspaceUsageUnpaidOrder) => {
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
    (profileOverride?.workspaceId === workspaceId ? profileOverride.profile : null) ??
    billingProfile ??
    null;
  const activeProfile = fullProfile ?? workspace.billing_profile ?? null;
  const accountKind = activeProfile?.account_kind ?? "individual";
  const profileLegalName = fullProfile?.legal_name ?? "";
  const profileTaxCode = fullProfile?.tax_code ?? "";
  const profileAddress = fullProfile?.address ?? "";
  const profileInvoiceEmail = fullProfile?.invoice_email ?? "";
  const isProfileVerified = activeProfile?.verified ?? false;
  const profileVerifiedAt = activeProfile?.verified_at ?? null;

  const activeProfileError =
    profileError?.workspaceId === workspaceId ? profileError.message : null;

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
        workspaceId,
        account_kind: profileKind,
        legal_name: profileLegalNameInput.trim(),
        tax_code: profileTaxCodeInput.trim(),
        address: profileAddressInput.trim(),
        invoice_email: profileInvoiceEmailInput.trim(),
      });
      const { changed: _changed, ...savedProfile } = res;
      setProfileOverride({ workspaceId, profile: savedProfile });
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
        workspaceId,
        verified: !isProfileVerified,
      });
      if (res) {
        setProfileOverride({
          workspaceId,
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
          workspaceId,
          message:
            fields.length > 0
              ? `Missing billing profile fields: ${fields.join(", ")}`
              : err.message,
        });
      } else {
        setProfileError({
          workspaceId,
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

  const currentStatus = workspace.state?.status || workspace.status;

  return (
    <div className="space-y-8">
      <PageHeader
        title={workspace.name}
        badge={<StatusBadge status={currentStatus} />}
        meta={
          <span className="font-mono">
            {workspace.slug} &bull; ID: {workspace.id}
          </span>
        }
        backAction={{ label: backLabel, onClick: onBack }}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={handleOpenStatusDialog}>
              Change status
            </Button>
            {canEditSubscription && (
              <Button variant="outline" size="sm" onClick={handleOpenSubDialog}>
                Edit subscription
              </Button>
            )}
          </>
        }
      />

      {(workspace.state?.reason || workspace.state?.deletion_scheduled_at) && (
        <div className="space-y-3">
          {workspace.state?.reason && (
            <Alert role="status" className="border-surface-border bg-surface">
              <Info className="size-4 text-info" aria-hidden="true" />
              <AlertTitle className="text-label font-semibold">State Reason</AlertTitle>
              <AlertDescription className="text-body text-muted-foreground">
                {workspace.state.reason}
              </AlertDescription>
            </Alert>
          )}
          {workspace.state?.deletion_scheduled_at && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
              <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
              <AlertTitle className="text-label font-semibold">Deletion Scheduled</AlertTitle>
              <AlertDescription className="text-body tabular-nums">
                {new Date(workspace.state.deletion_scheduled_at).toLocaleString()}
              </AlertDescription>
            </Alert>
          )}
        </div>
      )}

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
        <Section variant="transparent" padding={0} className="space-y-4 lg:col-span-2">
          <Heading level={2}>Workspace State</Heading>
          <MetadataList columns={3}>
            <MetadataListItem label="Status">
              <StatusBadge status={currentStatus} />
            </MetadataListItem>
            <MetadataListItem label="Members" tabular>
              {workspace.member_count}
            </MetadataListItem>
            <MetadataListItem label="Account groups" tabular>
              {workspace.group_count}
            </MetadataListItem>
            <MetadataListItem label="Owner">
              {workspace.owner?.email || "Unknown"}
            </MetadataListItem>
            <MetadataListItem label="Created" tabular>
              {workspace.created_at ? new Date(workspace.created_at).toLocaleString() : "-"}
            </MetadataListItem>
            <MetadataListItem label="Last Activity" tabular>
              {workspace.last_activity_at
                ? new Date(workspace.last_activity_at).toLocaleString()
                : "-"}
            </MetadataListItem>
            {workspace.state?.changed_at && (
              <MetadataListItem label="Status Changed At" tabular>
                {new Date(workspace.state.changed_at).toLocaleString()}
              </MetadataListItem>
            )}
          </MetadataList>
        </Section>

        <Section variant="transparent" padding={0} className="space-y-4">
          <Heading level={2}>Internal Notes</Heading>
          <form onSubmit={handleAddNote} className="space-y-2">
            <Textarea
              value={noteBody}
              onChange={(e) => setNoteBody(e.target.value)}
              placeholder="Write an internal note..."
              rows={3}
              disabled={addNote.isPending}
            />
            {noteError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{noteError}</AlertDescription>
              </Alert>
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
                <Plus className="size-3.5" aria-hidden="true" />
              )}
              Add note
            </Button>
          </form>

          <div className="space-y-2">
            {notesLoading && <LoadingState message="Loading notes..." />}
            {!notesLoading && notes.length === 0 && (
              <Text type="supporting" as="p" justify="center" className="py-4">
                No internal notes yet.
              </Text>
            )}
            {!notesLoading && notes.length > 0 && (
              <Card padding={0} className="divide-y divide-surface-border">
                {notes.map((note: Note) => (
                  <div key={note.id} className="p-3">
                    <Text type="body" as="p" className="whitespace-pre-wrap">
                      {note.body}
                    </Text>
                    <div className="mt-2 flex items-center justify-between">
                      <Text type="supporting">{note.author_email || "Staff"}</Text>
                      <Text type="supporting" hasTabularNumbers>
                        {note.created_at ? new Date(note.created_at).toLocaleDateString() : ""}
                      </Text>
                    </div>
                  </div>
                ))}
              </Card>
            )}
          </div>
        </Section>
      </div>

      <Divider />

      {/* Subscription Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Heading level={2}>Subscription</Heading>
          {workspace.subscription?.status && (
            <StatusBadge status={workspace.subscription.status} />
          )}
        </div>
        <MetadataList columns={4}>
          <MetadataListItem label="Plan" className="capitalize">
            {workspace.subscription?.plan_key || "free"}
          </MetadataListItem>
          <MetadataListItem label="Seats" tabular>
            {workspace.subscription?.seats ?? 0}
          </MetadataListItem>
          <MetadataListItem label="Interval" className="capitalize">
            {workspace.subscription?.interval || "month"}
          </MetadataListItem>
          <MetadataListItem label="Trial End" tabular>
            {workspace.subscription?.trial_end
              ? new Date(workspace.subscription.trial_end).toLocaleDateString()
              : "-"}
          </MetadataListItem>
        </MetadataList>
      </Section>

      <Divider />

      {/* Billing Profile Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Heading level={2}>Billing profile</Heading>
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
        </div>

        <MetadataList columns={3}>
          <MetadataListItem label="Account kind" className="capitalize">
            {accountKind}
          </MetadataListItem>
          <MetadataListItem label="Status">
            <StatusBadge status={isProfileVerified ? "verified" : "unverified"} />
          </MetadataListItem>
          <MetadataListItem label="Verified at" tabular>
            {formatTimestamp(profileVerifiedAt)}
          </MetadataListItem>
          <MetadataListItem label="Legal name">
            {profileLegalName || "-"}
          </MetadataListItem>
          <MetadataListItem label="Tax code" mono>
            {profileTaxCode || "-"}
          </MetadataListItem>
          <MetadataListItem label="Address">
            {profileAddress || "-"}
          </MetadataListItem>
          <MetadataListItem label="Invoice email" className="sm:col-span-2">
            {profileInvoiceEmail || "-"}
          </MetadataListItem>
        </MetadataList>

        {activeProfileError && (
          <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
            <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
            <AlertDescription className="text-caption">{activeProfileError}</AlertDescription>
          </Alert>
        )}
      </Section>

      <Divider />

      {/* Usage Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>Usage</Heading>
        <MetadataList data-testid="usage-current" columns={4}>
          <MetadataListItem label="Members now" tabular>
            {workspace.usage_current?.members ?? 0}
          </MetadataListItem>
          <MetadataListItem label="Agents now" tabular>
            {workspace.usage_current?.agents ?? 0}
          </MetadataListItem>
          <MetadataListItem label="Runtimes now" tabular>
            {workspace.usage_current?.runtimes ?? 0}
          </MetadataListItem>
          <MetadataListItem label="Issues now" tabular>
            {workspace.usage_current?.issues_total ?? 0}
          </MetadataListItem>
          <MetadataListItem label="Storage now" tabular>
            {formatBytes(workspace.usage_current?.storage_bytes ?? 0)}
          </MetadataListItem>
        </MetadataList>

        {workspace.usage_day ? (
          <div data-testid="usage-daily" className="space-y-3 pt-2">
            <Text type="code" color="secondary" as="div">
              Daily snapshot for {workspace.usage_day} (UTC)
            </Text>
            <MetadataList columns={4}>
              <MetadataListItem label="Members" tabular>
                {workspace.usage_members ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Agents" tabular>
                {workspace.usage_agents ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Runtimes" tabular>
                {workspace.usage_runtimes ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Issues" tabular>
                {workspace.usage_issues_total ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Issues created" tabular>
                {workspace.usage_issues_created ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Tasks run" tabular>
                {workspace.usage_tasks_run ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Storage" tabular>
                {formatBytes(workspace.usage_storage_bytes ?? 0)}
              </MetadataListItem>
            </MetadataList>
          </div>
        ) : (
          <Text type="supporting" as="p" data-testid="usage-daily">
            No daily snapshot yet.
          </Text>
        )}
      </Section>

      <Divider />

      {/* Monthly Usage Section */}
      <MonthlyUsageSection
        usageData={usageData}
        isLoading={usageLoading}
        isError={usageError}
        error={usageErrorObj}
        onRetry={refetchUsage}
        canWriteOff={canWriteOff}
        onWriteOff={handleOpenWriteOffDialog}
      />

      <Divider />

      {/* Recent Orders Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>Recent Orders</Heading>
        {!workspace.recent_orders || workspace.recent_orders.length === 0 ? (
          <EmptyState
            icon={<CreditCard className="size-5" aria-hidden="true" />}
            title="No recent orders"
            isCompact
          />
        ) : (
          <Card padding={0} className="overflow-x-auto">
            <Table
              data={workspace.recent_orders}
              idKey="id"
              density="compact"
              dividers="rows"
              hasHover
              columns={[
                {
                  key: "created_at",
                  header: "Time",
                  width: proportional(1.4, { minWidth: 160 }),
                  renderCell: (order: Order) => (
                    <Text type="supporting" hasTabularNumbers>
                      {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                    </Text>
                  ),
                },
                {
                  key: "transfer_code",
                  header: "Transfer Code",
                  width: proportional(1.4, { minWidth: 150 }),
                  renderCell: (order: Order) => (
                    <Text type="code" weight="semibold">
                      {order.transfer_code}
                    </Text>
                  ),
                },
                {
                  key: "plan_key",
                  header: "Plan & Seats",
                  width: proportional(2, { minWidth: 180 }),
                  renderCell: (order: Order) => (
                    <Text type="body" className="capitalize">
                      {order.plan_key} &bull; {order.seats} seats ({order.interval})
                    </Text>
                  ),
                },
                {
                  key: "amount",
                  header: "Total (incl. VAT)",
                  width: proportional(1.2, { minWidth: 150 }),
                  renderCell: (order: Order) => (
                    <Text type="body" weight="medium" hasTabularNumbers>
                      {formatMinorAmount(order.amount + order.vat_amount, order.currency)}
                    </Text>
                  ),
                },
                {
                  key: "status",
                  header: "Status",
                  width: proportional(1, { minWidth: 110 }),
                  renderCell: (order: Order) => <StatusBadge status={order.status} />,
                },
              ]}
            />
          </Card>
        )}
      </Section>

      <Divider />

      {/* Workspace Audit Log Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>Workspace Audit Log</Heading>
        {auditLoading && <LoadingState message="Loading audit events..." />}
        {!auditLoading && auditEvents.length === 0 && (
          <EmptyState
            icon={<Activity className="size-5" aria-hidden="true" />}
            title="No audit events"
            description="No events found for this workspace."
            isCompact
          />
        )}
        {!auditLoading && auditEvents.length > 0 && (
          <Card padding={0} className="overflow-x-auto">
            <Table
              data={auditEvents}
              idKey="id"
              density="compact"
              dividers="rows"
              hasHover
              columns={[
                {
                  key: "created_at",
                  header: "Time",
                  width: proportional(1.4, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => (
                    <Text type="supporting" hasTabularNumbers>
                      {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                    </Text>
                  ),
                },
                {
                  key: "action",
                  header: "Action",
                  width: proportional(1.4, { minWidth: 150 }),
                  renderCell: (event: AuditEvent) => <Text type="code">{event.action}</Text>,
                },
                {
                  key: "actor_label",
                  header: "Actor",
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => <Text type="body">{event.actor_label}</Text>,
                },
                {
                  key: "target_label",
                  header: "Target",
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => (
                    <Text type="body">{event.target_label || event.target_id}</Text>
                  ),
                },
              ]}
            />
          </Card>
        )}
      </Section>

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
        <Field>
          <FieldLabel htmlFor="target-status-select" className="text-label">
            Target status
          </FieldLabel>
          <Select
            items={availableStatuses}
            value={targetStatus}
            onValueChange={(val) => {
              if (val) setTargetStatus(val);
            }}
            disabled={changeStatus.isPending}
          >
            <SelectTrigger id="target-status-select" aria-label="Target status" className="w-full">
              <SelectValue>
                {availableStatuses.find((opt) => opt.value === targetStatus)?.label ?? targetStatus}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {availableStatuses.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
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
              <Field>
                <FieldLabel htmlFor="sub-plan-key-input" className="text-label">
                  Plan <span className="text-destructive">*</span>
                </FieldLabel>
                <Input
                  id="sub-plan-key-input"
                  value={subPlanKey}
                  onChange={(e) => setSubPlanKey(e.target.value)}
                  placeholder="e.g. pro, business, enterprise"
                  required
                  disabled={updateSubscription.isPending}
                />
              </Field>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="sub-seats-input" className="text-label">
                    Seats <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="sub-seats-input"
                    type="number"
                    min={0}
                    value={subSeats}
                    onChange={(e) => setSubSeats(Number(e.target.value))}
                    required
                    disabled={updateSubscription.isPending}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="sub-interval-select" className="text-label">
                    Billing Interval <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Select
                    items={BILLING_INTERVAL_ITEMS}
                    value={subInterval}
                    onValueChange={(val) => {
                      if (val) setSubInterval(val);
                    }}
                    disabled={updateSubscription.isPending}
                  >
                    <SelectTrigger
                      id="sub-interval-select"
                      aria-label="Billing Interval"
                      className="w-full"
                    >
                      <SelectValue>
                        {BILLING_INTERVAL_ITEMS.find((i) => i.value === subInterval)?.label ??
                          subInterval}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {BILLING_INTERVAL_ITEMS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              </div>

              <Field>
                <FieldLabel htmlFor="sub-trial-end-input" className="text-label">
                  Trial End (RFC3339 or blank)
                </FieldLabel>
                <Input
                  id="sub-trial-end-input"
                  value={subTrialEnd}
                  onChange={(e) => setSubTrialEnd(e.target.value)}
                  placeholder="e.g. 2026-12-01T00:00:00Z"
                  disabled={updateSubscription.isPending}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="sub-reason-input" className="text-label">
                  Reason <span className="text-destructive">*</span>
                </FieldLabel>
                <Textarea
                  id="sub-reason-input"
                  value={subReason}
                  onChange={(e) => setSubReason(e.target.value)}
                  placeholder="Provide a reason for this subscription change..."
                  rows={3}
                  required
                  disabled={updateSubscription.isPending}
                />
              </Field>
            </div>

            {subError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{subError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={updateSubscription.isPending} />}
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={!subReason.trim() || updateSubscription.isPending}
                aria-busy={updateSubscription.isPending}
              >
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
              <Field data-invalid={Boolean(profileFieldErrors.account_kind) || undefined}>
                <FieldLabel htmlFor="bp-account-kind" className="text-label">
                  Account kind
                </FieldLabel>
                <Select
                  items={ACCOUNT_KIND_ITEMS}
                  value={profileKind}
                  onValueChange={(val) => {
                    if (val === "individual" || val === "business") {
                      setProfileKind(val);
                      clearProfileFieldError("account_kind");
                    }
                  }}
                  disabled={updateBillingProfile.isPending}
                >
                  <SelectTrigger
                    id="bp-account-kind"
                    aria-label="Account kind"
                    aria-invalid={Boolean(profileFieldErrors.account_kind)}
                    className="w-full"
                  >
                    <SelectValue>
                      {ACCOUNT_KIND_ITEMS.find((i) => i.value === profileKind)?.label ??
                        profileKind}
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
                {profileFieldErrors.account_kind && (
                  <FieldError className="text-caption">
                    {profileFieldErrors.account_kind}
                  </FieldError>
                )}
              </Field>

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
                <Field
                  key={field}
                  data-invalid={Boolean(profileFieldErrors[field]) || undefined}
                >
                  <FieldLabel htmlFor={id} className="text-label">
                    {label}
                  </FieldLabel>
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
                    <FieldError className="text-caption">
                      {profileFieldErrors[field]}
                    </FieldError>
                  )}
                </Field>
              ))}
            </div>

            {profileFormError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{profileFormError}</AlertDescription>
              </Alert>
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
