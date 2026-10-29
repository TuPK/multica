import { Check, CheckCircle2, Loader2, XCircle } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { useWorkspaceId } from "@multica/core/hooks";
import { useRuntimeQuickstart } from "@multica/core/runtimes";
import type {
  RuntimeQuickstartRequest,
  RuntimeQuickstartStep,
} from "@multica/core/types";
import { useT } from "../../i18n";

const stepIcon: Record<
  RuntimeQuickstartStep["status"],
  { icon: typeof Check; color: string }
> = {
  installed: { icon: CheckCircle2, color: "text-success" },
  already_installed: { icon: Check, color: "text-muted-foreground" },
  failed: { icon: XCircle, color: "text-destructive" },
};

/**
 * Connected-machine quickstart: asks the machine's daemon to install whatever
 * part of the team's Claude Code setup is missing, then lists each item.
 * Renders nothing for a viewer who cannot send commands to the machine.
 */
export function QuickstartSection({
  runtimeId,
  isOnline,
}: {
  runtimeId: string | null;
  isOnline: boolean;
}) {
  if (!runtimeId) return null;
  return <QuickstartControls runtimeId={runtimeId} isOnline={isOnline} />;
}

function QuickstartControls({
  runtimeId,
  isOnline,
}: {
  runtimeId: string;
  isOnline: boolean;
}) {
  const { t } = useT("runtimes");
  const wsId = useWorkspaceId();
  const { request, start, isStarting, startFailed, isLive } =
    useRuntimeQuickstart(wsId, runtimeId);

  const busy = isStarting || isLive;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="xs"
          onClick={start}
          disabled={!isOnline || busy}
          aria-busy={busy}
          title={
            isOnline
              ? t(($) => $.quickstart.action_title)
              : t(($) => $.quickstart.offline_title)
          }
        >
          {busy && <Loader2 className="h-3 w-3 animate-spin" />}
          {t(($) => $.quickstart.action)}
        </Button>

        {request && (
          <span
            className={`inline-flex items-center gap-1 text-caption ${
              request.status === "failed"
                ? "text-destructive"
                : "text-muted-foreground"
            }`}
          >
            {t(($) => $.quickstart.status[request.status])}
          </span>
        )}
      </div>

      {startFailed && (
        <p className="text-caption text-destructive">
          {t(($) => $.quickstart.start_failed)}
        </p>
      )}

      {request && <QuickstartResult request={request} />}
    </div>
  );
}

function QuickstartResult({ request }: { request: RuntimeQuickstartRequest }) {
  const { t } = useT("runtimes");

  if (request.status === "pending" || request.status === "running") {
    return null;
  }

  if (request.status === "failed") {
    return (
      <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2">
        <p className="text-caption text-destructive">{request.error}</p>
      </div>
    );
  }

  const anyFailed = request.steps.some((step) => step.status === "failed");
  const allPresent = request.steps.every(
    (step) => step.status === "already_installed",
  );

  return (
    <div className="space-y-2 rounded-lg border px-3 py-2">
      <p
        className={`text-caption ${anyFailed ? "text-destructive" : "text-success"}`}
      >
        {anyFailed
          ? t(($) => $.quickstart.some_failed)
          : allPresent
            ? t(($) => $.quickstart.all_present)
            : t(($) => $.quickstart.status.completed)}
      </p>
      <ul className="space-y-1">
        {request.steps.map((step) => {
          const { icon: Icon, color } = stepIcon[step.status];
          return (
            <li
              key={step.name}
              className="flex min-w-0 items-center gap-2 text-caption"
            >
              <Icon className={`h-3 w-3 shrink-0 ${color}`} />
              <span className="font-mono">{step.name}</span>
              <span className="text-muted-foreground">
                {t(($) => $.quickstart.step_status[step.status])}
              </span>
              {step.detail && (
                <span
                  className="min-w-0 truncate text-muted-foreground"
                  title={step.detail}
                >
                  {step.detail}
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
