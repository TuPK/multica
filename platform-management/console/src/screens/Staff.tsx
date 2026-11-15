import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
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
import { UserPlus, Trash2 } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useStaff, useAddStaff, useRemoveStaff } from "../api/hooks";
import type { StaffMember, StaffRole } from "../components/types";

export function Staff() {
  const { data, isLoading, isError, error, refetch } = useStaff();
  const addStaff = useAddStaff();
  const removeStaff = useRemoveStaff();

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<StaffRole>("support");
  const [addError, setAddError] = useState<string | null>(null);

  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [staffToRemove, setStaffToRemove] = useState<StaffMember | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);

  const staffList: StaffMember[] = Array.isArray(data)
    ? data
    : (data as unknown as { staff?: StaffMember[] })?.staff || [];

  const handleOpenAddDialog = () => {
    setEmail("");
    setRole("support");
    setAddError(null);
    setAddDialogOpen(true);
  };

  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || addStaff.isPending) return;
    setAddError(null);
    try {
      await addStaff.mutateAsync({
        email: email.trim(),
        role: role as "admin" | "support" | "billing",
      });
      setAddDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : "Failed to add staff member";
      setAddError(msg);
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
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-heading text-title-lg font-bold">Staff Management</h1>
          <p className="text-body text-muted-foreground">
            Manage staff members and internal platform permissions.
          </p>
        </div>
        <Button onClick={handleOpenAddDialog} className="gap-1.5">
          <UserPlus className="size-4" />
          <span>Add staff</span>
        </Button>
      </div>

      {removeError && !removeDialogOpen && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-body text-destructive"
        >
          {removeError}
        </div>
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
          title="No staff members"
          description="There are currently no staff members configured."
        />
      )}

      {!isLoading && !isError && staffList.length > 0 && (
        <div className="rounded-lg border border-surface-border bg-surface shadow-xs">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Created By</TableHead>
                <TableHead>Created At</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {staffList.map((member: StaffMember) => (
                <TableRow key={member.user_id}>
                  <TableCell className="font-medium">{member.email}</TableCell>
                  <TableCell>
                    <StatusBadge status={member.role} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {member.created_by || "System"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {member.created_at ? new Date(member.created_at).toLocaleString() : "-"}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleOpenRemoveDialog(member)}
                      className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                      aria-label={`Remove staff member ${member.email}`}
                    >
                      <Trash2 className="size-4" />
                      <span>Remove</span>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
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

            <div className="space-y-1.5">
              <label htmlFor="staff-email-input" className="text-caption font-medium text-foreground">
                Email address
              </label>
              <Input
                id="staff-email-input"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="user@example.com"
                required
                disabled={addStaff.isPending}
              />
            </div>

            <div className="space-y-1.5">
              <label htmlFor="staff-role-select" className="text-caption font-medium text-foreground">
                Staff role
              </label>
              <select
                id="staff-role-select"
                value={role}
                onChange={(e) => setRole(e.target.value as StaffRole)}
                className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                disabled={addStaff.isPending}
              >
                <option value="support">Support</option>
                <option value="billing">Billing</option>
                <option value="admin">Admin</option>
              </select>
            </div>

            {addError && (
              <p role="alert" className="text-caption text-destructive">
                {addError}
              </p>
            )}

            <DialogFooter>
              <DialogClose render={<Button variant="outline" type="button" disabled={addStaff.isPending} />}>
                Cancel
              </DialogClose>
              <Button
                type="submit"
                disabled={!email.trim() || addStaff.isPending}
                aria-busy={addStaff.isPending}
              >
                {addStaff.isPending && <Spinner className="size-3.5" />}
                Add staff
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
            <div
              role="alert"
              className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-caption text-destructive"
            >
              {removeError}
            </div>
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
