"use client";

import { useMemo, useState } from "react";
import type { FlowDoc, FlowDocRole } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AlertCircle, Plus, Trash2, UserPlus } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink } from "../../navigation";

export interface RoleChoice {
  id: string;
  name: string;
}

export interface RolesPanelProps {
  doc: FlowDoc;
  onChangeRoles: (roles: FlowDocRole[]) => void;
  /** The agents of the squad, its lead excluded: the ones an agent role can be given to. */
  agentChoices?: RoleChoice[];
  /** The people of the workspace: the ones a member role can be given to. */
  memberChoices?: RoleChoice[];
  squadId?: string;
  roleLimit?: number;
}

export function RolesPanel({
  doc,
  onChangeRoles,
  agentChoices = [],
  memberChoices = [],
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

  // Gives a role to the people ticked: agents of the squad for an agent role, people of the workspace
  // for a member role. The server checks the binding again when the draft is published.
  const handleToggleBound = (role: FlowDocRole, id: string, on: boolean) => {
    const field = role.kind === "member" ? "member_ids" : "agent_ids";
    const current = role.bind?.[field] ?? [];
    const next = on ? [...current, id] : current.filter((x) => x !== id);
    onChangeRoles(
      (doc.roles ?? []).map((r) => (r.key === role.key ? { ...r, bind: { ...r.bind, [field]: next } } : r)),
    );
  };

  const handleSetPool = (key: string, pool: number) => {
    onChangeRoles((doc.roles ?? []).map((r) => (r.key === key ? { ...r, pool: Math.max(1, pool || 1) } : r)));
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
        return t(($) => $.flow_designer.roles.kind.lead);
      case "requester":
        return t(($) => $.flow_designer.roles.kind.requester);
      default:
        return String(kind);
    }
  };

  const renderAssigned = (role: FlowDocRole) => {
    if (role.kind !== "agent" && role.kind !== "member") return <span className="text-micro text-muted-foreground">—</span>;
    const choices = role.kind === "member" ? memberChoices : agentChoices;
    const bound = (role.kind === "member" ? role.bind?.member_ids : role.bind?.agent_ids) ?? [];
    if (choices.length === 0) {
      return <span className="text-micro text-muted-foreground">{t(($) => $.flow_designer.roles.no_choices)}</span>;
    }
    return (
      <div className="space-y-1">
        <span className="text-micro text-muted-foreground tabular-nums">
          {t(($) => $.flow_designer.roles.assigned_count, { count: bound.length, pool: role.pool ?? 1 })}
        </span>
        <div className="flex max-h-24 flex-col gap-0.5 overflow-y-auto">
          {choices.map((choice) => (
            <label key={choice.id} className="flex cursor-pointer items-center gap-1.5 text-micro">
              <input
                type="checkbox"
                checked={bound.includes(choice.id)}
                onChange={(e) => handleToggleBound(role, choice.id, e.target.checked)}
                className="size-3 rounded border-input"
              />
              <span className="truncate">{choice.name}</span>
            </label>
          ))}
        </div>
      </div>
    );
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

      {agentChoices.length === 0 && squadId && (
        <div className="flex items-center justify-between gap-2 p-3 rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-caption">
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" aria-hidden />
            <span>{t(($) => $.flow_designer.roles.add_agent_notice)}</span>
          </div>
          <AppLink href={`${paths.squads()}/${squadId}`}>
            <Button size="xs" variant="outline" className="h-7 gap-1">
              <UserPlus className="size-3" aria-hidden />
              <span>{t(($) => $.flow_designer.roles.add_agent)}</span>
            </Button>
          </AppLink>
        </div>
      )}

      {/* Roles table */}
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-caption border-collapse">
          <thead>
            <tr className="border-b border-border bg-muted/50 text-left text-muted-foreground">
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.key)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.label)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.common.kind)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.roles.col_pool)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.roles.col_assigned)}</th>
              <th className="p-2 font-medium">{t(($) => $.flow_designer.roles.col_usage)}</th>
              <th className="p-2 font-medium w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {/* Built-in lead role */}
            <tr className="bg-muted/20">
              {/* The key of the fixed lead role is a code token. */}
              {/* eslint-disable-next-line i18next/no-literal-string */}
              <td className="p-2 font-mono text-micro text-foreground">lead</td>
              <td className="p-2">{t(($) => $.flow_designer.roles.kind.lead)}</td>
              <td className="p-2">
                <Badge variant="secondary" className="text-micro font-normal">{t(($) => $.flow_designer.roles.kind.lead)}</Badge>
              </td>
              <td className="p-2">1</td>
              <td className="p-2 text-muted-foreground text-micro">{t(($) => $.flow_designer.roles.fixed_leader)}</td>
              <td className="p-2"></td>
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
                  <td className="p-2 tabular-nums">
                    <Input
                      type="number"
                      min={1}
                      value={role.pool ?? 1}
                      aria-label={t(($) => $.flow_designer.roles.pool_label, { key: role.key })}
                      onChange={(e) => handleSetPool(role.key, Number(e.target.value))}
                      className="h-7 w-16 text-micro"
                    />
                  </td>
                  <td className="p-2">{renderAssigned(role)}</td>
                  <td className="p-2">
                    {inUse ? (
                      <span className="text-micro text-muted-foreground">{t(($) => $.flow_designer.roles.in_use)}</span>
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
                        aria-label={t(($) => $.flow_designer.roles.delete, { key: role.key })}
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
          // A key is a code token: the same in every language.
          // eslint-disable-next-line no-restricted-syntax
          placeholder="role_key"
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          className="h-8 text-caption w-32"
          disabled={atLimit}
        />
        <Input
          placeholder={t(($) => $.flow_designer.roles.label_placeholder)}
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
          <span>{t(($) => $.flow_designer.roles.add)}</span>
        </Button>
        {atLimit && (
          <span className="text-micro text-muted-foreground">
            {t(($) => $.flow_designer.roles.limit_reached, { count: roleLimit })}
          </span>
        )}
      </div>
    </div>
  );
}
