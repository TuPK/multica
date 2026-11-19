"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  AlertCircle,
  ArrowUpCircle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CreditCard,
  ExternalLink,
  Info,
  Loader2,
  Plus,
  RefreshCw,
} from "@multica/ui/icons";
import {
  api,
  ApiError,
  clientErrorMessage,
  errorCode,
  getPlanLimitReached,
  planLimitReached,
  subscribePlanLimitReached,
} from "@multica/core/api";
import { autopilotQuotaUsageOptions } from "@multica/core/autopilots";
import {
  type WorkspaceOrder,
  useCancelWorkspaceScheduledPlanChange,
  useCancelWorkspaceSubscription,
  useCreateWorkspacePlanChange,
  useCreateWorkspaceSubscriptionCheckout,
  useCreateWorkspaceSubscriptionPortal,
  usePreviewWorkspacePlanChange,
  usePreviewWorkspaceSeatPurchase,
  usePurchaseWorkspaceSeats,
  useResumeWorkspaceSubscription,
  useUpdateWorkspaceBillingProfile,
  useUpdateWorkspaceOrderInvoiceBuyer,
  issueLimitUsageOptions,
  workspaceBillingProfileOptions,
  workspaceOrderOptions,
  workspaceOrdersOptions,
  workspacePlansOptions,
  workspaceSubscriptionPricesOptions,
  workspaceSubscriptionSummaryOptions,
  workspaceSubscriptionUsageOptions,
} from "@multica/core/billing";
import { useFeatureEnabled } from "@multica/core/config";
import { BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG } from "@multica/core/feature-flags";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useCan } from "@multica/core/permissions";
import type {
  PurchaseWorkspaceSeatsRequest,
  QuotaRequest,
  UpdateWorkspaceBillingProfileRequest,
  WorkspaceBillingAccountKind,
  WorkspaceBillingProfile,
  WorkspacePlanChangePreview,
  WorkspaceSeatPurchasePreview,
  WorkspaceSubscriptionInterval,
} from "@multica/core/types";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { IconButton } from "@multica/ui/components/common/icon-button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@multica/ui/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useLocale, useT } from "../../i18n";
import { useNavigation } from "../../navigation";
import { openExternal } from "../../platform";
import {
  BillingProfileForm,
} from "../../billing/billing-profile-form";
import {
  PendingChangeBanner,
} from "../../billing/pending-change-banner";
import {
  ScheduledPriceChangeBanner,
} from "../../billing/scheduled-price-change-banner";
import { plansForAccountKind } from "../../billing/plan-audience";
import { PlanChangeDialog } from "../../billing/plan-change-dialog";
import {
  SubscriptionLifecycleSection,
} from "../../billing/subscription-lifecycle-section";
import { UnpaidUsageBanner } from "../../billing/unpaid-usage-banner";
import {
  UsageCard,
  formatUsageMoney,
  formatUsageUnitPer,
  formatUsageUtcDate,
  resolveMeterLabel,
} from "../../billing/usage-card";
import {
  EinvoiceStatusSummary,
  InlineInvoiceBuyerForm,
  canEditEinvoiceBuyer,
} from "../../billing/einvoice-section";
import { UsageInvoiceLines } from "../../billing/usage-invoice-lines";
import {
  SettingsCard,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";
import {
  hasActiveWorkspaceSeatCapacity,
  resolveAutopilotUsage,
} from "./billing-state";
import { formatStripeMinorAmount } from "./billing-format";

export { formatStripeMinorAmount } from "./billing-format";

const CHECKOUT_SYNC_TIMEOUT_MS = 30_000;
const SEAT_PURCHASE_POLL_TIMEOUT_MS = 2 * 60_000;
const SEAT_PURCHASE_PREVIEW_DEBOUNCE_MS = 800;
const QUOTA_REQUEST_MAX_REASON_LENGTH = 500;
const QUOTA_REQUEST_GATES = [
  "agents",
  "runtimes",
  "storage_bytes",
  "issue_count",
  "autopilot_runs",
  "member_groups",
  "api_rate_per_minute",
  "audit_retention_days",
  "max_workspaces",
] as const;

type SettingsT = ReturnType<typeof useT<"settings">>["t"];

function quotaGateLabel(gate: string, tSettings: SettingsT): string {
  switch (gate) {
    case "agents":
      return tSettings(($) => $.quota_requests.gates.agents);
    case "runtimes":
      return tSettings(($) => $.quota_requests.gates.runtimes);
    case "storage_bytes":
      return tSettings(($) => $.quota_requests.gates.storage_bytes);
    case "issue_count":
      return tSettings(($) => $.quota_requests.gates.issue_count);
    case "autopilot_runs":
      return tSettings(($) => $.quota_requests.gates.autopilot_runs);
    case "member_groups":
      return tSettings(($) => $.quota_requests.gates.member_groups);
    case "api_rate_per_minute":
      return tSettings(($) => $.quota_requests.gates.api_rate_per_minute);
    case "audit_retention_days":
      return tSettings(($) => $.quota_requests.gates.audit_retention_days);
    case "max_workspaces":
      return tSettings(($) => $.quota_requests.gates.max_workspaces);
    default:
      return gate || tSettings(($) => $.quota_requests.gates.unknown);
  }
}

function quotaStateLabel(state: string, tSettings: SettingsT): string {
  switch (state) {
    case "pending":
      return tSettings(($) => $.quota_requests.states.pending);
    case "approved":
      return tSettings(($) => $.quota_requests.states.approved);
    case "denied":
      return tSettings(($) => $.quota_requests.states.denied);
    case "canceled":
      return tSettings(($) => $.quota_requests.states.canceled);
    default:
      return tSettings(($) => $.quota_requests.states.unknown);
  }
}

function quotaStateVariant(
  state: string,
): "default" | "secondary" | "destructive" | "outline" {
  switch (state) {
    case "approved":
      return "default";
    case "pending":
      return "outline";
    case "denied":
      return "destructive";
    case "canceled":
    default:
      return "secondary";
  }
}

function mapQuotaRequestError(error: unknown, tSettings: SettingsT): string {
  switch (errorCode(error)) {
    case "quota_request_pending":
      return tSettings(($) => $.quota_requests.errors.pending_exists);
    case "quota_request_invalid":
      return (
        clientErrorMessage(error) ??
        tSettings(($) => $.quota_requests.errors.invalid)
      );
    default:
      if (error instanceof ApiError && error.status === 403) {
        return tSettings(($) => $.quota_requests.errors.forbidden);
      }
      return tSettings(($) => $.quota_requests.errors.submit_failed);
  }
}

type WorkspaceBillingReturnResult = "success" | "cancel" | "portal";

function parseReturnResult(
  value: string | null,
): WorkspaceBillingReturnResult | null {
  switch (value) {
    case "success":
    case "cancel":
    case "portal":
      return value;
    default:
      return null;
  }
}

function createIdempotencyKey(prefix: string, wsId: string): string {
  const suffix =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `${prefix}-${wsId}-${suffix}`.slice(0, 255);
}

function formatDate(value: string | null, locale: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

function formatDateTime(value: string | null, locale: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function planBadgeVariant(plan: string): "default" | "secondary" | "outline" {
  if (plan === "pro") return "default";
  if (plan === "free") return "secondary";
  return "outline";
}

function statusBadgeVariant(
  status: string,
): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "active":
    case "trialing":
      return "default";
    case "past_due":
    case "incomplete":
    case "unpaid":
      return "destructive";
    case "inactive":
    case "canceled":
    case "incomplete_expired":
    case "paused":
      return "secondary";
    default:
      return "outline";
  }
}

function PaidOrderEinvoicePanel({
  wsId,
  order,
}: {
  wsId: string;
  order: WorkspaceOrder;
}) {
  const detailQuery = useQuery({
    ...workspaceOrderOptions(wsId, order.id),
    enabled: wsId.length > 0 && order.id.length > 0 && order.status === "paid",
  });
  const updateInvoiceBuyer = useUpdateWorkspaceOrderInvoiceBuyer(wsId);
  const [updatedOrder, setUpdatedOrder] = useState<WorkspaceOrder | null>(null);

  const effectiveOrder =
    updatedOrder && updatedOrder.id === order.id
      ? updatedOrder
      : (detailQuery.data ?? order);
  const einvoice = effectiveOrder.einvoice ?? null;

  if (detailQuery.isPending && !einvoice) {
    return (
      <div className="px-4 py-3">
        <Skeleton className="h-5 w-32" />
      </div>
    );
  }

  if (!einvoice) return null;

  return (
    <div className="space-y-4 px-4 py-3">
      <EinvoiceStatusSummary einvoice={einvoice} />
      {canEditEinvoiceBuyer(einvoice) ? (
        <InlineInvoiceBuyerForm
          idPrefix={`invoice-buyer-${order.id}`}
          initialBuyer={effectiveOrder.buyer}
          saving={updateInvoiceBuyer.isPending}
          onSave={async (buyer) => {
            const saved = await updateInvoiceBuyer.mutateAsync({
              orderId: order.id,
              buyer,
            });
            if (saved) setUpdatedOrder(saved);
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * A second gate inside the tab keeps direct/test mounts fail-closed. The
 * Settings shell also omits this component and its navigation entry while the
 * flag is absent, so no subscription request is issued in either path.
 */
export function BillingTab() {
  const enabled = useFeatureEnabled(
    BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG,
    false,
  );
  return enabled ? <BillingTabContent /> : null;
}

function BillingTabContent() {
  const { t } = useT("billing");
  const { t: tSettings } = useT("settings");
  const locale = useLocale();
  const navigation = useNavigation();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";
  const workspaceSlug = workspace?.slug ?? null;
  const canManageBilling = useCan(wsId, "billing.manage", { write: true }).allowed;
  const recordedPlanLimit = useSyncExternalStore(
    subscribePlanLimitReached,
    () => getPlanLimitReached(wsId, workspaceSlug),
    () => null,
  );
  const returnResultParam = parseReturnResult(
    navigation.searchParams.get("result"),
  );
  const returnSessionId = navigation.searchParams.get("session_id");
  const callbackKey =
    navigation.searchParams.has("result") ||
    navigation.searchParams.has("session_id")
      ? `${navigation.searchParams.get("result") ?? ""}:${returnSessionId ?? ""}`
      : null;
  const [returnState, setReturnState] = useState<{
    workspaceId: string | null;
    result: WorkspaceBillingReturnResult | null;
    observedAt: number | null;
  }>(() => ({
    workspaceId: wsId || null,
    result: returnResultParam,
    observedAt: returnResultParam === "success" ? Date.now() : null,
  }));
  const returnStateMatchesWorkspace =
    returnState.workspaceId === null || returnState.workspaceId === wsId;
  const returnResult = returnStateMatchesWorkspace ? returnState.result : null;
  const returnObservedAt = returnStateMatchesWorkspace
    ? returnState.observedAt
    : null;
  const [selectedPlan, setSelectedPlan] = useState<string | null>(null);
  const [interval, setInterval] =
    useState<WorkspaceSubscriptionInterval>("month");
  const [checkoutConfirmOpen, setCheckoutConfirmOpen] = useState(false);
  const [seatPurchaseOpen, setSeatPurchaseOpen] = useState(false);
  const [additionalSeatsInput, setAdditionalSeatsInput] = useState("1");
  const [seatPreview, setSeatPreview] =
    useState<WorkspaceSeatPurchasePreview | null>(null);
  const [seatPreviewRevision, setSeatPreviewRevision] = useState(0);
  const [seatPreviewRefreshing, setSeatPreviewRefreshing] = useState(false);
  const [seatPurchaseError, setSeatPurchaseError] = useState<string | null>(
    null,
  );
  const [seatPurchasePollingTimedOut, setSeatPurchasePollingTimedOut] =
    useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<{
    title: string;
    description: string;
  } | null>(null);
  const [portalUnavailable, setPortalUnavailable] = useState(false);
  const [seatPurchaseMessage, setSeatPurchaseMessage] = useState<string | null>(
    null,
  );
  const [planChangeOpen, setPlanChangeOpen] = useState(false);
  const [planChangePreview, setPlanChangePreview] =
    useState<WorkspacePlanChangePreview | null>(null);
  const [planChangePlanName, setPlanChangePlanName] = useState("");
  const [planChangeError, setPlanChangeError] = useState<string | null>(null);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [observedPlanLimitGate, setObservedPlanLimitGate] = useState<string | null>(null);
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false);
  const [quotaGate, setQuotaGate] = useState<string>("agents");
  const [quotaValueInput, setQuotaValueInput] = useState("");
  const [quotaReasonInput, setQuotaReasonInput] = useState("");
  const [quotaSubmitError, setQuotaSubmitError] = useState<string | null>(null);
  const [quotaSubmitting, setQuotaSubmitting] = useState(false);
  const [expandedOrderIds, setExpandedOrderIds] = useState<
    Record<string, boolean>
  >({});
  const [isSyncingCheckout, setIsSyncingCheckout] = useState(
    returnResult === "success",
  );
  const [syncTimedOut, setSyncTimedOut] = useState(false);
  const invoicesSectionRef = useRef<HTMLDivElement | null>(null);
  const checkoutIntentRef = useRef<{
    wsId: string;
    interval: WorkspaceSubscriptionInterval;
    key: string;
  } | null>(null);
  const portalIntentKeyRef = useRef<string | null>(null);
  const seatPurchaseIntentRef = useRef<{
    wsId: string;
    key: string;
    preview: WorkspaceSeatPurchasePreview;
    request: PurchaseWorkspaceSeatsRequest;
  } | null>(null);
  const seatPreviewInputRef = useRef("");
  const seatPreviewCapacityRetryRef = useRef<{
    inputKey: string;
    attempts: number;
  } | null>(null);
  const consumedCallbackKeyRef = useRef<string | null>(null);

  useEffect(() => {
    checkoutIntentRef.current = null;
    portalIntentKeyRef.current = null;
    seatPurchaseIntentRef.current = null;
    seatPreviewInputRef.current = "";
    seatPreviewCapacityRetryRef.current = null;
    setSeatPurchaseOpen(false);
    setSeatPreview(null);
    setSeatPreviewRevision(0);
    setSeatPreviewRefreshing(false);
    setSeatPurchaseError(null);
    setSeatPurchasePollingTimedOut(false);
    setPortalUnavailable(false);
    setActionError(null);
    setActionMessage(null);
    setSeatPurchaseMessage(null);
    setPlanChangeOpen(false);
    setPlanChangePreview(null);
    setPlanChangePlanName("");
    setPlanChangeError(null);
    setProfileError(null);
    setObservedPlanLimitGate(null);
    setQuotaDialogOpen(false);
    setQuotaSubmitError(null);
    setExpandedOrderIds({});
  }, [wsId]);

  // Consume Stripe callback params once, then remove them with replace so a
  // refresh, copied URL, or settings-tab round trip cannot replay a banner or
  // restart subscription polling. Keep unrelated params, especially `tab`.
  useEffect(() => {
    if (!callbackKey || consumedCallbackKeyRef.current === callbackKey) return;
    consumedCallbackKeyRef.current = callbackKey;
    setReturnState({
      workspaceId: wsId || null,
      result: returnResultParam,
      observedAt: returnResultParam === "success" ? Date.now() : null,
    });
    if (returnResultParam === "cancel") checkoutIntentRef.current = null;

    const params = new URLSearchParams(navigation.searchParams);
    params.delete("result");
    params.delete("session_id");
    const query = params.toString();
    navigation.replace(
      query ? `${navigation.pathname}?${query}` : navigation.pathname,
    );
  }, [callbackKey, navigation, returnResultParam, wsId]);

  const summaryQuery = useQuery({
    ...workspaceSubscriptionSummaryOptions(wsId),
    // Checkout activation and seat additions both finish asynchronously in a
    // Stripe webhook. Seat polling is bounded so a lost webhook cannot keep a
    // browser polling forever.
    refetchInterval: (query) =>
      isSyncingCheckout ||
      (query.state.data?.seatCapacity?.activePurchase &&
        !seatPurchasePollingTimedOut)
        ? 2_000
        : false,
  });
  const entitlements = summaryQuery.data?.entitlement;
  const isManualProvider =
    summaryQuery.data?.paymentProvider === "manual" ||
    (summaryQuery.data as any)?.payment_provider === "manual";
  const effectiveReturnResult = isManualProvider ? null : returnResult;
  const effectiveReturnObservedAt = isManualProvider ? null : returnObservedAt;

  useEffect(() => {
    if (effectiveReturnResult !== "success") {
      setIsSyncingCheckout(false);
      setSyncTimedOut(false);
      return;
    }
    setIsSyncingCheckout(true);
    setSyncTimedOut(false);
    const timeout = window.setTimeout(() => {
      setIsSyncingCheckout(false);
      setSyncTimedOut(true);
    }, CHECKOUT_SYNC_TIMEOUT_MS);
    return () => window.clearTimeout(timeout);
  }, [effectiveReturnResult, wsId]);

  const isCheckoutConfirmed =
    effectiveReturnResult === "success" &&
    effectiveReturnObservedAt !== null &&
    summaryQuery.data?.availableActions.checkout === false &&
    summaryQuery.isFetchedAfterMount &&
    summaryQuery.dataUpdatedAt >= effectiveReturnObservedAt;
  const activeSeatPurchaseRequestId =
    summaryQuery.data?.seatCapacity?.activePurchase?.requestId ?? null;

  useEffect(() => {
    if (!activeSeatPurchaseRequestId) {
      setSeatPurchasePollingTimedOut(false);
      return;
    }
    setSeatPurchasePollingTimedOut(false);
    const timeout = window.setTimeout(
      () => setSeatPurchasePollingTimedOut(true),
      SEAT_PURCHASE_POLL_TIMEOUT_MS,
    );
    return () => window.clearTimeout(timeout);
  }, [activeSeatPurchaseRequestId]);
  const summaryUnavailable =
    summaryQuery.isError ||
    (!summaryQuery.isPending && summaryQuery.data == null);
  const isUsagePlanSummary =
    summaryQuery.data?.billingModel === "usage" ||
    summaryQuery.data?.usage != null;
  const subscriptionUsageQuery = useQuery({
    ...workspaceSubscriptionUsageOptions(wsId),
    enabled: wsId.length > 0 && isUsagePlanSummary,
  });
  const ordersQuery = useQuery({
    ...workspaceOrdersOptions(wsId),
    enabled: wsId.length > 0 && summaryQuery.data != null,
  });
  const quotaUsageQuery = useQuery(autopilotQuotaUsageOptions(wsId));
  const issueLimitUsageQuery = useQuery({
    ...issueLimitUsageOptions(wsId),
    enabled:
      wsId.length > 0 &&
      entitlements?.limits.issueCount.mode === "limited",
  });
  const billingProfileQuery = useQuery(workspaceBillingProfileOptions(wsId));
  const quotaRequestsQuery = useQuery({
    queryKey: ["workspaces", wsId, "quota-requests"] as const,
    queryFn: () => api.listQuotaRequests(wsId),
    enabled: wsId.length > 0 && summaryQuery.data != null,
    retry: false,
  });
  const rawBillingProfile = billingProfileQuery.data;
  const billingProfile: WorkspaceBillingProfile | null =
    rawBillingProfile &&
    typeof rawBillingProfile === "object" &&
    "accountKind" in rawBillingProfile
      ? rawBillingProfile
      : null;
  const accountKind: WorkspaceBillingAccountKind =
    billingProfile?.accountKind === "business" ? "business" : "individual";

  const hasSeatCapacity = hasActiveWorkspaceSeatCapacity(summaryQuery.data);
  const canUpgrade = summaryQuery.data?.availableActions.checkout === true;
  const canChangePlan = summaryQuery.data?.availableActions.changePlan === true;
  const canCancelSubscription =
    summaryQuery.data?.availableActions.cancel === true;
  const canResumeSubscription =
    summaryQuery.data?.availableActions.resume === true;
  const canEditProfile =
    (
      summaryQuery.data?.availableActions as
        | { editProfile?: boolean }
        | undefined
    )?.editProfile === true;
  const pendingChange = summaryQuery.data?.pendingChange ?? null;

  const pricesQuery = useQuery({
    ...workspaceSubscriptionPricesOptions(wsId),
    enabled: wsId.length > 0 && canUpgrade,
  });
  const plansQuery = useQuery({
    ...workspacePlansOptions(wsId),
    enabled:
      wsId.length > 0 &&
      (canUpgrade || canChangePlan || pendingChange !== null),
  });
  const availablePlans = plansQuery.data ?? [];
  const pickerPlans = plansForAccountKind(availablePlans, accountKind);
  const activePlan =
    pickerPlans.find((p) => p.key === selectedPlan) ?? pickerPlans[0] ?? null;
  const checkoutMutation = useCreateWorkspaceSubscriptionCheckout(wsId);
  const portalMutation = useCreateWorkspaceSubscriptionPortal(wsId);
  const previewSeatPurchaseMutation = usePreviewWorkspaceSeatPurchase();
  const purchaseSeatsMutation = usePurchaseWorkspaceSeats(wsId);
  const updateBillingProfileMutation = useUpdateWorkspaceBillingProfile(wsId);
  const previewPlanChangeMutation = usePreviewWorkspacePlanChange();
  const createPlanChangeMutation = useCreateWorkspacePlanChange(wsId);
  const cancelScheduledPlanChangeMutation =
    useCancelWorkspaceScheduledPlanChange(wsId);
  const cancelSubscriptionMutation = useCancelWorkspaceSubscription(wsId);
  const resumeSubscriptionMutation = useResumeWorkspaceSubscription(wsId);
  const refetchSummary = summaryQuery.refetch;
  const previewSeatPurchase = previewSeatPurchaseMutation.mutateAsync;

  useEffect(() => {
    if (!seatPurchaseOpen) return;
    const value = additionalSeatsInput.trim();
    const additionalSeats = /^\d+$/.test(value) ? Number(value) : 0;
    const currentSeats = summaryQuery.data?.seatCapacity?.purchased ?? null;
    const purchaseVersion = summaryQuery.data?.seatCapacity?.version ?? null;
    const retryInputKey = `${wsId}:${value}`;
    if (seatPreviewCapacityRetryRef.current?.inputKey !== retryInputKey) {
      seatPreviewCapacityRetryRef.current = {
        inputKey: retryInputKey,
        attempts: 0,
      };
    }
    const requestKey = `${retryInputKey}:${currentSeats ?? ""}:${purchaseVersion ?? ""}:${seatPreviewRevision}`;
    seatPreviewInputRef.current = requestKey;
    const intent = seatPurchaseIntentRef.current;
    if (
      intent?.wsId === wsId &&
      intent.request.additionalSeats === additionalSeats &&
      intent.request.expectedCurrentSeats === currentSeats &&
      intent.request.expectedPurchaseVersion === purchaseVersion
    ) {
      setSeatPreview(intent.preview);
      setSeatPreviewRefreshing(false);
      setSeatPurchaseError(null);
      return;
    }
    seatPurchaseIntentRef.current = null;
    setSeatPreview(null);
    if (
      !Number.isSafeInteger(additionalSeats) ||
      additionalSeats < 1 ||
      currentSeats === null ||
      purchaseVersion === null
    ) {
      if (
        seatPreviewCapacityRetryRef.current?.inputKey === retryInputKey &&
        seatPreviewCapacityRetryRef.current.attempts > 0
      ) {
        setSeatPurchaseError(
          t(($) => $.workspace.seat_purchase.preview_failed),
        );
      }
      setSeatPreviewRefreshing(false);
      return;
    }

    const timeout = window.setTimeout(() => {
      void previewSeatPurchase({ additionalSeats })
        .then((preview) => {
          if (seatPreviewInputRef.current !== requestKey) return;
          if (
            !preview ||
            preview.additionalSeats !== additionalSeats ||
            preview.currentSeats !== currentSeats ||
            preview.purchaseVersion !== purchaseVersion ||
            preview.resultingSeats !== currentSeats + additionalSeats
          ) {
            setSeatPreviewRefreshing(false);
            setSeatPurchaseError(
              t(($) => $.workspace.seat_purchase.preview_unreadable),
            );
            return;
          }
          setSeatPreviewRefreshing(false);
          setSeatPreview(preview);
          setSeatPurchaseError(null);
        })
        .catch(async (error: unknown) => {
          if (seatPreviewInputRef.current !== requestKey) return;
          const previewErrorCode =
            error instanceof ApiError && error.status === 409
              ? errorCode(error)
              : null;
          if (previewErrorCode === "seat_capacity_changed") {
            const retry = seatPreviewCapacityRetryRef.current;
            if (retry?.inputKey !== retryInputKey || retry.attempts >= 1) {
              setSeatPreviewRefreshing(false);
              setSeatPurchaseError(
                t(($) => $.workspace.seat_purchase.capacity_out_of_sync),
              );
              return;
            }
            retry.attempts += 1;
            setSeatPreviewRefreshing(true);
            setSeatPurchaseError(null);
            try {
              const refreshed = await refetchSummary();
              if (seatPreviewInputRef.current !== requestKey) return;
              if (refreshed.isError === true) {
                setSeatPreviewRefreshing(false);
                setSeatPurchaseError(
                  t(($) => $.workspace.seat_purchase.preview_failed),
                );
                return;
              }
              const refreshedSeats =
                refreshed.data?.seatCapacity?.purchased ?? null;
              const refreshedVersion =
                refreshed.data?.seatCapacity?.version ?? null;
              if (
                refreshedSeats === currentSeats &&
                refreshedVersion === purchaseVersion
              ) {
                setSeatPreviewRefreshing(false);
                setSeatPurchaseError(
                  t(($) => $.workspace.seat_purchase.capacity_out_of_sync),
                );
                return;
              }
            } catch {
              if (seatPreviewInputRef.current === requestKey) {
                setSeatPreviewRefreshing(false);
                setSeatPurchaseError(
                  t(($) => $.workspace.seat_purchase.preview_failed),
                );
              }
              return;
            }
            if (seatPreviewInputRef.current !== requestKey) return;
            setSeatPreviewRevision((revision) => revision + 1);
            return;
          }
          setSeatPreviewRefreshing(false);
          setSeatPurchaseError(
            previewErrorCode === "seat_purchase_in_progress"
              ? t(($) => $.workspace.seat_purchase.in_progress)
              : t(($) => $.workspace.seat_purchase.preview_failed),
          );
        });
    }, SEAT_PURCHASE_PREVIEW_DEBOUNCE_MS);
    return () => window.clearTimeout(timeout);
  }, [
    additionalSeatsInput,
    previewSeatPurchase,
    refetchSummary,
    seatPurchaseOpen,
    seatPreviewRevision,
    summaryQuery.data?.seatCapacity?.purchased,
    summaryQuery.data?.seatCapacity?.version,
    t,
    wsId,
  ]);

  useEffect(() => {
    if (isSyncingCheckout && isCheckoutConfirmed) {
      setIsSyncingCheckout(false);
      setSyncTimedOut(false);
      checkoutIntentRef.current = null;
    }
  }, [isCheckoutConfirmed, isSyncingCheckout]);

  const planLabel = (plan: string) => {
    switch (plan) {
      case "free":
        return t(($) => $.workspace.plan.free);
      case "pro":
        return t(($) => $.workspace.plan.pro);
      case "payg":
        return t(($) => $.workspace.plan.payg);
      case "unlimited":
        return t(($) => $.workspace.plan.unlimited);
      default:
        return t(($) => $.workspace.plan.unknown);
    }
  };

  const orderKindLabel = (kind: string) => {
    switch (kind) {
      case "usage":
        return t(($) => $.workspace.invoices.usage_kind);
      case "subscribe":
        return t(($) => $.workspace.invoices.kind_subscribe);
      case "change_plan":
        return t(($) => $.workspace.invoices.kind_change_plan);
      case "add_seats":
        return t(($) => $.workspace.invoices.kind_add_seats);
      case "renew":
        return t(($) => $.workspace.invoices.kind_renew);
      default:
        return t(($) => $.workspace.invoices.kind_unknown);
    }
  };

  const orderStatusLabel = (status: string) => {
    switch (status) {
      case "pending":
        return t(($) => $.order.status.pending);
      case "paid":
        return t(($) => $.order.status.paid);
      case "expired":
        return t(($) => $.order.status.expired);
      case "canceled":
        return t(($) => $.order.status.canceled);
      case "refunded":
        return t(($) => $.order.status.refunded);
      default:
        return t(($) => $.order.status.unknown);
    }
  };

  const orderStatusVariant = (
    status: string,
  ): "default" | "secondary" | "outline" => {
    if (status === "paid") return "default";
    if (status === "pending") return "outline";
    return "secondary";
  };

  const handleViewUsageInvoice = (orderId: string) => {
    setExpandedOrderIds((prev) => ({ ...prev, [orderId]: true }));
    invoicesSectionRef.current?.scrollIntoView?.({
      behavior: "smooth",
      block: "nearest",
    });
  };

  const statusLabel = (status: string) => {
    switch (status) {
      case "inactive":
        return t(($) => $.workspace.status.inactive);
      case "active":
        return t(($) => $.workspace.status.active);
      case "trialing":
        return t(($) => $.workspace.status.trialing);
      case "past_due":
        return t(($) => $.workspace.status.past_due);
      case "canceled":
        return t(($) => $.workspace.status.canceled);
      case "incomplete":
        return t(($) => $.workspace.status.incomplete);
      case "incomplete_expired":
        return t(($) => $.workspace.status.incomplete_expired);
      case "paused":
        return t(($) => $.workspace.status.paused);
      case "unpaid":
        return t(($) => $.workspace.status.unpaid);
      default:
        return t(($) => $.workspace.status.unknown);
    }
  };

  const reportActionError = (error: unknown, fallback: string) => {
    const limit = planLimitReached(error);
    if (limit) {
      setObservedPlanLimitGate(limit.gate);
    } else if (errorCode(error) === "plan_limit_reached") {
      setObservedPlanLimitGate("agents");
    }
    const code = errorCode(error);
    if (
      code === "workspace_subscriptions_disabled" ||
      code === "cloud_runtime_not_configured"
    ) {
      setActionError(t(($) => $.workspace.errors.not_enabled));
      return;
    }
    if (error instanceof ApiError && error.status === 503) {
      setActionError(t(($) => $.workspace.errors.temporarily_unavailable));
      return;
    }
    if (error instanceof ApiError && error.status === 403) {
      setActionError(t(($) => $.workspace.errors.permission_changed));
      return;
    }
    setActionError(fallback);
  };

  const handleCheckout = async () => {
    setActionError(null);
    const existing = checkoutIntentRef.current;
    const intent =
      existing?.wsId === wsId && existing.interval === interval
        ? existing
        : {
            wsId,
            interval,
            key: createIdempotencyKey("workspace-checkout", wsId),
          };
    checkoutIntentRef.current = intent;
    try {
      const response = await checkoutMutation.mutateAsync({
        interval,
        idempotencyKey: intent.key,
        ...(selectedPlan ? { plan_key: selectedPlan, seats: actualSeats } : {}),
      } as any);
      if (!response?.url) {
        setCheckoutConfirmOpen(false);
        setActionError(t(($) => $.workspace.errors.checkout_response));
        return;
      }
      setCheckoutConfirmOpen(false);
      openExternal(response.url, { webTarget: "same-tab" });
    } catch (error) {
      setCheckoutConfirmOpen(false);
      if (error instanceof ApiError && error.status === 409) {
        checkoutIntentRef.current = null;
        setActionError(t(($) => $.workspace.errors.already_subscribed));
        await summaryQuery.refetch();
        return;
      }
      reportActionError(error, t(($) => $.workspace.errors.checkout_failed));
    }
  };

  const handleCheckoutConfirmOpenChange = (open: boolean) => {
    setCheckoutConfirmOpen(open);
    if (!open) checkoutIntentRef.current = null;
  };

  const handlePortal = async () => {
    setActionError(null);
    const key =
      portalIntentKeyRef.current ??
      createIdempotencyKey("workspace-portal", wsId);
    portalIntentKeyRef.current = key;
    try {
      const response = await portalMutation.mutateAsync(key);
      if (!response?.url) {
        setActionError(t(($) => $.workspace.errors.portal_response));
        return;
      }
      openExternal(response.url, { webTarget: "same-tab" });
      // A Portal URL is single-use. A later click is a new intent, while a
      // network failure above deliberately retains the key for safe retry.
      portalIntentKeyRef.current = null;
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        portalIntentKeyRef.current = null;
        setPortalUnavailable(true);
        setActionError(t(($) => $.workspace.errors.portal_unavailable));
        await summaryQuery.refetch();
        return;
      }
      reportActionError(error, t(($) => $.workspace.errors.portal_failed));
    }
  };

  const handleSeatPurchase = async () => {
    const confirmedPreview = seatPreview;
    if (!confirmedPreview) return;
    setSeatPurchaseError(null);
    const request: PurchaseWorkspaceSeatsRequest = {
      additionalSeats: confirmedPreview.additionalSeats,
      expectedCurrentSeats: confirmedPreview.currentSeats,
      expectedPurchaseVersion: confirmedPreview.purchaseVersion,
      acceptedProrationAmount: confirmedPreview.prorationAmount,
      currency: confirmedPreview.currency,
      idempotencyKey: "",
    };
    const existing = seatPurchaseIntentRef.current;
    const key =
      existing?.wsId === wsId &&
      existing.request.additionalSeats === request.additionalSeats &&
      existing.request.expectedCurrentSeats === request.expectedCurrentSeats &&
      existing.request.expectedPurchaseVersion ===
        request.expectedPurchaseVersion &&
      existing.request.acceptedProrationAmount ===
        request.acceptedProrationAmount &&
      existing.request.currency === request.currency
        ? existing.key
        : createIdempotencyKey("workspace-seat-purchase", wsId).slice(0, 200);
    request.idempotencyKey = key;
    seatPurchaseIntentRef.current = {
      wsId,
      key,
      preview: confirmedPreview,
      request,
    };
    try {
      const response = await purchaseSeatsMutation.mutateAsync(request);
      if (
        !response ||
        response.currentSeats !== confirmedPreview.currentSeats ||
        response.additionalSeats !== confirmedPreview.additionalSeats ||
        response.resultingSeats !== confirmedPreview.resultingSeats ||
        response.currency !== confirmedPreview.currency
      ) {
        setSeatPurchaseError(
          t(($) => $.workspace.seat_purchase.purchase_unreadable),
        );
        return;
      }
      seatPurchaseIntentRef.current = null;
      seatPreviewInputRef.current = "";
      setSeatPurchaseOpen(false);
      setSeatPreview(null);
      setSeatPurchaseMessage(
        t(($) => $.workspace.seat_purchase.submitted, {
          count: response.resultingSeats,
        }),
      );
      await summaryQuery.refetch();
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        const purchaseErrorCode = errorCode(error);
        seatPurchaseIntentRef.current = null;
        setSeatPreview(null);
        if (purchaseErrorCode === "seat_purchase_in_progress") {
          setSeatPurchaseError(
            t(($) => $.workspace.seat_purchase.in_progress),
          );
          await summaryQuery.refetch();
          return;
        }
        setSeatPurchaseError(t(($) => $.workspace.seat_purchase.quote_changed));
        await summaryQuery.refetch();
        setSeatPreviewRevision((revision) => revision + 1);
        return;
      }
      if (error instanceof ApiError && error.status === 402) {
        seatPurchaseIntentRef.current = null;
        setSeatPreview(null);
        setSeatPurchaseError(
          t(($) => $.workspace.seat_purchase.payment_failed),
        );
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        seatPurchaseIntentRef.current = null;
        reportActionError(
          error,
          t(($) => $.workspace.seat_purchase.purchase_failed),
        );
        return;
      }
      setSeatPurchaseError(
        t(($) => $.workspace.seat_purchase.purchase_failed),
      );
    }
  };

  const handleSeatPurchaseOpenChange = (open: boolean) => {
    if (!open && purchaseSeatsMutation.isPending) return;
    setSeatPurchaseOpen(open);
    seatPreviewCapacityRetryRef.current = null;
    setSeatPreviewRefreshing(false);
    if (!open) {
      seatPreviewInputRef.current = "";
      return;
    }
    setSeatPurchaseError(null);
    const intent = seatPurchaseIntentRef.current;
    if (intent?.wsId === wsId) {
      setAdditionalSeatsInput(String(intent.request.additionalSeats));
      setSeatPreview(intent.preview);
      return;
    }
    setAdditionalSeatsInput("1");
    setSeatPreview(null);
  };

  const handleOpenPlanChange = async () => {
    if (!activePlan) return;
    setActionError(null);
    setActionMessage(null);
    setPlanChangeError(null);
    const targetSeats = Math.max(
      billedSeats ?? actualSeats,
      activePlan.minSeats ?? 1,
    );
    try {
      const preview = await previewPlanChangeMutation.mutateAsync({
        planKey: activePlan.key,
        interval,
        seats: targetSeats,
      });
      if (!preview) {
        setActionError(t(($) => $.workspace.plan_change.preview_failed));
        return;
      }
      setPlanChangePreview(preview);
      setPlanChangePlanName(activePlan.name || activePlan.key);
      setPlanChangeOpen(true);
    } catch (error) {
      reportActionError(
        error,
        t(($) => $.workspace.plan_change.preview_failed),
      );
    }
  };

  const handleConfirmPlanChange = async () => {
    if (!planChangePreview) return;
    setPlanChangeError(null);
    setActionError(null);
    setActionMessage(null);
    try {
      const response = await createPlanChangeMutation.mutateAsync({
        planKey: planChangePreview.planKey,
        interval: planChangePreview.interval,
        seats: planChangePreview.resultingSeats,
        acceptedAmount: planChangePreview.amount,
        expectedVersion: planChangePreview.expectedVersion,
        idempotencyKey: createIdempotencyKey("plan-change", wsId),
      });
      if (!response) {
        setPlanChangeError(t(($) => $.workspace.plan_change.confirm_failed));
        return;
      }
      setPlanChangeOpen(false);
      setPlanChangePreview(null);
      setPlanChangeError(null);
      const targetPlanName =
        availablePlans.find((p) => p.key === response.planKey)?.name ??
        (planChangePlanName || response.planKey);
      if (response.outcome === "scheduled") {
        setActionMessage({
          title: t(($) => $.workspace.plan_change.scheduled_title),
          description: t(($) => $.workspace.plan_change.scheduled_description, {
            planName: targetPlanName,
          }),
        });
      } else {
        setActionMessage({
          title: t(($) => $.workspace.plan_change.order_title),
          description: t(($) => $.workspace.plan_change.order_description, {
            requestId: response.requestId,
          }),
        });
      }
    } catch (error) {
      if (error instanceof ApiError && errorCode(error) === "quote_changed") {
        try {
          const refreshedPreview = await previewPlanChangeMutation.mutateAsync({
            planKey: planChangePreview.planKey,
            interval: planChangePreview.interval,
            seats: planChangePreview.resultingSeats,
          });
          if (refreshedPreview) {
            setPlanChangePreview(refreshedPreview);
            setPlanChangeError(t(($) => $.workspace.plan_change.quote_changed));
            return;
          }
        } catch (previewError) {
          setPlanChangeOpen(false);
          reportActionError(
            previewError,
            t(($) => $.workspace.plan_change.preview_failed),
          );
          return;
        }
        setPlanChangeError(t(($) => $.workspace.plan_change.preview_failed));
        return;
      }
      setPlanChangeOpen(false);
      reportActionError(
        error,
        t(($) => $.workspace.plan_change.confirm_failed),
      );
    }
  };

  const handlePlanChangeOpenChange = (open: boolean) => {
    if (
      !open &&
      (createPlanChangeMutation.isPending || previewPlanChangeMutation.isPending)
    ) {
      return;
    }
    setPlanChangeOpen(open);
    if (!open) {
      setPlanChangeError(null);
    }
  };

  const handleCancelScheduledPlanChange = async () => {
    setActionError(null);
    setActionMessage(null);
    try {
      await cancelScheduledPlanChangeMutation.mutateAsync(
        createIdempotencyKey("cancel-plan-change", wsId),
      );
    } catch (error) {
      reportActionError(
        error,
        t(($) => $.workspace.pending_change.cancel_failed),
      );
    }
  };

  const handleCancelSubscription = async () => {
    setActionError(null);
    setActionMessage(null);
    try {
      await cancelSubscriptionMutation.mutateAsync(
        createIdempotencyKey("cancel", wsId),
      );
    } catch (error) {
      reportActionError(
        error,
        t(($) => $.workspace.lifecycle.cancel_failed),
      );
    }
  };

  const handleResumeSubscription = async () => {
    setActionError(null);
    setActionMessage(null);
    try {
      await resumeSubscriptionMutation.mutateAsync(
        createIdempotencyKey("resume", wsId),
      );
    } catch (error) {
      reportActionError(
        error,
        t(($) => $.workspace.lifecycle.resume_failed),
      );
    }
  };

  const handleSaveBillingProfile = async (
    data: UpdateWorkspaceBillingProfileRequest,
  ) => {
    setProfileError(null);
    setActionError(null);
    setActionMessage(null);
    try {
      await updateBillingProfileMutation.mutateAsync(data);
      setActionMessage({
        title: t(($) => $.workspace.profile.title),
        description: t(($) => $.workspace.profile.saved),
      });
    } catch (error) {
      const code = errorCode(error);
      if (
        code === "workspace_subscriptions_disabled" ||
        code === "cloud_runtime_not_configured"
      ) {
        setProfileError(t(($) => $.workspace.errors.not_enabled));
        return;
      }
      if (error instanceof ApiError && error.status === 503) {
        setProfileError(t(($) => $.workspace.errors.temporarily_unavailable));
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        setProfileError(t(($) => $.workspace.errors.permission_changed));
        return;
      }
      setProfileError(t(($) => $.workspace.profile.save_failed));
    }
  };

  if (summaryQuery.isPending) {
    return (
      <SettingsTab
        title={t(($) => $.workspace.title)}
        scope="workspace"
      >
        <SettingsCard>
          <div
            className="space-y-4 p-4 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
            aria-label={t(($) => $.workspace.loading)}
          >
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        </SettingsCard>
      </SettingsTab>
    );
  }

  if (summaryQuery.isError || !entitlements) {
    return (
      <SettingsTab
        title={t(($) => $.workspace.title)}
        scope="workspace"
      >
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>{t(($) => $.workspace.load_failed.title)}</AlertTitle>
          <AlertDescription>
            <p>{t(($) => $.workspace.load_failed.description)}</p>
            <Button
              className="mt-3 h-11"
              variant="outline"
              onClick={() => void summaryQuery.refetch()}
            >
              <RefreshCw />
              {t(($) => $.workspace.actions.retry)}
            </Button>
          </AlertDescription>
        </Alert>
      </SettingsTab>
    );
  }

  const summaryPeriodEnd = formatDate(
    summaryQuery.data?.entitlement.currentPeriodEnd ?? null,
    locale,
  );
  const graceUntilValue = summaryQuery.data?.graceUntil ?? null;
  const graceUntil = formatDate(graceUntilValue, locale);
  const actualSeats = summaryQuery.data?.humanMembers ?? entitlements.seats;
  const seatCapacity = summaryQuery.data?.seatCapacity ?? null;
  const usedSeats = seatCapacity?.used ?? actualSeats;
  const billedSeats = seatCapacity?.purchased ?? null;
  const pendingSeatQuantity = seatCapacity?.pendingQuantity ?? null;
  const reservedSeats = seatCapacity?.reserved ?? 0;
  const membersExceedPurchasedSeats = seatCapacity?.overcommitted === true;
  const availableSeats = seatCapacity?.available ?? null;
  const activeSeatPurchase = seatCapacity?.activePurchase ?? null;
  const activeSeatPurchaseExpiry = formatDateTime(
    activeSeatPurchase?.expiresAt ?? null,
    locale,
  );
  const canAddSeats =
    summaryQuery.data?.availableActions.purchaseSeats === true;
  const quotaUsage = resolveAutopilotUsage(
    entitlements,
    quotaUsageQuery.data,
    quotaUsageQuery.isError,
  );
  const quotaResetAt =
    quotaUsage.kind === "metered"
      ? formatDateTime(quotaUsage.resetAt, locale)
      : null;
  const numberFormatter = new Intl.NumberFormat(locale);
  const issueCountLimit = entitlements.limits.issueCount;
  const issueLimitUsage = issueLimitUsageQuery.data;
  const issueLimitValue =
    issueCountLimit.mode === "unlimited"
      ? t(($) => $.workspace.limits.unlimited)
      : issueLimitUsage?.limit === issueCountLimit.limit
        ? `${numberFormatter.format(issueLimitUsage.used)} / ${numberFormatter.format(issueLimitUsage.limit)}`
        : numberFormatter.format(issueCountLimit.limit);
  const isMutating =
    checkoutMutation.isPending ||
    portalMutation.isPending ||
    purchaseSeatsMutation.isPending ||
    previewPlanChangeMutation.isPending ||
    createPlanChangeMutation.isPending ||
    cancelScheduledPlanChangeMutation.isPending ||
    cancelSubscriptionMutation.isPending ||
    resumeSubscriptionMutation.isPending;
  const pendingPlanName = pendingChange
    ? (availablePlans.find((p) => p.key === pendingChange.planKey)?.name ??
      pendingChange.planKey)
    : "";
  const selectedPrice = pricesQuery.data?.[interval] ?? null;
  const planUnitPrice =
    activePlan
      ? interval === "month"
        ? activePlan.seatPriceMonth
        : activePlan.seatPriceYear
      : null;
  const formattedUnitPrice =
    planUnitPrice !== undefined && planUnitPrice !== null && activePlan
      ? formatStripeMinorAmount(planUnitPrice, activePlan.currency, locale)
      : selectedPrice
        ? formatStripeMinorAmount(
            selectedPrice.unitAmount,
            selectedPrice.currency,
            locale,
          )
        : null;
  const hasDisplayableUnitPrice =
    formattedUnitPrice !== null &&
    (planUnitPrice !== undefined && planUnitPrice !== null
      ? true
      : selectedPrice?.intervalCount === 1);
  const canRetryPrice = !pricesQuery.isLoading && selectedPrice === null;
  const formattedSeatProration = seatPreview
    ? formatStripeMinorAmount(
        seatPreview.prorationAmount,
        seatPreview.currency,
        locale,
      )
    : null;
  const formattedNextSeatInvoice = seatPreview
    ? formatStripeMinorAmount(
        seatPreview.nextInvoiceAmount,
        seatPreview.currency,
        locale,
      )
    : null;
  const isUsageBilling =
    isUsagePlanSummary || subscriptionUsageQuery.data?.billingModel === "usage";
  const activeUsagePeriod =
    subscriptionUsageQuery.data?.usage ?? summaryQuery.data?.usage ?? null;
  const unpaidUsageOrderId =
    summaryQuery.data?.usage?.unpaidOrderId ??
    subscriptionUsageQuery.data?.usage?.unpaidOrderId ??
    null;
  const orders: WorkspaceOrder[] = ordersQuery.data ?? [];
  const quotaRequests: QuotaRequest[] = quotaRequestsQuery.data ?? [];
  const queryPlanLimit =
    planLimitReached(summaryQuery.error) ??
    planLimitReached(ordersQuery.error) ??
    planLimitReached(quotaUsageQuery.error) ??
    planLimitReached(issueLimitUsageQuery.error);
  const queryHasPlanLimitCode =
    errorCode(summaryQuery.error) === "plan_limit_reached" ||
    errorCode(ordersQuery.error) === "plan_limit_reached" ||
    errorCode(quotaUsageQuery.error) === "plan_limit_reached" ||
    errorCode(issueLimitUsageQuery.error) === "plan_limit_reached";
  const usageLimitReached =
    (quotaUsage.kind === "metered" && quotaUsage.reached) ||
    (issueCountLimit.mode === "limited" &&
      issueLimitUsage != null &&
      issueLimitUsage.used >= issueLimitUsage.limit);
  const defaultQuotaGate =
    recordedPlanLimit?.gate ??
    queryPlanLimit?.gate ??
    observedPlanLimitGate ??
    (quotaUsage.kind === "metered" && quotaUsage.reached
      ? "autopilot_runs"
      : issueCountLimit.mode === "limited" &&
          issueLimitUsage != null &&
          issueLimitUsage.used >= issueLimitUsage.limit
        ? "issue_count"
        : "agents");
  const hasPlanLimitReached =
    recordedPlanLimit !== null ||
    queryPlanLimit !== undefined ||
    queryHasPlanLimitCode ||
    observedPlanLimitGate !== null ||
    usageLimitReached;
  const showQuotaRequestsSection =
    hasPlanLimitReached || quotaRequests.length > 0;
  const knownCurrentGateLimit =
    recordedPlanLimit?.gate === quotaGate
      ? recordedPlanLimit.limit
      : queryPlanLimit?.gate === quotaGate
        ? queryPlanLimit.limit
        : quotaGate === "issue_count" && issueCountLimit.mode === "limited"
          ? issueCountLimit.limit
          : quotaGate === "autopilot_runs" && quotaUsage.kind === "metered"
            ? quotaUsage.limit
            : null;
  const quotaGateOptions = QUOTA_REQUEST_GATES.includes(
    quotaGate as (typeof QUOTA_REQUEST_GATES)[number],
  )
    ? [...QUOTA_REQUEST_GATES]
    : [...QUOTA_REQUEST_GATES, quotaGate];
  const quotaGateSelectItems = quotaGateOptions.map((gate) => ({
    value: gate,
    label: quotaGateLabel(gate, tSettings),
  }));

  const handleOpenQuotaRequestDialog = () => {
    setQuotaGate(defaultQuotaGate);
    setQuotaValueInput("");
    setQuotaReasonInput("");
    setQuotaSubmitError(null);
    setQuotaDialogOpen(true);
  };

  const handleQuotaDialogOpenChange = (open: boolean) => {
    if (!open && quotaSubmitting) return;
    setQuotaDialogOpen(open);
    if (!open) {
      setQuotaSubmitError(null);
    }
  };

  const handleSubmitQuotaRequest = async () => {
    if (quotaSubmitting || !canManageBilling) return;
    const rawValue = quotaValueInput.trim();
    const parsedValue = /^\d+$/.test(rawValue) ? Number(rawValue) : NaN;
    if (!Number.isSafeInteger(parsedValue) || parsedValue <= 0) {
      setQuotaSubmitError(
        tSettings(($) => $.quota_requests.dialog.value_invalid),
      );
      return;
    }
    if (knownCurrentGateLimit !== null && parsedValue <= knownCurrentGateLimit) {
      setQuotaSubmitError(
        tSettings(($) => $.quota_requests.dialog.value_must_exceed, {
          current: numberFormatter.format(knownCurrentGateLimit),
        }),
      );
      return;
    }
    const trimmedReason = quotaReasonInput.trim();
    if (!trimmedReason) {
      setQuotaSubmitError(
        tSettings(($) => $.quota_requests.dialog.reason_required),
      );
      return;
    }
    if ([...trimmedReason].length > QUOTA_REQUEST_MAX_REASON_LENGTH) {
      setQuotaSubmitError(
        tSettings(($) => $.quota_requests.dialog.reason_too_long),
      );
      return;
    }

    setQuotaSubmitting(true);
    setQuotaSubmitError(null);
    try {
      await api.createQuotaRequest(wsId, {
        gate: quotaGate,
        requested_value: parsedValue,
        reason: trimmedReason,
      });
      setQuotaDialogOpen(false);
      setQuotaValueInput("");
      setQuotaReasonInput("");
      setActionMessage({
        title: tSettings(($) => $.quota_requests.submitted_title),
        description: tSettings(($) => $.quota_requests.submitted_description, {
          gate: quotaGateLabel(quotaGate, tSettings),
          value: numberFormatter.format(parsedValue),
        }),
      });
      void quotaRequestsQuery.refetch();
    } catch (error) {
      setQuotaSubmitError(mapQuotaRequestError(error, tSettings));
    } finally {
      setQuotaSubmitting(false);
    }
  };
  const showInvoicesSection =
    isUsageBilling ||
    isManualProvider ||
    orders.length > 0 ||
    ordersQuery.isError;

  return (
    <SettingsTab
      title={t(($) => $.workspace.title)}
      scope="workspace"
    >
      {effectiveReturnResult === "cancel" ? (
        <Alert>
          <AlertCircle />
          <AlertTitle>{t(($) => $.workspace.return.cancel_title)}</AlertTitle>
          <AlertDescription>
            {t(($) => $.workspace.return.cancel_description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {effectiveReturnResult === "portal" ? (
        <Alert>
          <CheckCircle2 />
          <AlertTitle>{t(($) => $.workspace.return.portal_title)}</AlertTitle>
        </Alert>
      ) : null}

      {effectiveReturnResult === "success" ? (
        <Alert>
          {isCheckoutConfirmed ? (
            <CheckCircle2 />
          ) : (
            <Loader2
              className={
                isSyncingCheckout
                  ? "animate-spin motion-reduce:animate-none"
                  : undefined
              }
            />
          )}
          <AlertTitle>
            {isCheckoutConfirmed
              ? t(($) => $.workspace.return.active_title)
              : t(($) => $.workspace.return.syncing_title)}
          </AlertTitle>
          <AlertDescription>
            {isCheckoutConfirmed
              ? t(($) => $.workspace.return.active_description)
              : syncTimedOut
                ? t(($) => $.workspace.return.timeout_description)
                : t(($) => $.workspace.return.syncing_description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {summaryQuery.data?.cancelAtPeriodEnd ? (
        <Alert>
          <AlertCircle />
          <AlertTitle>
            {t(($) => $.workspace.subscription_notice.canceling_title)}
          </AlertTitle>
          <AlertDescription>
            {summaryPeriodEnd
              ? t(
                  ($) =>
                    $.workspace.subscription_notice.canceling_description,
                  { date: summaryPeriodEnd },
                )
              : t(
                  ($) =>
                    $.workspace.subscription_notice
                      .canceling_description_without_date,
                )}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "past_due" ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>{t(($) => $.workspace.past_due.title)}</AlertTitle>
          <AlertDescription>
            {graceUntil
              ? t(($) => $.workspace.past_due.grace_description, {
                  date: graceUntil,
                })
              : t(($) => $.workspace.past_due.description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "incomplete" ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>
            {t(($) => $.workspace.subscription_notice.incomplete_title)}
          </AlertTitle>
          <AlertDescription>
            {t(($) => $.workspace.subscription_notice.incomplete_description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "incomplete_expired" ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>
            {t(
              ($) => $.workspace.subscription_notice.incomplete_expired_title,
            )}
          </AlertTitle>
          <AlertDescription>
            {t(
              ($) =>
                $.workspace.subscription_notice
                  .incomplete_expired_description,
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "paused" ? (
        <Alert>
          <AlertCircle />
          <AlertTitle>
            {t(($) => $.workspace.subscription_notice.paused_title)}
          </AlertTitle>
          <AlertDescription>
            {t(($) => $.workspace.subscription_notice.paused_description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "unpaid" ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>
            {t(($) => $.workspace.subscription_notice.unpaid_title)}
          </AlertTitle>
          <AlertDescription>
            {t(($) => $.workspace.subscription_notice.unpaid_description)}
          </AlertDescription>
        </Alert>
      ) : null}

      {entitlements.status === "canceled" ? (
        <Alert>
          <AlertCircle />
          <AlertTitle>
            {t(($) => $.workspace.subscription_notice.canceled_title)}
          </AlertTitle>
        </Alert>
      ) : null}

      {actionError ? (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertTitle>{t(($) => $.workspace.errors.action_title)}</AlertTitle>
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      ) : null}

      {actionMessage ? (
        <Alert>
          <CheckCircle2 />
          <AlertTitle>{actionMessage.title}</AlertTitle>
          <AlertDescription>{actionMessage.description}</AlertDescription>
        </Alert>
      ) : null}

      <PendingChangeBanner
        pendingChange={pendingChange}
        planName={pendingPlanName}
        effectiveAt={summaryQuery.data?.entitlement.currentPeriodEnd ?? null}
        canCancel={canChangePlan}
        cancelling={cancelScheduledPlanChangeMutation.isPending}
        onCancel={() => void handleCancelScheduledPlanChange()}
      />

      {(() => {
        if (!canUpgrade && !canChangePlan) return null;
        const currentPlan =
          availablePlans.find((p) => p.key === entitlements.plan) ?? null;
        if (!currentPlan?.scheduledPriceChange) return null;
        return (
          <ScheduledPriceChangeBanner
            scheduledPriceChange={currentPlan.scheduledPriceChange}
            planName={currentPlan.name}
            currency={currentPlan.currency}
          />
        );
      })()}

      <UnpaidUsageBanner
        unpaidOrderId={unpaidUsageOrderId}
        onViewInvoice={handleViewUsageInvoice}
      />

      <SettingsSection title={t(($) => $.workspace.current.title)}>
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.workspace.current.plan)}
          >
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              <Badge variant={planBadgeVariant(entitlements.plan)}>
                {planLabel(entitlements.plan)}
              </Badge>
              <Badge variant={statusBadgeVariant(entitlements.status)}>
                {statusLabel(entitlements.status)}
              </Badge>
            </div>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.workspace.current.members)}
            description={t(($) => $.workspace.current.members_description)}
          >
            <span className="tabular-nums">
              {t(($) => $.workspace.current.member_count, {
                count: actualSeats,
              })}
            </span>
          </SettingsRow>
          {summaryQuery.data?.billingInterval ? (
            <SettingsRow
              label={t(($) => $.workspace.current.billing_interval)}
            >
              <span>
                {summaryQuery.data.billingInterval === "month"
                  ? t(($) => $.workspace.upgrade.monthly)
                  : t(($) => $.workspace.upgrade.yearly)}
              </span>
            </SettingsRow>
          ) : null}
          {summaryPeriodEnd ? (
            <SettingsRow
              label={t(($) => $.workspace.current.period_end)}
            >
              <span className="tabular-nums">{summaryPeriodEnd}</span>
            </SettingsRow>
          ) : null}
        </SettingsCard>
        <SubscriptionLifecycleSection
          canCancel={canCancelSubscription}
          canResume={canResumeSubscription}
          periodEnd={summaryQuery.data?.entitlement.currentPeriodEnd ?? null}
          cancelling={cancelSubscriptionMutation.isPending}
          resuming={resumeSubscriptionMutation.isPending}
          onCancel={() => void handleCancelSubscription()}
          onResume={() => void handleResumeSubscription()}
        />
      </SettingsSection>

      {isUsageBilling ? (
        <UsageCard
          usage={activeUsagePeriod}
          loading={subscriptionUsageQuery.isPending && !activeUsagePeriod}
          error={
            (subscriptionUsageQuery.isError ||
              (!subscriptionUsageQuery.isPending &&
                subscriptionUsageQuery.data === null)) &&
            !activeUsagePeriod
          }
          onRetry={() => void subscriptionUsageQuery.refetch()}
        />
      ) : null}

      {canUpgrade || canChangePlan ? (
        <SettingsSection
          title={
            canUpgrade
              ? t(($) => $.workspace.upgrade.title)
              : t(($) => $.workspace.plan_change.action)
          }
          description={
            canUpgrade ? t(($) => $.workspace.upgrade.description) : undefined
          }
        >
          <SettingsCard>
            <div className="space-y-5 p-4 sm:p-5">
              {pickerPlans.length > 0 ? (
                <div className="space-y-2">
                  <p className="text-body font-medium">
                    {t(($) => $.workspace.upgrade.plan_label)}
                  </p>
                  <div
                    className="flex w-full flex-wrap gap-2 rounded-lg border border-surface-border p-1 sm:w-auto"
                    role="group"
                    aria-label={t(($) => $.workspace.upgrade.plan_label)}
                  >
                    {pickerPlans.map((p) => {
                      const isPressed =
                        (selectedPlan ?? pickerPlans[0]?.key) === p.key;
                      const isUsagePlan = p.billingModel === "usage";
                      const usageRateEntries = isUsagePlan
                        ? Object.entries(p.usageRates ?? {})
                        : [];
                      const discountPct = p.effectiveDiscountPct ?? 0;

                      if (!isUsagePlan) {
                        return (
                          <button
                            key={p.key}
                            type="button"
                            aria-pressed={isPressed}
                            className="min-h-11 flex-1 rounded-md px-4 text-body font-medium text-muted-foreground transition-[color,background-color,box-shadow] hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-pressed:bg-surface-selected aria-pressed:text-surface-selected-foreground aria-pressed:shadow-sm sm:min-w-28"
                            onClick={() => {
                              setSelectedPlan(p.key);
                              checkoutIntentRef.current = null;
                            }}
                          >
                            {p.name}
                          </button>
                        );
                      }

                      return (
                        <div
                          key={p.key}
                          className="flex w-full flex-col gap-2 rounded-md border border-surface-border p-3"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <button
                              type="button"
                              aria-pressed={isPressed}
                              className="min-h-11 rounded-md px-4 text-body font-medium text-muted-foreground transition-[color,background-color,box-shadow] hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-pressed:bg-surface-selected aria-pressed:text-surface-selected-foreground aria-pressed:shadow-sm"
                              onClick={() => {
                                setSelectedPlan(p.key);
                                checkoutIntentRef.current = null;
                              }}
                            >
                              {p.name}
                            </button>
                            {discountPct > 0 ? (
                              <Badge variant="secondary" className="tabular-nums">
                                {t(($) => $.workspace.upgrade.business_discount, {
                                  pct: discountPct,
                                })}
                              </Badge>
                            ) : null}
                          </div>
                          {usageRateEntries.length > 0 ? (
                            <ul className="space-y-1 text-caption text-muted-foreground">
                              {usageRateEntries.map(([meterKey, rate]) => (
                                <li
                                  key={meterKey}
                                  className="flex flex-wrap items-center justify-between gap-2 tabular-nums"
                                >
                                  <span>{resolveMeterLabel(meterKey, t)}</span>
                                  <span>
                                    {t(($) => $.workspace.usage.rate, {
                                      price: formatUsageMoney(
                                        rate.price,
                                        p.currency,
                                        locale,
                                      ),
                                      per: formatUsageUnitPer(
                                        meterKey,
                                        rate.per,
                                        locale,
                                      ),
                                    })}
                                  </span>
                                </li>
                              ))}
                            </ul>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : null}

              {activePlan?.billingModel === "usage" ? (
                <p className="text-body text-muted-foreground">
                  {t(($) => $.workspace.upgrade.usage_billing_note)}
                </p>
              ) : (
                <>
                  <div
                    className="inline-flex w-full rounded-lg border border-surface-border p-1 sm:w-auto"
                    role="group"
                    aria-label={t(($) => $.workspace.upgrade.interval_label)}
                  >
                    {(["month", "year"] as const).map((value) => (
                      <button
                        key={value}
                        type="button"
                        aria-pressed={interval === value}
                        className="min-h-11 flex-1 rounded-md px-4 text-body font-medium text-muted-foreground transition-[color,background-color,box-shadow] hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-pressed:bg-surface-selected aria-pressed:text-surface-selected-foreground aria-pressed:shadow-sm sm:min-w-32"
                        onClick={() => {
                          setInterval(value);
                          checkoutIntentRef.current = null;
                        }}
                      >
                        {value === "month"
                          ? t(($) => $.workspace.upgrade.monthly)
                          : t(($) => $.workspace.upgrade.yearly)}
                      </button>
                    ))}
                  </div>
                  {canUpgrade ? (
                    <div className="space-y-2">
                      <p className="text-body font-medium">
                        {t(($) => $.workspace.upgrade.pro_for_team, {
                          count: actualSeats,
                        })}
                      </p>
                      {pricesQuery.isLoading ? (
                        <div
                          className="space-y-2 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
                          role="status"
                          aria-label={t(($) => $.workspace.upgrade.price_loading)}
                        >
                          <Skeleton className="h-5 w-48" />
                          <Skeleton className="h-4 w-64 max-w-full" />
                        </div>
                      ) : hasDisplayableUnitPrice ? (
                        <div className="space-y-1">
                          <p className="text-body font-semibold tabular-nums">
                            {t(($) => $.workspace.upgrade.unit_price, {
                              price: formattedUnitPrice,
                            })}
                          </p>
                        </div>
                      ) : null}
                      <p className="max-w-[65ch] text-caption leading-5 text-muted-foreground">
                        {t(($) => $.workspace.upgrade.price_at_checkout)}
                      </p>
                      {canRetryPrice ? (
                        <>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            aria-busy={pricesQuery.isFetching}
                            disabled={pricesQuery.isFetching}
                            onClick={() => void pricesQuery.refetch()}
                          >
                            {pricesQuery.isFetching ? (
                              <Loader2 className="animate-spin" />
                            ) : (
                              <RefreshCw />
                            )}
                            {t(($) => $.workspace.actions.retry)}
                          </Button>
                          {pricesQuery.isFetching ? (
                            <span
                              className="sr-only"
                              role="status"
                              aria-label={t(
                                ($) => $.workspace.upgrade.price_loading,
                              )}
                            >
                              {t(($) => $.workspace.upgrade.price_loading)}
                            </span>
                          ) : null}
                        </>
                      ) : null}
                    </div>
                  ) : null}
                </>
              )}
              <div className="flex flex-wrap gap-3">
                {canUpgrade ? (
                  <Button
                    className="h-11 w-full sm:w-auto"
                    disabled={isMutating}
                    onClick={() => setCheckoutConfirmOpen(true)}
                  >
                    <CreditCard />
                    {t(($) => $.workspace.actions.upgrade)}
                  </Button>
                ) : null}
                {canChangePlan ? (
                  <Button
                    type="button"
                    className="h-11 w-full sm:w-auto"
                    variant={canUpgrade ? "outline" : "default"}
                    disabled={isMutating || !activePlan}
                    aria-busy={previewPlanChangeMutation.isPending}
                    onClick={() => void handleOpenPlanChange()}
                  >
                    {previewPlanChangeMutation.isPending ? (
                      <Loader2 className="animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {t(($) => $.workspace.plan_change.action)}
                  </Button>
                ) : null}
              </div>
            </div>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {billingProfileQuery.data ? (
        <SettingsSection title={t(($) => $.workspace.profile.title)}>
          <BillingProfileForm
            profile={billingProfile}
            canEdit={canEditProfile}
            saving={updateBillingProfileMutation.isPending}
            error={profileError}
            onSave={(data) => void handleSaveBillingProfile(data)}
          />
        </SettingsSection>
      ) : null}

      {summaryQuery.data?.availableActions.portal === true ? (
        <SettingsSection
          title={t(($) => $.workspace.management.title)}
          description={t(($) => $.workspace.management.description)}
        >
          <SettingsCard>
            <SettingsRow
              label={t(($) => $.workspace.management.portal)}
              description={
                portalUnavailable
                  ? t(($) => $.workspace.management.portal_unavailable)
                  : undefined
              }
            >
              {!portalUnavailable ? (
                <Button
                  className="h-11 w-full sm:w-auto"
                  disabled={isMutating}
                  onClick={() => void handlePortal()}
                >
                  {portalMutation.isPending ? (
                    <Loader2 className="animate-spin motion-reduce:animate-none" />
                  ) : (
                    <ExternalLink />
                  )}
                  {t(($) => $.workspace.actions.manage)}
                </Button>
              ) : null}
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      <SettingsSection
        title={t(($) => $.workspace.limits.title)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.workspace.limits.issues)}
          >
            <span className="tabular-nums">{issueLimitValue}</span>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.workspace.limits.autopilots)}
          >
            {quotaUsage.kind === "unlimited" ? (
              <span className="tabular-nums">
                {t(($) => $.workspace.limits.unlimited)}
              </span>
            ) : quotaUsageQuery.isPending ? (
              <div
                className="w-full max-w-72 space-y-2 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
                role="status"
                aria-label={t(($) => $.workspace.limits.usage_loading)}
              >
                <Skeleton className="h-5 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            ) : quotaUsage.kind === "metered" ? (
              <div className="w-full max-w-72 space-y-2">
                <Progress
                  value={quotaUsage.progress}
                  aria-label={t(($) => $.workspace.limits.usage_label)}
                >
                  <ProgressLabel>
                    {quotaUsage.reached
                      ? t(($) => $.workspace.limits.reached)
                      : t(($) => $.workspace.limits.current_usage)}
                  </ProgressLabel>
                  <ProgressValue>
                    {() =>
                      t(($) => $.workspace.limits.usage_total, {
                        total: numberFormatter.format(quotaUsage.total),
                        limit: numberFormatter.format(quotaUsage.limit),
                      })
                    }
                  </ProgressValue>
                </Progress>
                <p className="text-caption text-muted-foreground tabular-nums">
                  {t(($) => $.workspace.limits.usage_breakdown, {
                    used: numberFormatter.format(quotaUsage.used),
                    reserved: numberFormatter.format(quotaUsage.reserved),
                  })}
                </p>
                {quotaResetAt ? (
                  <p className="text-caption text-muted-foreground tabular-nums">
                    {t(($) => $.workspace.limits.resets_at, {
                      date: quotaResetAt,
                    })}
                  </p>
                ) : null}
              </div>
            ) : (
              <div className="flex flex-col gap-2 sm:items-end">
                {entitlements.limits.autopilotRuns.mode === "limited" &&
                entitlements.limits.autopilotRuns.limit !== null ? (
                  <span className="tabular-nums">
                    {t(($) => $.workspace.limits.per_month, {
                      count: entitlements.limits.autopilotRuns.limit,
                    })}
                  </span>
                ) : null}
                <span className="text-caption text-muted-foreground">
                  {t(($) => $.workspace.limits.usage_unavailable)}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  aria-label={t(($) => $.workspace.actions.retry_autopilots)}
                  aria-busy={quotaUsageQuery.isFetching}
                  disabled={quotaUsageQuery.isFetching}
                  onClick={() => void quotaUsageQuery.refetch()}
                >
                  {quotaUsageQuery.isFetching ? (
                    <Loader2 className="animate-spin motion-reduce:animate-none" />
                  ) : (
                    <RefreshCw />
                  )}
                  {t(($) => $.workspace.actions.retry)}
                </Button>
              </div>
            )}
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {showQuotaRequestsSection ? (
        <SettingsSection
          title={tSettings(($) => $.quota_requests.title)}
          action={
            hasPlanLimitReached && canManageBilling ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleOpenQuotaRequestDialog}
              >
                <ArrowUpCircle />
                {tSettings(($) => $.quota_requests.request_button)}
              </Button>
            ) : undefined
          }
        >
          {quotaRequests.length > 0 ? (
            <SettingsCard>
              {quotaRequests.map((request) => {
                const createdDate = formatDate(request.created_at, locale);
                const detailText = request.decision_reason || request.reason;
                return (
                  <SettingsRow
                    key={request.id || `${request.gate}-${request.created_at}`}
                    label={
                      <div className="flex flex-wrap items-center gap-2">
                        <span>{quotaGateLabel(request.gate, tSettings)}</span>
                        <Badge variant={quotaStateVariant(request.state)}>
                          {quotaStateLabel(request.state, tSettings)}
                        </Badge>
                      </div>
                    }
                    description={
                      createdDate || detailText ? (
                        <span className="tabular-nums">
                          {[createdDate, detailText].filter(Boolean).join(" · ")}
                        </span>
                      ) : undefined
                    }
                  >
                    <div className="flex flex-col items-end gap-0.5 text-body tabular-nums">
                      <span>
                        {tSettings(($) => $.quota_requests.requested_value, {
                          value: numberFormatter.format(request.requested_value),
                        })}
                      </span>
                      {request.approved_value !== null ? (
                        <span className="text-caption text-muted-foreground">
                          {tSettings(($) => $.quota_requests.approved_value, {
                            value: numberFormatter.format(request.approved_value),
                          })}
                        </span>
                      ) : null}
                    </div>
                  </SettingsRow>
                );
              })}
            </SettingsCard>
          ) : quotaRequestsQuery.isError ? (
            <SettingsCard>
              <div className="px-4 py-3 text-body text-muted-foreground">
                {tSettings(($) => $.quota_requests.load_failed)}
              </div>
            </SettingsCard>
          ) : (
            <SettingsCard>
              <div className="px-4 py-3 text-body text-muted-foreground">
                {tSettings(($) => $.quota_requests.empty)}
              </div>
            </SettingsCard>
          )}
        </SettingsSection>
      ) : null}

      {hasSeatCapacity || summaryUnavailable ? (
        <SettingsSection
          title={t(($) => $.workspace.seats.title)}
          description={t(($) => $.workspace.seats.description)}
        >
          {summaryUnavailable ? (
            <Alert className="mb-3">
              <AlertCircle />
              <AlertTitle>
                {t(($) => $.workspace.seats.summary_unavailable_title)}
              </AlertTitle>
              <AlertDescription>
                <p>
                  {t(($) => $.workspace.seats.summary_unavailable_description)}
                </p>
                <Button
                  className="mt-3"
                  type="button"
                  variant="outline"
                  size="sm"
                  aria-busy={summaryQuery.isFetching}
                  disabled={summaryQuery.isFetching}
                  onClick={() => void summaryQuery.refetch()}
                >
                  {summaryQuery.isFetching ? (
                    <Loader2 className="animate-spin motion-reduce:animate-none" />
                  ) : (
                    <RefreshCw />
                  )}
                  {t(($) => $.workspace.actions.retry)}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {seatPurchaseMessage ? (
            <Alert className="mb-3">
              <CheckCircle2 />
              <AlertTitle>{t(($) => $.workspace.seats.updated)}</AlertTitle>
              <AlertDescription>{seatPurchaseMessage}</AlertDescription>
            </Alert>
          ) : null}
          {membersExceedPurchasedSeats ? (
            <Alert className="mb-3">
              <AlertCircle />
              <AlertTitle>
                {t(($) => $.workspace.seats.members_over_capacity_title)}
              </AlertTitle>
              <AlertDescription>
                <p>
                  {t(($) => $.workspace.seats.members_over_capacity_description, {
                    actual: actualSeats,
                    purchased: billedSeats,
                  })}
                </p>
                {canAddSeats ? (
                  <Button
                    className="mt-3"
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={isMutating}
                    onClick={() => handleSeatPurchaseOpenChange(true)}
                  >
                    <Plus />
                    {t(($) => $.workspace.actions.add_seats)}
                  </Button>
                ) : null}
              </AlertDescription>
            </Alert>
          ) : null}
          {activeSeatPurchase ? (
            <Alert className="mb-3">
              {seatPurchasePollingTimedOut ? (
                <AlertCircle />
              ) : (
                <Loader2 className="animate-spin motion-reduce:animate-none" />
              )}
              <AlertTitle>
                {seatPurchasePollingTimedOut
                  ? t(($) => $.workspace.seat_purchase.delayed_title)
                  : t(($) => $.workspace.seat_purchase.pending_title)}
              </AlertTitle>
              <AlertDescription>
                {seatPurchasePollingTimedOut
                  ? activeSeatPurchaseExpiry
                    ? t(
                        ($) =>
                          $.workspace.seat_purchase
                            .delayed_description_with_expiry,
                        { date: activeSeatPurchaseExpiry },
                      )
                    : t(($) => $.workspace.seat_purchase.delayed_description)
                  : t(($) => $.workspace.seat_purchase.pending_description, {
                      count: activeSeatPurchase.targetSeats,
                    })}
              </AlertDescription>
            </Alert>
          ) : null}
          <SettingsCard>
            <SettingsRow
              label={t(($) => $.workspace.seats.human_members)}
            >
              <span className="tabular-nums">
                {t(($) => $.workspace.seats.seat_count, {
                  count: usedSeats,
                })}
              </span>
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.workspace.seats.billed)}
            >
              {summaryQuery.isPending ? (
                <Skeleton
                  className="h-5 w-20 motion-reduce:animate-none"
                  aria-label={t(($) => $.workspace.seats.summary_loading)}
                />
              ) : summaryUnavailable ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.unavailable)}
                </span>
              ) : billedSeats === null ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.not_subscribed)}
                </span>
              ) : (
                <div className="flex flex-col gap-2 sm:items-end">
                  <span className="tabular-nums">
                    {t(($) => $.workspace.seats.seat_count, {
                      count: billedSeats,
                    })}
                  </span>
                  {canAddSeats ? (
                    <Button
                      className="h-11 w-full sm:w-auto"
                      type="button"
                      disabled={isMutating}
                      onClick={() => handleSeatPurchaseOpenChange(true)}
                    >
                      <Plus />
                      {t(($) => $.workspace.actions.add_seats)}
                    </Button>
                  ) : null}
                </div>
              )}
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.workspace.seats.pending_invitations)}
              description={t(
                ($) => $.workspace.seats.pending_invitations_description,
              )}
            >
              {summaryQuery.isPending ? (
                <Skeleton
                  className="h-5 w-20 motion-reduce:animate-none"
                  aria-label={t(($) => $.workspace.seats.summary_loading)}
                />
              ) : summaryUnavailable ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.unavailable)}
                </span>
              ) : (
                <span className="tabular-nums">
                  {t(($) => $.workspace.seats.seat_count, {
                    count: reservedSeats,
                  })}
                </span>
              )}
            </SettingsRow>
            <SettingsRow
              label={
                <div className="flex items-center gap-1.5">
                  <span>{t(($) => $.workspace.seats.available)}</span>
                  <Popover>
                    <PopoverTrigger
                      render={
                        <IconButton
                          variant="ghost"
                          size="icon-xs"
                          label={t(($) => $.workspace.seats.available_help)}
                        >
                          <Info className="size-3.5" />
                        </IconButton>
                      }
                    />
                    <PopoverContent align="start">
                      <PopoverDescription>
                        {t(($) => $.workspace.seats.available_description)}
                      </PopoverDescription>
                    </PopoverContent>
                  </Popover>
                </div>
              }
            >
              {summaryQuery.isPending ? (
                <Skeleton
                  className="h-5 w-20 motion-reduce:animate-none"
                  aria-label={t(($) => $.workspace.seats.summary_loading)}
                />
              ) : summaryUnavailable || availableSeats === null ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.unavailable)}
                </span>
              ) : (
                <span className="tabular-nums">
                  {t(($) => $.workspace.seats.seat_count, {
                    count: availableSeats,
                  })}
                </span>
              )}
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.workspace.seats.pending)}
              description={t(($) => $.workspace.seats.pending_description)}
            >
              {summaryQuery.isPending ? (
                <Skeleton
                  className="h-5 w-28 motion-reduce:animate-none"
                  aria-label={t(($) => $.workspace.seats.summary_loading)}
                />
              ) : summaryUnavailable ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.unavailable)}
                </span>
              ) : pendingSeatQuantity === null ? (
                <span className="text-muted-foreground">
                  {t(($) => $.workspace.seats.none_pending)}
                </span>
              ) : summaryPeriodEnd ? (
                <span className="tabular-nums">
                  {t(($) => $.workspace.seats.pending_with_date, {
                    count: pendingSeatQuantity,
                    date: summaryPeriodEnd,
                  })}
                </span>
              ) : (
                <span className="tabular-nums">
                  {t(($) => $.workspace.seats.seat_count, {
                    count: pendingSeatQuantity,
                  })}
                </span>
              )}
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {showInvoicesSection ? (
        <div ref={invoicesSectionRef}>
          <SettingsSection title={t(($) => $.workspace.invoices.title)}>
            {ordersQuery.isPending ? (
              <SettingsCard>
                <div
                  className="space-y-3 p-4 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
                  role="status"
                  aria-label={t(($) => $.workspace.invoices.loading)}
                >
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-4 w-full" />
                </div>
              </SettingsCard>
            ) : ordersQuery.isError || ordersQuery.data === null ? (
              <Alert variant="destructive">
                <AlertCircle />
                <AlertDescription>
                  <p>{t(($) => $.workspace.invoices.unavailable)}</p>
                  <Button
                    className="mt-2"
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => void ordersQuery.refetch()}
                  >
                    <RefreshCw />
                    {t(($) => $.workspace.actions.retry)}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : orders.length === 0 ? (
              <SettingsCard>
                <div className="px-4 py-4 text-body text-muted-foreground">
                  {t(($) => $.workspace.invoices.empty)}
                </div>
              </SettingsCard>
            ) : (
              <SettingsCard>
                {orders.map((order) => {
                  const isUsageOrder = order.kind === "usage";
                  const canExpand = isUsageOrder || order.status === "paid";
                  const isExpanded = Boolean(expandedOrderIds[order.id]);
                  const periodStart = formatUsageUtcDate(
                    order.periodStart,
                    locale,
                  );
                  const periodEnd = formatUsageUtcDate(
                    order.periodEnd,
                    locale,
                  );
                  const periodText =
                    periodStart && periodEnd
                      ? t(($) => $.workspace.usage.period, {
                          start: periodStart,
                          end: periodEnd,
                        })
                      : formatDate(order.createdAt, locale);

                  return (
                    <div key={order.id} className="divide-y divide-surface-border">
                      <SettingsRow
                        label={
                          <div className="flex flex-wrap items-center gap-2">
                            <span>{orderKindLabel(order.kind)}</span>
                            <Badge variant={orderStatusVariant(order.status)}>
                              {orderStatusLabel(order.status)}
                            </Badge>
                          </div>
                        }
                        description={
                          periodText ? (
                            <span className="tabular-nums">{periodText}</span>
                          ) : undefined
                        }
                      >
                        <div className="flex flex-wrap items-center justify-end gap-2">
                          <span className="font-medium tabular-nums">
                            {formatUsageMoney(
                              order.totalAmount,
                              order.currency,
                              locale,
                            )}
                          </span>
                          {canExpand ? (
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              aria-expanded={isExpanded}
                              onClick={() =>
                                setExpandedOrderIds((prev) => ({
                                  ...prev,
                                  [order.id]: !isExpanded,
                                }))
                              }
                            >
                              {isExpanded ? <ChevronDown /> : <ChevronRight />}
                              {isExpanded
                                ? t(($) => $.workspace.invoices.hide_lines)
                                : t(($) => $.workspace.invoices.view_lines)}
                            </Button>
                          ) : null}
                        </div>
                      </SettingsRow>
                      {isUsageOrder && isExpanded ? (
                        <div className="px-4 py-3">
                          <UsageInvoiceLines
                            wsId={wsId}
                            orderId={order.id}
                            currency={order.currency}
                          />
                        </div>
                      ) : null}
                      {order.status === "paid" && isExpanded ? (
                        <PaidOrderEinvoicePanel wsId={wsId} order={order} />
                      ) : null}
                    </div>
                  );
                })}
              </SettingsCard>
            )}
          </SettingsSection>
        </div>
      ) : null}

      <Dialog
        open={seatPurchaseOpen}
        onOpenChange={handleSeatPurchaseOpenChange}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t(($) => $.workspace.seat_purchase.title)}
            </DialogTitle>
            <DialogDescription>
              {t(($) => $.workspace.seat_purchase.description)}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label
                className="text-body font-medium"
                htmlFor="workspace-additional-seats"
              >
                {t(($) => $.workspace.seat_purchase.additional_label)}
              </label>
              <Input
                id="workspace-additional-seats"
                className="h-11"
                type="number"
                inputMode="numeric"
                min={1}
                step={1}
                value={additionalSeatsInput}
                disabled={purchaseSeatsMutation.isPending}
                onChange={(event) => {
                  seatPreviewCapacityRetryRef.current = null;
                  setSeatPreviewRefreshing(false);
                  setAdditionalSeatsInput(event.currentTarget.value);
                  setSeatPurchaseError(null);
                }}
              />
              <p className="text-caption text-muted-foreground">
                {t(($) => $.workspace.seat_purchase.additional_hint)}
              </p>
            </div>
            {previewSeatPurchaseMutation.isPending ||
            seatPreviewRefreshing ? (
              <div
                className="flex items-center gap-2 text-body text-muted-foreground"
                role="status"
              >
                <Loader2 className="animate-spin motion-reduce:animate-none" />
                {t(($) => $.workspace.seat_purchase.preview_loading)}
              </div>
            ) : null}
            {seatPurchaseError ? (
              <Alert variant="destructive">
                <AlertCircle />
                <AlertTitle>
                  {t(($) => $.workspace.seat_purchase.error_title)}
                </AlertTitle>
                <AlertDescription>{seatPurchaseError}</AlertDescription>
              </Alert>
            ) : null}
            {seatPreview &&
            formattedSeatProration !== null &&
            formattedNextSeatInvoice !== null ? (
              <div className="space-y-3 rounded-lg border border-surface-border p-4">
                <div className="flex items-center justify-between gap-4 text-body">
                  <span className="text-muted-foreground">
                    {t(($) => $.workspace.seat_purchase.seats_after)}
                  </span>
                  <span className="font-medium tabular-nums">
                    {t(($) => $.workspace.seats.seat_count, {
                      count: seatPreview.resultingSeats,
                    })}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4 text-body">
                  <span className="text-muted-foreground">
                    {t(($) => $.workspace.seat_purchase.charge_today)}
                  </span>
                  <span className="font-medium tabular-nums">
                    {formattedSeatProration}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4 text-body">
                  <span className="text-muted-foreground">
                    {t(($) => $.workspace.seat_purchase.next_invoice)}
                  </span>
                  <span className="font-medium tabular-nums">
                    {formattedNextSeatInvoice}
                  </span>
                </div>
                <p className="text-caption leading-5 text-muted-foreground">
                  {t(($) => $.workspace.seat_purchase.tax_notice)}
                </p>
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              className="h-11"
              disabled={purchaseSeatsMutation.isPending}
              onClick={() => handleSeatPurchaseOpenChange(false)}
            >
              {t(($) => $.workspace.actions.cancel)}
            </Button>
            <Button
              type="button"
              className="h-11"
              disabled={
                !seatPreview ||
                formattedSeatProration === null ||
                formattedNextSeatInvoice === null ||
                previewSeatPurchaseMutation.isPending ||
                seatPreviewRefreshing ||
                purchaseSeatsMutation.isPending
              }
              onClick={() => void handleSeatPurchase()}
            >
              {purchaseSeatsMutation.isPending ? (
                <Loader2 className="animate-spin motion-reduce:animate-none" />
              ) : (
                <Plus />
              )}
              {seatPreview
                ? t(($) => $.workspace.seat_purchase.confirm, {
                    count: seatPreview.additionalSeats,
                  })
                : t(($) => $.workspace.actions.add_seats)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={checkoutConfirmOpen}
        onOpenChange={handleCheckoutConfirmOpenChange}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.workspace.confirm.title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.workspace.confirm.description, {
                interval:
                  interval === "month"
                    ? t(($) => $.workspace.upgrade.monthly)
                    : t(($) => $.workspace.upgrade.yearly),
                count: actualSeats,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              className="h-11"
              disabled={checkoutMutation.isPending}
            >
              {t(($) => $.workspace.actions.cancel)}
            </AlertDialogCancel>
            <AlertDialogAction
              className="h-11"
              disabled={checkoutMutation.isPending}
              onClick={() => void handleCheckout()}
            >
              {checkoutMutation.isPending ? (
                <Loader2 className="animate-spin motion-reduce:animate-none" />
              ) : (
                <ExternalLink />
              )}
              {isManualProvider
                ? t(($) => $.workspace.actions.create_order)
                : t(($) => $.workspace.actions.continue_to_stripe)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <PlanChangeDialog
        open={planChangeOpen}
        onOpenChange={handlePlanChangeOpenChange}
        planName={planChangePlanName}
        preview={planChangePreview}
        submitting={
          createPlanChangeMutation.isPending ||
          previewPlanChangeMutation.isPending
        }
        error={planChangeError}
        onConfirm={() => void handleConfirmPlanChange()}
      />

      <Dialog
        open={quotaDialogOpen}
        onOpenChange={handleQuotaDialogOpenChange}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {tSettings(($) => $.quota_requests.dialog.title)}
            </DialogTitle>
            <DialogDescription>
              {tSettings(($) => $.quota_requests.dialog.description)}
            </DialogDescription>
          </DialogHeader>
          <form
            noValidate
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              void handleSubmitQuotaRequest();
            }}
          >
            {quotaSubmitError ? (
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertDescription>{quotaSubmitError}</AlertDescription>
              </Alert>
            ) : null}
            <div className="space-y-2">
              <Label htmlFor="quota-request-gate">
                {tSettings(($) => $.quota_requests.dialog.gate_label)}
              </Label>
              <Select
                items={quotaGateSelectItems}
                value={quotaGate}
                disabled={quotaSubmitting}
                onValueChange={(value) => {
                  if (value) {
                    setQuotaGate(value);
                    setQuotaSubmitError(null);
                  }
                }}
              >
                <SelectTrigger
                  id="quota-request-gate"
                  aria-label={tSettings(($) => $.quota_requests.dialog.gate_label)}
                  className="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {quotaGateSelectItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="quota-request-value">
                {tSettings(($) => $.quota_requests.dialog.value_label)}
              </Label>
              <Input
                id="quota-request-value"
                type="number"
                inputMode="numeric"
                min={knownCurrentGateLimit !== null ? knownCurrentGateLimit + 1 : 1}
                step={1}
                value={quotaValueInput}
                disabled={quotaSubmitting}
                placeholder={tSettings(
                  ($) => $.quota_requests.dialog.value_placeholder,
                )}
                onChange={(event) => {
                  setQuotaValueInput(event.currentTarget.value);
                  setQuotaSubmitError(null);
                }}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="quota-request-reason">
                {tSettings(($) => $.quota_requests.dialog.reason_label)}
              </Label>
              <Textarea
                id="quota-request-reason"
                rows={3}
                maxLength={QUOTA_REQUEST_MAX_REASON_LENGTH}
                value={quotaReasonInput}
                disabled={quotaSubmitting}
                placeholder={tSettings(
                  ($) => $.quota_requests.dialog.reason_placeholder,
                )}
                onChange={(event) => {
                  setQuotaReasonInput(event.currentTarget.value);
                  setQuotaSubmitError(null);
                }}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                className="h-11"
                disabled={quotaSubmitting}
                onClick={() => handleQuotaDialogOpenChange(false)}
              >
                {tSettings(($) => $.quota_requests.dialog.cancel)}
              </Button>
              <Button
                type="submit"
                className="h-11"
                disabled={quotaSubmitting}
                aria-busy={quotaSubmitting || undefined}
              >
                {quotaSubmitting ? (
                  <Loader2 className="animate-spin motion-reduce:animate-none" />
                ) : null}
                {quotaSubmitting
                  ? tSettings(($) => $.quota_requests.dialog.submitting)
                  : tSettings(($) => $.quota_requests.dialog.submit)}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </SettingsTab>
  );
}
