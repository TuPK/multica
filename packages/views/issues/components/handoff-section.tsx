"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { issueHandoffOptions, useIssueHandoffDecision } from "@multica/core/squads";
import { Button } from "@multica/ui/components/ui/button";
import { useCompanyAdmin } from "../../company/lib/use-company-admin";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import { IssueSidebarSectionHeader } from "./issue-sidebar-section";

/**
 * Where a request stands in its squad's handoff to another squad. It shows only on a
 * request whose squad has a handoff rule, or that was already handed off. The boss
 * approves, declines, or hands off by hand; the squad's leader asks for it on its own.
 */
export function HandoffSection({ issueId }: { issueId: string }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const isAdmin = useCompanyAdmin();
  const [open, setOpen] = useState(true);
  const { data } = useQuery(issueHandoffOptions(wsId, issueId));
  const decide = useIssueHandoffDecision(wsId, issueId);

  if (!data || (!data.rule && !data.handoff)) return null;
  const squad = data.handoff?.target_squad_name ?? data.rule?.target_squad_name ?? "";
  const status = data.handoff?.status ?? (data.rule ? "none" : null);
  if (status === null) return null;
  // A rule with no target has nothing to hand off to.
  if (!data.handoff && !data.rule?.target_squad_id) return null;

  const run = (decision: "hand_off" | "decline") =>
    decide.mutate(decision, {
      onError: (err) => toast.error(clientErrorMessage(err) ?? t(($) => $.handoff.failed)),
    });

  const text =
    status === "handed_off"
      ? t(($) => $.handoff.handed_off, { squad })
      : status === "declined"
        ? t(($) => $.handoff.declined, { squad })
        : status === "pending"
          ? t(($) => $.handoff.pending, { squad })
          : t(($) => $.handoff.not_yet, { squad });

  return (
    <div>
      <IssueSidebarSectionHeader title={t(($) => $.handoff.title)} open={open} onOpenChange={setOpen} />
      {open && (
        <div className="flex flex-col gap-2 px-2 pb-2 text-body">
          <p>{text}</p>
          {status === "handed_off" && data.handoff?.target_issue_id ? (
            <AppLink
              href={paths.issueDetail(data.handoff.target_issue_id)}
              className="text-caption text-muted-foreground underline-offset-2 hover:underline"
            >
              {t(($) => $.handoff.open_request)}
            </AppLink>
          ) : null}
          {isAdmin && status !== "handed_off" ? (
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" disabled={decide.isPending} onClick={() => run("hand_off")}>
                {status === "pending" ? t(($) => $.handoff.approve) : t(($) => $.handoff.hand_off, { squad })}
              </Button>
              {status === "pending" ? (
                <Button type="button" size="sm" variant="outline" disabled={decide.isPending} onClick={() => run("decline")}>
                  {t(($) => $.handoff.decline)}
                </Button>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
