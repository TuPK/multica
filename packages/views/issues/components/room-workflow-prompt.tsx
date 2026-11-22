"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  useIssueWorkflows,
  useRoomWorkflowPolicy,
} from "@multica/core/issue-workflows/hooks";
import { useSetRoomWorkflowPolicy } from "@multica/core/issue-workflows/mutations";
import { issueWorkflowKeys } from "@multica/core/issue-workflows/queries";
import { useWorkspacePaths } from "@multica/core/paths";
import type {
  RoomWorkflowCheck,
  RoomWorkflowCode,
  RoomWorkflowPolicy,
} from "@multica/core/types";
import {
  memberListOptions,
  squadListOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { ArrowUpRight, Workflow } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { AppLink, resolveClickIntent } from "../../navigation";

/** Whether a server code is one of the two room-workflow refusal codes. */
export function isRoomWorkflowCode(
  code: string | null | undefined,
): code is RoomWorkflowCode {
  return (
    code === "room_workflow_required" ||
    code === "room_no_workflow_for_non_request"
  );
}

function asHttpError(
  err: unknown,
): { status: number; body?: unknown; message: string } | null {
  if (!err || typeof err !== "object") return null;
  const rec = err as { status?: unknown; body?: unknown; message?: unknown };
  if (typeof rec.status !== "number") return null;
  return {
    status: rec.status,
    body: rec.body,
    message: typeof rec.message === "string" ? rec.message : "",
  };
}

/**
 * Extracts a structured room-workflow refusal from an HTTP 422 ApiError, or
 * returns null for any other error.
 */
export function parseRoomWorkflowRefusal(
  err: unknown,
  fallbackSquadId?: string | null,
): RoomWorkflowCheck | null {
  const httpErr = asHttpError(err);
  if (!httpErr || httpErr.status !== 422) return null;
  if (!httpErr.body || typeof httpErr.body !== "object") return null;
  const body = httpErr.body as {
    code?: unknown;
    error?: unknown;
    message?: unknown;
    squad_id?: unknown;
  };
  if (typeof body.code !== "string" || !isRoomWorkflowCode(body.code)) {
    return null;
  }
  const message =
    typeof body.error === "string" && body.error
      ? body.error
      : typeof body.message === "string" && body.message
        ? body.message
        : httpErr.message;
  const squadId =
    typeof body.squad_id === "string" && body.squad_id
      ? body.squad_id
      : (fallbackSquadId ?? null);
  return {
    needsWorkflow: true,
    code: body.code,
    message,
    squadId,
  };
}

/**
 * Whether a preview check should open the room-workflow prompt before
 * submitting. Once the workspace has stored a non-request policy (anything
 * other than `no_action`), never re-open the non-request prompt.
 */
export function shouldPromptForRoomWorkflow(
  check: RoomWorkflowCheck | null | undefined,
  policy?: RoomWorkflowPolicy | null,
): check is RoomWorkflowCheck & { code: RoomWorkflowCode } {
  if (!check || !check.needsWorkflow || !isRoomWorkflowCode(check.code)) {
    return false;
  }
  if (
    check.code === "room_no_workflow_for_non_request" &&
    policy != null &&
    policy.kind !== "no_action"
  ) {
    return false;
  }
  return true;
}

export interface RoomWorkflowPromptProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  refusal: RoomWorkflowCheck | null;
  /** Called after the choice is stored so the caller continues the write. */
  onResolved: () => void | Promise<void>;
  /** Called when the user navigates away to the Workflow tab. */
  onNavigateAway?: () => void;
}

/**
 * Prompt shown when an issue write would be refused by room workflow rules
 * (squad-room-workflow spec, sections 6 and 13).
 *
 * Mounts its query/mutation body only while open for one of the two refusal
 * codes so closed write forms incur no extra hooks or requests.
 */
export function RoomWorkflowPrompt({
  open,
  onOpenChange,
  refusal,
  onResolved,
  onNavigateAway,
}: RoomWorkflowPromptProps) {
  if (
    !open ||
    !refusal ||
    !refusal.needsWorkflow ||
    !isRoomWorkflowCode(refusal.code)
  ) {
    return null;
  }

  return (
    <RoomWorkflowPromptDialog
      key={`${refusal.code}:${refusal.squadId ?? ""}`}
      refusal={{ ...refusal, code: refusal.code }}
      onOpenChange={onOpenChange}
      onResolved={onResolved}
      onNavigateAway={onNavigateAway}
    />
  );
}

function serverErrorMessage(err: unknown, fallback: string): string {
  const httpErr = asHttpError(err);
  if (httpErr?.body && typeof httpErr.body === "object") {
    const body = httpErr.body as { error?: unknown; message?: unknown };
    if (typeof body.error === "string" && body.error) return body.error;
    if (typeof body.message === "string" && body.message) return body.message;
  }
  return err instanceof Error && err.message ? err.message : fallback;
}

function RoomWorkflowPromptDialog({
  refusal,
  onOpenChange,
  onResolved,
  onNavigateAway,
}: {
  refusal: RoomWorkflowCheck & { code: RoomWorkflowCode };
  onOpenChange: (open: boolean) => void;
  onResolved: () => void | Promise<void>;
  onNavigateAway?: () => void;
}) {
  const { t } = useT("workflow");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const qc = useQueryClient();
  const currentUser = useAuthStore((s) => s.user);
  const { workflows } = useIssueWorkflows(wsId);
  const { policy } = useRoomWorkflowPolicy(wsId);
  const setPolicy = useSetRoomWorkflowPolicy();

  const isSquadRequired = refusal.code === "room_workflow_required";

  const { data: wsMembers = [] } = useQuery({
    ...memberListOptions(wsId),
    enabled: Boolean(wsId) && isSquadRequired,
  });
  const { data: squads = [] } = useQuery({
    ...squadListOptions(wsId),
    enabled: Boolean(wsId) && isSquadRequired,
  });

  const squad = useMemo(
    () =>
      refusal.squadId
        ? squads.find((s) => s.id === refusal.squadId) ?? null
        : null,
    [refusal.squadId, squads],
  );

  // Mirrors server canManageSquad: workspace owner/admin or squad creator.
  const myRole = currentUser
    ? wsMembers.find((m) => m.user_id === currentUser.id)?.role ?? null
    : null;
  const isWorkspaceAdmin = myRole === "owner" || myRole === "admin";
  const canManageSquad =
    isWorkspaceAdmin ||
    (Boolean(currentUser) && Boolean(squad) && squad?.creator_id === currentUser?.id);

  const nonDefaultWorkflows = useMemo(
    () => workflows.filter((w) => !w.is_default),
    [workflows],
  );
  const selectableWorkflows = isSquadRequired ? workflows : nonDefaultWorkflows;
  const workflowItems = useMemo(
    () => selectableWorkflows.map((w) => ({ value: w.id, label: w.name })),
    [selectableWorkflows],
  );

  const [choice, setChoice] = useState<"default" | "workflow">("default");
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string>("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Never re-open once a non-request workspace policy is already stored.
  if (
    refusal.code === "room_no_workflow_for_non_request" &&
    policy != null &&
    policy.kind !== "no_action"
  ) {
    return null;
  }

  const effectiveWorkflowId =
    selectedWorkflowId || selectableWorkflows[0]?.id || "";

  const handleOpenWorkflowTab = (e: React.MouseEvent) => {
    if (resolveClickIntent(e) !== "push") return;
    onOpenChange(false);
    onNavigateAway?.();
  };

  const handleConfirmNonRequest = async () => {
    if (saving) return;
    if (choice === "workflow" && !effectiveWorkflowId) return;
    setSaving(true);
    setError(null);
    try {
      await setPolicy.mutateAsync(
        choice === "default"
          ? { kind: "default", workflowId: null }
          : { kind: "workflow", workflowId: effectiveWorkflowId },
      );
      onOpenChange(false);
      await onResolved();
    } catch (err) {
      if (asHttpError(err)?.status === 403) {
        setError(t(($) => $.room_prompt.admin_only_error));
      } else {
        setError(serverErrorMessage(err, t(($) => $.room_prompt.save_failed)));
      }
    } finally {
      setSaving(false);
    }
  };

  const handleConfirmSquadWorkflow = async () => {
    if (saving || !refusal.squadId || !effectiveWorkflowId) return;
    setSaving(true);
    setError(null);
    try {
      await api.setSquadActiveWorkflow(refusal.squadId, {
        workflow_id: effectiveWorkflowId,
      });
      await Promise.all([
        qc.invalidateQueries({ queryKey: issueWorkflowKeys.roomChecks(wsId) }),
        qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) }),
        qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) }),
      ]);
      onOpenChange(false);
      await onResolved();
    } catch (err) {
      setError(serverErrorMessage(err, t(($) => $.room_prompt.save_failed)));
    } finally {
      setSaving(false);
    }
  };

  const canPickSquadWorkflow = isSquadRequired && canManageSquad && Boolean(refusal.squadId);

  return (
    <Dialog
      open
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !saving) onOpenChange(false);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Workflow className="size-4 text-muted-foreground" aria-hidden />
            <span>
              {isSquadRequired
                ? t(($) => $.room_prompt.squad_required_title)
                : t(($) => $.room_prompt.non_request_title)}
            </span>
          </DialogTitle>
          <DialogDescription>
            {isSquadRequired
              ? squad?.name
                ? t(($) => $.room_prompt.squad_required_description_named, {
                    squad: squad.name,
                  })
                : t(($) => $.room_prompt.squad_required_description)
              : t(($) => $.room_prompt.non_request_description)}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {!isSquadRequired ? (
            <div role="radiogroup" aria-label={t(($) => $.room_prompt.non_request_title)} className="space-y-2">
              <label
                className={cn(
                  "flex cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2.5 text-body transition-colors",
                  choice === "default"
                    ? "border-primary bg-accent/60 hover:bg-accent/80"
                    : "border-surface-border hover:bg-accent/40",
                )}
              >
                <input
                  type="radio"
                  name="room-workflow-choice"
                  value="default"
                  checked={choice === "default"}
                  disabled={saving}
                  onChange={() => {
                    setChoice("default");
                    setError(null);
                  }}
                  className="accent-primary"
                />
                <span className="font-medium text-foreground">
                  {t(($) => $.room_prompt.option_default)}
                </span>
              </label>

              <div
                className={cn(
                  "rounded-lg border px-3 py-2.5 transition-colors",
                  choice === "workflow"
                    ? "border-primary bg-accent/60 hover:bg-accent/80"
                    : "border-surface-border hover:bg-accent/40",
                  nonDefaultWorkflows.length === 0 && "opacity-60",
                )}
              >
                <label className="flex cursor-pointer items-center gap-2.5 text-body">
                  <input
                    type="radio"
                    name="room-workflow-choice"
                    value="workflow"
                    checked={choice === "workflow"}
                    disabled={saving || nonDefaultWorkflows.length === 0}
                    onChange={() => {
                      setChoice("workflow");
                      setError(null);
                    }}
                    className="accent-primary"
                  />
                  <span className="font-medium text-foreground">
                    {t(($) => $.room_prompt.option_existing)}
                  </span>
                </label>

                {choice === "workflow" && (
                  <div className="mt-2.5 pl-6">
                    {workflowItems.length > 0 ? (
                      <Select
                        items={workflowItems}
                        value={effectiveWorkflowId}
                        disabled={saving}
                        onValueChange={(value) => {
                          if (value !== null) setSelectedWorkflowId(value);
                        }}
                      >
                        <SelectTrigger
                          aria-label={t(($) => $.room_prompt.option_existing)}
                          className="w-full"
                        >
                          <SelectValue
                            placeholder={t(($) => $.room_prompt.select_workflow_placeholder)}
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {workflowItems.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : (
                      <p className="text-caption text-muted-foreground">
                        {t(($) => $.room_prompt.no_custom_workflows)}
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>
          ) : canPickSquadWorkflow ? (
            <div className="space-y-1.5">
              <span className="block text-caption font-medium text-foreground">
                {t(($) => $.room_prompt.squad_picker_label)}
              </span>
              {workflowItems.length > 0 ? (
                <Select
                  items={workflowItems}
                  value={effectiveWorkflowId}
                  disabled={saving}
                  onValueChange={(value) => {
                    if (value !== null) setSelectedWorkflowId(value);
                  }}
                >
                  <SelectTrigger
                    aria-label={t(($) => $.room_prompt.squad_picker_label)}
                    className="w-full"
                  >
                    <SelectValue
                      placeholder={t(($) => $.room_prompt.select_workflow_placeholder)}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {workflowItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.room_prompt.no_custom_workflows)}
                </p>
              )}
            </div>
          ) : (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.room_prompt.squad_member_note)}
            </p>
          )}

          <div>
            <AppLink
              href={paths.workflow()}
              onClick={handleOpenWorkflowTab}
              className="inline-flex items-center gap-1 text-caption font-medium text-primary hover:underline"
            >
              <span>{t(($) => $.room_prompt.open_workflow_tab)}</span>
              <ArrowUpRight className="size-3.5" aria-hidden />
            </AppLink>
          </div>

          {error && (
            <p role="alert" className="text-caption text-destructive">
              {error}
            </p>
          )}
        </div>

        <DialogFooter>
          {!isSquadRequired || canPickSquadWorkflow ? (
            <>
              <Button
                type="button"
                variant="outline"
                disabled={saving}
                onClick={() => onOpenChange(false)}
              >
                {t(($) => $.room_prompt.cancel)}
              </Button>
              <Button
                type="button"
                disabled={
                  saving ||
                  (!isSquadRequired
                    ? choice === "workflow" && !effectiveWorkflowId
                    : !effectiveWorkflowId)
                }
                aria-busy={saving || undefined}
                onClick={() =>
                  void (isSquadRequired
                    ? handleConfirmSquadWorkflow()
                    : handleConfirmNonRequest())
                }
              >
                {saving ? (
                  <>
                    <Spinner className="size-4" />
                    <span>{t(($) => $.room_prompt.saving)}</span>
                  </>
                ) : (
                  t(($) => $.room_prompt.ok)
                )}
              </Button>
            </>
          ) : (
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t(($) => $.room_prompt.close)}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
