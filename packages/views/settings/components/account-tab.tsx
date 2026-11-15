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
import { downloadAccountExport, useRevokeAllSessions } from "@multica/core/account";
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
    </SettingsTab>
  );
}
