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
import { ArrowLeft, Plus } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import {
  useTenant,
  useTenantNotes,
  useTenantAudit,
  useChangeTenantStatus,
  useAddTenantNote,
  useUpdateSubscription,
} from "../api/hooks";
import type { StaffRole } from "../components/types";
import type { Note, AuditEvent, Order } from "../api/schemas";

interface TenantDetailProps {
  tenantId: string;
  onBack: () => void;
  userRole: StaffRole;
}

export function TenantDetail({ tenantId, onBack, userRole }: TenantDetailProps) {
  const {
    data: tenant,
    isLoading: tenantLoading,
    isError: tenantError,
    error: tenantErrorObj,
    refetch: refetchTenant,
  } = useTenant(tenantId);

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

  const canEditSubscription = userRole === "billing" || userRole === "admin";

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

  const availableStatuses = [
    { value: "active", label: "Active" },
    { value: "read_only", label: "Read-only" },
    { value: "suspended", label: "Suspended" },
    ...(userRole === "admin"
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
    </div>
  );
}
