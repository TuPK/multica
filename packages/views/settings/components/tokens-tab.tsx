"use client";

import { useEffect, useState, useCallback, useId, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Trash2, Info } from "@multica/ui/icons";
import { CopyCheckIcon } from "@multica/ui/components/common/copy-check-icon";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import type {
  CreatePersonalAccessTokenRequest,
  PersonalAccessToken,
} from "@multica/core/types";
import { workspaceListOptions } from "@multica/core/workspace/queries";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Button } from "@multica/ui/components/ui/button";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
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
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { copyText } from "@multica/ui/lib/clipboard";
import { toast } from "sonner";
import { ApiError, api, errorCode } from "@multica/core/api";
import { useLocale, useT } from "../../i18n";
import { SettingsCard, SettingsSection, SettingsTab } from "./settings-layout";

const EXPIRY_KEYS = ["30", "90", "365", "never"] as const;
const ANY_WORKSPACE = "__any__";

export function TokensTab() {
  const { t } = useT("settings");
  const locale = useLocale();
  const baseId = useId();
  const { data: workspaces = [] } = useQuery(workspaceListOptions());

  const expiryItems = EXPIRY_KEYS.map((value) => ({
    value,
    label: t(($) => $.tokens.expiry[value]),
  }));
  const workspaceItems = useMemo(
    () => [
      { value: ANY_WORKSPACE, label: t(($) => $.tokens.workspace_any) },
      ...workspaces.map((ws) => ({ value: ws.id, label: ws.name })),
    ],
    [t, workspaces],
  );
  const workspaceNamesById = useMemo(
    () => new Map(workspaces.map((ws) => [ws.id, ws.name])),
    [workspaces],
  );

  const [tokens, setTokens] = useState<PersonalAccessToken[]>([]);
  const [tokenName, setTokenName] = useState("");
  const [tokenExpiry, setTokenExpiry] = useState("90");
  const [workspaceId, setWorkspaceId] = useState(ANY_WORKSPACE);
  const [readOnly, setReadOnly] = useState(false);
  const [tokenCreating, setTokenCreating] = useState(false);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);
  const [commandCopied, setCommandCopied] = useState(false);
  const [storedConfirmed, setStoredConfirmed] = useState(false);
  const [tokenRevoking, setTokenRevoking] = useState<string | null>(null);
  const [revokeConfirmId, setRevokeConfirmId] = useState<string | null>(null);
  const [tokensLoading, setTokensLoading] = useState(true);
  const [tokensLoadFailed, setTokensLoadFailed] = useState(false);

  const loadTokens = useCallback(async () => {
    try {
      const list = await api.listPersonalAccessTokens();
      setTokens(list);
      setTokensLoadFailed(false);
    } catch (e) {
      setTokensLoadFailed(true);
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_load_failed));
    } finally {
      setTokensLoading(false);
    }
  }, [t]);

  useEffect(() => { loadTokens(); }, [loadTokens]);

  const handleCreateToken = async () => {
    setTokenCreating(true);
    try {
      const expiresInDays = tokenExpiry === "never" ? undefined : Number(tokenExpiry);
      const payload: CreatePersonalAccessTokenRequest = {
        name: tokenName,
        ...(expiresInDays !== undefined ? { expires_in_days: expiresInDays } : {}),
        ...(workspaceId !== ANY_WORKSPACE ? { workspace_id: workspaceId } : {}),
        ...(readOnly ? { read_only: true } : {}),
      };
      const result = await api.createPersonalAccessToken(payload);
      setNewToken(result.token);
      setTokenName("");
      setTokenExpiry("90");
      setWorkspaceId(ANY_WORKSPACE);
      setReadOnly(false);
      await loadTokens();
    } catch (e) {
      if (
        errorCode(e) === "viewer_read_only" ||
        (e instanceof ApiError && e.status === 403)
      ) {
        toast.error(t(($) => $.tokens.toast_forbidden));
      } else {
        toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_create_failed));
      }
    } finally {
      setTokenCreating(false);
    }
  };

  const handleRevokeToken = async (id: string) => {
    setTokenRevoking(id);
    try {
      await api.revokePersonalAccessToken(id);
      await loadTokens();
      toast.success(t(($) => $.tokens.toast_revoked));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_revoke_failed));
    } finally {
      setTokenRevoking(null);
    }
  };

  const handleCopyToken = async () => {
    if (!newToken) return;
    if (await copyText(newToken)) {
      setTokenCopied(true);
      setTimeout(() => setTokenCopied(false), 2000);
    }
  };

  const handleCopyCommand = async () => {
    if (!newToken) return;
    if (await copyText(`multica login --token ${newToken}`)) {
      setCommandCopied(true);
      setTimeout(() => setCommandCopied(false), 2000);
    }
  };

  const closeCreatedDialog = () => {
    setNewToken(null);
    setTokenCopied(false);
    setCommandCopied(false);
    setStoredConfirmed(false);
  };

  const readOnlyId = `${baseId}-read-only`;

  return (
    <SettingsTab title={t(($) => $.tokens.title)} description={t(($) => $.tokens.purpose)} scope="account">
      <SettingsSection
        description={t(($) => $.tokens.security_note)}
      >
        <p className="text-caption text-muted-foreground">
          {t(($) => $.tokens.description)}
        </p>
        <SettingsCard>
          <div className="space-y-3 p-4">
            <div className="grid gap-3 sm:grid-cols-[1fr_120px_auto]">
              <Input
                type="text"
                name="token-name"
                autoComplete="off"
                aria-label={t(($) => $.tokens.name_placeholder)}
                value={tokenName}
                onChange={(e) => setTokenName(e.target.value)}
                placeholder={t(($) => $.tokens.name_placeholder)}
              />
              <Select
                items={expiryItems}
                value={tokenExpiry}
                onValueChange={(v) => { if (v) setTokenExpiry(v); }}
              >
                <SelectTrigger
                  size="sm"
                  aria-label={t(($) => $.tokens.title)}
                ><SelectValue /></SelectTrigger>
                <SelectContent>
                  {expiryItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button onClick={handleCreateToken} disabled={tokenCreating || !tokenName.trim()}>
                {tokenCreating ? t(($) => $.tokens.creating) : t(($) => $.tokens.create)}
              </Button>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
              <div className="flex items-center gap-2">
                <span className="text-caption text-muted-foreground">
                  {t(($) => $.tokens.workspace_label)}
                </span>
                <Select
                  items={workspaceItems}
                  value={workspaceId}
                  onValueChange={(v) => { if (v) setWorkspaceId(v); }}
                >
                  <SelectTrigger
                    size="sm"
                    aria-label={t(($) => $.tokens.workspace_label)}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {workspaceItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  id={readOnlyId}
                  checked={readOnly}
                  onCheckedChange={setReadOnly}
                  aria-label={t(($) => $.tokens.read_only_label)}
                />
                <Label htmlFor={readOnlyId} className="text-body font-normal">
                  {t(($) => $.tokens.read_only_label)}
                </Label>
                <span className="text-caption text-muted-foreground">
                  {t(($) => $.tokens.read_only_hint)}
                </span>
              </div>
            </div>
          </div>
        </SettingsCard>

        {tokensLoading ? (
          <SettingsCard>
            {Array.from({ length: 2 }).map((_, i) => (
              <div key={i} className="flex items-center gap-3 px-4 py-3">
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-3 w-48" />
                </div>
                <Skeleton className="h-8 w-8 rounded-xs" />
              </div>
            ))}
          </SettingsCard>
        ) : tokens.length === 0 ? (
          <SettingsCard>
            <div className="px-4 py-4">
              <p className="text-caption text-muted-foreground">
                {tokensLoadFailed ? t(($) => $.tokens.load_failed) : t(($) => $.tokens.empty)}
              </p>
            </div>
          </SettingsCard>
        ) : (
          <SettingsCard>
            {tokens.map((token) => {
              const boundWorkspaceName = token.workspace_id
                ? workspaceNamesById.get(token.workspace_id)
                : undefined;
              return (
                <div key={token.id} className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1 space-y-0.5">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="truncate text-body font-medium">{token.name}</span>
                      {token.workspace_id ? (
                        <Badge variant="secondary">
                          {boundWorkspaceName ?? t(($) => $.tokens.workspace_scoped)}
                        </Badge>
                      ) : null}
                      {token.read_only ? (
                        <Badge variant="outline">
                          {t(($) => $.tokens.read_only_badge)}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="text-caption tabular-nums text-muted-foreground">
                      {t(($) => $.tokens.metadata_prefix, {
                        prefix: token.token_prefix,
                        created: new Date(token.created_at).toLocaleDateString(locale),
                        lastUsed: token.last_used_at
                          ? t(($) => $.tokens.last_used_with_date, {
                              date: new Date(token.last_used_at!).toLocaleDateString(locale),
                            })
                          : t(($) => $.tokens.last_used_never),
                      })}
                      {token.expires_at && t(($) => $.tokens.expires_with_date, {
                        date: new Date(token.expires_at!).toLocaleDateString(locale),
                      })}
                    </div>
                  </div>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => setRevokeConfirmId(token.id)}
                          disabled={tokenRevoking === token.id}
                          aria-label={t(($) => $.tokens.revoke_aria, { name: token.name })}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      }
                    />
                    <TooltipContent>{t(($) => $.tokens.revoke_tooltip)}</TooltipContent>
                  </Tooltip>
                </div>
              );
            })}
          </SettingsCard>
        )}
      </SettingsSection>

      <AlertDialog open={!!revokeConfirmId} onOpenChange={(v) => { if (!v) setRevokeConfirmId(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.tokens.revoke_dialog.title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.tokens.revoke_dialog.description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($) => $.tokens.revoke_dialog.cancel)}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={async () => {
                if (revokeConfirmId) await handleRevokeToken(revokeConfirmId);
                setRevokeConfirmId(null);
              }}
            >
              {t(($) => $.tokens.revoke_dialog.confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={!!newToken} onOpenChange={(v) => { if (!v) closeCreatedDialog(); }}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t(($) => $.tokens.created_dialog.title)}</DialogTitle>
          </DialogHeader>
          <Alert>
            <Info />
            <AlertDescription>
              {t(($) => $.tokens.created_dialog.warning_prefix)}
              <span className="font-medium text-foreground">{t(($) => $.tokens.created_dialog.warning_emphasis)}</span>
              {t(($) => $.tokens.created_dialog.warning_suffix)}
            </AlertDescription>
          </Alert>
          <div className="flex min-w-0 items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 text-body select-all">
              {newToken}
            </code>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={handleCopyToken}
                    aria-label={t(($) => $.tokens.created_dialog.copy_tooltip)}
                  >
                    <CopyCheckIcon copied={tokenCopied} className="size-4" />
                  </Button>
                }
              />
              <TooltipContent>{t(($) => $.tokens.created_dialog.copy_tooltip)}</TooltipContent>
            </Tooltip>
          </div>
          <div className="min-w-0 space-y-1.5">
            <p className="text-caption text-muted-foreground">{t(($) => $.tokens.created_dialog.cli_hint)}</p>
            <div className="flex min-w-0 items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 text-body select-all">
                {`multica login --token ${newToken}`}
              </code>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={handleCopyCommand}
                      aria-label={t(($) => $.tokens.created_dialog.copy_command_tooltip)}
                    >
                      <CopyCheckIcon copied={commandCopied} className="size-4" />
                    </Button>
                  }
                />
                <TooltipContent>{t(($) => $.tokens.created_dialog.copy_command_tooltip)}</TooltipContent>
              </Tooltip>
            </div>
          </div>
          <DialogFooter className="items-center sm:justify-between">
            <label className="flex items-center gap-2 text-body">
              <Checkbox
                checked={storedConfirmed}
                onCheckedChange={(v) => setStoredConfirmed(v === true)}
              />
              {t(($) => $.tokens.created_dialog.confirm_stored)}
            </label>
            <Button disabled={!storedConfirmed} onClick={closeCreatedDialog}>
              {t(($) => $.tokens.created_dialog.done)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsTab>
  );
}
