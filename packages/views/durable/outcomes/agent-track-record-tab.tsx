"use client";

import { toast } from "sonner";
import { VERIFICATION_MODES, type VerificationMode } from "@multica/core/durable/outcomes";
import { usePinTrackRecord, useTrackRecords, type TrackRecord } from "@multica/core/durable/track-records";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentMember } from "@multica/core/permissions";
import { canGovern } from "@multica/core/governance";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

// "Automatic" is the mode the server picks from the record; choosing it unpins the agent.
const AUTO = "auto";

/**
 * The agent's track record (durable spec 3.5), one row per work class: the counts of each
 * verdict, the lower bound of the score, and the check mode. An owner or admin may pin a mode;
 * everyone else reads it.
 */
export function AgentTrackRecordTab({ agentId }: { agentId: string }) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const { role } = useCurrentMember(wsId);
  const editable = canGovern(role);
  const records = useTrackRecords(wsId, { agent_id: agentId });
  const pin = usePinTrackRecord(wsId);
  const rows = records.data ?? [];

  const onPin = (row: TrackRecord, value: string) => {
    const mode = value === AUTO ? null : (value as VerificationMode);
    pin.mutateAsync({ agent_id: row.agent_id, work_class: row.work_class, mode }).then(
      () => toast.success(t(($) => $.outcomes.track_record.pinned)),
      () => toast.error(t(($) => $.outcomes.track_record.pin_failed)),
    );
  };

  const modeItems = [
    { value: AUTO, label: t(($) => $.outcomes.track_record.pin_automatic) },
    ...VERIFICATION_MODES.map((mode) => ({ value: mode, label: t(($) => $.outcomes.mode[mode]) })),
  ];

  return (
    <section aria-label={t(($) => $.outcomes.track_record.title)} className="flex flex-col gap-3">
      <h2 className="text-heading">{t(($) => $.outcomes.track_record.title)}</h2>
      {records.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.outcomes.track_record.load_failed)}
        </p>
      ) : null}
      {!records.isPending && !records.isError && rows.length === 0 ? (
        <p className="text-body text-muted-foreground">{t(($) => $.outcomes.track_record.empty)}</p>
      ) : null}
      {rows.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-left text-body">
            <thead className="text-caption text-muted-foreground">
              <tr>
                <th scope="col" className="py-1.5 pr-3 font-medium">{t(($) => $.outcomes.track_record.column_work)}</th>
                <th scope="col" className="py-1.5 pr-3 font-medium">{t(($) => $.outcomes.track_record.column_mode)}</th>
                {editable ? <th scope="col" className="py-1.5 pr-3 font-medium">{t(($) => $.outcomes.track_record.column_pin)}</th> : null}
                <th scope="col" className="py-1.5 pr-3 text-right font-medium">{t(($) => $.outcomes.track_record.column_pass)}</th>
                <th scope="col" className="py-1.5 pr-3 text-right font-medium">{t(($) => $.outcomes.track_record.column_partial)}</th>
                <th scope="col" className="py-1.5 pr-3 text-right font-medium">{t(($) => $.outcomes.track_record.column_fail)}</th>
                <th scope="col" className="py-1.5 pr-3 text-right font-medium">{t(($) => $.outcomes.track_record.column_unverified)}</th>
                <th scope="col" className="py-1.5 pr-3 text-right font-medium">{t(($) => $.outcomes.track_record.column_high_fail)}</th>
                <th scope="col" className="py-1.5 text-right font-medium">{t(($) => $.outcomes.track_record.column_score)}</th>
              </tr>
            </thead>
            <tbody className="tabular-nums">
              {rows.map((row) => (
                <tr key={`${row.agent_id}:${row.work_class}`} className="border-t">
                  <td className="py-2 pr-3 break-all">{row.work_class}</td>
                  <td className="py-2 pr-3">{t(($) => $.outcomes.mode[row.verification_mode])}</td>
                  {editable ? (
                    <td className="py-2 pr-3">
                      <Select
                        items={modeItems}
                        value={row.mode_pinned ? row.verification_mode : AUTO}
                        disabled={pin.isPending}
                        onValueChange={(value) => {
                          if (value) onPin(row, value);
                        }}
                      >
                        <SelectTrigger
                          aria-label={t(($) => $.outcomes.track_record.pin_label, { work: row.work_class })}
                          size="sm"
                          className="w-40"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {modeItems.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </td>
                  ) : null}
                  <td className="py-2 pr-3 text-right">{row.n_pass}</td>
                  <td className="py-2 pr-3 text-right">{row.n_partial}</td>
                  <td className="py-2 pr-3 text-right">{row.n_fail}</td>
                  <td className="py-2 pr-3 text-right">{row.n_unverified}</td>
                  <td className="py-2 pr-3 text-right">{row.n_high_fail}</td>
                  <td className="py-2 text-right">{Math.round(row.score_lower * 100)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
