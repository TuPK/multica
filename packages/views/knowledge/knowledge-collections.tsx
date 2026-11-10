"use client";

import { useMemo, useState, type DragEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  knowledgeCollectionOptions,
  knowledgeKindsOptions,
  useUpdateKnowledgeProperties,
  type KindField,
  type KnowledgeBase,
  type KnowledgeCollectionParams,
  type KnowledgeDocument,
  type KnowledgeKind,
} from "@multica/core/knowledge";
import { useT } from "../i18n";

/**
 * A saved view of the Sales Loop: a kind, the filters that make it useful, and the layout that
 * shows it best. A view is a way of reading the knowledge base; the documents are not copied.
 */
interface SavedView {
  id: "pipeline" | "needs_you" | "due" | "lessons" | "sources";
  kind: string;
  match?: Record<string, string>;
  dueKey?: string;
  layout: "board" | "table";
  /** The select field a board groups its columns by. */
  groupBy?: string;
}

export const SALES_SAVED_VIEWS: readonly SavedView[] = [
  { id: "pipeline", kind: "sales_account", layout: "board", groupBy: "stage" },
  { id: "needs_you", kind: "sales_outreach", match: { state: "checked" }, layout: "table" },
  { id: "due", kind: "sales_account", dueKey: "next_action_date", layout: "table" },
  { id: "lessons", kind: "sales_lesson", layout: "table" },
  { id: "sources", kind: "sales_source", layout: "table" },
];

/** Today as a YYYY-MM-DD day, in UTC, which the server compares with its stored dates. */
export function todayISO(now: Date = new Date()): string {
  return now.toISOString().slice(0, 10);
}

/** Fields a table shows as columns: the first few, skipping the long ones. */
export function tableFields(fields: KindField[]): KindField[] {
  return fields.filter((f) => f.type !== "url").slice(0, 5);
}

/** The select field a board groups by: the one the view names, else the first ordered select. */
export function boardField(fields: KindField[], groupBy?: string): KindField | undefined {
  return (
    fields.find((f) => f.key === groupBy && f.type === "select") ??
    fields.find((f) => f.type === "select" && f.ordered)
  );
}

/** Groups documents by one select value. The columns follow the field's options, in order. */
export function groupByValue(
  docs: KnowledgeDocument[],
  field: KindField,
): { option: string; docs: KnowledgeDocument[] }[] {
  const columns = field.options.map((option) => ({ option, docs: [] as KnowledgeDocument[] }));
  for (const doc of docs) {
    const value = doc.properties?.[field.key];
    const column = columns.find((c) => c.option === value);
    if (column) column.docs.push(doc);
  }
  return columns;
}

function formatValue(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "boolean") return value ? "yes" : "no";
  if (typeof value === "number") return value.toLocaleString();
  return String(value);
}

/**
 * The Collections tab of a knowledge base: the documents of its kinds that carry fields, as a table or
 * a board, with saved views for the Sales Loop. A property is edited in the detail dialog; a board card
 * moved to another column is a person's change to that property.
 */
export function KnowledgeCollections({ wsId, base }: { wsId: string; base: KnowledgeBase }) {
  const { t } = useT("knowledge");
  const kindsQuery = useQuery(knowledgeKindsOptions(wsId));
  const kinds = useMemo(
    () =>
      (kindsQuery.data?.kinds ?? []).filter(
        (k: KnowledgeKind) => k.default_base_key === base.key && k.fields.length > 0,
      ),
    [kindsQuery.data, base.key],
  );
  const [viewId, setViewId] = useState<SavedView["id"] | "browse">("browse");
  const [kindKey, setKindKey] = useState("");
  const [open, setOpen] = useState<KnowledgeDocument | null>(null);

  const saved = SALES_SAVED_VIEWS.find((v) => v.id === viewId);
  const kind = kinds.find((k) => k.key === (saved?.kind ?? (kindKey || kinds[0]?.key)));
  const view: SavedView = saved ?? { id: "pipeline", kind: kind?.key ?? "", layout: "table" };

  const params: KnowledgeCollectionParams | null = kind
    ? {
        kind: kind.key,
        kb: base.key,
        match: saved?.match,
        dueKey: saved?.dueKey,
        dueOn: saved?.dueKey ? todayISO() : undefined,
      }
    : null;
  const collection = useQuery({
    ...knowledgeCollectionOptions(wsId, params ?? { kind: "" }),
    enabled: params !== null,
  });
  const update = useUpdateKnowledgeProperties(wsId);

  if (kindsQuery.isPending) {
    return (
      <div className="flex justify-center py-8">
        <Spinner />
      </div>
    );
  }
  if (kinds.length === 0) {
    return <p className="text-caption text-muted-foreground">{t(($) => $.collections.no_kinds)}</p>;
  }

  const fields = collection.data?.fields ?? kind?.fields ?? [];
  const docs = collection.data?.documents ?? [];
  const board = view.layout === "board" ? boardField(fields, view.groupBy) : undefined;
  const columns = board ? groupByValue(docs, board) : [];
  const availableViews = SALES_SAVED_VIEWS.filter((v) => kinds.some((k) => k.key === v.kind));

  const moveTo = (doc: KnowledgeDocument, field: KindField, option: string) => {
    if (doc.properties?.[field.key] === option) return;
    update.mutate({ id: doc.id, properties: { [field.key]: option } });
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant={viewId === "browse" ? "default" : "outline"}
          onClick={() => setViewId("browse")}
        >
          {t(($) => $.collections.browse)}
        </Button>
        {availableViews.map((v) => (
          <Button
            key={v.id}
            size="sm"
            variant={viewId === v.id ? "default" : "outline"}
            onClick={() => setViewId(v.id)}
          >
            {t(($) => $.collections.views[v.id])}
          </Button>
        ))}
        {viewId === "browse" ? (
          <Select
            items={kinds.map((k) => ({ value: k.key, label: k.name }))}
            value={kind?.key ?? ""}
            onValueChange={(next) => next && setKindKey(next)}
          >
            <SelectTrigger size="sm" aria-label={t(($) => $.collections.kind_label)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {kinds.map((k) => (
                <SelectItem key={k.key} value={k.key}>
                  {k.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
      </div>

      {collection.isPending ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : collection.isError ? (
        <p className="text-caption text-destructive">{t(($) => $.collections.load_error)}</p>
      ) : docs.length === 0 ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.collections.empty)}</p>
      ) : board ? (
        <div className="grid gap-3 overflow-x-auto md:grid-flow-col md:auto-cols-[minmax(12rem,1fr)]">
          {columns.map((column) => (
            <div
              key={column.option}
              className="flex min-h-24 flex-col gap-2 rounded-lg border bg-muted/30 p-2"
              onDragOver={(event: DragEvent<HTMLDivElement>) => event.preventDefault()}
              onDrop={(event: DragEvent<HTMLDivElement>) => {
                event.preventDefault();
                const id = event.dataTransfer.getData("text/plain");
                const doc = docs.find((d) => d.id === id);
                if (doc && board) moveTo(doc, board, column.option);
              }}
            >
              <div className="flex items-center justify-between text-caption font-medium">
                <span>{column.option}</span>
                <span className="text-muted-foreground">{column.docs.length}</span>
              </div>
              {column.docs.map((doc) => (
                <button
                  key={doc.id}
                  type="button"
                  draggable
                  onDragStart={(event) => event.dataTransfer.setData("text/plain", doc.id)}
                  onClick={() => setOpen(doc)}
                  className="rounded-md border bg-background px-2 py-1.5 text-left text-body hover:bg-accent/50"
                >
                  <span className="block truncate">{doc.title}</span>
                  <span className="block truncate text-caption text-muted-foreground">
                    {tableFields(fields)
                      .filter((f) => f.key !== board.key)
                      .slice(0, 2)
                      .map((f) => formatValue(doc.properties?.[f.key]))
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </button>
              ))}
            </div>
          ))}
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-left text-body">
            <thead className="bg-muted/40 text-caption text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">{t(($) => $.collections.title_column)}</th>
                {tableFields(fields).map((f) => (
                  <th key={f.key} className="px-3 py-2 font-medium">
                    {f.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {docs.map((doc) => (
                <tr
                  key={doc.id}
                  className="cursor-pointer border-t hover:bg-accent/50"
                  onClick={() => setOpen(doc)}
                >
                  <td className="px-3 py-2">{doc.title}</td>
                  {tableFields(fields).map((f) => (
                    <td key={f.key} className="px-3 py-2 text-muted-foreground">
                      {formatValue(doc.properties?.[f.key])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open ? (
        <PropertyDialog
          doc={open}
          fields={fields}
          onClose={() => setOpen(null)}
          onSave={(properties) => update.mutateAsync({ id: open.id, properties })}
        />
      ) : null}
    </div>
  );
}

/** The detail of one document: its properties, edited in place. A failed save is shown, not kept. */
function PropertyDialog({
  doc,
  fields,
  onClose,
  onSave,
}: {
  doc: KnowledgeDocument;
  fields: KindField[];
  onClose: () => void;
  onSave: (properties: Record<string, unknown>) => Promise<unknown>;
}) {
  const { t } = useT("knowledge");
  const [error, setError] = useState("");
  const save = async (key: string, value: unknown) => {
    setError("");
    try {
      await onSave({ [key]: value });
    } catch (err) {
      setError(err instanceof Error ? err.message : t(($) => $.collections.save_error));
    }
  };
  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-title-sm font-semibold">{doc.title}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          {fields.map((field) => (
            <div key={field.key} className="grid grid-cols-[9rem_1fr] items-center gap-3">
              <span className="text-caption text-muted-foreground">{field.label}</span>
              <PropertyInput
                field={field}
                value={doc.properties?.[field.key]}
                onChange={(value) => save(field.key, value)}
              />
            </div>
          ))}
          {error ? <p className="text-caption text-destructive">{error}</p> : null}
          {doc.properties && Object.keys(doc.properties).length === 0 ? (
            <Badge variant="outline">{t(($) => $.collections.no_properties)}</Badge>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function PropertyInput({
  field,
  value,
  onChange,
}: {
  field: KindField;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  if (field.type === "bool") {
    return <Switch checked={value === true} onCheckedChange={(checked) => onChange(checked === true)} />;
  }
  if (field.type === "select") {
    return (
      <Select
        items={field.options.map((option) => ({ value: option, label: option }))}
        value={typeof value === "string" ? value : ""}
        onValueChange={(next) => next && onChange(next)}
      >
        <SelectTrigger size="sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {field.options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  if (field.type === "number") {
    return (
      <Input
        type="number"
        defaultValue={typeof value === "number" ? String(value) : ""}
        onBlur={(event) => {
          if (event.target.value !== "") onChange(Number(event.target.value));
        }}
      />
    );
  }
  if (field.type === "date") {
    return (
      <Input
        type="date"
        defaultValue={typeof value === "string" ? value : ""}
        onChange={(event) => {
          if (event.target.value) onChange(event.target.value);
        }}
      />
    );
  }
  return (
    <Input
      type={field.type === "url" ? "url" : "text"}
      defaultValue={typeof value === "string" ? value : ""}
      onBlur={(event) => {
        const next = event.target.value.trim();
        if (next !== formatValue(value)) onChange(next);
      }}
    />
  );
}
