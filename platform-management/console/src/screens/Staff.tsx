import { useState } from "react";
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
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { AlertCircle, Trash2, UserPlus } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { useStaff, useAddStaff, useRemoveStaff } from "../api/hooks";
import type { StaffMember, StaffRole } from "../components/types";

const STAFF_ROLE_ITEMS = [
  { value: "support", label: "Support" },
  { value: "billing", label: "Billing" },
  { value: "admin", label: "Admin" },
];

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
        <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
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
                  <TableCell className="text-muted-foreground tabular-nums">
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
                      <Trash2 className="size-4" aria-hidden="true" />
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
                  if (val) setRole(val as StaffRole);
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
