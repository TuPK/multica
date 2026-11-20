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
import { Field, FieldError, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  AlertCircle,
  CalendarClock,
  CheckCircle2,
  CircleOff,
  History,
  Layers,
  Pencil,
  Plus,
  Trash2,
} from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { RowActions, useRowActionsWidth, type RowAction } from "../components/RowActions";
import { StatusBadge } from "../components/StatusBadge";
import { PlanHistory } from "./PlanHistory";
import { PriceSchedule } from "./PriceSchedule";
import {
  usePlans,
  useCreatePlan,
  usePutPlan,
  useActivatePlan,
  useDeactivatePlan,
  useDeletePlan,
  useMe,
} from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type { StaffRole } from "../components/types";
import type { Limits, Plan, PlanAudience, PlanBillingModel, PlanState } from "../api/schemas";
import { formatMinorAmount } from "../money";
import { t } from "../i18n";

const HISTORY_PLAN_KEYS = ["pro", "business", "unlimited", "enterprise", "free"] as const;

const AUDIENCE_LABELS: Record<PlanAudience, string> = {
  any: "Any",
  individual: "Individual",
  business: "Business",
  unknown: "Unknown",
};

const BILLING_MODEL_LABELS: Record<PlanBillingModel, string> = {
  seat: "Seat",
  usage: "Usage",
  unknown: "Unknown",
};

const LIMIT_FIELDS: ReadonlyArray<{ key: string; label: string }> = [
  { key: "issue_count", label: "Issue limit" },
  { key: "autopilot_runs", label: "Autopilot runs limit" },
  { key: "agents", label: "Agents limit" },
  { key: "runtimes", label: "Runtimes limit" },
  { key: "storage_bytes", label: "Storage limit (bytes)" },
  { key: "api_rate_per_minute", label: "API rate limit (/min)" },
  { key: "audit_retention_days", label: "Audit retention (days)" },
  { key: "member_groups", label: "Member groups limit" },
  { key: "max_workspaces", label: "Max workspaces" },
];

interface LoweredLimitChange {
  key: string;
  label: string;
  previous: number | null;
  next: number;
}

function getPlanState(plan: Plan): PlanState {
  if (plan.state) return plan.state;
  return plan.archived_at ? "inactive" : "active";
}

function findLoweredLimits(previousLimits: Limits, nextLimits: Limits): LoweredLimitChange[] {
  const lowered: LoweredLimitChange[] = [];
  const allKeys = new Set([
    ...LIMIT_FIELDS.map((f) => f.key),
    ...Object.keys(previousLimits),
    ...Object.keys(nextLimits),
  ]);

  for (const key of allKeys) {
    const prevVal = previousLimits[key];
    const nextVal = nextLimits[key];
    if (typeof nextVal !== "number" || !Number.isFinite(nextVal)) continue;
    const isLowered =
      prevVal === null ||
      prevVal === undefined ||
      (typeof prevVal === "number" && Number.isFinite(prevVal) && nextVal < prevVal);
    if (isLowered) {
      const label = t(LIMIT_FIELDS.find((f) => f.key === key)?.label ?? key);
      lowered.push({
        key,
        label,
        previous: typeof prevVal === "number" ? prevVal : null,
        next: nextVal,
      });
    }
  }
  return lowered;
}

interface PlansProps {
  userRole?: StaffRole;
  onOpenHistory?: (planKey: string) => void;
}

export function Plans({ userRole, onOpenHistory }: PlansProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;
  const isSystemAdmin = effectiveRole === "system_admin";
  const isTenantAdmin = effectiveRole === "admin";
  const isBilling = effectiveRole === "billing";
  const canReadPlans = isSystemAdmin || isTenantAdmin;
  const canWritePlans = isSystemAdmin;
  const actionsWidth = useRowActionsWidth(204);

  const { data: plans, isLoading, isError, error, refetch } = usePlans(canReadPlans);
  const createPlan = useCreatePlan();
  const putPlan = usePutPlan();
  const activatePlan = useActivatePlan();
  const deactivatePlan = useDeactivatePlan();
  const deletePlan = useDeletePlan();

  // Billing read-only price history selection
  const [historyPlanKey, setHistoryPlanKey] = useState<string>("pro");
  const [customPlanKey, setCustomPlanKey] = useState<string>("");

  // Plan change audit history view state (fallback when standalone)
  const [planHistoryViewKey, setPlanHistoryViewKey] = useState<string | null>(null);

  // Price schedule view state (system_admin)
  const [schedulePlanKey, setSchedulePlanKey] = useState<string | null>(null);
  const [selectedSchedulePlan, setSelectedSchedulePlan] = useState<Plan | null>(null);

  // Inline lifecycle toggle state
  const [lifecyclePendingKey, setLifecyclePendingKey] = useState<string | null>(null);
  const [lifecycleError, setLifecycleError] = useState<string | null>(null);

  // Create dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newCurrency, setNewCurrency] = useState("vnd");
  const [newAudience, setNewAudience] = useState<Exclude<PlanAudience, "unknown">>("any");
  const [newBillingModel, setNewBillingModel] =
    useState<Exclude<PlanBillingModel, "unknown">>("seat");
  const [newSeatPriceMonth, setNewSeatPriceMonth] = useState<string>("");
  const [newSeatPriceYear, setNewSeatPriceYear] = useState<string>("");
  const [newMinSeats, setNewMinSeats] = useState<number | null>(1);
  const [newSeatsFixed, setNewSeatsFixed] = useState<number | null>(null);
  const [newTrialDays, setNewTrialDays] = useState(14);
  const [newSortOrder, setNewSortOrder] = useState(10);
  const [newIsPublic, setNewIsPublic] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  // Edit dialog state
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState<Plan | null>(null);
  const [planName, setPlanName] = useState("");
  const [planDescription, setPlanDescription] = useState("");
  const [draftLimits, setDraftLimits] = useState<Limits>({});
  const [currency, setCurrency] = useState("");
  const [trialDays, setTrialDays] = useState(0);
  const [minSeats, setMinSeats] = useState<number | null>(null);
  const [seatsFixed, setSeatsFixed] = useState<number | null>(null);
  const [isPublic, setIsPublic] = useState(false);
  const [sortOrder, setSortOrder] = useState(0);
  const [archived, setArchived] = useState(false);
  const [confirmLowerLimits, setConfirmLowerLimits] = useState(false);
  const [affectedSubscriptions, setAffectedSubscriptions] = useState<number | null>(null);
  const [currencyError, setCurrencyError] = useState<string | null>(null);
  const [editError, setEditError] = useState<string | null>(null);

  // Delete dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [planToDelete, setPlanToDelete] = useState<Plan | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  if (effectiveRole && !canReadPlans && !isBilling) {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  if (isBilling) {
    const handleCustomSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      const trimmed = customPlanKey.trim();
      if (trimmed) setHistoryPlanKey(trimmed);
    };

    return (
      <div className="space-y-6">
        <PageHeader
          title={t("Price Change History")}
          description={t("Read-only price change history per plan.")}
        />

        <div className="flex flex-wrap items-center gap-2">
          {HISTORY_PLAN_KEYS.map((key) => (
            <Button
              key={key}
              type="button"
              size="sm"
              variant={historyPlanKey === key ? "default" : "outline"}
              onClick={() => setHistoryPlanKey(key)}
            >
              {key}
            </Button>
          ))}
          <form onSubmit={handleCustomSubmit} className="flex items-center gap-2">
            <Input
              aria-label={t("Custom plan key")}
              value={customPlanKey}
              onChange={(e) => setCustomPlanKey(e.target.value)}
              placeholder={t("Other plan key...")}
              className="h-7 w-44"
            />
            <Button type="submit" size="sm" variant="outline" disabled={!customPlanKey.trim()}>
              {t("Load")}
            </Button>
          </form>
        </div>

        <PriceSchedule planKey={historyPlanKey} readOnly />
      </div>
    );
  }

  if (planHistoryViewKey) {
    return (
      <PlanHistory
        planKey={planHistoryViewKey}
        onBack={() => setPlanHistoryViewKey(null)}
        userRole={effectiveRole}
      />
    );
  }

  if (schedulePlanKey) {
    const currentSchedulePlan =
      plans?.find((p) => p.key === schedulePlanKey) ?? selectedSchedulePlan ?? undefined;
    return (
      <PriceSchedule
        plan={currentSchedulePlan}
        planKey={schedulePlanKey}
        onBack={() => {
          setSchedulePlanKey(null);
          setSelectedSchedulePlan(null);
        }}
      />
    );
  }

  const loweredLimits = selectedPlan
    ? findLoweredLimits(selectedPlan.limits, draftLimits)
    : [];

  const handleOpenCreate = () => {
    setNewKey("");
    setNewName("");
    setNewDescription("");
    setNewCurrency("vnd");
    setNewAudience("any");
    setNewBillingModel("seat");
    setNewSeatPriceMonth("");
    setNewSeatPriceYear("");
    setNewMinSeats(1);
    setNewSeatsFixed(null);
    setNewTrialDays(14);
    setNewSortOrder(10);
    setNewIsPublic(false);
    setCreateError(null);
    setCreateDialogOpen(true);
  };

  const handleCreatePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canWritePlans || createPlan.isPending) return;
    setCreateError(null);
    try {
      await createPlan.mutateAsync({
        key: newKey.trim(),
        name: newName.trim(),
        description: newDescription.trim(),
        limits: {},
        features: {},
        currency: newCurrency.trim(),
        seat_price_month:
          newSeatPriceMonth.trim() === "" ? null : Number(newSeatPriceMonth),
        seat_price_year:
          newSeatPriceYear.trim() === "" ? null : Number(newSeatPriceYear),
        seats_fixed:
          newSeatsFixed !== null && !isNaN(newSeatsFixed) ? Number(newSeatsFixed) : null,
        min_seats:
          newMinSeats !== null && !isNaN(newMinSeats) ? Number(newMinSeats) : null,
        trial_days: Number(newTrialDays),
        is_public: newIsPublic,
        sort: Number(newSortOrder),
        audience: newAudience,
        billing_model: newBillingModel,
      });
      setCreateDialogOpen(false);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError || err instanceof Error) {
        setCreateError(err.message);
      } else {
        setCreateError(t("Failed to create plan"));
      }
    }
  };

  const handleOpenEdit = (plan: Plan) => {
    setSelectedPlan(plan);
    setPlanName(plan.name);
    setPlanDescription(plan.description ?? "");
    setDraftLimits({ ...plan.limits });
    setCurrency(plan.currency);
    setTrialDays(plan.trial_days);
    setMinSeats(plan.min_seats);
    setSeatsFixed(plan.seats_fixed);
    setIsPublic(plan.is_public);
    setSortOrder(plan.sort);
    setArchived(Boolean(plan.archived_at));
    setConfirmLowerLimits(false);
    setAffectedSubscriptions(null);
    setCurrencyError(null);
    setEditError(null);
    setEditDialogOpen(true);
  };

  const submitSavePlan = async () => {
    if (!selectedPlan || putPlan.isPending) return;
    setCurrencyError(null);
    setEditError(null);

    try {
      await putPlan.mutateAsync({
        key: selectedPlan.key,
        name: planName.trim(),
        description: planDescription.trim(),
        ...(selectedPlan.version !== undefined
          ? { expected_version: selectedPlan.version }
          : {}),
        trial_days: Number(trialDays),
        limits: draftLimits,
        features: selectedPlan.features,
        currency: currency.trim(),
        seats_fixed: seatsFixed !== null && !isNaN(seatsFixed) ? Number(seatsFixed) : null,
        min_seats: minSeats !== null && !isNaN(minSeats) ? Number(minSeats) : null,
        is_public: isPublic,
        sort: Number(sortOrder),
        archived,
      });
      setEditDialogOpen(false);
      setSelectedPlan(null);
      setConfirmLowerLimits(false);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError && err.code === "plan_currency_locked") {
        setCurrencyError(
          t("The currency can't change because this plan has subscriptions or orders."),
        );
      } else if (
        err instanceof PlatformApiError &&
        err.code === "plan_limit_change_confirm_required"
      ) {
        const affected =
          typeof err.details?.affected_subscriptions === "number"
            ? err.details.affected_subscriptions
            : null;
        setAffectedSubscriptions(affected);
        setConfirmLowerLimits(true);
      } else if (err instanceof Error) {
        setEditError(err.message);
      } else {
        setEditError(t("Failed to update plan"));
      }
    }
  };

  const handleSavePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlan || putPlan.isPending) return;
    if (loweredLimits.length > 0 && !confirmLowerLimits) {
      setConfirmLowerLimits(true);
      return;
    }
    await submitSavePlan();
  };

  const handleToggleActive = async (plan: Plan, activate: boolean) => {
    if (!canWritePlans || lifecyclePendingKey) return;
    setLifecycleError(null);
    setLifecyclePendingKey(plan.key);
    try {
      if (activate) {
        await activatePlan.mutateAsync({ key: plan.key });
      } else {
        await deactivatePlan.mutateAsync({ key: plan.key });
      }
    } catch (err: unknown) {
      if (err instanceof PlatformApiError || err instanceof Error) {
        setLifecycleError(err.message);
      } else {
        setLifecycleError(
          activate ? t("Failed to activate plan") : t("Failed to deactivate plan"),
        );
      }
    } finally {
      setLifecyclePendingKey(null);
    }
  };

  const handleViewHistory = (planKey: string) => {
    if (onOpenHistory) {
      onOpenHistory(planKey);
    } else {
      setPlanHistoryViewKey(planKey);
    }
  };

  const handleOpenDelete = (plan: Plan) => {
    setPlanToDelete(plan);
    setDeleteError(null);
    setDeleteDialogOpen(true);
  };

  const handleConfirmDelete = async () => {
    if (!planToDelete || deletePlan.isPending) return;
    setDeleteError(null);

    try {
      await deletePlan.mutateAsync({ key: planToDelete.key });
      setDeleteDialogOpen(false);
      setPlanToDelete(null);
    } catch (err: unknown) {
      if (err instanceof PlatformApiError && err.code === "plan_in_use") {
        setDeleteError(
          err.message ||
            t("Subscriptions use this plan; deactivate or archive it instead."),
        );
      } else if (err instanceof PlatformApiError || err instanceof Error) {
        setDeleteError(err.message);
      } else {
        setDeleteError(t("Failed to delete plan"));
      }
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Plans Catalogue")}
        actions={
          canWritePlans ? (
            <Button type="button" size="sm" onClick={handleOpenCreate}>
              <Plus className="size-3.5" aria-hidden="true" />
              {t("Create plan")}
            </Button>
          ) : undefined
        }
      />

      {lifecycleError && (
        <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          <AlertDescription className="text-caption">{lifecycleError}</AlertDescription>
        </Alert>
      )}

      {isLoading && <LoadingState message={t("Loading plans...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load plans")}
          message={error instanceof Error ? error.message : t("An error occurred while loading plans.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!plans || plans.length === 0) && (
        <EmptyState
          icon={<Layers className="size-4" />}
          title={t("No plans")}
          description={t("No plans configured in the catalogue.")}
        />
      )}

      {!isLoading && !isError && plans && plans.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
        <Table
          data={plans}
          idKey="key"
          density="compact"
          dividers="rows"
          hasHover
          columns={[
            {
              key: "name",
              header: t("Plan"),
              width: proportional(2, { minWidth: 140 }),
              renderCell: (plan) => (
                <>
                  <Text type="body" weight="medium" as="div" className="block">
                    {plan.name}
                  </Text>
                  <Text type="code" color="secondary" as="div" className="block">
                    {plan.key}
                  </Text>
                  {plan.description && (
                    <div className="max-w-64 truncate" title={plan.description}>
                      <Text
                        type="supporting"
                        color="secondary"
                        as="span"
                      >
                        {plan.description}
                      </Text>
                    </div>
                  )}
                </>
              ),
            },
            {
              key: "audience",
              header: t("Audience"),
              width: proportional(1, { minWidth: 90 }),
              renderCell: (plan) => {
                const audience = plan.audience ?? "any";
                return <Badge variant="neutral" label={t(AUDIENCE_LABELS[audience] ?? audience)} />;
              },
            },
            {
              key: "billing_model",
              header: t("Billing Model"),
              width: proportional(1, { minWidth: 96 }),
              renderCell: (plan) => {
                const billingModel = plan.billing_model ?? "seat";
                return (
                  <Badge variant="blue" label={t(BILLING_MODEL_LABELS[billingModel] ?? billingModel)} />
                );
              },
            },
            {
              key: "pricing",
              header: t("Pricing (Monthly / Yearly)"),
              width: proportional(2, { minWidth: 120 }),
              renderCell: (plan) => (
                <>
                  <Text type="body" weight="medium" hasTabularNumbers as="div" className="block">
                    {plan.seat_price_month !== null
                      ? `${formatMinorAmount(plan.seat_price_month, plan.currency)}/mo`
                      : t("Free / Fixed")}
                  </Text>
                  <Text type="supporting" hasTabularNumbers as="div" className="block">
                    {plan.seat_price_year !== null
                      ? `${formatMinorAmount(plan.seat_price_year, plan.currency)}/yr`
                      : "-"}
                  </Text>
                </>
              ),
            },
            {
              key: "seats",
              header: t("Seats Configuration"),
              width: proportional(1.5, { minWidth: 96 }),
              renderCell: (plan) => (
                <Text type="body" hasTabularNumbers>
                  {plan.seats_fixed !== null
                    ? t("{0} fixed seats", plan.seats_fixed)
                    : t("{0} min seats", plan.min_seats || 1)}
                </Text>
              ),
            },
            {
              key: "trial_days",
              header: t("Trial"),
              width: proportional(1, { minWidth: 64 }),
              renderCell: (plan) => (
                <Text type="body" hasTabularNumbers>
                  {plan.trial_days > 0 ? t("{0} days", plan.trial_days) : t("No trial")}
                </Text>
              ),
            },
            {
              key: "is_public",
              header: t("Visibility"),
              width: proportional(1, { minWidth: 84 }),
              renderCell: (plan) => (
                <Badge
                  variant={plan.is_public ? "teal" : "neutral"}
                  label={plan.is_public ? t("Public") : t("Private")}
                />
              ),
            },
            {
              key: "status",
              header: t("Status"),
              width: proportional(1, { minWidth: 76 }),
              renderCell: (plan) => (
                <StatusBadge status={getPlanState(plan)} />
              ),
            },
            ...(canWritePlans
              ? [
                  {
                    key: "actions",
                    header: t("Actions"),
                    width: pixel(actionsWidth),
                    align: "end" as const,
                    renderCell: (plan: Plan) => {
                      const rowBusy = lifecyclePendingKey === plan.key;
                      const lifecycle: RowAction =
                        getPlanState(plan) === "inactive"
                          ? { label: t("Activate"), icon: CheckCircle2, busy: rowBusy, onSelect: () => handleToggleActive(plan, true) }
                          : { label: t("Deactivate"), icon: CircleOff, busy: rowBusy, onSelect: () => handleToggleActive(plan, false) };
                      return (
                        <RowActions
                          label={t("plan {0}", plan.name)}
                          actions={[
                            lifecycle,
                            { label: t("History"), icon: History, onSelect: () => handleViewHistory(plan.key) },
                            {
                              label: t("Price schedule"),
                              icon: CalendarClock,
                              onSelect: () => {
                                setSelectedSchedulePlan(plan);
                                setSchedulePlanKey(plan.key);
                              },
                            },
                            { label: t("Edit"), icon: Pencil, onSelect: () => handleOpenEdit(plan) },
                            { label: t("Delete"), icon: Trash2, destructive: true, onSelect: () => handleOpenDelete(plan) },
                          ]}
                        />
                      );
                    },
                  },
                ]
              : []),
          ]}
        />
        </Card>
      )}

      {/* Create Plan Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <form onSubmit={handleCreatePlan} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>{t("Create plan")}</DialogTitle>
              <DialogDescription>
                {t("Define a new subscription plan and its initial pricing.")}
              </DialogDescription>
            </DialogHeader>

            <div className="flex max-h-[60vh] flex-col gap-3 overflow-y-auto pr-1">
              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="create-plan-key" className="text-label">
                    {t("Plan Key")} <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="create-plan-key"
                    value={newKey}
                    onChange={(e) => setNewKey(e.target.value)}
                    placeholder={t("e.g. team_pro")}
                    required
                    disabled={createPlan.isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-plan-name" className="text-label">
                    {t("Plan Name")} <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="create-plan-name"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    required
                    disabled={createPlan.isPending}
                  />
                </Field>
              </div>

              <Field>
                <FieldLabel htmlFor="create-plan-description" className="text-label">
                  {t("Description")}
                </FieldLabel>
                <Textarea
                  id="create-plan-description"
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                  placeholder={t("Optional summary shown in catalogue and plan settings")}
                  disabled={createPlan.isPending}
                />
              </Field>

              <div className="grid grid-cols-3 gap-3">
                <Field>
                  <FieldLabel htmlFor="create-plan-currency" className="text-label">
                    {t("Currency")} <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="create-plan-currency"
                    value={newCurrency}
                    onChange={(e) => setNewCurrency(e.target.value)}
                    required
                    disabled={createPlan.isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-plan-audience" className="text-label">
                    {t("Audience")}
                  </FieldLabel>
                  <Select
                    items={[
                      { value: "any", label: t("Any") },
                      { value: "individual", label: t("Individual") },
                      { value: "business", label: t("Business") },
                    ]}
                    value={newAudience}
                    onValueChange={(v) =>
                      setNewAudience((v ?? "any") as Exclude<PlanAudience, "unknown">)
                    }
                    disabled={createPlan.isPending}
                  >
                    <SelectTrigger id="create-plan-audience">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="any">{t("Any")}</SelectItem>
                      <SelectItem value="individual">{t("Individual")}</SelectItem>
                      <SelectItem value="business">{t("Business")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-plan-billing" className="text-label">
                    {t("Billing model")}
                  </FieldLabel>
                  <Select
                    items={[
                      { value: "seat", label: t("Seat") },
                      { value: "usage", label: t("Usage") },
                    ]}
                    value={newBillingModel}
                    onValueChange={(v) =>
                      setNewBillingModel((v ?? "seat") as Exclude<PlanBillingModel, "unknown">)
                    }
                    disabled={createPlan.isPending}
                  >
                    <SelectTrigger id="create-plan-billing">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="seat">{t("Seat")}</SelectItem>
                      <SelectItem value="usage">{t("Usage")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="create-price-month" className="text-label">
                    {t("Monthly seat price (minor)")}
                  </FieldLabel>
                  <Input
                    id="create-price-month"
                    type="number"
                    min={0}
                    value={newSeatPriceMonth}
                    onChange={(e) => setNewSeatPriceMonth(e.target.value)}
                    placeholder={t("Blank if free / N/A")}
                    disabled={createPlan.isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-price-year" className="text-label">
                    {t("Yearly seat price (minor)")}
                  </FieldLabel>
                  <Input
                    id="create-price-year"
                    type="number"
                    min={0}
                    value={newSeatPriceYear}
                    onChange={(e) => setNewSeatPriceYear(e.target.value)}
                    placeholder={t("Blank if free / N/A")}
                    disabled={createPlan.isPending}
                  />
                </Field>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="create-trial-days" className="text-label">
                    {t("Trial Days")}
                  </FieldLabel>
                  <Input
                    id="create-trial-days"
                    type="number"
                    min={0}
                    value={newTrialDays}
                    onChange={(e) => setNewTrialDays(Number(e.target.value))}
                    disabled={createPlan.isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-sort-order" className="text-label">
                    {t("Sort Order")}
                  </FieldLabel>
                  <Input
                    id="create-sort-order"
                    type="number"
                    value={newSortOrder}
                    onChange={(e) => setNewSortOrder(Number(e.target.value))}
                    disabled={createPlan.isPending}
                  />
                </Field>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="create-min-seats" className="text-label">
                    {t("Min Seats (Optional)")}
                  </FieldLabel>
                  <Input
                    id="create-min-seats"
                    type="number"
                    min={1}
                    value={newMinSeats ?? ""}
                    onChange={(e) =>
                      setNewMinSeats(e.target.value === "" ? null : Number(e.target.value))
                    }
                    disabled={createPlan.isPending}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="create-fixed-seats" className="text-label">
                    {t("Fixed Seats (Optional)")}
                  </FieldLabel>
                  <Input
                    id="create-fixed-seats"
                    type="number"
                    min={1}
                    value={newSeatsFixed ?? ""}
                    onChange={(e) =>
                      setNewSeatsFixed(e.target.value === "" ? null : Number(e.target.value))
                    }
                    disabled={createPlan.isPending}
                  />
                </Field>
              </div>

              <Field orientation="horizontal">
                <Checkbox
                  id="create-plan-public"
                  checked={newIsPublic}
                  onCheckedChange={(checked) => setNewIsPublic(Boolean(checked))}
                  disabled={createPlan.isPending}
                />
                <FieldLabel htmlFor="create-plan-public" className="text-body font-normal">
                  {t("Public plan")}
                </FieldLabel>
              </Field>
            </div>

            {createError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{createError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={createPlan.isPending} />}
              >
                {t("Cancel")}
              </DialogClose>
              <Button
                type="submit"
                disabled={!newKey.trim() || !newName.trim() || !newCurrency.trim() || createPlan.isPending}
                aria-busy={createPlan.isPending || undefined}
              >
                {createPlan.isPending && <Spinner className="size-3.5" />}
                {t("Create plan")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Plan Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent>
          <form onSubmit={handleSavePlan} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>{t("Edit Plan:")} {selectedPlan?.name}</DialogTitle>
              <DialogDescription>
                {t("Update non-price metadata and seat configuration for this plan.")}
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <Field>
                <FieldLabel htmlFor="plan-name-input" className="text-label">
                  {t("Plan Name")} <span className="text-destructive">*</span>
                </FieldLabel>
                <Input
                  id="plan-name-input"
                  value={planName}
                  onChange={(e) => setPlanName(e.target.value)}
                  required
                  disabled={putPlan.isPending}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="plan-description-input" className="text-label">
                  {t("Description")}
                </FieldLabel>
                <Textarea
                  id="plan-description-input"
                  value={planDescription}
                  onChange={(e) => setPlanDescription(e.target.value)}
                  placeholder={t("Optional summary shown in catalogue and plan settings")}
                  disabled={putPlan.isPending}
                />
              </Field>

              <Field data-invalid={Boolean(currencyError) || undefined}>
                <FieldLabel htmlFor="plan-currency-input" className="text-label">
                  {t("Currency")}
                </FieldLabel>
                <Input
                  id="plan-currency-input"
                  value={currency}
                  onChange={(e) => {
                    setCurrency(e.target.value);
                    setCurrencyError(null);
                  }}
                  placeholder={t("ISO 4217 code")}
                  aria-invalid={Boolean(currencyError)}
                  aria-describedby={currencyError ? "plan-currency-error" : undefined}
                  disabled={putPlan.isPending}
                />
                {currencyError && (
                  <FieldError id="plan-currency-error" role="alert" className="text-caption">
                    {currencyError}
                  </FieldError>
                )}
              </Field>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="trial-days-input" className="text-label">
                    {t("Trial Days")}
                  </FieldLabel>
                  <Input
                    id="trial-days-input"
                    type="number"
                    min={0}
                    value={trialDays}
                    onChange={(e) => setTrialDays(Number(e.target.value))}
                    disabled={putPlan.isPending}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="sort-order-input" className="text-label">
                    {t("Sort Order")}
                  </FieldLabel>
                  <Input
                    id="sort-order-input"
                    type="number"
                    value={sortOrder}
                    onChange={(e) => setSortOrder(Number(e.target.value))}
                    disabled={putPlan.isPending}
                  />
                </Field>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="min-seats-input" className="text-label">
                    {t("Min Seats (Optional)")}
                  </FieldLabel>
                  <Input
                    id="min-seats-input"
                    type="number"
                    min={1}
                    value={minSeats ?? ""}
                    onChange={(e) => setMinSeats(e.target.value === "" ? null : Number(e.target.value))}
                    placeholder={t("Leave blank if N/A")}
                    disabled={putPlan.isPending}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="seats-fixed-input" className="text-label">
                    {t("Fixed Seats (Optional)")}
                  </FieldLabel>
                  <Input
                    id="seats-fixed-input"
                    type="number"
                    min={1}
                    value={seatsFixed ?? ""}
                    onChange={(e) => setSeatsFixed(e.target.value === "" ? null : Number(e.target.value))}
                    placeholder={t("Leave blank if variable")}
                    disabled={putPlan.isPending}
                  />
                </Field>
              </div>

              <div className="flex flex-col gap-2.5 pt-1">
                <Field orientation="horizontal">
                  <Checkbox
                    id="plan-is-public"
                    checked={isPublic}
                    onCheckedChange={(checked) => setIsPublic(Boolean(checked))}
                    disabled={putPlan.isPending}
                  />
                  <FieldLabel htmlFor="plan-is-public" className="text-body font-normal">
                    {t("Public plan (visible in client pricing catalogue)")}
                  </FieldLabel>
                </Field>

                <Field orientation="horizontal">
                  <Checkbox
                    id="plan-archived"
                    checked={archived}
                    onCheckedChange={(checked) => setArchived(Boolean(checked))}
                    disabled={putPlan.isPending}
                  />
                  <FieldLabel htmlFor="plan-archived" className="text-body font-normal">
                    {t("Archived (existing subscribers retain, no new signups)")}
                  </FieldLabel>
                </Field>
              </div>

              <div className="space-y-2 border-t border-border pt-3">
                <Text type="supporting" weight="medium" as="div" className="block">
                  {t("Limits (blank for unlimited)")}
                </Text>
                <div className="grid max-h-48 grid-cols-2 gap-3 overflow-y-auto pr-1">
                  {LIMIT_FIELDS.map((field) => (
                    <Field key={field.key}>
                      <FieldLabel
                        htmlFor={`edit-limit-${field.key}`}
                        className="text-label"
                      >
                        {t(field.label)}
                      </FieldLabel>
                      <Input
                        id={`edit-limit-${field.key}`}
                        type="number"
                        min={0}
                        value={draftLimits[field.key] ?? ""}
                        onChange={(e) => {
                          const raw = e.target.value;
                          setConfirmLowerLimits(false);
                          setAffectedSubscriptions(null);
                          setDraftLimits((prev) => ({
                            ...prev,
                            [field.key]: raw === "" ? null : Number(raw),
                          }));
                        }}
                        placeholder={t("Unlimited")}
                        disabled={putPlan.isPending}
                      />
                    </Field>
                  ))}
                </div>
              </div>
            </div>

            {confirmLowerLimits && (loweredLimits.length > 0 || affectedSubscriptions !== null) && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption space-y-1">
                  <div className="font-medium">
                    {t("Lowered limits affect active subscribers")}
                    {affectedSubscriptions !== null
                      ? t(" ({0} subscribers)", affectedSubscriptions)
                      : ""}
                    :
                  </div>
                  {loweredLimits.length > 0 && (
                    <div>
                      {loweredLimits
                        .map(
                          (item) =>
                            `${t(item.label)}: ${item.previous === null ? t("Unlimited") : item.previous} -> ${item.next}`,
                        )
                        .join(", ")}
                    </div>
                  )}
                </AlertDescription>
              </Alert>
            )}

            {editError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{editError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={putPlan.isPending} />}
              >
                {t("Cancel")}
              </DialogClose>
              {confirmLowerLimits && (loweredLimits.length > 0 || affectedSubscriptions !== null) ? (
                <Button
                  type="button"
                  variant="destructive"
                  disabled={!planName.trim() || putPlan.isPending}
                  aria-busy={putPlan.isPending}
                  onClick={() => void submitSavePlan()}
                >
                  {putPlan.isPending && <Spinner className="size-3.5" />}
                  {t("Confirm limit reduction")}
                </Button>
              ) : (
                <Button
                  type="submit"
                  disabled={!planName.trim() || putPlan.isPending}
                  aria-busy={putPlan.isPending}
                >
                  {putPlan.isPending && <Spinner className="size-3.5" />}
                  {t("Save changes")}
                </Button>
              )}
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Plan Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <div className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>{t("Delete Plan:")} {planToDelete?.name}</DialogTitle>
              <DialogDescription>
                {t("Are you sure you want to delete plan \"")}{planToDelete?.key}{t("\"? A plan with any active or past workspace subscriptions cannot be deleted and must be archived instead.")}
              </DialogDescription>
            </DialogHeader>

            {deleteError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{deleteError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={deletePlan.isPending} />}
              >
                {t("Cancel")}
              </DialogClose>
              <Button
                variant="destructive"
                onClick={handleConfirmDelete}
                disabled={deletePlan.isPending}
                aria-busy={deletePlan.isPending}
              >
                {deletePlan.isPending && <Spinner className="size-3.5" />}
                {t("Confirm delete")}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
