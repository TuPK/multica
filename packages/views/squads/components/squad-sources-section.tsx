"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Spinner } from "@multica/ui/components/ui/spinner";
import {
  knowledgeCollectionOptions,
  useUpdateKnowledgeProperties,
  type KnowledgeDocument,
} from "@multica/core/knowledge";
import { useCreateSquadShare, useCreateSquadSource } from "@multica/core/squads";
import { useT } from "../../i18n";
import { PropertyDialog } from "../../knowledge/knowledge-collections";

/** The status values a source document can hold, in the order the Studio lists them. */
const STATUS_ORDER = ["active", "proposed", "needs_connector", "manual_only", "degraded", "blocked", "disabled"] as const;
type SourceStatus = (typeof STATUS_ORDER)[number];

function isSourceStatus(value: string): value is SourceStatus {
  return (STATUS_ORDER as readonly string[]).includes(value);
}

/** A source's status as the document holds it; a value this build does not know is "unknown". */
function statusOf(doc: KnowledgeDocument): SourceStatus | "unknown" {
  const value = doc.properties?.status;
  return typeof value === "string" && isSourceStatus(value) ? value : "unknown";
}

/** The sources of a Sales Loop squad: a list the knowledge base holds, and the two intake boxes. */
export function SquadSourcesSection({
  wsId,
  squadId,
  canManage,
}: {
  wsId: string;
  squadId: string;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const sources = useQuery(knowledgeCollectionOptions(wsId, { kind: "sales_source", kb: "sales" }));
  const addSource = useCreateSquadSource(wsId, squadId);
  const share = useCreateSquadShare(wsId, squadId);
  const update = useUpdateKnowledgeProperties(wsId);
  const [open, setOpen] = useState<KnowledgeDocument | null>(null);
  const [input, setInput] = useState("");
  const [shared, setShared] = useState("");
  const [message, setMessage] = useState("");

  const submit = async (fn: () => Promise<unknown>, done: string) => {
    setMessage("");
    try {
      await fn();
      setMessage(done);
    } catch (err) {
      setMessage(err instanceof Error ? err.message : t(($) => $.sources.error));
    }
  };

  const docs = sources.data?.documents ?? [];
  const groups = STATUS_ORDER.map((status) => ({
    status,
    count: docs.filter((d) => statusOf(d) === status).length,
  })).filter((g) => g.count > 0);

  return (
    <section className="space-y-4 rounded-lg border p-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-body font-semibold">{t(($) => $.sources.title)}</h2>
        <div className="flex flex-wrap gap-1.5">
          {groups.map((g) => (
            <Badge key={g.status} variant="outline">
              {t(($) => $.sources.status[g.status])}: {g.count}
            </Badge>
          ))}
        </div>
      </header>

      {canManage ? (
        <div className="grid gap-4 md:grid-cols-2">
          <form
            className="space-y-2"
            onSubmit={(event) => {
              event.preventDefault();
              const value = input.trim();
              if (!value) return;
              void submit(async () => {
                await addSource.mutateAsync({ input: value });
                setInput("");
              }, t(($) => $.sources.added));
            }}
          >
            <Input
              value={input}
              onChange={(event) => setInput(event.target.value)}
              placeholder={t(($) => $.sources.add_placeholder)}
              aria-label={t(($) => $.sources.add_placeholder)}
            />
            <Button type="submit" size="sm" disabled={!input.trim() || addSource.isPending}>
              {t(($) => $.sources.add)}
            </Button>
          </form>
          <form
            className="space-y-2"
            onSubmit={(event) => {
              event.preventDefault();
              const value = shared.trim();
              if (!value) return;
              void submit(async () => {
                await share.mutateAsync({ input: value });
                setShared("");
              }, t(($) => $.sources.shared));
            }}
          >
            <Textarea
              value={shared}
              onChange={(event) => setShared(event.target.value)}
              rows={3}
              placeholder={t(($) => $.sources.share_placeholder)}
              aria-label={t(($) => $.sources.share_placeholder)}
            />
            <Button type="submit" size="sm" variant="outline" disabled={!shared.trim() || share.isPending}>
              {t(($) => $.sources.share)}
            </Button>
          </form>
        </div>
      ) : null}
      {message ? <p className="text-caption text-muted-foreground">{message}</p> : null}

      {sources.isPending ? (
        <div className="flex justify-center py-4">
          <Spinner />
        </div>
      ) : docs.length === 0 ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.sources.empty)}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-left text-body">
            <thead className="bg-muted/40 text-caption text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.name)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.platform)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.rung)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.status)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.last_run)}</th>
                <th className="px-3 py-2 font-medium">{t(($) => $.sources.columns.items)}</th>
              </tr>
            </thead>
            <tbody>
              {docs.map((doc) => (
                <tr key={doc.id} className="border-t">
                  <td className="px-3 py-2">
                    <button
                      type="button"
                      className="text-left hover:underline"
                      onClick={() => setOpen(doc)}
                    >
                      {doc.title}
                    </button>
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">{String(doc.properties?.platform ?? "")}</td>
                  <td className="px-3 py-2 text-muted-foreground">{String(doc.properties?.rung ?? "")}</td>
                  <td className="px-3 py-2">{t(($) => $.sources.status[statusOf(doc)])}</td>
                  <td className="px-3 py-2 text-muted-foreground">{String(doc.properties?.last_run ?? "")}</td>
                  <td className="px-3 py-2 text-muted-foreground">{String(doc.properties?.hits_7d ?? 0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open ? (
        <PropertyDialog
          doc={open}
          fields={sources.data?.fields ?? []}
          onClose={() => setOpen(null)}
          onSave={(properties) => update.mutateAsync({ id: open.id, properties })}
        />
      ) : null}
    </section>
  );
}
