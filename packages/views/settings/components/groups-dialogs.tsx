"use client";

import { useMemo, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2, Search, Trash2, Users } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { IconButton } from "@multica/ui/components/common/icon-button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { api, errorCode } from "@multica/core/api";
import {
  groupKeys,
  memberGroupDetailOptions,
  permissionCatalogueOptions,
  useAddMemberGroupMembers,
  useCreateMemberGroup,
  useDeleteMemberGroup,
  useRemoveMemberGroupMember,
  useSetMemberGroupPermissions,
  useUpdateMemberGroup,
  type MemberGroup,
} from "@multica/core/groups";
import type { PermissionKey } from "@multica/core/permissions";
import { memberListOptions } from "@multica/core/workspace/queries";
import type { MemberWithUser } from "@multica/core/types";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";

const GROUP_NAME_MAX_LENGTH = 64;
const GROUP_DESCRIPTION_MAX_LENGTH = 280;
const MAX_GROUP_ADD_BATCH = 100;

interface PermissionOption {
  key: PermissionKey;
  labelKey:
    | "members_invite"
    | "audit_read"
    | "billing_view"
    | "billing_manage"
    | "integrations_manage"
    | "runtime_profiles_manage";
  descriptionKey?: "members_invite_description" | "billing_manage_description";
}

const PERMISSION_OPTIONS: readonly PermissionOption[] = [
  {
    key: "members.invite",
    labelKey: "members_invite",
    descriptionKey: "members_invite_description",
  },
  {
    key: "audit.read",
    labelKey: "audit_read",
  },
  {
    key: "billing.view",
    labelKey: "billing_view",
  },
  {
    key: "billing.manage",
    labelKey: "billing_manage",
    descriptionKey: "billing_manage_description",
  },
  {
    key: "integrations.manage",
    labelKey: "integrations_manage",
  },
  {
    key: "runtime_profiles.manage",
    labelKey: "runtime_profiles_manage",
  },
];

function mapGroupError(
  err: unknown,
  fallback: string,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  switch (errorCode(err)) {
    case "group_name_taken":
      return t(($) => $.groups.errors.name_taken);
    case "group_member_limit":
      return t(($) => $.groups.errors.member_limit);
    case "too_many_user_ids":
      return t(($) => $.groups.errors.too_many_user_ids);
    case "plan_feature_unavailable":
      return t(($) => $.groups.errors.plan_unavailable);
    case "plan_limit_reached":
      return t(($) => $.groups.errors.plan_limit);
    default:
      return fallback;
  }
}

export function GroupFormDialog({
  wsId,
  group,
  onClose,
}: {
  wsId: string;
  group: MemberGroup | null;
  onClose: () => void;
}) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const [createdGroup, setCreatedGroup] = useState<MemberGroup | null>(null);
  const activeGroup = group ?? createdGroup;
  const isEditing = activeGroup !== null;

  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>(
    group?.permissions ?? [],
  );
  const [pendingUserIds, setPendingUserIds] = useState<string[]>([]);
  const [memberSearch, setMemberSearch] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [memberError, setMemberError] = useState<string | null>(null);
  const [isCreatingComposite, setIsCreatingComposite] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);

  const activeGroupId = activeGroup?.id ?? "";

  const { data: catalogue } = useQuery({
    ...permissionCatalogueOptions(wsId),
    enabled: Boolean(wsId),
  });
  const { data: members = [] } = useQuery({
    ...memberListOptions(wsId),
    enabled: Boolean(wsId),
  });
  const { data: detail, isLoading: detailLoading } = useQuery({
    ...memberGroupDetailOptions(wsId, activeGroupId),
    enabled: Boolean(wsId && activeGroupId),
  });

  const createGroup = useCreateMemberGroup(wsId);
  const updateGroup = useUpdateMemberGroup(wsId, activeGroupId);
  const setGroupPermissions = useSetMemberGroupPermissions(wsId, activeGroupId);
  const addMembers = useAddMemberGroupMembers(wsId, activeGroupId);
  const removeMember = useRemoveMemberGroupMember(wsId, activeGroupId);
  const deleteGroup = useDeleteMemberGroup(wsId);

  const isSaving =
    isCreatingComposite ||
    createGroup.isPending ||
    updateGroup.isPending ||
    setGroupPermissions.isPending;

  const permissionItems = useMemo(() => {
    if (!catalogue || catalogue.length === 0) return PERMISSION_OPTIONS;
    const allowed = new Set(catalogue);
    const filtered = PERMISSION_OPTIONS.filter((opt) => allowed.has(opt.key));
    return filtered.length > 0 ? filtered : PERMISSION_OPTIONS;
  }, [catalogue]);

  const membersById = useMemo(
    () => new Map<string, MemberWithUser>(members.map((m) => [m.user_id, m])),
    [members],
  );

  const currentMemberIds = useMemo(
    () => new Set((detail?.members ?? []).map((m) => m.user_id)),
    [detail?.members],
  );

  const candidateMembers = useMemo(() => {
    const query = memberSearch.trim().toLowerCase();
    return members.filter((m) => {
      if (isEditing && currentMemberIds.has(m.user_id)) return false;
      if (!query) return true;
      return (
        m.name.toLowerCase().includes(query) ||
        m.email.toLowerCase().includes(query)
      );
    });
  }, [members, isEditing, currentMemberIds, memberSearch]);

  const togglePermission = (key: string, checked: boolean) => {
    setSelectedPermissions((prev) =>
      checked
        ? Array.from(new Set([...prev, key]))
        : prev.filter((item) => item !== key),
    );
  };

  const togglePendingMember = (userId: string, checked: boolean) => {
    setMemberError(null);
    setPendingUserIds((prev) => {
      if (!checked) return prev.filter((id) => id !== userId);
      if (prev.includes(userId)) return prev;
      if (prev.length >= MAX_GROUP_ADD_BATCH) {
        setMemberError(t(($) => $.groups.members_select_limit));
        return prev;
      }
      return [...prev, userId];
    });
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);

    const trimmedName = name.trim();
    if (trimmedName.length === 0) {
      setFormError(t(($) => $.groups.name_required));
      return;
    }
    if (trimmedName.length > GROUP_NAME_MAX_LENGTH) {
      setFormError(t(($) => $.groups.name_too_long));
      return;
    }
    if (description.length > GROUP_DESCRIPTION_MAX_LENGTH) {
      setFormError(t(($) => $.groups.description_too_long));
      return;
    }

    try {
      if (isEditing && activeGroup) {
        const metaChanged =
          trimmedName !== activeGroup.name ||
          description !== activeGroup.description;
        if (metaChanged) {
          await updateGroup.mutateAsync({
            name: trimmedName,
            description,
          });
        }

        const prevPerms = [...activeGroup.permissions].sort().join(",");
        const nextPerms = [...selectedPermissions].sort().join(",");
        if (prevPerms !== nextPerms) {
          await setGroupPermissions.mutateAsync(selectedPermissions);
        }

        toast.success(t(($) => $.groups.updated_toast));
        onClose();
        return;
      }

      setIsCreatingComposite(true);
      try {
        const created = await createGroup.mutateAsync({
          name: trimmedName,
          description,
        });
        setCreatedGroup(created);

        if (selectedPermissions.length > 0) {
          await api.setMemberGroupPermissions(
            wsId,
            created.id,
            selectedPermissions,
          );
          setCreatedGroup((prev) =>
            prev ? { ...prev, permissions: selectedPermissions } : prev,
          );
        }

        if (pendingUserIds.length > 0) {
          await api.addMemberGroupMembers(
            wsId,
            created.id,
            pendingUserIds.slice(0, MAX_GROUP_ADD_BATCH),
          );
          setPendingUserIds([]);
        }

        await qc.invalidateQueries({ queryKey: groupKeys.list(wsId) });
        await qc.invalidateQueries({ queryKey: groupKeys.myPermissions(wsId) });
        toast.success(t(($) => $.groups.created_toast));
        onClose();
      } finally {
        setIsCreatingComposite(false);
      }
    } catch (err) {
      setFormError(
        mapGroupError(err, t(($) => $.groups.errors.save_failed), t),
      );
    }
  };

  const handleAddSelectedMembers = async () => {
    if (!activeGroup || pendingUserIds.length === 0) return;
    setMemberError(null);
    try {
      await addMembers.mutateAsync(pendingUserIds.slice(0, MAX_GROUP_ADD_BATCH));
      setPendingUserIds([]);
      toast.success(t(($) => $.groups.members_added_toast));
    } catch (err) {
      setMemberError(
        mapGroupError(err, t(($) => $.groups.errors.members_failed), t),
      );
    }
  };

  const handleRemoveMember = async (userId: string) => {
    if (!activeGroup) return;
    setMemberError(null);
    try {
      await removeMember.mutateAsync(userId);
      toast.success(t(($) => $.groups.member_removed_toast));
    } catch (err) {
      setMemberError(
        mapGroupError(err, t(($) => $.groups.errors.members_failed), t),
      );
    }
  };

  return (
    <>
      <Dialog
        open
        onOpenChange={(nextOpen) => {
          if (!nextOpen && !isSaving && !deleteGroup.isPending) {
            onClose();
          }
        }}
      >
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {isEditing
                ? t(($) => $.groups.edit_title)
                : t(($) => $.groups.create_title)}
            </DialogTitle>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="space-y-1.5">
              <Label htmlFor="group-name-input">
                {t(($) => $.groups.name_label)}
              </Label>
              <Input
                id="group-name-input"
                value={name}
                maxLength={GROUP_NAME_MAX_LENGTH}
                placeholder={t(($) => $.groups.name_placeholder)}
                onChange={(e) => {
                  setName(e.target.value);
                  if (formError) setFormError(null);
                }}
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="group-description-input">
                {t(($) => $.groups.description_label)}
              </Label>
              <Textarea
                id="group-description-input"
                value={description}
                maxLength={GROUP_DESCRIPTION_MAX_LENGTH}
                rows={2}
                placeholder={t(($) => $.groups.description_placeholder)}
                onChange={(e) => {
                  setDescription(e.target.value);
                  if (formError) setFormError(null);
                }}
              />
            </div>

            <div className="space-y-2">
              <div>
                <Label>{t(($) => $.groups.permissions_label)}</Label>
                <p className="mt-0.5 text-caption text-muted-foreground">
                  {t(($) => $.groups.permissions_hint)}
                </p>
              </div>
              <div className="divide-y divide-surface-border rounded-lg border">
                {permissionItems.map((option) => {
                  const checked = selectedPermissions.includes(option.key);
                  const checkboxId = `group-perm-${option.key}`;
                  return (
                    <label
                      key={option.key}
                      htmlFor={checkboxId}
                      className="flex cursor-pointer items-start gap-3 px-3 py-2.5 hover:bg-muted/40"
                    >
                      <Checkbox
                        id={checkboxId}
                        checked={checked}
                        disabled={isSaving}
                        onCheckedChange={(val) =>
                          togglePermission(option.key, val === true)
                        }
                        className="mt-0.5"
                      />
                      <div className="min-w-0 flex-1">
                        <div className="text-body font-medium leading-5">
                          {t(($) => $.groups.permission_keys[option.labelKey])}
                        </div>
                        {option.descriptionKey ? (
                          <div className="text-caption leading-4 text-muted-foreground">
                            {t(($) =>
                              $.groups.permission_keys[option.descriptionKey!],
                            )}
                          </div>
                        ) : null}
                      </div>
                    </label>
                  );
                })}
              </div>
            </div>

            <div className="space-y-2.5">
              <div className="flex items-center justify-between gap-2">
                <Label>{t(($) => $.groups.members_label)}</Label>
                {isEditing && activeGroup ? (
                  <span className="inline-flex items-center gap-1 text-caption tabular-nums text-muted-foreground">
                    <Users className="size-3.5" aria-hidden="true" />
                    {detail?.members.length ?? activeGroup.member_count}
                  </span>
                ) : null}
              </div>

              {isEditing ? (
                detailLoading ? (
                  <div className="flex items-center gap-2 py-2 text-caption text-muted-foreground">
                    <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                    <span>{t(($) => $.groups.loading)}</span>
                  </div>
                ) : (detail?.members.length ?? 0) === 0 ? (
                  <p className="text-caption text-muted-foreground">
                    {t(($) => $.groups.members_empty)}
                  </p>
                ) : (
                  <div className="max-h-40 divide-y divide-surface-border overflow-y-auto rounded-lg border">
                    {(detail?.members ?? []).map((entry) => {
                      const member = membersById.get(entry.user_id);
                      const label = member?.name ?? entry.user_id;
                      return (
                        <div
                          key={entry.user_id}
                          className="flex items-center justify-between gap-2 px-3 py-2"
                        >
                          <div className="flex min-w-0 items-center gap-2">
                            <ActorAvatar
                              actorType="member"
                              actorId={entry.user_id}
                              size="sm"
                            />
                            <div className="min-w-0">
                              <div className="truncate text-body font-medium">
                                {label}
                              </div>
                              {member?.email ? (
                                <div className="truncate text-caption text-muted-foreground">
                                  {member.email}
                                </div>
                              ) : null}
                            </div>
                          </div>
                          <IconButton
                            type="button"
                            size="icon-xs"
                            variant="ghost"
                            label={t(($) => $.groups.members_remove_aria, {
                              name: label,
                            })}
                            disabled={removeMember.isPending}
                            onClick={() => void handleRemoveMember(entry.user_id)}
                          >
                            <Trash2 className="size-3.5 text-muted-foreground" />
                          </IconButton>
                        </div>
                      );
                    })}
                  </div>
                )
              ) : null}

              <div className="space-y-2 rounded-lg border p-2.5">
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <Input
                    type="search"
                    value={memberSearch}
                    onChange={(e) => setMemberSearch(e.target.value)}
                    placeholder={t(($) => $.groups.members_search_placeholder)}
                    className="h-8 pl-8 text-caption"
                  />
                </div>

                {candidateMembers.length === 0 ? (
                  <p className="px-1 py-1 text-caption text-muted-foreground">
                    {t(($) => $.groups.members_no_match)}
                  </p>
                ) : (
                  <div className="max-h-36 divide-y divide-surface-border overflow-y-auto">
                    {candidateMembers.map((candidate) => {
                      const selected = pendingUserIds.includes(candidate.user_id);
                      const limitReached =
                        !selected && pendingUserIds.length >= MAX_GROUP_ADD_BATCH;
                      const cbId = `group-add-member-${candidate.user_id}`;
                      return (
                        <label
                          key={candidate.user_id}
                          htmlFor={cbId}
                          className="flex cursor-pointer items-center gap-2.5 px-1.5 py-1.5 hover:bg-muted/40"
                        >
                          <Checkbox
                            id={cbId}
                            checked={selected}
                            disabled={addMembers.isPending || isSaving || limitReached}
                            onCheckedChange={(val) =>
                              togglePendingMember(candidate.user_id, val === true)
                            }
                          />
                          <ActorAvatar
                            actorType="member"
                            actorId={candidate.user_id}
                            size="sm"
                          />
                          <div className="min-w-0 flex-1">
                            <div className="truncate text-caption font-medium">
                              {candidate.name}
                            </div>
                            <div className="truncate text-caption text-muted-foreground">
                              {candidate.email}
                            </div>
                          </div>
                        </label>
                      );
                    })}
                  </div>
                )}

                {isEditing && pendingUserIds.length > 0 ? (
                  <div className="flex items-center justify-end">
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={addMembers.isPending}
                      aria-busy={addMembers.isPending}
                      onClick={() => void handleAddSelectedMembers()}
                    >
                      {addMembers.isPending ? (
                        <Loader2 className="animate-spin" aria-hidden="true" />
                      ) : null}
                      {t(($) => $.groups.members_add_button, {
                        count: pendingUserIds.length,
                      })}
                    </Button>
                  </div>
                ) : null}
              </div>

              {memberError ? (
                <p className="text-caption text-destructive" role="alert">
                  {memberError}
                </p>
              ) : null}
            </div>

            {formError ? (
              <p className="text-caption text-destructive" role="alert">
                {formError}
              </p>
            ) : null}

            <DialogFooter className="gap-2 sm:justify-between">
              {isEditing ? (
                <Button
                  type="button"
                  variant="destructive"
                  disabled={isSaving || deleteGroup.isPending}
                  onClick={() => setConfirmDeleteOpen(true)}
                >
                  {t(($) => $.groups.delete_button)}
                </Button>
              ) : (
                <span />
              )}
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={isSaving}
                  onClick={onClose}
                >
                  {t(($) => $.groups.cancel)}
                </Button>
                <Button
                  type="submit"
                  disabled={isSaving}
                  aria-busy={isSaving}
                >
                  {isSaving ? (
                    <Loader2 className="animate-spin" aria-hidden="true" />
                  ) : null}
                  {isSaving
                    ? isEditing
                      ? t(($) => $.groups.saving)
                      : t(($) => $.groups.creating)
                    : isEditing
                      ? t(($) => $.groups.save_button)
                      : t(($) => $.groups.create_submit)}
                </Button>
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {isEditing && activeGroup && confirmDeleteOpen ? (
        <DeleteGroupConfirmDialog
          group={activeGroup}
          deleteGroup={deleteGroup}
          onDeleted={onClose}
          onClose={() => setConfirmDeleteOpen(false)}
        />
      ) : null}
    </>
  );
}

function DeleteGroupConfirmDialog({
  group,
  deleteGroup,
  onDeleted,
  onClose,
}: {
  group: MemberGroup;
  deleteGroup: ReturnType<typeof useDeleteMemberGroup>;
  onDeleted: () => void;
  onClose: () => void;
}) {
  const { t } = useT("settings");
  const [error, setError] = useState<string | null>(null);

  const handleConfirm = async () => {
    setError(null);
    try {
      await deleteGroup.mutateAsync(group.id);
      toast.success(t(($) => $.groups.deleted_toast));
      onClose();
      onDeleted();
    } catch (err) {
      setError(
        mapGroupError(err, t(($) => $.groups.errors.delete_failed), t),
      );
    }
  };

  return (
    <AlertDialog
      open
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !deleteGroup.isPending) {
          onClose();
        }
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(($) => $.groups.delete_title, { name: group.name })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.groups.delete_description)}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error ? (
          <p className="text-caption text-destructive" role="alert">
            {error}
          </p>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleteGroup.isPending}>
            {t(($) => $.groups.cancel)}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={deleteGroup.isPending}
            onClick={(event) => {
              event.preventDefault();
              void handleConfirm();
            }}
          >
            {deleteGroup.isPending
              ? t(($) => $.groups.deleting)
              : t(($) => $.groups.delete_confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
