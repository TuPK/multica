import { Badge } from "@multica/ui/components/ui/badge";
import { Workflow } from "@multica/ui/icons";
import type { Squad } from "@multica/core/types";
import { CollectionEmptyValue } from "../../layout/collection-list";
import { useT } from "../../i18n";

type SquadWithFlow = Squad & {
  workflow_count?: number;
  active_workflow_name?: string | null;
  active_workflow_id?: string | null;
  flow_enforced?: boolean;
};

export function SquadWorkflowsCell({ squad }: { squad: Squad }) {
  const { t } = useT("squads");
  const s = squad as SquadWithFlow;
  const count = s.workflow_count ?? 0;
  const activeName = s.active_workflow_name;
  const flowEnforced = Boolean(s.flow_enforced);

  if (!count && !activeName) {
    return <CollectionEmptyValue />;
  }

  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <Workflow className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      <span className="truncate text-body text-foreground">
        {activeName || "—"}
      </span>
      {count > 0 && (
        <span className="inline-flex items-center justify-center rounded-full bg-muted px-1.5 py-0.5 text-micro font-medium text-muted-foreground tabular-nums">
          {count}
        </span>
      )}
      {flowEnforced && (
        <Badge variant="secondary" className="text-micro font-medium shrink-0">
          {t(($) => $.workflows.badge_enforced)}
        </Badge>
      )}
    </div>
  );
}
