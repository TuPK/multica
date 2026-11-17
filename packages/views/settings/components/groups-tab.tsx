"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, ChevronRight, Plus, Users } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { memberGroupListOptions, type MemberGroup } from "@multica/core/groups";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import { useT } from "../../i18n";
import { SettingsCard, SettingsTab } from "./settings-layout";
import { GroupFormDialog } from "./groups-dialogs";

export function GroupsTab() {
  const { t } = useT("settings");
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";
  const { role } = useCurrentMember(wsId);
  const isManager = role === "owner" || role === "admin";

  const [createOpen, setCreateOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<MemberGroup | null>(null);

  const {
    data: groups = [],
    isLoading,
    isError,
    refetch,
  } = useQuery({
    ...memberGroupListOptions(wsId),
    enabled: Boolean(wsId && isManager),
  });

  if (!isManager) {
    return null;
  }

  return (
    <SettingsTab
      title={t(($) => $.groups.title)}
      scope="workspace"
      actions={
        <Button onClick={() => setCreateOpen(true)}>
          <Plus />
          {t(($) => $.groups.create_button)}
        </Button>
      }
    >
      {isLoading ? (
        <SettingsCard>
          <div className="px-5 py-6 text-body text-muted-foreground">
            {t(($) => $.groups.loading)}
          </div>
        </SettingsCard>
      ) : isError ? (
        <SettingsCard>
          <div className="flex items-center justify-between gap-4 px-5 py-4">
            <div className="flex items-center gap-2 text-body text-destructive">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <span>{t(($) => $.groups.load_failed)}</span>
            </div>
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              {t(($) => $.groups.retry)}
            </Button>
          </div>
        </SettingsCard>
      ) : groups.length === 0 ? (
        <SettingsCard>
          <div className="px-5 py-6 text-body text-muted-foreground">
            {t(($) => $.groups.empty)}
          </div>
        </SettingsCard>
      ) : (
        <SettingsCard>
          {groups.map((group) => (
            <button
              key={group.id}
              type="button"
              onClick={() => setEditingGroup(group)}
              className="flex w-full items-center justify-between gap-4 px-5 py-3.5 text-left transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
            >
              <div className="min-w-0 flex-1">
                <div className="truncate text-body font-medium text-foreground">
                  {group.name}
                </div>
                {group.description ? (
                  <div className="mt-0.5 truncate text-caption text-muted-foreground">
                    {group.description}
                  </div>
                ) : null}
              </div>
              <div className="flex shrink-0 items-center gap-2 text-caption text-muted-foreground">
                <span
                  className="inline-flex items-center gap-1 tabular-nums"
                  aria-label={t(($) => $.groups.member_count_aria, {
                    count: group.member_count,
                  })}
                >
                  <Users className="size-3.5" aria-hidden="true" />
                  <span>{group.member_count}</span>
                </span>
                <ChevronRight className="size-4" aria-hidden="true" />
              </div>
            </button>
          ))}
        </SettingsCard>
      )}

      {createOpen ? (
        <GroupFormDialog
          key="create"
          wsId={wsId}
          group={null}
          onClose={() => setCreateOpen(false)}
        />
      ) : null}

      {editingGroup ? (
        <GroupFormDialog
          key={editingGroup.id}
          wsId={wsId}
          group={editingGroup}
          onClose={() => setEditingGroup(null)}
        />
      ) : null}
    </SettingsTab>
  );
}
