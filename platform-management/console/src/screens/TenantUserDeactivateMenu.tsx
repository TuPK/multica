import { useState } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Card } from "@astryxdesign/core/Card";
import { Text } from "@astryxdesign/core/Text";
import { UserMinus } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { RowActions, type RowAction } from "../components/RowActions";
import { StatusBadge } from "../components/StatusBadge";
import { getMemberChangeErrorMessage } from "../api/client";
import { useDeactivateTenantUser, type TenantUser } from "../api/hooks";
import { t } from "../i18n";

interface TenantUserDeactivateMenuProps {
  tenantId: string;
  user: TenantUser;
  canManageMembers: boolean;
}

// "Deactivate in tenant" for one user of a tenant, shown in the user's row of
// the tenant users table. It removes the user from that tenant's workspaces
// only; the global account and other tenants are untouched.
export function TenantUserDeactivateMenu({ tenantId, user, canManageMembers }: TenantUserDeactivateMenuProps) {
  const deactivateTenantUser = useDeactivateTenantUser();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const memberships = user.workspaces.map((w) => ({
    workspace_id: w.workspace_id,
    name: w.name,
    role: w.member_role,
  }));
  const hasOwnerWorkspace = memberships.some((m) => m.role === "owner");
  const canDeactivate = canManageMembers && memberships.length > 0 && !user.is_staff;

  const handleConfirm = async (reason: string) => {
    setError(null);
    try {
      await deactivateTenantUser.mutateAsync({ tenantId, userId: user.user_id, reason });
      setDialogOpen(false);
    } catch (err: unknown) {
      setError(getMemberChangeErrorMessage(err, t("Failed to deactivate user in tenant")));
    }
  };

  const actions: RowAction[] = canDeactivate
    ? [
        {
          label: t("Deactivate in tenant"),
          icon: UserMinus,
          onSelect: () => {
            setError(null);
            setDialogOpen(true);
          },
        },
      ]
    : [];

  return (
    <>
      <RowActions label={t("user {0}", user.name || user.email)} actions={actions} />

      <ConfirmReasonDialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) setError(null);
        }}
        title={t("Deactivate user in tenant")}
        description={t("Removes this user from the tenant workspaces below without changing their global account or other tenants.")}
        confirmLabel={t("Deactivate in tenant")}
        confirmVariant="destructive"
        isPending={deactivateTenantUser.isPending}
        error={error}
        onConfirm={handleConfirm}
      >
        <div className="space-y-2">
          <Text type="supporting" weight="medium" as="div">
            {t("Workspaces affected (")}{memberships.length})
          </Text>
          <div className="space-y-1.5">
            {memberships.map((ws) => (
              <Card key={ws.workspace_id} padding={2}>
                <div className="flex items-center justify-between gap-2">
                  <Text type="body" weight="medium">
                    {ws.name || ws.workspace_id}
                  </Text>
                  <StatusBadge status={ws.role} />
                </div>
              </Card>
            ))}
          </div>
          {hasOwnerWorkspace && (
            <Banner
              status="info"
              title={t("This user is an owner in at least one workspace. Staff cannot remove an owner until ownership is transferred.")}
            />
          )}
        </div>
      </ConfirmReasonDialog>
    </>
  );
}
