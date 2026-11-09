"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { BookOpen, Loader2 } from "@multica/ui/icons";
import {
  knowledgeWikiOverviewOptions,
  knowledgeWikiPageOptions,
  knowledgeWikiPagesOptions,
  useGenerateKnowledgeWiki,
  type KnowledgeBase,
  type KnowledgeWikiPage,
  type KnowledgeWikiPageType,
} from "@multica/core/knowledge";
import { useT } from "../i18n";
import { ReadonlyContent } from "../editor/readonly-content";

type TypeFilter = "all" | Exclude<KnowledgeWikiPageType, "unknown">;

const TYPE_FILTERS: TypeFilter[] = ["all", "summary", "entity", "concept"];

/**
 * The Wiki of one base: the pages the knowledge service writes from its
 * documents. Owners and admins start the generation; everyone who can read the
 * base reads the pages. While documents are still being turned into pages the
 * panel refreshes on its own.
 */
export function KnowledgeWikiPanel({
  wsId,
  base,
  canManage,
}: {
  wsId: string;
  base: KnowledgeBase;
  canManage: boolean;
}) {
  const { t } = useT("knowledge");
  const [filter, setFilter] = useState<TypeFilter>("all");
  const [slug, setSlug] = useState("");
  // Set after a generation request so the panel keeps refreshing while the
  // pages appear; the server has no "still writing" signal to read instead.
  const [watching, setWatching] = useState(false);

  const overview = useQuery(knowledgeWikiOverviewOptions(wsId, base.key, watching));
  const enabled = overview.data?.enabled ?? false;
  const params = filter === "all" ? { page_size: 200 } : { page_size: 200, type: filter };
  const pages = useQuery({
    ...knowledgeWikiPagesOptions(wsId, base.key, params, watching),
    enabled: enabled,
  });
  const page = useQuery(knowledgeWikiPageOptions(wsId, base.key, slug));
  const generate = useGenerateKnowledgeWiki(wsId);

  const start = () =>
    generate.mutate(
      { key: base.key },
      {
        onSuccess: (result) => {
          setWatching(true);
          toast.success(t(($) => $.wiki.toast.queued, { count: result.queued }));
        },
        onError: (error) => toast.error(error.message || t(($) => $.wiki.toast.failed)),
      },
    );

  if (overview.isPending) return <Spinner />;
  if (overview.isError) {
    return (
      <p role="alert" className="text-caption text-destructive">
        {t(($) => $.load_error.title)}
      </p>
    );
  }
  const info = overview.data;

  const typeLabel = (value: string): string => {
    switch (value) {
      case "summary":
        return t(($) => $.wiki.type.summary);
      case "entity":
        return t(($) => $.wiki.type.entity);
      case "concept":
        return t(($) => $.wiki.type.concept);
      case "all":
        return t(($) => $.wiki.type.all);
      default:
        return t(($) => $.wiki.type.unknown);
    }
  };

  const generateButton = canManage ? (
    <Button
      size="sm"
      variant={info.pages === 0 ? "default" : "outline"}
      onClick={start}
      disabled={generate.isPending || info.documents === 0 || !info.chat_configured}
      aria-busy={generate.isPending}
    >
      {info.enabled && info.pages > 0 ? t(($) => $.wiki.rewrite) : t(($) => $.wiki.generate)}
    </Button>
  ) : null;

  if (!info.enabled) {
    return (
      <div className="flex flex-col items-start gap-3 rounded-lg border border-dashed p-6">
        <div className="flex items-center gap-2 text-body font-medium">
          <BookOpen className="size-4 text-muted-foreground" />
          {t(($) => $.wiki.off_title)}
        </div>
        <p className="text-caption text-muted-foreground">
          {info.documents === 0
            ? t(($) => $.wiki.no_documents)
            : !info.chat_configured
              ? t(($) => $.wiki.no_chat_model)
              : canManage
                ? t(($) => $.wiki.off_hint, { count: info.documents })
                : t(($) => $.wiki.off_member)}
        </p>
        {generateButton}
      </div>
    );
  }

  const list = pages.data?.pages ?? [];
  const selected: KnowledgeWikiPage | undefined = slug ? page.data : undefined;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">{t(($) => $.wiki.pages, { count: info.pages })}</Badge>
        {watching && info.pages < info.documents ? (
          <span role="status" className="flex items-center gap-1 text-caption text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t(($) => $.wiki.writing)}
          </span>
        ) : null}
        <span className="flex-1" />
        {generateButton}
      </div>
      <div className="flex flex-wrap items-center gap-1">
        {TYPE_FILTERS.map((value) => (
          <Button
            key={value}
            size="sm"
            variant={filter === value ? "secondary" : "ghost"}
            aria-pressed={filter === value}
            onClick={() => {
              setFilter(value);
              setSlug("");
            }}
          >
            {typeLabel(value)}
            {value !== "all" && info.pages_by_type[value] ? (
              <span className="ml-1 text-muted-foreground">{info.pages_by_type[value]}</span>
            ) : null}
          </Button>
        ))}
      </div>
      {pages.isPending ? (
        <Spinner />
      ) : list.length === 0 ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.wiki.none_yet)}</p>
      ) : (
        <div className="grid min-h-0 gap-4 md:grid-cols-[16rem_minmax(0,1fr)]">
          <ul className="flex max-h-[28rem] flex-col overflow-y-auto rounded-lg border">
            {list.map((item) => (
              <li key={item.slug}>
                <button
                  type="button"
                  aria-current={item.slug === slug ? "true" : undefined}
                  onClick={() => setSlug(item.slug)}
                  className={`flex w-full flex-col gap-0.5 px-3 py-2 text-left hover:bg-accent ${
                    item.slug === slug ? "bg-accent" : ""
                  }`}
                >
                  <span className="truncate text-body">{item.title}</span>
                  <span className="text-caption text-muted-foreground">{typeLabel(item.page_type)}</span>
                </button>
              </li>
            ))}
          </ul>
          <article className="min-w-0 rounded-lg border p-4">
            {!slug ? (
              <p className="text-caption text-muted-foreground">{t(($) => $.wiki.pick)}</p>
            ) : page.isPending ? (
              <Spinner />
            ) : page.isError || !selected ? (
              <p role="alert" className="text-caption text-destructive">
                {t(($) => $.load_error.title)}
              </p>
            ) : (
              <div className="flex flex-col gap-3">
                <h3 className="text-body font-semibold">{selected.title}</h3>
                {selected.category_path.length > 0 ? (
                  <p className="text-caption text-muted-foreground">{selected.category_path.join(" / ")}</p>
                ) : null}
                <ReadonlyContent content={selected.content} />
              </div>
            )}
          </article>
        </div>
      )}
    </div>
  );
}
