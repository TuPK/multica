import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Selector } from "@astryxdesign/core/Selector";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { Shield, UserPlus } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { getApiErrorMessage } from "../api/client";
import {
  useStaff,
  useAddStaff,
  useUpdateStaff,
  useRemoveStaff,
  useTenants,
  type SettableStaffRole } from "../api/hooks";
import type { StaffMember } from "../components/types";
import { t } from "../i18n";

const STAFF_ROLE_ITEMS: Array<{ value: SettableStaffRole; label: string }> = [
  { value: "support", label: "Support" },
  { value: "billing", label: "Billing" },
  { value: "admin", label: "Admin" },
  { value: "system_admin", label: "System Admin" },
];

export function Staff() {
  const { data, isLoading, isError, error, refetch } = useStaff();
  const { data: tenantsData } = useTenants();
  const addStaff = useAddStaff();
  const updateStaff = useUpdateStaff();
  const removeStaff = useRemoveStaff();

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<SettableStaffRole>("support");
  const [tenantId, setTenantId] = useState("");
  const [addError, setAddError] = useState<string | null>(null);

  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [staffToEdit, setStaffToEdit] = useState<StaffMember | null>(null);
  const [editRole, setEditRole] = useState<SettableStaffRole>("support");
  const [editTenantId, setEditTenantId] = useState("");
  const [editError, setEditError] = useState<string | null>(null);

  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [staffToRemove, setStaffToRemove] = useState<StaffMember | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);

  const staffList: StaffMember[] = Array.isArray(data)
    ? data
    : (data as unknown as { staff?: StaffMember[] })?.staff || [];

  const tenants = Array.isArray(tenantsData) ? tenantsData : [];
  const tenantItems = tenants.map((t) => ({
    value: t.id,
    label: t.name ? `${t.name} (${t.id})` : t.id }));
  const tenantNameById = new Map(tenants.map((t) => [t.id, t.name || t.id]));

  const handleOpenAddDialog = () => {
    setEmail("");
    setRole("support");
    setTenantId("");
    setAddError(null);
    setAddDialogOpen(true);
  };

  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || addStaff.isPending) return;
    if (role === "admin" && !tenantId) return;
    setAddError(null);
    try {
      await addStaff.mutateAsync(
        role === "admin"
          ? { email: email.trim(), role, tenant_id: tenantId }
          : { email: email.trim(), role },
      );
      setAddDialogOpen(false);
    } catch (err: unknown) {
      setAddError(getApiErrorMessage(err, t("Failed to add staff member")));
    }
  };

  const handleOpenEditDialog = (member: StaffMember) => {
    const knownRole = STAFF_ROLE_ITEMS.some((item) => item.value === member.role)
      ? (member.role as SettableStaffRole)
      : "support";
    setStaffToEdit(member);
    setEditRole(knownRole);
    setEditTenantId(member.tenant_id ?? "");
    setEditError(null);
    setEditDialogOpen(true);
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!staffToEdit || updateStaff.isPending) return;
    if (editRole === "admin" && !editTenantId) return;
    setEditError(null);
    try {
      await updateStaff.mutateAsync(
        editRole === "admin"
          ? { userId: staffToEdit.user_id, role: editRole, tenant_id: editTenantId }
          : { userId: staffToEdit.user_id, role: editRole },
      );
      setEditDialogOpen(false);
      setStaffToEdit(null);
    } catch (err: unknown) {
      setEditError(getApiErrorMessage(err, t("Failed to update staff member")));
    }
  };

  const handleOpenRemoveDialog = (member: StaffMember) => {
    setStaffToRemove(member);
    setRemoveError(null);
    setRemoveDialogOpen(true);
  };

  const handleConfirmRemove = async () => {
    if (!staffToRemove || removeStaff.isPending) return;
    setRemoveError(null);
    try {
      await removeStaff.mutateAsync({
        userId: staffToRemove.user_id });
      setRemoveDialogOpen(false);
      setStaffToRemove(null);
    } catch (err: unknown) {
      const msg =
        err instanceof Error
          ? err.message
          : (err as { code?: string })?.code === "last_admin"
            ? t("the last active administrator cannot be removed")
            : t("Failed to remove staff member");
      setRemoveError(msg);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Staff Management")}
        actions={
          <Button
            label={t("Add staff")}
            variant="primary"
            size="sm"
            icon={<UserPlus className="size-4" aria-hidden="true" />}
            onClick={handleOpenAddDialog}
            className="gap-1.5"
          />
        }
      />

      {removeError && !removeDialogOpen && (
        <Banner
          status="error"
          role="alert"
          title={removeError}
        />
      )}

      {isLoading && <LoadingState message={t("Loading staff...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load staff")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && staffList.length === 0 && (
        <EmptyState
          icon={<Shield className="size-5" aria-hidden="true" />}
          title={t("No staff members")}
          description={t("There are currently no staff members configured.")}
          actions={
            <Button
              label={t("Add staff")}
              variant="secondary"
              size="sm"
              onClick={handleOpenAddDialog}
            />
          }
        />
      )}

      {!isLoading && !isError && staffList.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table density="balanced" dividers="rows" hasHover>
            <TableHeader>
              <TableRow isHeaderRow>
                <TableHeaderCell>{t("Email")}</TableHeaderCell>
                <TableHeaderCell>{t("Role")}</TableHeaderCell>
                <TableHeaderCell>{t("Tenant")}</TableHeaderCell>
                <TableHeaderCell>{t("Created By")}</TableHeaderCell>
                <TableHeaderCell>{t("Created At")}</TableHeaderCell>
                <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
              </TableRow>
            </TableHeader>
            <TableBody>
              {staffList.map((member: StaffMember) => {
                const tenantDisplay = member.tenant_name
                  ? member.tenant_name
                  : member.tenant_id
                    ? (tenantNameById.get(member.tenant_id) ?? member.tenant_id)
                    : "—";
                return (
                  <TableRow key={member.user_id}>
                    <TableCell>
                      <Text type="body" weight="medium">
                        {member.email}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={member.role} />
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">{tenantDisplay}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">{member.created_by || t("System")}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {member.created_at ? new Date(member.created_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right">
                      <RowActions
                        label={t("staff member {0}", member.email)}
                        actions={[
                          { label: t("Edit"), onSelect: () => handleOpenEditDialog(member) },
                          { label: t("Remove"), destructive: true, onSelect: () => handleOpenRemoveDialog(member) },
                        ]}
                      />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog
        isOpen={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        purpose="form"
      >

          <form onSubmit={handleAddSubmit} className="space-y-4">
            <DialogHeader title={t("Add staff member")} subtitle={t("Grant platform console access to an existing Multica account.")} />

            <TextInput
              id="staff-email-input"
              label={t("Email address")}
              type="email"
              value={email}
              onChange={(value) => setEmail(value)}
              placeholder={t("user@example.com")}
              isRequired
              isDisabled={addStaff.isPending}
              width="100%"
            />

            <Selector
              label={t("Staff role")}
              width="100%"
              options={STAFF_ROLE_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={role}
              onChange={(val) => {
                  if (val) {
                    const nextRole = val as SettableStaffRole;
                    setRole(nextRole);
                    if (nextRole !== "admin") setTenantId("");
                  }
                }}
            />

            {role === "admin" && (
              <Selector
                label={t("Tenant")}
                width="100%"
                options={tenantItems.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={tenantId}
                onChange={(val) => setTenantId(val ?? "")}
              />
            )}

            {addError && (
              <Banner
                status="error"
                role="alert"
                title={addError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
        type="button"
        isDisabled={addStaff.isPending}
        label={t("Cancel")}
        variant="secondary"
        onClick={() => setAddDialogOpen(false)}
      />
              <Button
                type="submit"
                label={t("Add staff")}
                variant="primary"
                isLoading={addStaff.isPending}
                aria-busy={addStaff.isPending}
                isDisabled={!email.trim() || (role === "admin" && !tenantId) || addStaff.isPending}
              />
            </div>
          </form>

      </Dialog>

      <Dialog
        isOpen={editDialogOpen}
        onOpenChange={(open) => !updateStaff.isPending && setEditDialogOpen(open)}
        purpose="form"
      >

          <form onSubmit={handleEditSubmit} className="space-y-4">
            <DialogHeader title={t("Edit staff member")} subtitle={staffToEdit?.email} />

            <Selector
              label={t("Staff role")}
              width="100%"
              options={STAFF_ROLE_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={editRole}
              onChange={(val) => {
                  if (val) {
                    const nextRole = val as SettableStaffRole;
                    setEditRole(nextRole);
                    if (nextRole !== "admin") setEditTenantId("");
                  }
                }}
            />

            {editRole === "admin" && (
              <Selector
                label={t("Tenant")}
                width="100%"
                options={tenantItems.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={editTenantId}
                onChange={(val) => setEditTenantId(val ?? "")}
              />
            )}

            {editError && (
              <Banner
                status="error"
                role="alert"
                title={editError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
        type="button"
        isDisabled={updateStaff.isPending}
        label={t("Cancel")}
        variant="secondary"
        onClick={() => ((open) => !updateStaff.isPending && setEditDialogOpen(open))(false)}
      />
              <Button
                type="submit"
                label={t("Save changes")}
                variant="primary"
                isLoading={updateStaff.isPending}
                aria-busy={updateStaff.isPending}
                isDisabled={(editRole === "admin" && !editTenantId) || updateStaff.isPending}
              />
            </div>
          </form>

      </Dialog>

      <Dialog
        isOpen={removeDialogOpen}
        onOpenChange={setRemoveDialogOpen}
        purpose="info"
      >

          <DialogHeader title={t("Remove staff member")} subtitle={<>{t("Are you sure you want to revoke staff access for")}{" "}
              <strong className="text-foreground">{staffToRemove?.email}</strong>?</>} />

          {removeError && (
            <Banner
              status="error"
              role="alert"
              title={removeError}
            />
          )}

          <div className="flex justify-end gap-2">
            <Button
        type="button"
        isDisabled={removeStaff.isPending}
        label={t("Cancel")}
        variant="secondary"
        onClick={() => setRemoveDialogOpen(false)}
      />
            <Button
              label={t("Remove")}
              variant="destructive"
              isLoading={removeStaff.isPending}
              onClick={handleConfirmRemove}
              aria-busy={removeStaff.isPending}
              isDisabled={removeStaff.isPending}
            />
          </div>

      </Dialog>
    </div>
  );
}
