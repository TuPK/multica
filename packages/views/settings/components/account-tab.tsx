"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Loader2 } from "@multica/ui/icons";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import {
  downloadAccountExport,
  usePreviewAccountDeletion,
  useRequestAccountDeletion,
  useRevokeAllSessions,
} from "@multica/core/account";
import { useLogout } from "../../auth";
import { saveBlob } from "../../governance/save-blob";
import { securityEventsOptions } from "@multica/core/audit";
import { AvatarUploadControl } from "../../common/avatar-upload-control";
import { useLocale, useT } from "../../i18n";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";
import { useAutoSave } from "./use-auto-save";

// Mirror server/internal/handler/auth.go:MaxProfileDescriptionLen. Counted in
// JS String.length (UTF-16 code units) here while the server counts runes,
// so a profile full of supplementary-plane emoji will trip the client cap
// before the server's — which is the safer direction of drift.
const MAX_PROFILE_DESCRIPTION_LEN = 2000;

interface ProfileDraft {
  name: string;
  profileDescription: string;
}

function profilesEqual(left: ProfileDraft, right: ProfileDraft) {
  return left.name === right.name && left.profileDescription === right.profileDescription;
}

export function AccountTab() {
  const { t } = useT("settings");
  const locale = useLocale();
  const user = useAuthStore((s) => s.user);
  const setUser = useAuthStore((s) => s.setUser);

  const [profileName, setProfileName] = useState(user?.name ?? "");
  const [profileDescription, setProfileDescription] = useState(
    user?.profile_description ?? "",
  );
  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [isExporting, setIsExporting] = useState(false);
  const revokeAllSessions = useRevokeAllSessions();
  const isRevoking = Boolean(revokeAllSessions?.isPending);

  const logout = useLogout();
  const previewAccountDeletion = usePreviewAccountDeletion();
  const requestAccountDeletion = useRequestAccountDeletion();
  const isDeleting = Boolean(requestAccountDeletion?.isPending);

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [soleWorkspaces, setSoleWorkspaces] = useState<Array<{ id: string; name: string }>>([]);
  const [soleOwnerWorkspaces, setSoleOwnerWorkspaces] = useState<
    Array<{ id: string; name: string; member_count?: number }>
  >([]);
  const [isSoleOwnerBlocked, setIsSoleOwnerBlocked] = useState(false);
  const [isRecentLoginRequired, setIsRecentLoginRequired] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [confirmEmail, setConfirmEmail] = useState("");
  const [deletionSuccessDate, setDeletionSuccessDate] = useState<string | null>(null);

  const securityEventsQuery = useQuery({
    ...securityEventsOptions(user?.id ?? "", 20),
  });
  const securityEvents = useMemo(
    () => (securityEventsQuery.data?.events ?? []).slice(0, 20),
    [securityEventsQuery.data?.events],
  );

  const formatSecurityEventTime = (value: string) => {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  };

  const handleOpenChange = (open: boolean) => {
    if (!open && isRevoking) return;
    setRevokeDialogOpen(open);
  };

  const handleRevokeAllSessions = async () => {
    try {
      await revokeAllSessions.mutateAsync();
      setRevokeDialogOpen(false);
      toast.success(t(($) => $.account.toast_revoke_all_sessions_success));
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t(($) => $.account.toast_revoke_all_sessions_failed),
      );
    }
  };

  const handleDownloadExport = async () => {
    if (isExporting) return;
    setIsExporting(true);
    try {
      const data = await downloadAccountExport();
      const blob =
        data instanceof Blob
          ? data
          : new Blob(
              [
                typeof data === "string"
                  ? data
                  : JSON.stringify(data, null, 2),
              ],
              {
                type: "application/json",
              },
            );
      saveBlob(blob, "multica-account-export.json");
      toast.success(t(($) => $.account.toast_export_success));
    } catch (error: unknown) {
      const err = error as { code?: string; status?: number };
      if (err?.code === "export_rate_limited" || err?.status === 429) {
        toast.error(t(($) => $.account.toast_export_rate_limited));
      } else {
        toast.error(
          error instanceof Error
            ? error.message
            : t(($) => $.account.toast_export_failed),
        );
      }
    } finally {
      setIsExporting(false);
    }
  };

  const formatDeletionDate = (value: string) => {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  };

  const handleOpenDeleteDialog = async () => {
    setDeleteDialogOpen(true);
    setPreviewLoading(true);
    setIsSoleOwnerBlocked(false);
    setIsRecentLoginRequired(false);
    setPreviewError(null);
    setConfirmEmail("");
    setDeletionSuccessDate(null);
    setSoleWorkspaces([]);
    setSoleOwnerWorkspaces([]);

    try {
      const res = await previewAccountDeletion.mutateAsync();
      setSoleWorkspaces(res?.sole_workspaces ?? []);
    } catch (error: unknown) {
      const err = error as {
        code?: string;
        workspaces?: Array<{ id: string; name: string; member_count?: number }>;
        message?: string;
      };
      if (err?.code === "sole_owner") {
        setIsSoleOwnerBlocked(true);
        setSoleOwnerWorkspaces(err.workspaces ?? []);
      } else if (err?.code === "recent_login_required") {
        setIsRecentLoginRequired(true);
      } else {
        setPreviewError(
          err?.message || t(($) => $.account.delete_account_dialog_description),
        );
      }
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleDeleteOpenChange = (open: boolean) => {
    if (!open && isDeleting) return;
    if (!open && deletionSuccessDate) {
      setDeleteDialogOpen(false);
      logout();
      return;
    }
    setDeleteDialogOpen(open);
  };

  const handleRequestDelete = async () => {
    if (isDeleting || !user?.email || confirmEmail !== user.email) return;
    try {
      const res = await requestAccountDeletion.mutateAsync();
      setDeletionSuccessDate(res.scheduled_at);
    } catch (error: unknown) {
      const err = error as {
        code?: string;
        workspaces?: Array<{ id: string; name: string; member_count?: number }>;
        message?: string;
      };
      if (err?.code === "recent_login_required") {
        setIsRecentLoginRequired(true);
      } else if (err?.code === "sole_owner") {
        setIsSoleOwnerBlocked(true);
        setSoleOwnerWorkspaces(err.workspaces ?? []);
      } else {
        toast.error(
          error instanceof Error
            ? error.message
            : t(($) => $.account.delete_account_dialog_description),
        );
      }
    }
  };

  const handleSuccessSignOut = () => {
    setDeleteDialogOpen(false);
    logout();
  };

  useEffect(() => {
    setProfileName(user?.name ?? "");
    setProfileDescription(user?.profile_description ?? "");
    // Preserve in-progress edits when an avatar upload or auto-save replaces
    // the current user object in the auth store.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally keyed on user identity
  }, [user?.id]);

  const descriptionTooLong = profileDescription.length > MAX_PROFILE_DESCRIPTION_LEN;

  const draft = useMemo(
    () => ({ name: profileName, profileDescription }),
    [profileDescription, profileName],
  );
  const savedDraft = useMemo(
    () => ({
      name: user?.name ?? "",
      profileDescription: user?.profile_description ?? "",
    }),
    [user?.name, user?.profile_description],
  );
  const saveProfile = useCallback(
    async (next: ProfileDraft) => {
      const updated = await api.updateMe({
        name: next.name,
        profile_description: next.profileDescription,
      });
      setUser(updated);
    },
    [setUser],
  );
  const autoSave = useAutoSave({
    value: draft,
    savedValue: savedDraft,
    onSave: saveProfile,
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(($) => $.account.toast_profile_failed),
      ),
    enabled: !!user && !!profileName.trim() && !descriptionTooLong,
    isEqual: profilesEqual,
  });

  return (
    <SettingsTab title={t(($) => $.page.tabs.profile)} scope="account">
      <SettingsSection
        title={t(($) => $.account.section_profile)}
        action={
          <SettingsSaveState
            status={autoSave.status}
            savingLabel={t(($) => $.auto_save.saving)}
            savedLabel={t(($) => $.auto_save.saved)}
            errorLabel={t(($) => $.auto_save.failed)}
          />
        }
      >
        <SettingsCard>
          <SettingsRow
            anchor="avatar"
            label={t(($) => $.account.avatar_label)}
            description={t(($) => $.account.click_avatar_hint)}
            size="none"
          >
            <div className="flex justify-start sm:justify-end">
              <AvatarUploadControl
                variant="user"
                value={user?.avatar_url ?? null}
                name={user?.name ?? ""}
                size={64}
                onUploaded={async (url) => {
                  try {
                    const updated = await api.updateMe({ avatar_url: url });
                    setUser(updated);
                  } catch (error) {
                    toast.error(
                      error instanceof Error
                        ? error.message
                        : t(($) => $.account.toast_avatar_failed),
                    );
                  }
                }}
              />
            </div>
          </SettingsRow>

          <SettingsRow
            anchor="name"
            label={t(($) => $.account.name_label)}
            size="text"
          >
            <Input
              type="text"
              name="profile-name"
              autoComplete="name"
              aria-label={t(($) => $.account.name_label)}
              value={profileName}
              onChange={(event) => setProfileName(event.target.value)}
              onBlur={autoSave.flush}
            />
          </SettingsRow>

          <SettingsRow
            anchor="about"
            label={t(($) => $.account.profile_description_label)}
            description={t(($) => $.account.profile_description_hint)}
            size="text"
            align="start"
          >
            <div>
              <Textarea
                name="profile-description"
                autoComplete="off"
                aria-label={t(($) => $.account.profile_description_label)}
                value={profileDescription}
                onChange={(event) => setProfileDescription(event.target.value)}
                onBlur={autoSave.flush}
                placeholder={t(($) => $.account.profile_description_placeholder)}
                rows={5}
                maxLength={MAX_PROFILE_DESCRIPTION_LEN}
                aria-invalid={descriptionTooLong}
                className="resize-y"
              />
              <div className="mt-1 flex justify-end text-caption text-muted-foreground">
                <span
                  className={descriptionTooLong ? "text-destructive shrink-0" : "shrink-0"}
                  aria-live="polite"
                >
                  {profileDescription.length}/{MAX_PROFILE_DESCRIPTION_LEN}
                </span>
              </div>
              {descriptionTooLong ? (
                <p className="mt-1 text-caption text-destructive">
                  {t(($) => $.account.profile_description_too_long, {
                    max: MAX_PROFILE_DESCRIPTION_LEN,
                    count: profileDescription.length,
                  })}
                </p>
              ) : null}
            </div>
          </SettingsRow>

          <SettingsRow
            anchor="export-data"
            label={t(($) => $.account.export_title)}
            description={t(($) => $.account.export_description)}
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleDownloadExport}
              disabled={isExporting}
              aria-busy={isExporting || undefined}
            >
              {isExporting ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              {t(($) => $.account.export_button)}
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.account.section_sessions)}
        anchor="sessions"
      >
        <SettingsCard>
          <SettingsRow
            anchor="revoke-all-sessions"
            label={t(($) => $.account.revoke_all_sessions_title)}
            description={t(($) => $.account.revoke_all_sessions_description)}
          >
            <Dialog open={revokeDialogOpen} onOpenChange={handleOpenChange}>
              <DialogTrigger render={<Button variant="outline" size="sm" />}>
                {t(($) => $.account.revoke_all_sessions_button)}
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>
                    {t(($) => $.account.revoke_all_sessions_title)}
                  </DialogTitle>
                  <DialogDescription>
                    {t(($) => $.account.revoke_all_sessions_dialog_description)}
                  </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                  <DialogClose
                    render={
                      <Button
                        type="button"
                        variant="outline"
                        disabled={isRevoking}
                      />
                    }
                  >
                    {t(($) => $.account.revoke_all_sessions_cancel)}
                  </DialogClose>
                  <Button
                    type="button"
                    variant="destructive"
                    onClick={handleRevokeAllSessions}
                    disabled={isRevoking}
                    aria-busy={isRevoking || undefined}
                  >
                    {isRevoking ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : null}
                    {isRevoking
                      ? t(($) => $.account.revoke_all_sessions_submitting)
                      : t(($) => $.account.revoke_all_sessions_confirm)}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.account.section_security_events)}
        anchor="security-events"
      >
        <SettingsCard>
          {securityEventsQuery.isError ? (
            <div className="px-4 py-3 text-caption text-destructive">
              {t(($) => $.account.security_events_load_failed)}
            </div>
          ) : securityEventsQuery.isLoading ? (
            <div className="flex justify-center py-6">
              <Loader2 className="size-4 animate-spin text-muted-foreground" />
            </div>
          ) : securityEvents.length === 0 ? (
            <div className="px-4 py-6 text-center text-body text-muted-foreground">
              {t(($) => $.account.security_events_empty)}
            </div>
          ) : (
            <ul className="divide-y divide-surface-border">
              {securityEvents.map((event) => (
                <li
                  key={event.id}
                  className="flex items-center justify-between px-4 py-3 text-body"
                >
                  <code className="font-mono text-caption text-foreground">
                    {event.action}
                  </code>
                  <span className="text-caption text-muted-foreground">
                    {formatSecurityEventTime(event.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.account.section_delete)}
        anchor="delete-account"
      >
        <SettingsCard>
          <SettingsRow
            anchor="delete-account"
            label={
              <span className="text-destructive">
                {t(($) => $.account.delete_account_title)}
              </span>
            }
            description={t(($) => $.account.delete_account_description)}
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleOpenDeleteDialog}
            >
              {t(($) => $.account.delete_account_button)}
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <Dialog open={deleteDialogOpen} onOpenChange={handleDeleteOpenChange}>
        <DialogContent>
          {previewLoading ? (
            <div className="flex flex-col items-center justify-center py-6 gap-2">
              <Loader2 className="size-6 animate-spin text-muted-foreground" />
              <p className="text-caption text-muted-foreground">
                {t(($) => $.account.delete_account_preview_loading)}
              </p>
            </div>
          ) : deletionSuccessDate ? (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.account.delete_account_success_title)}
                </DialogTitle>
                <DialogDescription>
                  {t(($) => $.account.delete_account_success_description, {
                    date: formatDeletionDate(deletionSuccessDate),
                  })}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button type="button" onClick={handleSuccessSignOut}>
                  {t(($) => $.account.delete_account_success_sign_out)}
                </Button>
              </DialogFooter>
            </>
          ) : isRecentLoginRequired ? (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.account.delete_account_recent_login_title)}
                </DialogTitle>
                <DialogDescription>
                  {t(($) => $.account.delete_account_recent_login_description)}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setDeleteDialogOpen(false)}
                    />
                  }
                >
                  {t(($) => $.account.delete_account_cancel)}
                </DialogClose>
                <Button type="button" variant="outline" onClick={logout}>
                  {t(($) => $.account.delete_account_recent_login_sign_out)}
                </Button>
              </DialogFooter>
            </>
          ) : isSoleOwnerBlocked ? (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.account.delete_account_sole_owner_title)}
                </DialogTitle>
                <DialogDescription>
                  {t(($) => $.account.delete_account_sole_owner_description)}
                </DialogDescription>
              </DialogHeader>
              {soleOwnerWorkspaces.length > 0 && (
                <ul className="max-h-48 overflow-y-auto rounded-md border border-surface-border p-2 space-y-1 text-body">
                  {soleOwnerWorkspaces.map((ws) => (
                    <li key={ws.id} className="text-body font-medium">
                      {ws.name}
                    </li>
                  ))}
                </ul>
              )}
              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setDeleteDialogOpen(false)}
                    />
                  }
                >
                  {t(($) => $.account.delete_account_close)}
                </DialogClose>
              </DialogFooter>
            </>
          ) : previewError ? (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.account.delete_account_dialog_title)}
                </DialogTitle>
                <DialogDescription className="text-destructive">
                  {previewError}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setDeleteDialogOpen(false)}
                    />
                  }
                >
                  {t(($) => $.account.delete_account_close)}
                </DialogClose>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(($) => $.account.delete_account_dialog_title)}
                </DialogTitle>
                <DialogDescription>
                  {t(($) => $.account.delete_account_dialog_description)}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-2">
                {soleWorkspaces.length > 0 && (
                  <div className="space-y-2">
                    <p className="text-body text-muted-foreground">
                      {t(($) => $.account.delete_account_sole_workspaces_title)}
                    </p>
                    <ul className="max-h-40 overflow-y-auto rounded-md border border-surface-border p-2 space-y-1 text-body">
                      {soleWorkspaces.map((ws) => (
                        <li key={ws.id} className="font-medium">
                          {ws.name}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                <div className="space-y-1.5">
                  <label
                    htmlFor="confirm-delete-email"
                    className="text-caption font-medium text-foreground"
                  >
                    {t(($) => $.account.delete_account_confirm_prompt, {
                      email: user?.email ?? "",
                    })}
                  </label>
                  <Input
                    id="confirm-delete-email"
                    type="email"
                    value={confirmEmail}
                    onChange={(e) => setConfirmEmail(e.target.value)}
                    placeholder={t(($) => $.account.delete_account_confirm_placeholder)}
                    autoComplete="off"
                  />
                </div>
              </div>
              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      disabled={isDeleting}
                    />
                  }
                >
                  {t(($) => $.account.delete_account_cancel)}
                </DialogClose>
                <Button
                  type="button"
                  variant="destructive"
                  onClick={handleRequestDelete}
                  disabled={
                    isDeleting ||
                    !user?.email ||
                    confirmEmail !== user.email
                  }
                  aria-busy={isDeleting || undefined}
                >
                  {isDeleting ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : null}
                  {isDeleting
                    ? t(($) => $.account.delete_account_deleting)
                    : t(($) => $.account.delete_account_confirm)}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </SettingsTab>
  );
}
