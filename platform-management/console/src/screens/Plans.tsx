import { useState } from "react";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
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
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { AlertCircle, CalendarClock, Pencil, Trash2 } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { PriceSchedule } from "./PriceSchedule";
import { usePlans, usePutPlan, useDeletePlan, useMe } from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type { StaffRole } from "../components/types";
import type { Plan, PlanAudience, PlanBillingModel } from "../api/schemas";
import { formatMinorAmount } from "../money";

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

interface PlansProps {
  userRole?: StaffRole;
}

export function Plans({ userRole }: PlansProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;
  const isAdmin = effectiveRole === "admin";
  const isBilling = effectiveRole === "billing";

  const { data: plans, isLoading, isError, error, refetch } = usePlans(isAdmin);
  const putPlan = usePutPlan();
  const deletePlan = useDeletePlan();

  // Billing read-only history selection
  const [historyPlanKey, setHistoryPlanKey] = useState<string>("pro");
  const [customPlanKey, setCustomPlanKey] = useState<string>("");

  // Price schedule view state (admin)
  const [schedulePlanKey, setSchedulePlanKey] = useState<string | null>(null);
  const [selectedSchedulePlan, setSelectedSchedulePlan] = useState<Plan | null>(null);

  // Edit dialog state
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState<Plan | null>(null);
  const [planName, setPlanName] = useState("");
  const [currency, setCurrency] = useState("");
  const [trialDays, setTrialDays] = useState(0);
  const [minSeats, setMinSeats] = useState<number | null>(null);
  const [seatsFixed, setSeatsFixed] = useState<number | null>(null);
  const [isPublic, setIsPublic] = useState(false);
  const [sortOrder, setSortOrder] = useState(0);
  const [archived, setArchived] = useState(false);
  const [currencyError, setCurrencyError] = useState<string | null>(null);
  const [editError, setEditError] = useState<string | null>(null);

  // Delete dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [planToDelete, setPlanToDelete] = useState<Plan | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  if (effectiveRole && !isAdmin && !isBilling) {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
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
          title="Price Change History"
          description="Read-only price change history per plan."
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
              aria-label="Custom plan key"
              value={customPlanKey}
              onChange={(e) => setCustomPlanKey(e.target.value)}
              placeholder="Other plan key..."
              className="h-7 w-44"
            />
            <Button type="submit" size="sm" variant="outline" disabled={!customPlanKey.trim()}>
              Load
            </Button>
          </form>
        </div>

        <PriceSchedule planKey={historyPlanKey} readOnly />
      </div>
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

  const handleOpenEdit = (plan: Plan) => {
    setSelectedPlan(plan);
    setPlanName(plan.name);
    setCurrency(plan.currency);
    setTrialDays(plan.trial_days);
    setMinSeats(plan.min_seats);
    setSeatsFixed(plan.seats_fixed);
    setIsPublic(plan.is_public);
    setSortOrder(plan.sort);
    setArchived(Boolean(plan.archived_at));
    setCurrencyError(null);
    setEditError(null);
    setEditDialogOpen(true);
  };

  const handleSavePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlan || putPlan.isPending) return;
    setCurrencyError(null);
    setEditError(null);

    try {
      await putPlan.mutateAsync({
        key: selectedPlan.key,
        name: planName.trim(),
        trial_days: Number(trialDays),
        limits: selectedPlan.limits,
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
    } catch (err: unknown) {
      if (err instanceof PlatformApiError && err.code === "plan_currency_locked") {
        setCurrencyError(
          "The currency can't change because this plan has subscriptions or orders.",
        );
      } else if (err instanceof Error) {
        setEditError(err.message);
      } else {
        setEditError("Failed to update plan");
      }
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
      if (err instanceof PlatformApiError || err instanceof Error) {
        setDeleteError(err.message);
      } else {
        setDeleteError("Failed to delete plan");
      }
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader title="Plans Catalogue" />

      {isLoading && <LoadingState message="Loading plans..." />}

      {isError && (
        <ErrorState
          title="Failed to load plans"
          message={error instanceof Error ? error.message : "An error occurred while loading plans."}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!plans || plans.length === 0) && (
        <EmptyState
          title="No plans"
          description="No plans configured in the catalogue."
        />
      )}

      {!isLoading && !isError && plans && plans.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Plan</TableHead>
                <TableHead>Audience</TableHead>
                <TableHead>Billing Model</TableHead>
                <TableHead>Pricing (Monthly / Yearly)</TableHead>
                <TableHead>Seats Configuration</TableHead>
                <TableHead>Trial</TableHead>
                <TableHead>Visibility</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {plans.map((plan: Plan) => {
                const isArchived = Boolean(plan.archived_at);
                const audience = plan.audience ?? "any";
                const billingModel = plan.billing_model ?? "seat";

                return (
                  <TableRow key={plan.key}>
                    <TableCell>
                      <div className="font-medium text-foreground">{plan.name}</div>
                      <div className="font-mono text-caption text-muted-foreground">{plan.key}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {AUDIENCE_LABELS[audience] ?? audience}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {BILLING_MODEL_LABELS[billingModel] ?? billingModel}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="text-body font-medium tabular-nums">
                        {plan.seat_price_month !== null
                          ? `${formatMinorAmount(plan.seat_price_month, plan.currency)}/mo`
                          : "Free / Fixed"}
                      </div>
                      <div className="text-caption text-muted-foreground tabular-nums">
                        {plan.seat_price_year !== null
                          ? `${formatMinorAmount(plan.seat_price_year, plan.currency)}/yr`
                          : "-"}
                      </div>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      <div className="text-body">
                        {plan.seats_fixed !== null
                          ? `${plan.seats_fixed} fixed seats`
                          : `${plan.min_seats || 1} min seats`}
                      </div>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      <div className="text-body">
                        {plan.trial_days > 0 ? `${plan.trial_days} days` : "No trial"}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={plan.is_public ? "secondary" : "outline"}
                        className={plan.is_public ? undefined : "text-muted-foreground"}
                      >
                        {plan.is_public ? "Public" : "Private"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={isArchived ? "archived" : "active"} />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setSelectedSchedulePlan(plan);
                            setSchedulePlanKey(plan.key);
                          }}
                          aria-label={`Schedule price for ${plan.name}`}
                          className="gap-1.5"
                        >
                          <CalendarClock className="size-3.5" aria-hidden="true" />
                          <span>Price schedule</span>
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenEdit(plan)}
                          aria-label={`Edit plan ${plan.name}`}
                          className="gap-1.5"
                        >
                          <Pencil className="size-3.5" aria-hidden="true" />
                          <span>Edit</span>
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenDelete(plan)}
                          aria-label={`Delete plan ${plan.name}`}
                          className="gap-1.5 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        >
                          <Trash2 className="size-3.5" aria-hidden="true" />
                          <span>Delete</span>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Edit Plan Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent>
          <form onSubmit={handleSavePlan} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Edit Plan: {selectedPlan?.name}</DialogTitle>
              <DialogDescription>
                Update non-price metadata and seat configuration for this plan.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-3">
              <Field>
                <FieldLabel htmlFor="plan-name-input" className="text-label">
                  Plan Name <span className="text-destructive">*</span>
                </FieldLabel>
                <Input
                  id="plan-name-input"
                  value={planName}
                  onChange={(e) => setPlanName(e.target.value)}
                  required
                  disabled={putPlan.isPending}
                />
              </Field>

              <Field data-invalid={Boolean(currencyError) || undefined}>
                <FieldLabel htmlFor="plan-currency-input" className="text-label">
                  Currency
                </FieldLabel>
                <Input
                  id="plan-currency-input"
                  value={currency}
                  onChange={(e) => {
                    setCurrency(e.target.value);
                    setCurrencyError(null);
                  }}
                  placeholder="ISO 4217 code"
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
                    Trial Days
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
                    Sort Order
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
                    Min Seats (Optional)
                  </FieldLabel>
                  <Input
                    id="min-seats-input"
                    type="number"
                    min={1}
                    value={minSeats ?? ""}
                    onChange={(e) => setMinSeats(e.target.value === "" ? null : Number(e.target.value))}
                    placeholder="Leave blank if N/A"
                    disabled={putPlan.isPending}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="seats-fixed-input" className="text-label">
                    Fixed Seats (Optional)
                  </FieldLabel>
                  <Input
                    id="seats-fixed-input"
                    type="number"
                    min={1}
                    value={seatsFixed ?? ""}
                    onChange={(e) => setSeatsFixed(e.target.value === "" ? null : Number(e.target.value))}
                    placeholder="Leave blank if variable"
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
                    Public plan (visible in client pricing catalogue)
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
                    Archived (existing subscribers retain, no new signups)
                  </FieldLabel>
                </Field>
              </div>
            </div>

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
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={!planName.trim() || putPlan.isPending}
                aria-busy={putPlan.isPending}
              >
                {putPlan.isPending && <Spinner className="size-3.5" />}
                Save changes
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Plan Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <div className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Delete Plan: {planToDelete?.name}</DialogTitle>
              <DialogDescription>
                Are you sure you want to delete plan "{planToDelete?.key}"? A plan with any active or past workspace subscriptions cannot be deleted and must be archived instead.
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
                Cancel
              </DialogClose>
              <Button
                variant="destructive"
                onClick={handleConfirmDelete}
                disabled={deletePlan.isPending}
                aria-busy={deletePlan.isPending}
              >
                {deletePlan.isPending && <Spinner className="size-3.5" />}
                Confirm delete
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
