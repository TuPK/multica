"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { AlertCircle, Copy, Loader2, Search } from "@multica/ui/icons";
import { api, clientErrorMessage } from "@multica/core/api";
import { issueWorkflowListOptions } from "@multica/core/issue-workflows";
import type { IssueWorkflow } from "@multica/core/types";
import { useT } from "../../i18n";

export interface CloneWorkflowDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
  onSelectWorkflow: (workflow: IssueWorkflow) => void;
}

export function CloneWorkflowDialog({
  open,
  onOpenChange,
  wsId,
  onSelectWorkflow,
}: CloneWorkflowDialogProps) {
  const { t } = useT("workflow");
  const [searchQuery, setSearchQuery] = useState("");
  const [fetchingId, setFetchingId] = useState<string | null>(null);

  const query = useQuery({
    ...issueWorkflowListOptions(wsId),
    enabled: Boolean(wsId) && open,
  });

  const filteredWorkflows = useMemo(() => {
    const list = (query.data ?? []).filter((w) => !w.template_ref);
    const q = searchQuery.trim().toLowerCase();
    if (!q) return list;
    return list.filter((w) => w.name.toLowerCase().includes(q));
  }, [query.data, searchQuery]);

  const handlePickWorkflow = async (workflow: IssueWorkflow) => {
    if (fetchingId) return;
    setFetchingId(workflow.id);
    try {
      const detail = await api.getIssueWorkflow(workflow.id);
      onSelectWorkflow(detail);
      onOpenChange(false);
    } catch (err) {
      toast.error(
        clientErrorMessage(err) ?? t(($) => $.designer.clone_dialog.load_failed),
      );
    } finally {
      setFetchingId(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-6">
        <DialogHeader className="pb-2">
          <DialogTitle>{t(($) => $.designer.clone_dialog.title)}</DialogTitle>
        </DialogHeader>

        {/* Search input */}
        <div className="relative my-2">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t(($) => $.designer.clone_dialog.search_placeholder)}
            className="w-full h-9 pl-9 pr-3 rounded-md border border-input bg-background text-body placeholder:text-muted-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        {/* Workflow List / States */}
        <div className="flex flex-col gap-1 max-h-80 overflow-y-auto py-1">
          {query.isLoading ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground gap-2">
              <Loader2 className="size-5 animate-spin" />
              <span className="text-caption">
                {t(($) => $.designer.clone_dialog.loading)}
              </span>
            </div>
          ) : query.isError ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground gap-3">
              <AlertCircle className="size-5 text-destructive" />
              <span className="text-caption text-center">
                {t(($) => $.designer.clone_dialog.load_failed)}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void query.refetch()}
              >
                {t(($) => $.manage.action_failed)}
              </Button>
            </div>
          ) : filteredWorkflows.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <span className="text-caption">
                {t(($) => $.designer.clone_dialog.empty_title)}
              </span>
            </div>
          ) : (
            filteredWorkflows.map((w) => {
              const isFetching = fetchingId === w.id;
              const stepCount = w.steps?.length ?? 0;

              return (
                <button
                  key={w.id}
                  type="button"
                  disabled={Boolean(fetchingId)}
                  onClick={() => void handlePickWorkflow(w)}
                  className="flex items-center justify-between p-3 rounded-lg border border-border bg-card hover:bg-accent/50 text-left transition-colors disabled:opacity-50 group"
                >
                  <div className="flex items-center gap-2.5 min-w-0">
                    <Copy className="size-4 text-muted-foreground shrink-0" />
                    <div className="min-w-0 flex flex-col">
                      <div className="flex items-center gap-2">
                        <span className="text-body font-medium text-foreground truncate">
                          {w.name}
                        </span>
                        {w.is_default && (
                          <Badge variant="secondary" className="text-micro h-4 px-1">
                            {t(($) => $.manage.default_badge)}
                          </Badge>
                        )}
                      </div>
                      {w.description && (
                        <span className="text-caption text-muted-foreground truncate">
                          {w.description}
                        </span>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0 ml-3">
                    <span className="text-caption text-muted-foreground">
                      {t(($) => $.manage.steps_count, { count: stepCount })}
                    </span>
                    {isFetching && <Loader2 className="size-3.5 animate-spin text-muted-foreground" />}
                  </div>
                </button>
              );
            })
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
