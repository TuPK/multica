"use client";

import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { runtimeListOptions } from "@multica/core/runtimes";
import { parseWebEngine, type WebEngineStatus } from "@multica/core/squads";
import { useT } from "../../i18n";

const STATUS_ORDER: readonly WebEngineStatus[] = ["running", "not_running", "not_configured", "unknown"];

/**
 * The Web engine on each machine that has reported it. The daemon checks its own machine, so a
 * machine with no report has no engine status yet; the card says so rather than guessing.
 */
export function WebEngineCard({ wsId }: { wsId: string }) {
  const { t } = useT("squads");
  const runtimes = useQuery(runtimeListOptions(wsId));

  if (runtimes.isPending) {
    return (
      <section className="flex justify-center rounded-lg border p-4">
        <Spinner />
      </section>
    );
  }

  const rows = (runtimes.data ?? []).flatMap((runtime) => {
    const engine = parseWebEngine(runtime.metadata);
    return engine ? [{ runtime, engine }] : [];
  });

  return (
    <section className="space-y-4 rounded-lg border p-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-body font-semibold">{t(($) => $.web_engine.title)}</h2>
        <div className="flex flex-wrap gap-1.5">
          {STATUS_ORDER.map((status) => {
            const count = rows.filter((row) => row.engine.status === status).length;
            return count > 0 ? (
              <Badge key={status} variant="outline">
                {t(($) => $.web_engine.status[status])}: {count}
              </Badge>
            ) : null;
          })}
        </div>
      </header>

      {rows.length === 0 ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.web_engine.empty)}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-left text-body">
            <thead className="bg-muted/40 text-caption text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">{t(($) => $.web_engine.columns.runtime)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.web_engine.columns.status)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.web_engine.columns.version)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.web_engine.columns.checked)}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ runtime, engine }) => (
                <tr key={runtime.id} className="border-t">
                  <td className="px-3 py-2">{runtime.custom_name || runtime.name}</td>
                  <td className="px-3 py-2">{t(($) => $.web_engine.status[engine.status])}</td>
                  <td className="px-3 py-2 text-muted-foreground">{engine.version || "-"}</td>
                  <td className="px-3 py-2 text-muted-foreground">
                    {engine.checkedAt ? new Date(engine.checkedAt).toLocaleString() : "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
