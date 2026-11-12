"use client";

import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Plus } from "@multica/ui/icons";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import { StatusIcon } from "../../issues/components/status-icon";
import { useStatusLabel } from "../../issues/utils/status-label";
import { useT } from "../../i18n";

export interface StatusPaletteProps {
  wsId: string;
  activeStatusKeys: Set<string>;
  onAddStatus: (statusKey: string) => void;
}

export function StatusPalette({
  wsId,
  activeStatusKeys,
  onAddStatus,
}: StatusPaletteProps) {
  const { t } = useT("workflow");
  const { statuses } = useIssueStatuses(wsId);
  const getStatusLabel = useStatusLabel(wsId);

  return (
    <div className="flex flex-col h-full space-y-4">
      <div>
        <h3 className="text-body font-semibold text-foreground">
          {t(($) => $.designer.status_palette)}
        </h3>
      </div>

      <div className="space-y-2 overflow-y-auto pr-1">
        {statuses.map((status) => {
          const isAdded = activeStatusKeys.has(status.key);

          return (
            <div
              key={status.key}
              className={`flex items-center justify-between p-2.5 rounded-lg border text-body-sm transition-all ${
                isAdded
                  ? "border-border/60 bg-muted/40 opacity-70"
                  : "border-border bg-card hover:border-sidebar-accent shadow-2xs"
              }`}
            >
              <div className="flex items-center gap-2.5 min-w-0">
                <StatusIcon status={status.key} className="size-4 shrink-0" />
                <span className="font-medium text-foreground truncate">
                  {getStatusLabel(status.key)}
                </span>
                <span className="text-micro font-mono text-muted-foreground">
                  ({status.key})
                </span>
              </div>

              {isAdded ? (
                <Badge variant="outline" className="text-micro text-muted-foreground">
                  {t(($) => $.designer.in_workflow)}
                </Badge>
              ) : (
                <Button
                  type="button"
                  variant="outline"
                  size="icon-xs"
                  onClick={() => onAddStatus(status.key)}
                  aria-label={t(($) => $.designer.add_status)}
                >
                  <Plus className="size-3.5" />
                </Button>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
