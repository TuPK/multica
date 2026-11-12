"use client";

import { useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Loader2,
  Megaphone,
  Plus,
  RefreshCw,
  Terminal,
  Trash2,
} from "@multica/ui/icons";
import { toast } from "sonner";
import { useWorkspaceId } from "@multica/core/hooks";
import { ApiError, errorCode } from "@multica/core/api";
import { useCurrentMember } from "@multica/core/permissions";
import {
  usePublishChannels,
  useCreatePublishChannel,
  useDeletePublishChannel,
  useConnectPublishChannel,
  useCheckPublishChannel,
  useDryRunPublishChannel,
  usePublishSettings,
  useSavePublishSettings,
} from "@multica/core/publish-channels";
import type { PublishChannel, SavePublishSettingsInput } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { useT, useTimeAgo } from "../../i18n";
import {
  SettingsCard,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

const NO_CHANNELS: PublishChannel[] = [];

function sanitizeError(msg: string, keys: string[]): string {
  let result = msg;
  for (const k of keys) {
    const trimmed = k.trim();
    if (trimmed.length > 0) {
      result = result.split(trimmed).join("[redacted]");
    }
  }
  return result;
}

const SUPPORTED_PLATFORMS = [
  { id: "x", name: "X (Twitter)" },
  { id: "linkedin", name: "LinkedIn" },
  { id: "facebook", name: "Facebook Pages" },
  { id: "tiktok", name: "TikTok" },
  { id: "youtube", name: "YouTube" },
  { id: "threads", name: "Threads" },
  { id: "bluesky", name: "Bluesky" },
] as const;

export function PublishChannelsTab() {
  const { t } = useT("settings");
  const timeAgo = useTimeAgo();
  const wsId = useWorkspaceId();
  const { member } = useCurrentMember(wsId);
  const canManage = member?.role === "owner" || member?.role === "admin";

  const channelsQuery = usePublishChannels(wsId);
  const createChannel = useCreatePublishChannel(wsId);
  const deleteChannel = useDeletePublishChannel(wsId);
  const connectChannel = useConnectPublishChannel(wsId);
  const checkChannel = useCheckPublishChannel(wsId);
  const dryRunChannel = useDryRunPublishChannel(wsId);

  const channels = channelsQuery.data ?? NO_CHANNELS;

  const settingsQuery = usePublishSettings(wsId);
  const saveSettings = useSavePublishSettings(wsId);
  const settings = settingsQuery.data;

  const [ayrshareKey, setAyrshareKey] = useState("");
  const [xKey, setXKey] = useState("");
  const [xSecret, setXSecret] = useState("");
  const [xExpanded, setXExpanded] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const handleSaveSettings = async () => {
    const payload: SavePublishSettingsInput = {};
    if (ayrshareKey.trim()) payload.ayrshare_api_key = ayrshareKey.trim();
    if (xKey.trim()) payload.x_api_key = xKey.trim();
    if (xSecret.trim()) payload.x_api_secret = xSecret.trim();

    if (Object.keys(payload).length === 0) return;

    setSaveError(null);
    try {
      await saveSettings.mutateAsync(payload);
      toast.success(t(($) => $.publish_channels.toast_settings_saved));
      setAyrshareKey("");
      setXKey("");
      setXSecret("");
    } catch (err) {
      let rawMessage = "";
      if (
        err instanceof ApiError &&
        err.body &&
        typeof err.body === "object" &&
        "error" in err.body &&
        typeof (err.body as { error: unknown }).error === "string"
      ) {
        rawMessage = (err.body as { error: string }).error;
      } else if (err instanceof Error && err.message) {
        rawMessage = err.message;
      } else {
        rawMessage = t(($) => $.publish_channels.toast_failed);
      }
      const sanitized = sanitizeError(rawMessage, [ayrshareKey, xKey, xSecret]);
      toast.error(sanitized);
      setSaveError(sanitized);
    }
  };

  const renderConfiguredBadge = (configured: boolean) => {
    if (configured) {
      return (
        <Badge
          variant="outline"
          className="border-emerald-600/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
        >
          <CheckCircle2 className="mr-1 size-3" />
          {t(($) => $.publish_channels.status_connected)}
        </Badge>
      );
    }
    return (
      <Badge variant="secondary">
        {t(($) => $.publish_channels.status_not_set)}
      </Badge>
    );
  };

  // Dialog states
  const [addOpen, setAddOpen] = useState(false);
  const [newPlatform, setNewPlatform] = useState<string>("x");
  const [newMarket, setNewMarket] = useState<string>("intl");
  const [newAccount, setNewAccount] = useState("");

  const [deletingChannel, setDeletingChannel] = useState<PublishChannel | null>(null);

  const [checkingId, setCheckingId] = useState<string | null>(null);
  const [connectingId, setConnectingId] = useState<string | null>(null);

  const [dryRunTarget, setDryRunTarget] = useState<PublishChannel | null>(null);
  const [dryRunText, setDryRunText] = useState("Multica Content Studio publish test");
  const [dryRunMedia, setDryRunMedia] = useState("");
  const [dryRunResult, setDryRunResult] = useState<Record<string, unknown> | null>(null);

  const handleOpenAdd = () => {
    setNewPlatform("x");
    setNewMarket("intl");
    setNewAccount("");
    setAddOpen(true);
  };

  const handleCreate = async () => {
    if (!newAccount.trim()) return;
    try {
      await createChannel.mutateAsync({
        platform: newPlatform,
        market: newMarket,
        account_name: newAccount.trim(),
      });
      toast.success(t(($) => $.publish_channels.toast_created));
      setAddOpen(false);
    } catch (err) {
      const code = errorCode(err);
      if (code === "platform_taken") {
        toast.error(t(($) => $.publish_channels.toast_platform_taken));
      } else {
        toast.error(t(($) => $.publish_channels.toast_failed));
      }
    }
  };

  const handleDelete = async () => {
    if (!deletingChannel) return;
    try {
      await deleteChannel.mutateAsync(deletingChannel.id);
      toast.success(t(($) => $.publish_channels.toast_deleted));
      setDeletingChannel(null);
    } catch {
      toast.error(t(($) => $.publish_channels.toast_failed));
    }
  };

  const handleConnect = async (channel: PublishChannel) => {
    setConnectingId(channel.id);
    try {
      const res = await connectChannel.mutateAsync(channel.id);
      if (res?.url) {
        if (typeof window !== "undefined") {
          window.open(res.url, "_blank", "noopener,noreferrer");
        }
      }
    } catch (err) {
      if (errorCode(err) === "ayrshare_not_configured") {
        toast.error(t(($) => $.publish_channels.toast_not_configured));
      } else {
        toast.error(t(($) => $.publish_channels.toast_failed));
      }
    } finally {
      setConnectingId(null);
    }
  };

  const handleCheck = async (channel: PublishChannel) => {
    setCheckingId(channel.id);
    try {
      await checkChannel.mutateAsync(channel.id);
      toast.success(t(($) => $.publish_channels.toast_checked));
    } catch {
      toast.error(t(($) => $.publish_channels.toast_failed));
    } finally {
      setCheckingId(null);
    }
  };

  const handleOpenDryRun = (channel: PublishChannel) => {
    setDryRunTarget(channel);
    setDryRunText("Multica Content Studio publish preview");
    setDryRunMedia("");
    setDryRunResult(null);
  };

  const handleExecuteDryRun = async () => {
    if (!dryRunTarget) return;
    try {
      const body = await dryRunChannel.mutateAsync({
        id: dryRunTarget.id,
        text: dryRunText,
        media_url: dryRunMedia.trim() || undefined,
      });
      setDryRunResult(body);
    } catch {
      toast.error(t(($) => $.publish_channels.toast_failed));
    }
  };

  const renderStatusBadge = (channel: PublishChannel) => {
    switch (channel.status) {
      case "ready":
        return (
          <Badge variant="outline" className="border-emerald-600/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="mr-1 size-3" />
            {t(($) => $.publish_channels.status_ready)}
          </Badge>
        );
      case "not_connected":
        return (
          <Badge variant="secondary">
            {t(($) => $.publish_channels.status_not_connected)}
          </Badge>
        );
      case "error":
        return (
          <Badge variant="destructive">
            <AlertCircle className="mr-1 size-3" />
            {t(($) => $.publish_channels.status_error)}
          </Badge>
        );
      case "expired":
        return (
          <Badge variant="outline" className="border-amber-600/30 bg-amber-500/10 text-amber-600 dark:text-amber-400">
            <AlertCircle className="mr-1 size-3" />
            {t(($) => $.publish_channels.status_expired)}
          </Badge>
        );
      default:
        return <Badge variant="secondary">{channel.status}</Badge>;
    }
  };

  const getPlatformLabel = (platform: string) => {
    const found = SUPPORTED_PLATFORMS.find((p) => p.id === platform);
    return found ? found.name : platform;
  };

  return (
    <SettingsTab title={t(($) => $.publish_channels.title)} scope="workspace">
      <SettingsSection title={t(($) => $.publish_channels.ayrshare_section_title)}>
        <SettingsCard>
          <div className="space-y-4 p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="space-y-0.5">
                <span className="text-body font-medium">
                  {t(($) => $.publish_channels.ayrshare_key_label)}
                </span>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.publish_channels.ayrshare_note)}
                </p>
              </div>
              {renderConfiguredBadge(Boolean(settings?.ayrshare_configured))}
            </div>

            <div>
              <Input
                type="password"
                autoComplete="off"
                spellCheck={false}
                aria-label={t(($) => $.publish_channels.ayrshare_key_label)}
                placeholder={t(($) => $.publish_channels.ayrshare_key_placeholder)}
                value={ayrshareKey}
                onChange={(e) => setAyrshareKey(e.target.value)}
                disabled={saveSettings.isPending}
                className="max-w-md"
              />
            </div>

            <div className="border-t border-surface-border pt-3">
              <button
                type="button"
                onClick={() => setXExpanded(!xExpanded)}
                className="flex w-full items-center justify-between py-1 text-left text-body font-medium hover:text-foreground"
                aria-expanded={xExpanded}
              >
                <div className="flex items-center gap-2">
                  {xExpanded ? (
                    <ChevronDown className="size-4 text-muted-foreground" aria-hidden="true" />
                  ) : (
                    <ChevronRight className="size-4 text-muted-foreground" aria-hidden="true" />
                  )}
                  <span>{t(($) => $.publish_channels.x_section_title)}</span>
                </div>
                {renderConfiguredBadge(Boolean(settings?.x_configured))}
              </button>

              {xExpanded ? (
                <div className="mt-3 space-y-3 pl-6">
                  <div className="space-y-1.5">
                    <label className="text-caption font-medium text-muted-foreground">
                      {t(($) => $.publish_channels.x_key_label)}
                    </label>
                    <Input
                      type="password"
                      autoComplete="off"
                      spellCheck={false}
                      aria-label={t(($) => $.publish_channels.x_key_label)}
                      placeholder={t(($) => $.publish_channels.x_key_placeholder)}
                      value={xKey}
                      onChange={(e) => setXKey(e.target.value)}
                      disabled={saveSettings.isPending}
                      className="max-w-md"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-caption font-medium text-muted-foreground">
                      {t(($) => $.publish_channels.x_secret_label)}
                    </label>
                    <Input
                      type="password"
                      autoComplete="off"
                      spellCheck={false}
                      aria-label={t(($) => $.publish_channels.x_secret_label)}
                      placeholder={t(($) => $.publish_channels.x_secret_placeholder)}
                      value={xSecret}
                      onChange={(e) => setXSecret(e.target.value)}
                      disabled={saveSettings.isPending}
                      className="max-w-md"
                    />
                  </div>
                </div>
              ) : null}
            </div>

            {saveError ? (
              <p className="text-caption text-destructive" role="alert">
                {saveError}
              </p>
            ) : null}

            <div className="flex items-center gap-2 pt-1">
              <Button
                size="sm"
                disabled={
                  saveSettings.isPending ||
                  (!ayrshareKey.trim() && !xKey.trim() && !xSecret.trim())
                }
                onClick={handleSaveSettings}
              >
                {saveSettings.isPending ? (
                  <Loader2 className="mr-1.5 size-4 animate-spin" />
                ) : null}
                {t(($) => $.publish_channels.save)}
              </Button>
            </div>
          </div>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.publish_channels.title)}
        action={
          canManage && channels.length > 0 ? (
            <Button size="sm" onClick={handleOpenAdd}>
              <Plus className="mr-1.5 size-4" />
              {t(($) => $.publish_channels.add_channel)}
            </Button>
          ) : undefined
        }
      >
        <SettingsCard>
          {channels.length === 0 ? (
            <div className="flex flex-col items-center justify-center px-4 py-12 text-center">
              <span className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
                <Megaphone className="size-6" aria-hidden="true" />
              </span>
              <h3 className="mt-3 text-heading font-medium">
                {t(($) => $.publish_channels.empty_title)}
              </h3>
              <p className="mt-1 max-w-sm text-caption text-muted-foreground">
                {t(($) => $.publish_channels.empty_description)}
              </p>
              {canManage ? (
                <Button className="mt-4" onClick={handleOpenAdd}>
                  <Plus className="mr-1.5 size-4" />
                  {t(($) => $.publish_channels.add_channel)}
                </Button>
              ) : null}
            </div>
          ) : (
            <ul className="divide-y divide-surface-border">
              {channels.map((channel) => {
                const isChecking = checkingId === channel.id;
                const isConnecting = connectingId === channel.id;
                return (
                  <li
                    key={channel.id}
                    data-testid={`channel-row-${channel.id}`}
                    className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold text-body">
                          {getPlatformLabel(channel.platform)}
                        </span>
                        <span className="text-body text-muted-foreground">
                          {channel.account_name}
                        </span>
                        <Badge variant="outline" className="text-micro font-medium uppercase tracking-wider">
                          {channel.market === "vn"
                            ? t(($) => $.publish_channels.market_vn)
                            : t(($) => $.publish_channels.market_intl)}
                        </Badge>
                        {renderStatusBadge(channel)}
                      </div>

                      {channel.status_detail ? (
                        <p className="text-caption text-destructive">
                          {channel.status_detail}
                        </p>
                      ) : null}

                      <div className="flex flex-wrap items-center gap-3 text-caption text-muted-foreground">
                        {channel.last_checked_at ? (
                          <span>
                            {t(($) => $.publish_channels.last_checked, {
                              time: timeAgo(channel.last_checked_at),
                            })}
                          </span>
                        ) : null}
                        {channel.limits.max_chars > 0 ? (
                          <span>
                            {t(($) => $.publish_channels.limit_chars, {
                              count: channel.limits.max_chars,
                            })}
                          </span>
                        ) : null}
                        {channel.limits.aspect ? (
                          <span>
                            {t(($) => $.publish_channels.limit_aspect, {
                              ratio: channel.limits.aspect,
                            })}
                          </span>
                        ) : null}
                      </div>
                    </div>

                    <div className="flex flex-wrap items-center gap-2 sm:shrink-0">
                      {(channel.status === "not_connected" ||
                        channel.status === "error" ||
                        channel.status === "expired") && canManage ? (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleConnect(channel)}
                          disabled={isConnecting}
                        >
                          {isConnecting ? (
                            <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                          ) : (
                            <ExternalLink className="mr-1.5 size-3.5" />
                          )}
                          {t(($) => $.publish_channels.connect)}
                        </Button>
                      ) : null}

                      {canManage ? (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleCheck(channel)}
                          disabled={isChecking}
                        >
                          {isChecking ? (
                            <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                          ) : (
                            <RefreshCw className="mr-1.5 size-3.5" />
                          )}
                          {t(($) => $.publish_channels.check_status)}
                        </Button>
                      ) : null}

                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleOpenDryRun(channel)}
                      >
                        <Terminal className="mr-1.5 size-3.5" />
                        {t(($) => $.publish_channels.dry_run)}
                      </Button>

                      {canManage ? (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => setDeletingChannel(channel)}
                          aria-label={t(($) => $.publish_channels.delete)}
                        >
                          <Trash2 className="size-4 text-muted-foreground hover:text-destructive" />
                        </Button>
                      ) : null}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </SettingsCard>
      </SettingsSection>

      {/* Add Channel Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(($) => $.publish_channels.add_title)}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <label htmlFor="publish-channel-platform" className="text-caption font-medium">
                {t(($) => $.publish_channels.platform)}
              </label>
              <select
                id="publish-channel-platform"
                value={newPlatform}
                onChange={(e) => setNewPlatform(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-body shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                {SUPPORTED_PLATFORMS.map((platform) => (
                  <option key={platform.id} value={platform.id}>
                    {platform.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <label htmlFor="publish-channel-market" className="text-caption font-medium">
                {t(($) => $.publish_channels.market)}
              </label>
              <select
                id="publish-channel-market"
                value={newMarket}
                onChange={(e) => setNewMarket(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-body shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                <option value="intl">{t(($) => $.publish_channels.market_intl)}</option>
                <option value="vn">{t(($) => $.publish_channels.market_vn)}</option>
              </select>
            </div>

            <div className="space-y-1.5">
              <label htmlFor="publish-channel-account" className="text-caption font-medium">
                {t(($) => $.publish_channels.account_name)}
              </label>
              <Input
                id="publish-channel-account"
                placeholder={t(($) => $.publish_channels.account_placeholder)}
                value={newAccount}
                onChange={(e) => setNewAccount(e.target.value)}
              />
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setAddOpen(false)}
              disabled={createChannel.isPending}
            >
              {t(($) => $.publish_channels.cancel)}
            </Button>
            <Button
              type="button"
              onClick={handleCreate}
              disabled={!newAccount.trim() || createChannel.isPending}
            >
              {createChannel.isPending ? (
                <Loader2 className="mr-1.5 size-3.5 animate-spin" />
              ) : null}
              {t(($) => $.publish_channels.submit_add)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Dry Run Dialog */}
      <Dialog
        open={dryRunTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDryRunTarget(null);
            setDryRunResult(null);
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t(($) => $.publish_channels.dry_run_title)}</DialogTitle>
            <DialogDescription>
              {dryRunTarget ? `${getPlatformLabel(dryRunTarget.platform)} (${dryRunTarget.account_name})` : ""}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <label htmlFor="dry-run-text" className="text-caption font-medium">
                {t(($) => $.publish_channels.dry_run_text_label)}
              </label>
              <Textarea
                id="dry-run-text"
                rows={3}
                value={dryRunText}
                onChange={(e) => setDryRunText(e.target.value)}
              />
            </div>

            <div className="space-y-1.5">
              <label htmlFor="dry-run-media" className="text-caption font-medium">
                {t(($) => $.publish_channels.dry_run_media_label)}
              </label>
              <Input
                id="dry-run-media"
                placeholder="https://example.com/image.jpg"
                value={dryRunMedia}
                onChange={(e) => setDryRunMedia(e.target.value)}
              />
            </div>

            <div className="flex justify-end">
              <Button
                size="sm"
                onClick={handleExecuteDryRun}
                disabled={dryRunChannel.isPending}
              >
                {dryRunChannel.isPending ? (
                  <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                ) : (
                  <Terminal className="mr-1.5 size-3.5" />
                )}
                {t(($) => $.publish_channels.dry_run_submit)}
              </Button>
            </div>

            {dryRunResult ? (
              <div className="space-y-1.5">
                <label className="text-caption font-medium">
                  {t(($) => $.publish_channels.dry_run_payload_label)}
                </label>
                <pre
                  data-testid="dry-run-json-output"
                  className="max-h-60 overflow-x-auto rounded-md border border-surface-border bg-muted p-3 font-mono text-xs text-foreground"
                >
                  <code>{JSON.stringify(dryRunResult, null, 2)}</code>
                </pre>
              </div>
            ) : null}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setDryRunTarget(null);
                setDryRunResult(null);
              }}
            >
              {t(($) => $.publish_channels.cancel)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Alert Dialog */}
      <AlertDialog
        open={deletingChannel !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingChannel(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.publish_channels.delete_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.publish_channels.delete_description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteChannel.isPending}>
              {t(($) => $.publish_channels.cancel)}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void handleDelete();
              }}
              disabled={deleteChannel.isPending}
            >
              {deleteChannel.isPending ? (
                <Loader2 className="mr-1.5 size-3.5 animate-spin" />
              ) : null}
              {t(($) => $.publish_channels.delete_confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsTab>
  );
}
