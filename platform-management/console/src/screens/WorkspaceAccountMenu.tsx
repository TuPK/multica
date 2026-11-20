import { useState } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { Selector } from "@astryxdesign/core/Selector";
import { TextArea } from "@astryxdesign/core/TextArea";
import { TextInput } from "@astryxdesign/core/TextInput";
import { ArrowUpCircle, Ban, CalendarClock, CheckCircle2, Download, Lock, RotateCcw, Trash2, UserPlus } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { RowActions, type RowAction } from "../components/RowActions";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { getApiErrorMessage } from "../api/client";
import {
  useAddWorkspaceMember,
  useChangeWorkspaceStatus,
  useCreateQuotaRequest,
  useExportWorkspace,
  useDeleteWorkspaceNow,
  useUpdateSubscription,
  useWorkspace,
  type MemberRole } from "../api/hooks";
import { LIMIT_GATES } from "../api/schemas";
import { formatWorkspaceConflictError } from "./WorkspaceDetail";
import { t } from "../i18n";
import type { StaffRole } from "../components/types";

const MEMBER_ROLE_ITEMS: Array<{ value: MemberRole; label: string }> = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
  { value: "viewer", label: "Viewer" },
];
const BILLING_INTERVAL_ITEMS = [
  { value: "month", label: "Month" },
  { value: "year", label: "Year" },
];

interface WorkspaceAccountMenuProps {
  workspaceId: string;
  workspaceName: string;
  workspaceSlug: string;
  workspaceStatus: string;
  role: StaffRole | undefined;
}

// The workspace actions that used to sit in the workspace detail header. They
// live in the workspace's row of the workspaces table; success needs no notice
// because the table refreshes after each mutation.
export function WorkspaceAccountMenu({
  workspaceId,
  workspaceName,
  workspaceSlug,
  workspaceStatus,
  role,
}: WorkspaceAccountMenuProps) {
  const canChangeStatus = role === "system_admin" || role === "admin" || role === "support";
  const canEditSubscription = role === "billing" || role === "system_admin";
  const canManageMembers = role === "system_admin" || role === "admin";
  const canRequestQuota = !role || role === "system_admin" || role === "admin";
  const canDeleteWorkspace = role === "system_admin" || role === "admin";
  const canExportWorkspace = role === "system_admin" || role === "admin";
  const canSetPendingDeletion = role === "system_admin" || role === "admin";

  const changeStatus = useChangeWorkspaceStatus();
  const exportWorkspace = useExportWorkspace();
  const [exportError, setExportError] = useState<string | null>(null);
  const addWorkspaceMember = useAddWorkspaceMember();
  const deleteWorkspaceNow = useDeleteWorkspaceNow();
  const createQuotaRequest = useCreateQuotaRequest();

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [targetStatus, setTargetStatus] = useState("active");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [subDialogOpen, setSubDialogOpen] = useState(false);
  // The workspace detail (with its subscription) is fetched only while the
  // subscription dialog is open, so rows do not fetch it up front.
  const subscriptionQuery = useWorkspace(subDialogOpen ? workspaceId : undefined);

  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false);
  const [quotaGate, setQuotaGate] = useState<string>("agents");
  const [quotaRequestedValue, setQuotaRequestedValue] = useState("");
  const [quotaReason, setQuotaReason] = useState("");
  const [quotaError, setQuotaError] = useState<string | null>(null);

  const [memberDialogOpen, setMemberDialogOpen] = useState(false);
  const [memberEmail, setMemberEmail] = useState("");
  const [memberRole, setMemberRole] = useState<MemberRole>("member");
  const [memberError, setMemberError] = useState<string | null>(null);

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const availableStatuses = [
    { value: "active", label: t("Active") },
    { value: "read_only", label: t("Read-only") },
    { value: "suspended", label: t("Suspended") },
    ...(canSetPendingDeletion ? [{ value: "pending_deletion", label: t("Pending deletion") }] : []),
  ];

  // Opens the status dialog with the target preset, as the detail's quick
  // actions did.
  const handleOpenQuickStatus = (nextStatus: "active" | "read_only" | "suspended" | "pending_deletion") => {
    setTargetStatus(nextStatus);
    setStatusError(null);
    setStatusDialogOpen(true);
  };

  const handleExport = async () => {
    if (exportWorkspace.isPending) return;
    setExportError(null);
    try {
      await exportWorkspace.mutateAsync({ workspaceId });
    } catch (err: unknown) {
      setExportError(formatWorkspaceConflictError(err, t("Failed to request workspace export")));
    }
  };

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await changeStatus.mutateAsync({
        workspaceId,
        status: targetStatus as "active" | "read_only" | "suspended" | "pending_deletion",
        reason });
      setStatusDialogOpen(false);
    } catch (err: unknown) {
      setStatusError(
        formatWorkspaceConflictError(err, t("Failed to change workspace status")),
      );
    }
  };

  const handleOpenSubDialog = () => {
    setSubDialogOpen(true);
  };

  const handleOpenDeleteDialog = () => {
    setDeleteError(null);
    setDeleteDialogOpen(true);
  };

  const handleOpenMemberDialog = () => {
    setMemberEmail("");
    setMemberRole("member");
    setMemberError(null);
    setMemberDialogOpen(true);
  };

  const handleAddMemberSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = memberEmail.trim();
    if (!trimmed || addWorkspaceMember.isPending) return;
    setMemberError(null);
    try {
      await addWorkspaceMember.mutateAsync({
        workspaceId,
        email: trimmed,
        role: memberRole });
      setMemberDialogOpen(false);
    } catch (err: unknown) {
      setMemberError(getApiErrorMessage(err, t("Failed to add member")));
    }
  };

  const handleOpenQuotaDialog = () => {
    setQuotaGate("agents");
    setQuotaRequestedValue("");
    setQuotaReason("");
    setQuotaError(null);
    setQuotaDialogOpen(true);
  };

  const handleSubmitQuotaRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    if (createQuotaRequest.isPending) return;
    const parsedValue = Number(quotaRequestedValue.trim());
    if (quotaRequestedValue.trim() === "" || !Number.isInteger(parsedValue) || parsedValue <= 0) {
      setQuotaError(t("Requested value must be a positive whole number."));
      return;
    }
    const trimmedReason = quotaReason.trim();
    if (!trimmedReason) {
      setQuotaError(t("A reason is required."));
      return;
    }
    setQuotaError(null);
    try {
      await createQuotaRequest.mutateAsync({
        workspaceId,
        gate: quotaGate,
        requested_value: parsedValue,
        reason: trimmedReason });
      setQuotaDialogOpen(false);
    } catch (err: unknown) {
      setQuotaError(getApiErrorMessage(err, t("Failed to submit quota increase request")));
    }
  };

  const handleConfirmDeleteNow = async ({
    reason,
    confirm,
    export_first }: {
    reason: string;
    confirm: string;
    export_first?: boolean;
  }) => {
    setDeleteError(null);
    try {
      await deleteWorkspaceNow.mutateAsync({
        workspaceId,
        reason,
        confirm,
        ...(export_first !== undefined ? { export_first } : {}) });
      setDeleteDialogOpen(false);
    } catch (err: unknown) {
      setDeleteError(getApiErrorMessage(err, t("Failed to start workspace deletion")));
    }
  };

  // The status shortcuts follow the detail's quick actions: each shows when
  // the workspace is not already in that state.
  const actions: RowAction[] = [];
  if (canChangeStatus && workspaceStatus !== "suspended" && workspaceStatus !== "pending_deletion") {
    actions.push({ label: t("Suspend"), icon: Ban, onSelect: () => handleOpenQuickStatus("suspended") });
  }
  if (canChangeStatus && workspaceStatus !== "read_only" && workspaceStatus !== "pending_deletion") {
    actions.push({ label: t("Read-only"), icon: Lock, onSelect: () => handleOpenQuickStatus("read_only") });
  }
  if (canChangeStatus && workspaceStatus !== "active") {
    actions.push({ label: t("Unlock"), icon: CheckCircle2, onSelect: () => handleOpenQuickStatus("active") });
  }
  if (canDeleteWorkspace && workspaceStatus !== "pending_deletion") {
    actions.push({ label: t("Schedule deletion"), icon: CalendarClock, destructive: true, onSelect: () => handleOpenQuickStatus("pending_deletion") });
  }
  if (canDeleteWorkspace && workspaceStatus === "pending_deletion") {
    actions.push({ label: t("Cancel deletion"), icon: RotateCcw, onSelect: () => handleOpenQuickStatus("active") });
  }
  if (canExportWorkspace) {
    actions.push({ label: t("Export data"), icon: Download, disabled: exportWorkspace.isPending, onSelect: () => void handleExport() });
  }
  if (canEditSubscription) {
    actions.push({ label: t("Edit subscription"), onSelect: handleOpenSubDialog });
  }
  if (canManageMembers) {
    actions.push({ label: t("Add member"), icon: UserPlus, onSelect: () => handleOpenMemberDialog() });
  }
  if (canRequestQuota) {
    actions.push({ label: t("Request quota increase"), icon: ArrowUpCircle, onSelect: () => handleOpenQuotaDialog() });
  }
  if (canDeleteWorkspace) {
    actions.push({ label: t("Delete workspace"), icon: Trash2, destructive: true, onSelect: handleOpenDeleteDialog });
  }

  return (
    <>
      <RowActions label={t("workspace {0}", workspaceName)} actions={actions} />
      {exportError && <Banner status="error" role="alert" title={exportError} />}

      {subDialogOpen && subscriptionQuery.data && (
        <SubscriptionDialog
          workspaceId={workspaceId}
          subscription={subscriptionQuery.data.subscription}
          onClose={() => setSubDialogOpen(false)}
        />
      )}

      <ConfirmReasonDialog
        open={statusDialogOpen}
        onOpenChange={setStatusDialogOpen}
        title={t("Change workspace status")}
        description={t("Select the new status and provide a justification.")}
        confirmLabel={t("Update status")}
        isPending={changeStatus.isPending}
        error={statusError}
        onConfirm={handleConfirmStatusChange}
      >
        <Selector
          label={t("Target status")}
          width="100%"
          options={availableStatuses.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={targetStatus}
          onChange={(val) => {
              if (val) setTargetStatus(val);
            }}
        />
      </ConfirmReasonDialog>

      <Dialog
        isOpen={memberDialogOpen}
        onOpenChange={(open) => !addWorkspaceMember.isPending && setMemberDialogOpen(open)}
        purpose="form"
      >

          <form onSubmit={handleAddMemberSubmit} className="space-y-4">
            <DialogHeader title={t("Add member")} />

            <TextInput
              id="add-member-email"
              label={t("Email address")}
              type="email"
              value={memberEmail}
              onChange={(value) => setMemberEmail(value)}
              placeholder={t("member@example.com")}
              isRequired
              isDisabled={addWorkspaceMember.isPending}
              width="100%"
            />

            <Selector
              label={t("Member role")}
              width="100%"
              options={MEMBER_ROLE_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={memberRole}
              onChange={(val) => {
                  if (val) setMemberRole(val as MemberRole);
                }}
            />

            {memberError && (
              <Banner
                status="error"
                role="alert"
                title={memberError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
                variant="secondary"
                type="button"
                isDisabled={addWorkspaceMember.isPending}
                label={t("Cancel")}
                onClick={() => ((open) => !addWorkspaceMember.isPending && setMemberDialogOpen(open))(false)}
              />
              <Button
                type="submit"
                label={t("Add member")}
                variant="primary"
                isLoading={addWorkspaceMember.isPending}
                aria-busy={addWorkspaceMember.isPending}
                isDisabled={!memberEmail.trim() || addWorkspaceMember.isPending}
              />
            </div>
          </form>

      </Dialog>

      <Dialog
        isOpen={quotaDialogOpen}
        onOpenChange={(open) => {
          if (!createQuotaRequest.isPending) {
            setQuotaDialogOpen(open);
            if (!open) setQuotaError(null);
          }
        }}
        purpose="form"
        width={448}
      >

          <form onSubmit={handleSubmitQuotaRequest} className="space-y-4">
            <DialogHeader title={t("Request quota increase")} subtitle={<>{t("Submit a limit increase request for")} {workspaceName || workspaceSlug}{t(". System administrators review pending requests.")}</>} />

            {quotaError && (
              <Banner
                status="error"
                role="alert"
                title={quotaError}
              />
            )}

            <Selector
              label={t("Gate")}
              width="100%"
              options={LIMIT_GATES.map((g) => ({ value: g, label: g }))}
              value={quotaGate}
              onChange={(val) => val && setQuotaGate(val)}
            />

            <NumberInput
              id="workspace-quota-value"
              label={t("Requested value")}
              min={1}
              step={1}
              value={quotaRequestedValue.trim() === "" ? null : Number(quotaRequestedValue)}
              onChange={(value) => setQuotaRequestedValue(value === null ? "" : String(value))}
              placeholder="25"
              isDisabled={createQuotaRequest.isPending}
              width="100%"
            />

            <TextArea
              id="workspace-quota-reason"
              label={t("Reason")}
              value={quotaReason}
              onChange={(value) => setQuotaReason(value)}
              placeholder={t("Explain why the workspace needs this quota increase...")}
              isDisabled={createQuotaRequest.isPending}
              rows={3}
              width="100%"
            />

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="secondary"
                isDisabled={createQuotaRequest.isPending}
                label={t("Cancel")}
                onClick={() => ((open) => {
          if (!createQuotaRequest.isPending) {
            setQuotaDialogOpen(open);
            if (!open) setQuotaError(null);
          }
        })(false)}
              />
              <Button
                type="submit"
                label={t("Submit request")}
                variant="primary"
                isLoading={createQuotaRequest.isPending}
                aria-busy={createQuotaRequest.isPending || undefined}
                isDisabled={
                  !quotaRequestedValue.trim() ||
                  !quotaReason.trim() ||
                  createQuotaRequest.isPending
                }
              />
            </div>
          </form>

      </Dialog>

      <TypeToConfirmDialog
        open={deleteDialogOpen}
        onOpenChange={(open) => {
          setDeleteDialogOpen(open);
          if (!open) setDeleteError(null);
        }}
        title={t("Delete workspace")}
        description={t("Immediate deletion permanently removes this workspace and cannot be undone once started.")}
        expectedText={workspaceSlug}
        confirmInputLabel={t("Workspace slug")}
        confirmLabel={t("Delete workspace")}
        confirmVariant="destructive"
        showExportFirst
        error={deleteError}
        onConfirm={handleConfirmDeleteNow}
      />

    </>
  );
}

// Edit-subscription dialog. Mounted only once the workspace is loaded, so its
// form starts from the current subscription.
function SubscriptionDialog({
  workspaceId,
  subscription,
  onClose,
}: {
  workspaceId: string;
  subscription: { plan_key?: string | null; seats?: number | null; interval?: string | null; trial_end?: string | null } | null | undefined;
  onClose: () => void;
}) {
  const updateSubscription = useUpdateSubscription();
  const [subPlanKey, setSubPlanKey] = useState(subscription?.plan_key || "pro");
  const [subSeats, setSubSeats] = useState(subscription?.seats ?? 1);
  const [subInterval, setSubInterval] = useState(subscription?.interval || "month");
  const [subTrialEnd, setSubTrialEnd] = useState(subscription?.trial_end || "");
  const [subReason, setSubReason] = useState("");
  const [subError, setSubError] = useState<string | null>(null);

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
        reason: subReason.trim() });
      onClose();
    } catch (err: unknown) {
      setSubError(err instanceof Error ? err.message : t("Failed to update subscription"));
    }
  };

  return (
    <>
      {/* Edit Subscription Dialog */}
      <Dialog
        isOpen
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
        purpose="form"
      >

          <form onSubmit={handleSaveSubscription} className="flex flex-col gap-4">
            <DialogHeader title={t("Edit Subscription")} subtitle={t("Update subscription plan, seats, billing interval, and trial period.")} />

            <div className="flex flex-col gap-3">
              <TextInput
                label={t("Plan")}
                id={"sub-plan-key-input"}
                isRequired
                value={subPlanKey}
                onChange={(value) => setSubPlanKey(value)}
                placeholder={t("e.g. pro, business, enterprise")}
                isDisabled={updateSubscription.isPending}
                width="100%"
              />

              <div className="grid grid-cols-2 gap-3">
                <NumberInput
                  label={t("Seats")}
                  id={"sub-seats-input"}
                  isRequired
                  min={0}
                  value={subSeats}
                  onChange={(value) => setSubSeats(Number(value))}
                  isDisabled={updateSubscription.isPending}
                  width="100%"
                />

                <Selector
                  label={t("Billing Interval")}
                  isRequired
                  width="100%"
                  options={BILLING_INTERVAL_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                  value={subInterval}
                  onChange={(val) => {
                      if (val) setSubInterval(val);
                    }}
                />
              </div>

              <TextInput
                id="sub-trial-end-input"
                label={t("Trial End (RFC3339 or blank)")}
                value={subTrialEnd}
                onChange={(value) => setSubTrialEnd(value)}
                placeholder={t("e.g. 2026-12-01T00:00:00Z")}
                isDisabled={updateSubscription.isPending}
                width="100%"
              />

              <TextArea
                label={t("Reason")}
                id={"sub-reason-input"}
                isRequired
                value={subReason}
                onChange={(value) => setSubReason(value)}
                placeholder={t("Provide a reason for this subscription change...")}
                rows={3}
                isDisabled={updateSubscription.isPending}
                width="100%"
              />
            </div>

            {subError && (
              <Banner
                status="error"
                role="alert"
                title={subError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
        type="button"
        isDisabled={updateSubscription.isPending}
        label={t("Cancel")}
        variant="secondary"
        onClick={() => onClose()}
      />
              <Button
                type="submit"
                label={t("Save subscription")}
                variant="primary"
                isLoading={updateSubscription.isPending}
                aria-busy={updateSubscription.isPending}
                isDisabled={!subReason.trim() || updateSubscription.isPending}
              />
            </div>
          </form>

      </Dialog>

    </>
  );
}
