"use client";

import { toast } from "sonner";
import { useSetCompanySeats, type CompanyFloor } from "@multica/core/company";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Squad } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@multica/ui/components/ui/dropdown-menu";
import { ChevronDown } from "@multica/ui/icons";
import { useNavigation } from "../../navigation";
import { SquadDetailView } from "../../squads/components/squad-detail-page";
import { useT } from "../../i18n";
import { firstFreeSlot } from "../lib/company-seats";
import { UNSEATED_DEPARTMENT } from "../lib/company-selection";
import { CompanyIssueList } from "./company-issue-list";

/**
 * A squad in the company hub: its department, a new agent for it, the full
 * squad detail, and the work it holds. Members and issues open in place.
 */
export function CompanySquadDetail({
  wsId,
  squad,
  floors,
  squads,
  departmentId,
  canSeat,
  onOpenAgent,
  onOpenIssue,
  onDeleted,
  onMoved,
}: {
  wsId: string;
  squad: Squad;
  floors: readonly CompanyFloor[];
  squads: readonly Squad[];
  /** The floor id, or the unseated key for a squad on no floor. */
  departmentId: string;
  /** Whether this viewer may move seats (the workspace owner or admin). */
  canSeat: boolean;
  onOpenAgent: (agentId: string) => void;
  onOpenIssue: (issueId: string) => void;
  onDeleted: () => void;
  /** Called with the squad's new department after a move. */
  onMoved: (departmentId: string) => void;
}) {
  const { t } = useT("company");
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const setSeats = useSetCompanySeats();

  const departmentName =
    floors.find((floor) => floor.id === departmentId)?.name ?? t(($) => $.unseated);

  const moveTo = async (target: string) => {
    if (target === departmentId) return;
    try {
      if (target === UNSEATED_DEPARTMENT) {
        await setSeats.mutateAsync([{ squad_id: squad.id, floor_id: null, slot: null }]);
      } else {
        const slot = firstFreeSlot(squads.filter((other) => other.id !== squad.id), target);
        if (slot === null) {
          toast.error(t(($) => $.department_full));
          return;
        }
        await setSeats.mutateAsync([{ squad_id: squad.id, floor_id: target, slot }]);
      }
      onMoved(target);
    } catch {
      toast.error(t(($) => $.squad_move_failed));
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center gap-2">
        {canSeat ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button variant="outline" size="sm">
                  {departmentName}
                  <ChevronDown className="size-3.5" />
                </Button>
              }
            />
            <DropdownMenuContent align="start" className="min-w-44">
              {floors.map((floor) => (
                <DropdownMenuItem key={floor.id} onClick={() => void moveTo(floor.id)}>
                  {floor.name}
                </DropdownMenuItem>
              ))}
              <DropdownMenuItem onClick={() => void moveTo(UNSEATED_DEPARTMENT)}>
                {t(($) => $.squad_move_unseated)}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <span className="text-sm text-muted-foreground">{departmentName}</span>
        )}
        <Button variant="outline" size="sm" onClick={() => navigation.push(paths.newAgent())}>
          {t(($) => $.agent_add)}
        </Button>
      </div>

      <SquadDetailView
        squadId={squad.id}
        onDeleted={onDeleted}
        onOpenAgent={onOpenAgent}
        onOpenIssue={onOpenIssue}
      />

      <section aria-label={t(($) => $.work)} className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">{t(($) => $.work)}</h3>
        <CompanyIssueList wsId={wsId} assignee={{ type: "squad", id: squad.id }} />
      </section>
    </div>
  );
}
