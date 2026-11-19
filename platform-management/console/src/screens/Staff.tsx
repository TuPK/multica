import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
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
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, Pencil, Shield, Trash2, UserPlus } from "@multica/ui/icons";
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
  type SettableStaffRole,
} from "../api/hooks";
import type { StaffMember } from "../components/types";

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
    label: t.name ? `${t.name} (${t.id})` : t.id,
  }));
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
      setAddError(getApiErrorMessage(err, "Failed to add staff member"));
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
      setEditError(getApiErrorMessage(err, "Failed to update staff member"));
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
        userId: staffToRemove.user_id,
      });
      setRemoveDialogOpen(false);
      setStaffToRemove(null);
    } catch (err: unknown) {
      const msg =
        err instanceof Error
          ? err.message
          : (err as { code?: string })?.code === "last_admin"
            ? "the last active administrator cannot be removed"
            : "Failed to remove staff member";
      setRemoveError(msg);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Staff Management"
        actions={
          <Button size="sm" onClick={handleOpenAddDialog} className="gap-1.5">
            <UserPlus className="size-4" aria-hidden="true" />
            <span>Add staff</span>
          </Button>
        }
      />

      {removeError && !removeDialogOpen && (
        <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          <AlertDescription>{removeError}</AlertDescription>
        </Alert>
      )}

      {isLoading && <LoadingState message="Loading staff..." />}

      {isError && (
        <ErrorState
          title="Failed to load staff"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && staffList.length === 0 && (
        <EmptyState
          icon={<Shield className="size-5" aria-hidden="true" />}
          title="No staff members"
          description="There are currently no staff members configured."
          actions={
            <Button variant="outline" size="sm" onClick={handleOpenAddDialog}>
              Add staff
            </Button>
          }
        />
      )}

      {!isLoading && !isError && staffList.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table density="balanced" dividers="rows" hasHover>
            <TableHeader>
              <TableRow isHeaderRow>
                <TableHeaderCell>Email</TableHeaderCell>
                <TableHeaderCell>Role</TableHeaderCell>
                <TableHeaderCell>Tenant</TableHeaderCell>
                <TableHeaderCell>Created By</TableHeaderCell>
                <TableHeaderCell>Created At</TableHeaderCell>
                <TableHeaderCell className="text-right">Action</TableHeaderCell>
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
                      <Text type="supporting">{member.created_by || "System"}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {member.created_at ? new Date(member.created_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="inline-flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenEditDialog(member)}
                          aria-label={`Edit staff member ${member.email}`}
                        >
                          <Pencil className="size-4" aria-hidden="true" />
                          <span>Edit</span>
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenRemoveDialog(member)}
                          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                          aria-label={`Remove staff member ${member.email}`}
                        >
                          <Trash2 className="size-4" aria-hidden="true" />
                          <span>Remove</span>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
        <DialogContent>
          <form onSubmit={handleAddSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>Add staff member</DialogTitle>
              <DialogDescription>
                Grant platform console access to an existing Multica account.
              </DialogDescription>
            </DialogHeader>

            <Field>
              <FieldLabel htmlFor="staff-email-input" className="text-label">
                Email address
              </FieldLabel>
              <Input
                id="staff-email-input"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="user@example.com"
                required
                disabled={addStaff.isPending}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="staff-role-select" className="text-label">
                Staff role
              </FieldLabel>
              <Select
                items={STAFF_ROLE_ITEMS}
                value={role}
                onValueChange={(val) => {
                  if (val) {
                    const nextRole = val as SettableStaffRole;
                    setRole(nextRole);
                    if (nextRole !== "admin") setTenantId("");
                  }
                }}
                disabled={addStaff.isPending}
              >
                <SelectTrigger id="staff-role-select" aria-label="Staff role" className="w-full">
                  <SelectValue>
                    {STAFF_ROLE_ITEMS.find((item) => item.value === role)?.label ?? role}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {STAFF_ROLE_ITEMS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            {role === "admin" && (
              <Field>
                <FieldLabel htmlFor="staff-tenant-select" className="text-label">
                  Tenant
                </FieldLabel>
                <Select
                  items={tenantItems}
                  value={tenantId}
                  onValueChange={(val) => setTenantId(val ?? "")}
                  disabled={addStaff.isPending}
                >
                  <SelectTrigger id="staff-tenant-select" aria-label="Tenant" className="w-full">
                    <SelectValue placeholder="Select tenant">
                      {tenantItems.find((item) => item.value === tenantId)?.label}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {tenantItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )}

            {addError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{addError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={addStaff.isPending} />}
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={!email.trim() || (role === "admin" && !tenantId) || addStaff.isPending}
                aria-busy={addStaff.isPending}
              >
                {addStaff.isPending && <Spinner className="size-3.5" />}
                Add staff
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={editDialogOpen} onOpenChange={(open) => !updateStaff.isPending && setEditDialogOpen(open)}>
        <DialogContent>
          <form onSubmit={handleEditSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>Edit staff member</DialogTitle>
              <DialogDescription>
                {staffToEdit?.email}
              </DialogDescription>
            </DialogHeader>

            <Field>
              <FieldLabel htmlFor="edit-staff-role-select" className="text-label">
                Staff role
              </FieldLabel>
              <Select
                items={STAFF_ROLE_ITEMS}
                value={editRole}
                onValueChange={(val) => {
                  if (val) {
                    const nextRole = val as SettableStaffRole;
                    setEditRole(nextRole);
                    if (nextRole !== "admin") setEditTenantId("");
                  }
                }}
                disabled={updateStaff.isPending}
              >
                <SelectTrigger id="edit-staff-role-select" aria-label="Staff role" className="w-full">
                  <SelectValue>
                    {STAFF_ROLE_ITEMS.find((item) => item.value === editRole)?.label ?? editRole}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {STAFF_ROLE_ITEMS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            {editRole === "admin" && (
              <Field>
                <FieldLabel htmlFor="edit-staff-tenant-select" className="text-label">
                  Tenant
                </FieldLabel>
                <Select
                  items={tenantItems}
                  value={editTenantId}
                  onValueChange={(val) => setEditTenantId(val ?? "")}
                  disabled={updateStaff.isPending}
                >
                  <SelectTrigger id="edit-staff-tenant-select" aria-label="Tenant" className="w-full">
                    <SelectValue placeholder="Select tenant">
                      {tenantItems.find((item) => item.value === editTenantId)?.label}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {tenantItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )}

            {editError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{editError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={<Button variant="outline" type="button" disabled={updateStaff.isPending} />}
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={(editRole === "admin" && !editTenantId) || updateStaff.isPending}
                aria-busy={updateStaff.isPending}
              >
                {updateStaff.isPending && <Spinner className="size-3.5" />}
                Save changes
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={removeDialogOpen} onOpenChange={setRemoveDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove staff member</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke staff access for{" "}
              <strong className="text-foreground">{staffToRemove?.email}</strong>?
            </DialogDescription>
          </DialogHeader>

          {removeError && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">{removeError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={removeStaff.isPending} />}>
              Cancel
            </DialogClose>
            <Button
              variant="destructive"
              onClick={handleConfirmRemove}
              disabled={removeStaff.isPending}
              aria-busy={removeStaff.isPending}
            >
              {removeStaff.isPending && <Spinner className="size-3.5" />}
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
