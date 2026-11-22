import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { Section } from "@astryxdesign/core/Section";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Token } from "@astryxdesign/core/Token";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Selector } from "@astryxdesign/core/Selector";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import {
  Activity,
  Ban,
  CreditCard,
  History,
  Plus,
  UserPlus } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { RowActions, useRowActionsWidth } from "../components/RowActions";
import {
  useMe,
  useWorkspace,
  useWorkspaceAICost,
  useWorkspaceUsage,
  useWorkspaceBillingProfile,
  useWorkspaceNotes,
  useWorkspaceAudit,
  useAddWorkspaceNote,
  useSetBillingProfileVerified,
  useUpdateWorkspaceBillingProfile,
  useCancelOrder,
  useMemberRequests,
  useCancelMemberRequest,
  } from "../api/hooks";
import { getApiErrorMessage, PlatformApiError } from "../api/client";
import { formatBytes } from "./Tenants";
import type { StaffRole } from "../components/types";
import {
  type Note,
  type AuditEvent,
  type Order,
  type BillingProfile,
  type MemberRequest,
  type WorkspaceUsage,
  type WorkspaceUsageRow,
  type WorkspaceUsageUnpaidOrder } from "../api/schemas";
import { formatMinorAmount } from "../money";
import { t } from "../i18n";

export function formatWorkspaceConflictError(err: unknown, fallback: string): string {
  if (err instanceof PlatformApiError) {
    const status = typeof err.details?.status === "string" ? err.details.status : null;
    const allowedList = Array.isArray(err.details?.allowed)
      ? err.details.allowed.filter((x): x is string => typeof x === "string")
      : null;
    if (
      err.code === "workspace_state_conflict" &&
      (status || (allowedList && allowedList.length > 0))
    ) {
      const parts: string[] = [err.message || fallback];
      if (status) parts.push(t("Current status: {0}", status));
      if (allowedList && allowedList.length > 0) {
        parts.push(t("Allowed: {0}", allowedList.join(", ")));
      }
      return parts.join(" — ");
    }
    return err.message || fallback;
  }
  return err instanceof Error ? err.message : fallback;
}


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

const AI_COST_WINDOWS = ["all", "7d", "30d", "90d"] as const;
type AICostWindow = (typeof AI_COST_WINDOWS)[number];

const AI_COST_WINDOW_LABELS: Record<AICostWindow, string> = {
  all: "All time",
  "7d": "Last 7 days",
  "30d": "Last 30 days",
  "90d": "Last 90 days",
};

const usdFormat = new Intl.NumberFormat(undefined, { style: "currency", currency: "USD" });

/** Provider-reported AI spend of a workspace over one window, with a period picker. */
function AICostSection({ workspaceId }: { workspaceId: string }) {
  const [period, setPeriod] = useState<AICostWindow>("all");
  const { data, isLoading, isError, error, refetch } = useWorkspaceAICost(workspaceId, period);

  return (
    <Section variant="transparent" padding={0} data-testid="ai-cost" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Heading level={2}>{t("AI spend")}</Heading>
        <Selector
          label={t("Period")}
          id="ai-cost-window"
          options={AI_COST_WINDOWS.map((value) => ({ value, label: t(AI_COST_WINDOW_LABELS[value]) }))}
          value={period}
          onChange={(val) => {
            if (AI_COST_WINDOWS.includes(val as AICostWindow)) setPeriod(val as AICostWindow);
          }}
        />
      </div>

      {isLoading && <LoadingState message={t("Loading AI spend...")} />}
      {isError && (
        <ErrorState
          title={t("Failed to load AI spend")}
          message={error instanceof Error ? error.message : t("Unable to load AI spend.")}
          onRetry={() => void refetch()}
        />
      )}
      {!isLoading && !isError && data && (
        <div className="space-y-1" data-testid="ai-cost-total">
          <Heading level={3}>
            {data.complete ? "" : "≥ "}
            {usdFormat.format(data.cost_usd)}
          </Heading>
          <Text>
            {data.since.slice(0, 10)} {t("to")} {data.until.slice(0, 10)} {t("(UTC)")}
          </Text>
          {!data.complete && <Text>{t("Some models have no rate, so this total is a floor.")}</Text>}
        </div>
      )}
    </Section>
  );
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
  onWriteOff }: MonthlyUsageSectionProps) {
  const actionsWidth = useRowActionsWidth(120);
  return (
    <Section variant="transparent" padding={0} data-testid="usage-month" className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <Heading level={2}>{t("Monthly Usage")}</Heading>
        {usageData && (
          <Text type="code" color="secondary">
            {usageData.period_start.slice(0, 10)} {t("to")} {usageData.period_end.slice(0, 10)} {t("(UTC)")}
          </Text>
        )}
      </div>

      {isLoading && <LoadingState message={t("Loading monthly usage...")} />}
      {isError && (
        <ErrorState
          title={t("Failed to load monthly usage")}
          message={error instanceof Error ? error.message : t("Unable to load monthly usage.")}
          onRetry={onRetry}
        />
      )}
      {!isLoading && !isError && usageData && (
        <div className="space-y-6">
          <div data-testid="usage-rows" className="space-y-2">
            <Heading level={3}>{t("Daily charges")}</Heading>
            {usageData.rows.length === 0 ? (
              <Text type="supporting" as="p">
                {t("No rated usage rows for this month.")}
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
                      header: t("Day"),
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (row) => (
                        <Text type="code" hasTabularNumbers>
                          {row.day}
                        </Text>
                      ) },
                    {
                      key: "meter",
                      header: t("Meter"),
                      width: proportional(1.2, { minWidth: 130 }),
                      renderCell: (row) => <Text type="code">{row.meter}</Text> },
                    {
                      key: "quantity",
                      header: t("Quantity"),
                      width: proportional(1, { minWidth: 100 }),
                      align: "end",
                      renderCell: (row) => (
                        <span
                          title={row.meter === "storage_bytes" ? t("{0} bytes", row.quantity.toLocaleString()) : undefined}
                        >
                          <Text type="body" hasTabularNumbers>
                            {formatUsageQuantity(row)}
                          </Text>
                        </span>
                      ) },
                    {
                      key: "unit_price",
                      header: t("Unit price"),
                      width: proportional(1, { minWidth: 120 }),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="supporting" hasTabularNumbers>
                          {row.unit_price.toLocaleString()} / {row.per.toLocaleString()}
                        </Text>
                      ) },
                    {
                      key: "discount_pct",
                      header: t("Discount"),
                      width: pixel(90),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="supporting" hasTabularNumbers>
                          {row.discount_pct > 0 ? `${row.discount_pct}%` : "-"}
                        </Text>
                      ) },
                    {
                      key: "net_amount",
                      header: t("Net amount"),
                      width: proportional(1, { minWidth: 110 }),
                      align: "end",
                      renderCell: (row) => (
                        <Text type="body" weight="medium" hasTabularNumbers>
                          {row.net_amount.toLocaleString()}
                        </Text>
                      ) },
                    {
                      key: "note",
                      header: t("Note"),
                      width: proportional(1.5, { minWidth: 120 }),
                      renderCell: (row) => <Text type="supporting">{row.note || "-"}</Text> },
                  ]}
                />
              </Card>
            )}
          </div>

          {usageData.missing_days.length > 0 && (
            <div data-testid="usage-missing-days" className="space-y-2">
              <Heading level={3}>{t("Missing days")}</Heading>
              <ul className="flex flex-wrap gap-2">
                {usageData.missing_days.map((day) => (
                  <li key={day} className="inline-flex items-center gap-1.5">
                    <Token label={day} />
                    <Badge variant="neutral" label={t("missing")} />
                  </li>
                ))}
              </ul>
            </div>
          )}

          <div data-testid="usage-unpaid-orders" className="space-y-2">
            <Heading level={3}>{t("Unpaid usage orders")}</Heading>
            {usageData.unpaid_orders.length === 0 ? (
              <Text type="supporting" as="p">
                {t("No unpaid usage orders.")}
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
                      header: t("Order"),
                      width: proportional(1.5, { minWidth: 140 }),
                      renderCell: (order) => <Text type="code">{order.id}</Text> },
                    {
                      key: "period_start",
                      header: t("Period"),
                      width: proportional(1.8, { minWidth: 170 }),
                      renderCell: (order) => (
                        <Text type="code" color="secondary" hasTabularNumbers>
                          {order.period_start && order.period_end
                            ? `${order.period_start.slice(0, 10)} – ${order.period_end.slice(0, 10)}`
                            : "-"}
                        </Text>
                      ) },
                    {
                      key: "created_at",
                      header: t("Created"),
                      width: proportional(1.5, { minWidth: 150 }),
                      renderCell: (order) => (
                        <Text type="supporting" hasTabularNumbers>
                          {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                        </Text>
                      ) },
                    {
                      key: "amount",
                      header: t("Amount"),
                      width: proportional(1, { minWidth: 100 }),
                      align: "end",
                      renderCell: (order) => (
                        <Text type="body" weight="medium" hasTabularNumbers>
                          {order.amount.toLocaleString()}
                        </Text>
                      ) },
                    {
                      key: "vat_amount",
                      header: t("VAT"),
                      width: proportional(1, { minWidth: 90 }),
                      align: "end",
                      renderCell: (order) => (
                        <Text type="supporting" hasTabularNumbers>
                          {order.vat_amount.toLocaleString()}
                        </Text>
                      ) },
                    {
                      key: "status",
                      header: t("Status"),
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (order) => <StatusBadge status={order.status} /> },
                    ...(canWriteOff
                      ? [
                          {
                            key: "actions",
                            header: t("Action"),
                            width: pixel(actionsWidth),
                            align: "end" as const,
                            renderCell: (order: WorkspaceUsageUnpaidOrder) => (
                              <RowActions
                                label={t("unpaid order {0}", order.id)}
                                actions={[
                                  { label: t("Write off"), destructive: true, onSelect: () => onWriteOff(order) },
                                ]}
                              />
                            ) },
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
  invoice_email: "Invoice email is required." };

const BILLING_PROFILE_INVALID_MESSAGES: Record<BillingProfileField, string> = {
  account_kind: "Select a valid account kind.",
  legal_name: "Legal name is invalid or exceeds 255 characters.",
  tax_code: "Enter a valid 10-digit (or 10-3) tax code.",
  address: "Address is invalid or exceeds 255 characters.",
  invoice_email: "Enter a valid invoice email address." };

function isBillingProfileField(value: string): value is BillingProfileField {
  return (BILLING_PROFILE_FIELDS as readonly string[]).includes(value);
}


const ACCOUNT_KIND_ITEMS = [
  { value: "individual", label: "Individual" },
  { value: "business", label: "Business" },
];

export function WorkspaceDetail({ workspaceId, onBack, backLabel = t("Back to tenants"), userRole }: WorkspaceDetailProps) {
  const { data: me } = useMe();
  const {
    data: workspace,
    isLoading: workspaceLoading,
    isError: workspaceError,
    error: workspaceErrorObj,
    refetch: refetchWorkspace } = useWorkspace(workspaceId);

  const {
    data: usageData,
    isLoading: usageLoading,
    isError: usageError,
    error: usageErrorObj,
    refetch: refetchUsage } = useWorkspaceUsage(workspaceId);

  const {
    data: billingProfile } = useWorkspaceBillingProfile(workspaceId);

  const {
    data: notesData,
    isLoading: notesLoading } = useWorkspaceNotes(workspaceId);

  const {
    data: auditData,
    isLoading: auditLoading } = useWorkspaceAudit(workspaceId);

  const addNote = useAddWorkspaceNote();
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



  const [noteBody, setNoteBody] = useState("");
  const [noteError, setNoteError] = useState<string | null>(null);

  // Usage debt write-off dialog state
  const [writeOffOrder, setWriteOffOrder] = useState<WorkspaceUsageUnpaidOrder | null>(null);
  const [writeOffError, setWriteOffError] = useState<string | null>(null);

  const effectiveRole = userRole ?? me?.role;
  const canEditSubscription = effectiveRole === "billing" || effectiveRole === "system_admin";
  const canWriteOff = me === null ? false : (me?.role ?? userRole) === "system_admin";
  const canManageMembers = effectiveRole === "system_admin" || effectiveRole === "admin";

  const {
    data: memberRequests = [],
    isLoading: memberRequestsLoading,
    isError: memberRequestsError,
    error: memberRequestsErrorObj,
    refetch: refetchMemberRequests } = useMemberRequests(workspaceId, canManageMembers);

  const cancelMemberRequest = useCancelMemberRequest();

  const [cancelingRequestId, setCancelingRequestId] = useState<string | null>(null);
  const [memberRequestError, setMemberRequestError] = useState<string | null>(null);

  if (workspaceLoading) {
    return <LoadingState message={t("Loading workspace details...")} />;
  }

  if (workspaceError || !workspace) {
    return (
      <ErrorState
        title={t("Failed to load workspace")}
        message={
          workspaceErrorObj instanceof Error
            ? workspaceErrorObj.message
            : t("The workspace could not be found.")
        }
        onRetry={refetchWorkspace}
      />
    );
  }

  const handleAddNote = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!noteBody.trim() || addNote.isPending) return;
    setNoteError(null);
    try {
      await addNote.mutateAsync({
        workspaceId,
        body: noteBody.trim() });
      setNoteBody("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("Failed to add note");
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
        reason });
      setWriteOffOrder(null);
    } catch (err: unknown) {
      setWriteOffError(err instanceof Error ? err.message : t("Failed to write off order"));
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
        invoice_email: profileInvoiceEmailInput.trim() });
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
              nextFieldErrors[f] = t(messages[f]);
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
        err instanceof Error ? err.message : t("Failed to update billing profile"),
      );
    }
  };

  const handleToggleProfileVerified = async () => {
    if (setBillingProfileVerified.isPending) return;
    setProfileError(null);
    try {
      const res = await setBillingProfileVerified.mutateAsync({
        workspaceId,
        verified: !isProfileVerified });
      if (res) {
        setProfileOverride({
          workspaceId,
          profile: {
            legal_name: profileLegalName,
            tax_code: profileTaxCode,
            address: profileAddress,
            invoice_email: profileInvoiceEmail,
            ...res } });
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
              ? t("Missing billing profile fields: {0}", fields.join(", "))
              : err.message });
      } else {
        setProfileError({
          workspaceId,
          message:
            err instanceof Error
              ? err.message
              : t("Failed to update billing profile verification") });
      }
    }
  };


  const handleCancelMemberRequest = async (requestId: string) => {
    setCancelingRequestId(requestId);
    setMemberRequestError(null);
    try {
      await cancelMemberRequest.mutateAsync({ workspaceId, requestId });
    } catch (err: unknown) {
      setMemberRequestError(getApiErrorMessage(err, t("Failed to cancel member request")));
    } finally {
      setCancelingRequestId(null);
    }
  };

  const notes: Note[] = Array.isArray(notesData)
    ? notesData
    : (notesData as unknown as { notes?: Note[] })?.notes || [];
  const auditEvents: AuditEvent[] = Array.isArray(auditData)
    ? auditData
    : (auditData as unknown as { events?: AuditEvent[] })?.events || [];

  const currentStatus = workspace.state?.status || workspace.status;

  const seatsPurchased =
    workspace.seats?.purchased ?? workspace.subscription?.seats ?? 0;
  const seatsUsed =
    workspace.seats?.used ??
    workspace.usage_current?.members ??
    workspace.member_count;
  const seatsHeld = workspace.seats?.held ?? 0;

  const statusHistoryRows = workspace.status_history
    ? workspace.status_history.map((entry, idx) => ({
        key: entry.id || `${entry.created_at}-${idx}`,
        status: entry.status as string | null,
        statusLabel: "",
        reason: entry.reason || "-",
        actor: entry.actor || "-",
        created_at: entry.created_at }))
    : auditEvents
        .filter(
          (ev) =>
            ev.action === "workspace.status_changed" ||
            ev.action === "workspace.deletion_scheduled" ||
            ev.action === "workspace.deletion_cancelled" ||
            ev.action === "workspace.immediate_deletion_requested",
        )
        .map((ev) => ({
          key: ev.id,
          status: null as string | null,
          statusLabel:
            ev.action === "workspace.deletion_scheduled"
              ? t("Deletion scheduled")
              : ev.action === "workspace.deletion_cancelled"
                ? t("Deletion cancelled")
                : ev.action === "workspace.immediate_deletion_requested"
                  ? t("Immediate deletion requested")
                  : t("Status changed"),
          reason: "-",
          actor: ev.actor_label || ev.actor_id || "-",
          created_at: ev.created_at }));

  return (
    <div className="space-y-8">
      <PageHeader
        title={workspace.name}
        details={
          <>
            <StatusBadge status={currentStatus} />
            <span className="font-mono text-caption text-muted-foreground">
              {workspace.slug} {t("• ID:")} {workspace.id}
            </span>
          </>
        }
        backAction={{ label: backLabel, onClick: onBack }}
      />



      {(workspace.state?.reason || workspace.state?.deletion_scheduled_at) && (
        <div className="space-y-3">
          {workspace.state?.reason && (
            <Banner
              status="info"
              role="status"
              title={t("State Reason")}
              description={workspace.state.reason}
            />
          )}
          {workspace.state?.deletion_scheduled_at && (
            <Banner
              status="error"
              role="alert"
              title={t("Deletion Scheduled")}
              description={new Date(workspace.state.deletion_scheduled_at).toLocaleString()}
            />
          )}
        </div>
      )}

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
        <Section variant="transparent" padding={0} className="space-y-4 lg:col-span-2">
          <Heading level={2}>{t("Workspace State")}</Heading>
          <MetadataList columns={3}>
            <MetadataListItem label={t("Status")}>
              <StatusBadge status={currentStatus} />
            </MetadataListItem>
            {workspace.tenant && (
              <MetadataListItem label={t("Tenant")}>
                {workspace.tenant.name || workspace.tenant.id}
              </MetadataListItem>
            )}
            <MetadataListItem label={t("Members")} tabular>
              {workspace.member_count}
            </MetadataListItem>
            <MetadataListItem label={t("Account groups")} tabular>
              {workspace.group_count}
            </MetadataListItem>
            <MetadataListItem label={t("Owner")}>
              {workspace.owner?.email || t("Unknown")}
            </MetadataListItem>
            <MetadataListItem label={t("Created")} tabular>
              {workspace.created_at ? new Date(workspace.created_at).toLocaleString() : "-"}
            </MetadataListItem>
            <MetadataListItem label={t("Last Activity")} tabular>
              {workspace.last_activity_at
                ? new Date(workspace.last_activity_at).toLocaleString()
                : "-"}
            </MetadataListItem>
            {workspace.state?.changed_at && (
              <MetadataListItem label={t("Status Changed At")} tabular>
                {new Date(workspace.state.changed_at).toLocaleString()}
              </MetadataListItem>
            )}
          </MetadataList>
        </Section>

        <Section variant="transparent" padding={0} className="space-y-4">
          <Heading level={2}>{t("Internal Notes")}</Heading>
          <form onSubmit={handleAddNote} className="space-y-2">
            <TextArea
              label={t("Internal Notes")}
              isLabelHidden
              width="100%"
              value={noteBody}
              onChange={(value) => setNoteBody(value)}
              placeholder={t("Write an internal note...")}
              rows={3}
              isDisabled={addNote.isPending}
            />
            {noteError && (
              <Banner
                status="error"
                role="alert"
                title={noteError}
              />
            )}
            <Button
              type="submit"
              label={t("Add note")}
              variant="primary"
              size="sm"
              icon={<Plus className="size-3.5" aria-hidden="true" />}
              isLoading={addNote.isPending}
              className="w-full gap-1"
              aria-busy={addNote.isPending}
              isDisabled={!noteBody.trim() || addNote.isPending}
            />
          </form>

          <div className="space-y-2">
            {notesLoading && <LoadingState message={t("Loading notes...")} />}
            {!notesLoading && notes.length === 0 && (
              <Text type="supporting" as="p" justify="center" className="py-4">
                {t("No internal notes yet.")}
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
                      <Text type="supporting">{note.author_email || t("Staff")}</Text>
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
          <Heading level={2}>{t("Subscription")}</Heading>
          {workspace.subscription?.status && (
            <StatusBadge status={workspace.subscription.status} />
          )}
        </div>
        <MetadataList columns={3}>
          <MetadataListItem label={t("Plan")} className="capitalize">
            {workspace.subscription?.plan_key || "free"}
          </MetadataListItem>
          <MetadataListItem label={t("Seats")} tabular>
            {seatsPurchased}
          </MetadataListItem>
          <MetadataListItem label={t("Seats used")} tabular>
            {seatsUsed}
          </MetadataListItem>
          <MetadataListItem label={t("Seats held")} tabular>
            {seatsHeld}
          </MetadataListItem>
          <MetadataListItem label={t("Interval")} className="capitalize">
            {workspace.subscription?.interval || "month"}
          </MetadataListItem>
          <MetadataListItem label={t("Trial End")} tabular>
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
          <Heading level={2}>{t("Billing profile")}</Heading>
          {canEditSubscription && (
            <div className="flex items-center gap-2">
              <Button
                label={t("Edit")}
                variant="secondary"
                size="sm"
                onClick={handleOpenProfileDialog}
              />
              <Button
                label={isProfileVerified ? t("Unverify") : t("Verify")}
                variant="secondary"
                size="sm"
                isLoading={setBillingProfileVerified.isPending}
                onClick={handleToggleProfileVerified}
                aria-busy={setBillingProfileVerified.isPending}
                isDisabled={setBillingProfileVerified.isPending}
              />
            </div>
          )}
        </div>

        <MetadataList columns={3}>
          <MetadataListItem label={t("Account kind")} className="capitalize">
            {accountKind}
          </MetadataListItem>
          <MetadataListItem label={t("Status")}>
            <StatusBadge status={isProfileVerified ? "verified" : "unverified"} />
          </MetadataListItem>
          <MetadataListItem label={t("Verified at")} tabular>
            {formatTimestamp(profileVerifiedAt)}
          </MetadataListItem>
          <MetadataListItem label={t("Legal name")}>
            {profileLegalName || "-"}
          </MetadataListItem>
          <MetadataListItem label={t("Tax code")} mono>
            {profileTaxCode || "-"}
          </MetadataListItem>
          <MetadataListItem label={t("Address")}>
            {profileAddress || "-"}
          </MetadataListItem>
          <MetadataListItem label={t("Invoice email")} className="sm:col-span-2">
            {profileInvoiceEmail || "-"}
          </MetadataListItem>
        </MetadataList>

        {activeProfileError && (
          <Banner
            status="error"
            role="alert"
            title={activeProfileError}
          />
        )}
      </Section>

      <Divider />

      {/* Usage Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>{t("Usage")}</Heading>
        <MetadataList data-testid="usage-current" columns={4}>
          <MetadataListItem label={t("Members now")} tabular>
            {workspace.usage_current?.members ?? 0}
          </MetadataListItem>
          <MetadataListItem label={t("Agents now")} tabular>
            {workspace.usage_current?.agents ?? 0}
          </MetadataListItem>
          <MetadataListItem label={t("Runtimes now")} tabular>
            {workspace.usage_current?.runtimes ?? 0}
          </MetadataListItem>
          <MetadataListItem label={t("Issues now")} tabular>
            {workspace.usage_current?.issues_total ?? 0}
          </MetadataListItem>
          <MetadataListItem label={t("Storage now")} tabular>
            {formatBytes(workspace.usage_current?.storage_bytes ?? 0)}
          </MetadataListItem>
        </MetadataList>

        {workspace.usage_day ? (
          <div data-testid="usage-daily" className="space-y-3 pt-2">
            <Text type="code" color="secondary" as="div">
              {t("Daily snapshot for")} {workspace.usage_day} {t("(UTC)")}
            </Text>
            <MetadataList columns={4}>
              <MetadataListItem label={t("Members")} tabular>
                {workspace.usage_members ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Agents")} tabular>
                {workspace.usage_agents ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Runtimes")} tabular>
                {workspace.usage_runtimes ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Issues")} tabular>
                {workspace.usage_issues_total ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Issues created")} tabular>
                {workspace.usage_issues_created ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Tasks run")} tabular>
                {workspace.usage_tasks_run ?? 0}
              </MetadataListItem>
              <MetadataListItem label={t("Storage")} tabular>
                {formatBytes(workspace.usage_storage_bytes ?? 0)}
              </MetadataListItem>
            </MetadataList>
          </div>
        ) : (
          <Text type="supporting" as="p" data-testid="usage-daily">
            {t("No daily snapshot yet.")}
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

      <AICostSection workspaceId={workspaceId} />

      {canManageMembers && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={2}>{t("Member Requests")}</Heading>
            {memberRequestError && (
              <Banner
                status="error"
                role="alert"
                title={memberRequestError}
              />
            )}
            {memberRequestsLoading && <LoadingState message={t("Loading member requests...")} />}
            {memberRequestsError && (
              <ErrorState
                title={t("Failed to load member requests")}
                message={getApiErrorMessage(memberRequestsErrorObj, t("Could not load member requests."))}
                onRetry={refetchMemberRequests}
              />
            )}
            {!memberRequestsLoading && !memberRequestsError && memberRequests.length === 0 && (
              <EmptyState
                icon={<UserPlus className="size-5" aria-hidden="true" />}
                title={t("No member requests")}
                isCompact
              />
            )}
            {!memberRequestsLoading && !memberRequestsError && memberRequests.length > 0 && (
              <Card padding={0} className="overflow-x-auto">
                <Table
                  data={memberRequests}
                  idKey="id"
                  density="compact"
                  dividers="rows"
                  hasHover
                  columns={[
                    {
                      key: "email",
                      header: t("Email"),
                      width: proportional(1.6, { minWidth: 180 }),
                      renderCell: (req: MemberRequest) => (
                        <Text type="body" weight="medium">
                          {req.email}
                        </Text>
                      ) },
                    {
                      key: "role",
                      header: t("Role"),
                      width: pixel(110),
                      renderCell: (req: MemberRequest) => <StatusBadge status={req.role} /> },
                    {
                      key: "state",
                      header: t("State"),
                      width: pixel(120),
                      renderCell: (req: MemberRequest) => <StatusBadge status={req.state} /> },
                    {
                      key: "error_code",
                      header: t("Error"),
                      width: proportional(1.2, { minWidth: 140 }),
                      renderCell: (req: MemberRequest) =>
                        req.error_code ? (
                          <span className="font-mono text-caption text-destructive">
                            {req.error_code}
                          </span>
                        ) : (
                          <Text type="supporting">—</Text>
                        ) },
                    {
                      key: "attempts",
                      header: t("Attempts"),
                      width: pixel(90),
                      renderCell: (req: MemberRequest) => (
                        <Text type="body" hasTabularNumbers>
                          {req.attempts}
                        </Text>
                      ) },
                    {
                      key: "created_at",
                      header: t("Requested"),
                      width: proportional(1.2, { minWidth: 160 }),
                      renderCell: (req: MemberRequest) => (
                        <Text type="supporting" hasTabularNumbers>
                          {req.created_at ? new Date(req.created_at).toLocaleString() : "—"}
                        </Text>
                      ) },
                    {
                      key: "action",
                      header: "",
                      width: pixel(100),
                      renderCell: (req: MemberRequest) => {
                        if (req.state !== "pending") return null;
                        const rowBusy = cancelingRequestId === req.id;
                        return (
                          <Button
                            label={t("Cancel")}
                            variant="destructive"
                            size="sm"
                            icon={<Ban className="size-3" aria-hidden="true" />}
                            isLoading={rowBusy}
                            aria-busy={rowBusy || undefined}
                            onClick={() => handleCancelMemberRequest(req.id)}
                            isDisabled={rowBusy}
                          />
                        );
                      } },
                  ]}
                />
              </Card>
            )}
          </Section>
        </>
      )}

      <Divider />

      {/* Recent Orders Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>{t("Recent Orders")}</Heading>
        {!workspace.recent_orders || workspace.recent_orders.length === 0 ? (
          <EmptyState
            icon={<CreditCard className="size-5" aria-hidden="true" />}
            title={t("No recent orders")}
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
                  header: t("Time"),
                  width: proportional(1.4, { minWidth: 160 }),
                  renderCell: (order: Order) => (
                    <Text type="supporting" hasTabularNumbers>
                      {order.created_at ? new Date(order.created_at).toLocaleString() : "-"}
                    </Text>
                  ) },
                {
                  key: "transfer_code",
                  header: t("Transfer Code"),
                  width: proportional(1.4, { minWidth: 150 }),
                  renderCell: (order: Order) => (
                    <Text type="code" weight="semibold">
                      {order.transfer_code}
                    </Text>
                  ) },
                {
                  key: "plan_key",
                  header: t("Plan & Seats"),
                  width: proportional(2, { minWidth: 180 }),
                  renderCell: (order: Order) => (
                    <Text type="body" className="capitalize">
                      {order.plan_key} &bull; {order.seats} {t("seats (")}{order.interval})
                    </Text>
                  ) },
                {
                  key: "amount",
                  header: t("Total (incl. VAT)"),
                  width: proportional(1.2, { minWidth: 150 }),
                  renderCell: (order: Order) => (
                    <Text type="body" weight="medium" hasTabularNumbers>
                      {formatMinorAmount(order.amount + order.vat_amount, order.currency)}
                    </Text>
                  ) },
                {
                  key: "status",
                  header: t("Status"),
                  width: proportional(1, { minWidth: 110 }),
                  renderCell: (order: Order) => <StatusBadge status={order.status} /> },
              ]}
            />
          </Card>
        )}
      </Section>

      <Divider />

      {/* Status History Section */}
      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>{t("Status History")}</Heading>
        {statusHistoryRows.length === 0 ? (
          <EmptyState
            icon={<History className="size-5" aria-hidden="true" />}
            title={t("No status transitions recorded")}
            isCompact
          />
        ) : (
          <div className="overflow-x-auto">
            <Table
              data={statusHistoryRows}
              idKey="key"
              density="compact"
              dividers="rows"
              hasHover
              columns={[
                {
                  key: "created_at",
                  header: t("When"),
                  width: proportional(1.4, { minWidth: 160 }),
                  renderCell: (row) => (
                    <Text type="supporting" hasTabularNumbers>
                      {row.created_at
                        ? new Date(row.created_at).toLocaleString()
                        : "-"}
                    </Text>
                  ) },
                {
                  key: "status",
                  header: t("Status"),
                  width: proportional(1.2, { minWidth: 130 }),
                  renderCell: (row) =>
                    row.status ? (
                      <StatusBadge status={row.status} />
                    ) : (
                      <Text type="body">{row.statusLabel}</Text>
                    ) },
                {
                  key: "reason",
                  header: t("Reason"),
                  width: proportional(1.6, { minWidth: 160 }),
                  renderCell: (row) => (
                    <Text type="supporting">{row.reason}</Text>
                  ) },
                {
                  key: "actor",
                  header: t("Actor"),
                  width: proportional(1.4, { minWidth: 150 }),
                  renderCell: (row) => <Text type="body">{row.actor}</Text> },
              ]}
            />
          </div>
        )}
      </Section>

      <Divider />

      {/* Workspace Audit Log Section */}
      <Section
        id="workspace-audit-log"
        variant="transparent"
        padding={0}
        className="space-y-4"
      >
        <Heading level={2}>{t("Workspace Audit Log")}</Heading>
        {auditLoading && <LoadingState message={t("Loading audit events...")} />}
        {!auditLoading && auditEvents.length === 0 && (
          <EmptyState
            icon={<Activity className="size-5" aria-hidden="true" />}
            title={t("No audit events")}
            description={t("No events found for this workspace.")}
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
                  header: t("Time"),
                  width: proportional(1.4, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => (
                    <Text type="supporting" hasTabularNumbers>
                      {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                    </Text>
                  ) },
                {
                  key: "action",
                  header: t("Action"),
                  width: proportional(1.4, { minWidth: 150 }),
                  renderCell: (event: AuditEvent) => <Text type="code">{event.action}</Text> },
                {
                  key: "actor_label",
                  header: t("Actor"),
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => <Text type="body">{event.actor_label}</Text> },
                {
                  key: "target_label",
                  header: t("Target"),
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event: AuditEvent) => (
                    <Text type="body">{event.target_label || event.target_id}</Text>
                  ) },
              ]}
            />
          </Card>
        )}
      </Section>



      {/* Edit Billing Profile Dialog */}
      <Dialog
        isOpen={profileDialogOpen}
        onOpenChange={setProfileDialogOpen}
        purpose="form"
      >

          <form noValidate onSubmit={handleSaveBillingProfile} className="flex flex-col gap-4">
            <DialogHeader title={t("Edit Billing Profile")} subtitle={t("Update workspace account kind and billing contact details.")} />

            <div className="flex flex-col gap-3">
              <Selector
                label={t("Account kind")}
                id="bp-account-kind"
                width="100%"
                options={ACCOUNT_KIND_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={profileKind}
                onChange={(val) => {
                    if (val === "individual" || val === "business") {
                      setProfileKind(val);
                      clearProfileFieldError("account_kind");
                    }
                  }}
                status={profileFieldErrors.account_kind ? { type: "error", message: profileFieldErrors.account_kind } : undefined}
              />

              {[
                {
                  field: "legal_name" as const,
                  id: "bp-legal-name",
                  label: t("Legal name"),
                  type: "text",
                  value: profileLegalNameInput,
                  setValue: setProfileLegalNameInput },
                {
                  field: "tax_code" as const,
                  id: "bp-tax-code",
                  label: t("Tax code"),
                  type: "text",
                  value: profileTaxCodeInput,
                  setValue: setProfileTaxCodeInput },
                {
                  field: "address" as const,
                  id: "bp-address",
                  label: t("Address"),
                  type: "text",
                  value: profileAddressInput,
                  setValue: setProfileAddressInput },
                {
                  field: "invoice_email" as const,
                  id: "bp-invoice-email",
                  label: t("Invoice email"),
                  type: "email",
                  value: profileInvoiceEmailInput,
                  setValue: setProfileInvoiceEmailInput },
              ].map(({ field, id, label, type, value, setValue }) => (
                <TextInput
                  label={label}
                  id={id}
                  type={type as "text" | "email"}
                  value={value}
                  onChange={(value) => {
                      setValue(value);
                      clearProfileFieldError(field);
                    }}
                  isDisabled={updateBillingProfile.isPending}
                  width="100%"
                  status={profileFieldErrors[field] ? { type: "error", message: profileFieldErrors[field] } : undefined}
                />
              ))}
            </div>

            {profileFormError && (
              <Banner
                status="error"
                role="alert"
                title={profileFormError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
                variant="secondary"
                type="button"
                isDisabled={updateBillingProfile.isPending}
                label={t("Cancel")}
                onClick={() => setProfileDialogOpen(false)}
              />
              <Button
                type="submit"
                label={t("Save")}
                variant="primary"
                isLoading={updateBillingProfile.isPending}
                aria-busy={updateBillingProfile.isPending}
                isDisabled={updateBillingProfile.isPending}
              />
            </div>
          </form>

      </Dialog>

      <ConfirmReasonDialog
        open={writeOffOrder !== null}
        onOpenChange={(open) => {
          if (!open) {
            setWriteOffOrder(null);
            setWriteOffError(null);
          }
        }}
        title={t("Write off usage order")}
        description={
          writeOffOrder
            ? t("Write off pending usage order {0} ({1} total)? Its lines will stay excluded from future invoices.", writeOffOrder.id, (writeOffOrder.amount + writeOffOrder.vat_amount).toLocaleString())
            : ""
        }
        confirmLabel={t("Write off order")}
        confirmVariant="destructive"
        isPending={cancelOrder.isPending}
        error={writeOffError}
        onConfirm={handleConfirmWriteOff}
      />



    </div>
  );
}
