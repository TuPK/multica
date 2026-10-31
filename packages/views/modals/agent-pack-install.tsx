"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "@multica/ui/icons";
import { useAuthStore } from "@multica/core/auth";
import {
  AGENT_PACK_IDS,
  loadAgentPack,
  useInstallAgentPack,
  type AgentPackItemStatus,
  type AgentPackOutcome,
} from "@multica/core/agent-packs";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { isRuntimeUsableForUser, runtimeListOptions } from "@multica/core/runtimes";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { toast } from "sonner";

import { RuntimePicker } from "../agents/components/runtime-picker";
import { useT } from "../i18n";
import { useNavigation } from "../navigation";

const PACK_ID = AGENT_PACK_IDS[0];

function hasProblems(outcome: AgentPackOutcome): boolean {
  return (
    outcome.skills.some((s) => s.status === "failed") ||
    outcome.agent.status === "failed" ||
    outcome.attach.status === "failed"
  );
}

export function AgentPackInstallModal({ onClose }: { onClose: () => void }) {
  const { t } = useT("modals");
  const router = useNavigation();
  const wsPaths = useWorkspacePaths();
  const wsId = useWorkspaceId();
  const currentUserId = useAuthStore((s) => s.user?.id ?? null);

  const { data: runtimes = [], isLoading: runtimesLoading } = useQuery(runtimeListOptions(wsId));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: pack } = useQuery({
    queryKey: ["agent-pack", PACK_ID],
    queryFn: () => loadAgentPack(PACK_ID),
    staleTime: Infinity,
  });

  const [runtimeId, setRuntimeId] = useState("");
  const [outcome, setOutcome] = useState<AgentPackOutcome | null>(null);
  const install = useInstallAgentPack(wsId);

  const runtime = runtimes.find((r) => r.id === runtimeId);
  const needsRuntime =
    !!pack && !agents.some((a) => !a.archived_at && a.name === pack.agent.name);
  const runtimeOk =
    !needsRuntime || (!!runtime && isRuntimeUsableForUser(runtime, currentUserId));
  const running = install.isPending;
  const canSubmit = !!pack && runtimeOk && !running;

  const run = async () => {
    if (!pack || !canSubmit) return;
    try {
      const done = await install.mutateAsync({
        pack,
        runtimeId: needsRuntime ? runtimeId : null,
      });
      setOutcome(done);
      if (!hasProblems(done)) toast.success(t(($) => $.agent_pack_install.toast_done));
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t(($) => $.agent_pack_install.toast_failed),
      );
    }
  };

  const finished = outcome !== null && !running;
  const itemStatus: Record<AgentPackItemStatus, string> = {
    created: t(($) => $.agent_pack_install.status_created),
    kept: t(($) => $.agent_pack_install.status_kept),
    failed: t(($) => $.agent_pack_install.status_failed),
  };

  return (
    <Dialog open onOpenChange={(v) => { if (!v && !running) onClose(); }}>
      <DialogContent className="p-0 gap-0 flex flex-col overflow-hidden !top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2 !w-full !max-w-2xl !max-h-[85vh]">
        <DialogHeader className="border-b px-5 py-3 space-y-0">
          <DialogTitle className="text-title-sm font-semibold">
            {t(($) => $.agent_pack_install.title)}
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto p-5 space-y-5">
          {!outcome && pack && (
            <>
              <div className="space-y-1 rounded-lg border px-3 py-2">
                <p className="text-body font-medium">
                  {t(($) => $.agent_pack_install.packs[PACK_ID].name)}
                </p>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.agent_pack_install.packs[PACK_ID].description)}
                </p>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.agent_pack_install.skills_count, { count: pack.skills.length })}
                </p>
              </div>

              {needsRuntime && (
                <div className="space-y-2">
                  <Label className="text-caption text-muted-foreground">
                    {t(($) => $.agent_pack_install.runtime_label)}
                  </Label>
                  <RuntimePicker
                    runtimes={runtimes}
                    runtimesLoading={runtimesLoading}
                    members={members}
                    currentUserId={currentUserId}
                    selectedRuntimeId={runtimeId}
                    onSelect={setRuntimeId}
                  />
                </div>
              )}
            </>
          )}

          {outcome && (
            <div className="space-y-4" aria-live="polite">
              <p className="text-body font-medium">
                {running
                  ? t(($) => $.agent_pack_install.installing)
                  : hasProblems(outcome)
                    ? t(($) => $.agent_pack_install.result_problems)
                    : t(($) => $.agent_pack_install.result_ready)}
              </p>
              <ul className="space-y-1">
                {outcome.skills.map((s) => (
                  <li key={s.name} className="flex items-start justify-between gap-3 text-body">
                    <span className="truncate">{s.name}</span>
                    <span
                      className={`shrink-0 text-caption ${
                        s.status === "failed" ? "text-destructive" : "text-muted-foreground"
                      }`}
                    >
                      {s.status === "failed" && s.error
                        ? `${itemStatus.failed}: ${s.error}`
                        : itemStatus[s.status]}
                    </span>
                  </li>
                ))}
                <li className="flex items-start justify-between gap-3 text-body font-medium">
                  <span>{t(($) => $.agent_pack_install.agent_row)}</span>
                  <span
                    className={`shrink-0 text-caption ${
                      outcome.agent.status === "failed" ? "text-destructive" : "text-muted-foreground"
                    }`}
                  >
                    {outcome.agent.status === "failed" && outcome.agent.error
                      ? `${itemStatus.failed}: ${outcome.agent.error}`
                      : itemStatus[outcome.agent.status]}
                  </span>
                </li>
                {outcome.attach.status !== "skipped" && (
                  <li className="flex items-start justify-between gap-3 text-body font-medium">
                    <span>{t(($) => $.agent_pack_install.attach_row)}</span>
                    <span
                      className={`shrink-0 text-caption ${
                        outcome.attach.status === "failed" ? "text-destructive" : "text-muted-foreground"
                      }`}
                    >
                      {outcome.attach.status === "failed"
                        ? `${itemStatus.failed}: ${outcome.attach.error}`
                        : outcome.attach.status === "attached"
                          ? t(($) => $.agent_pack_install.attach_added, { count: outcome.attach.count })
                          : t(($) => $.agent_pack_install.attach_kept)}
                    </span>
                  </li>
                )}
              </ul>
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t px-5 py-3">
          {finished ? (
            <>
              {hasProblems(outcome) && (
                <Button variant="outline" onClick={() => void run()}>
                  {t(($) => $.agent_pack_install.retry)}
                </Button>
              )}
              <Button variant="outline" onClick={onClose}>
                {t(($) => $.agent_pack_install.close)}
              </Button>
              {outcome.agent.id && (
                <Button
                  onClick={() => {
                    onClose();
                    router.push(wsPaths.agentDetail(outcome.agent.id!));
                  }}
                >
                  {t(($) => $.agent_pack_install.open_agent)}
                </Button>
              )}
            </>
          ) : (
            <>
              <Button variant="outline" onClick={onClose} disabled={running}>
                {t(($) => $.agent_pack_install.cancel)}
              </Button>
              <Button onClick={() => void run()} disabled={!canSubmit}>
                {running && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
                {running
                  ? t(($) => $.agent_pack_install.installing)
                  : t(($) => $.agent_pack_install.install)}
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
