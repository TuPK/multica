/* eslint-disable i18next/no-literal-string, no-restricted-syntax */
import { useMemo, useState } from "react";
import type { FlowDoc, FlowDocRole, SquadMember } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AlertCircle, Plus, Trash2, UserPlus } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink } from "../../navigation";

export interface RolesPanelProps {
  doc: FlowDoc;
  onChangeRoles: (roles: FlowDocRole[]) => void;
  squadMembers?: SquadMember[];
  squadId?: string;
  roleLimit?: number;
}

export function RolesPanel({
  doc,
  onChangeRoles,
  squadMembers = [],
  squadId,
  roleLimit = 16,
}: RolesPanelProps) {
  const { t } = useT("squads");
  const paths = useWorkspacePaths();
  const [newKey, setNewKey] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [newKind, setNewKind] = useState<FlowDocRole["kind"]>("agent");

  // Roles in use by any task or variable
  const rolesInUse = useMemo(() => {
    const inUse = new Set<string>();
    for (const step of doc.steps) {
      for (const task of step.tasks ?? []) {
        if (task.role) inUse.add(task.role);
      }
    }
    for (const v of doc.vars ?? []) {
      for (const r of v.set_by ?? []) {
        inUse.add(r);
      }
    }
    return inUse;
  }, [doc.steps, doc.vars]);

  // Squad agents excluding leader
  const squadAgents = useMemo(() => {
    return squadMembers.filter((m) => m.member_type === "agent");
  }, [squadMembers]);

  const atLimit = (doc.roles?.length ?? 0) >= roleLimit;

  const handleAddRole = () => {
    if (atLimit) return;
    const key = newKey.trim().toLowerCase().replace(/[^a-z0-9_]/g, "_");
    const label = newLabel.trim() || key;
    if (!key) return;
    if ((doc.roles ?? []).some((r) => r.key === key)) return;

    const newRole: FlowDocRole = {
      key,
      label,
      kind: newKind,
      pool: 1,
    };
    onChangeRoles([...(doc.roles ?? []), newRole]);
    setNewKey("");
    setNewLabel("");
  };

  const handleDeleteRole = (key: string) => {
    if (rolesInUse.has(key)) return;
    onChangeRoles((doc.roles ?? []).filter((r) => r.key !== key));
  };

  const getRoleKindLabel = (kind: FlowDocRole["kind"]) => {
    switch (kind) {
      case "agent":
        return t(($) => $.flow_designer.roles.kind.agent);
      case "member":
        return t(($) => $.flow_designer.roles.kind.member);
      case "managers":
        return t(($) => $.flow_designer.roles.kind.managers);
      case "lead":
        return "Leader";
      case "requester":
        return "Requester";
      default:
        return String(kind);
    }
  };

  return (
    <div className="space-y-4 p-4 text-foreground">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-body font-semibold">{t(($) => $.flow_designer.roles.title)}</h3>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.flow_designer.roles.leader)}
          </p>
        </div>
        <span className="text-caption text-muted-foreground tabular-nums">
          {doc.roles?.length ?? 0} / {roleLimit}
        </span>
      </div>

      {squadAgents.length === 0 && squadId && (
        <div className="flex items-center justify-between gap-2 p-3 rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-caption">
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" aria-hidden />
            <span>Add agent to squad</span>
          </div>
          <AppLink href={`${paths.squads()}/${squadId}`}>
            <Button size="xs" variant="outline" className="h-7 gap-1">
              <UserPlus className="size-3" aria-hidden />
              <span>Add agent</span>
            </Button>
          </AppLink>
        </div>
      )}

      {/* Roles table */}
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">Key</th>
              <th className="p-2 font-medium">Label</th>
              <th className="p-2 font-medium">Kind</th>
              <th className="p-2 font-medium">Pool</th>
              <th className="p-2 font-medium">Usage</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {/* Built-in lead role */}
            <tr className="bg-muted/20">
              <td className="p-2 font-mono text-micro text-foreground">lead</td>
              <td className="p-2">Leader</td>
              <td className="p-2">
                <Badge variant="secondary" className="text-micro font-normal">Leader</Badge>
              </td>
              <td className="p-2">1</td>
              <td className="p-2 text-muted-foreground text-micro">Fixed leader</td>
              <td className="p-2"></td>
            </tr>

            {(doc.roles ?? []).map((role) => {
              const inUse = rolesInUse.has(role.key);
              return (
                <tr key={role.key} className="hover:bg-muted/30">
                  <td className="p-2 font-mono text-micro text-foreground">{role.key}</td>
                  <td className="p-2">{role.label}</td>
                  <td className="p-2">
                    <Badge variant="secondary" className="text-micro font-normal">
                      {getRoleKindLabel(role.kind)}
                    </Badge>
                  </td>
                  <td className="p-2 tabular-nums">{role.pool ?? 1}</td>
                  <td className="p-2">
                    {inUse ? (
                      <span className="text-micro text-muted-foreground">In use</span>
                    ) : (
                      <span className="text-micro text-amber-600 dark:text-amber-400">
                        {t(($) => $.flow_designer.roles.unused)}
                      </span>
                    )}
                  </td>
                  <td className="p-2 text-right">
                    {!inUse && role.kind !== "lead" && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        aria-label={`Delete ${role.key}`}
                        onClick={() => handleDeleteRole(role.key)}
                        className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 className="size-3.5" aria-hidden />
                      </Button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Add Role Row */}
      <div className="flex items-center gap-2 pt-2 flex-wrap">
        <Input
          placeholder="role_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-32"
          disabled={atLimit}
        />
        <Input
          placeholder="Label"
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          className="h-8 text-caption w-40"
          disabled={atLimit}
        />
        <select
          value={newKind}
          onChange={(e) => setNewKind(e.target.value as FlowDocRole["kind"])}
          className="h-8 rounded-md border border-input bg-background px-2 text-caption"
          disabled={atLimit}
        >
          <option value="agent">{t(($) => $.flow_designer.roles.kind.agent)}</option>
          <option value="member">{t(($) => $.flow_designer.roles.kind.member)}</option>
          <option value="managers">{t(($) => $.flow_designer.roles.kind.managers)}</option>
        </select>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={atLimit || !newKey.trim()}
          onClick={handleAddRole}
          className="h-8 gap-1 text-caption"
        >
          <Plus className="size-3.5" aria-hidden />
          <span>Add role</span>
        </Button>
        {atLimit && (
          <span className="text-micro text-muted-foreground">
            Role limit of {roleLimit} reached
          </span>
        )}
      </div>
    </div>
  );
}
