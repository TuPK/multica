"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { ExternalLink } from "@multica/ui/icons";
import { knowledgeDocumentsOptions, type KnowledgeBase } from "@multica/core/knowledge";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink } from "../navigation";
import { useT } from "../i18n";
import { KnowledgeDocuments } from "./knowledge-documents";
import { KnowledgeSearch } from "./knowledge-search";
import { KnowledgeUploadPanel } from "./knowledge-upload";
import { KnowledgeBaseControls } from "./knowledge-setup";
import { KnowledgeWikiPanel } from "./knowledge-wiki";
import { buildKnowledgeTree, isProjectBase } from "./knowledge-tree";
import { ALL_DOCUMENTS, KnowledgeTreeView, type KnowledgeSelection } from "./knowledge-tree-view";

type BaseTab = "documents" | "wiki" | "add" | "settings";

/**
 * The library on one screen: the tree of bases and folders on the left, and on
 * the right what is selected. Reading, searching, adding documents, the Wiki and
 * the base's settings all live in that panel, so nothing about a base is
 * managed anywhere else.
 */
export function KnowledgeExplorer({
  wsId,
  knowledgeBases,
  canManage,
  secretaryAgentId,
}: {
  wsId: string;
  knowledgeBases: KnowledgeBase[];
  canManage: boolean;
  secretaryAgentId: string | null;
}) {
  const [selection, setSelection] = useState<KnowledgeSelection>(ALL_DOCUMENTS);
  const documents = useQuery(knowledgeDocumentsOptions(wsId, {}));
  const docs = documents.data?.documents;
  const tree = useMemo(() => buildKnowledgeTree(knowledgeBases, docs ?? []), [knowledgeBases, docs]);
  const base = knowledgeBases.find((candidate) => candidate.key === selection.kb);

  return (
    <div className="flex flex-col gap-5">
      <KnowledgeSearch
        wsId={wsId}
        knowledgeBases={knowledgeBases}
        secretaryAgentId={secretaryAgentId}
        scopeKb={base?.key}
      />
      <div className="grid items-start gap-6 md:grid-cols-[17rem_minmax(0,1fr)]">
        <aside className="md:sticky md:top-0 md:max-h-[calc(100vh-12rem)] md:overflow-y-auto">
          {documents.isPending ? (
            <Spinner />
          ) : (
            <KnowledgeTreeView
              tree={tree}
              totalCount={docs?.length ?? 0}
              selection={selection}
              onSelect={setSelection}
            />
          )}
        </aside>
        <section className="min-w-0">
          {base ? (
            <KnowledgeBasePanel
              key={base.key}
              wsId={wsId}
              base={base}
              knowledgeBases={knowledgeBases}
              folder={selection.folder}
              canManage={canManage}
              onFolder={(folder) => setSelection({ kb: base.key, folder })}
            />
          ) : (
            <KnowledgeDocuments wsId={wsId} knowledgeBases={knowledgeBases} />
          )}
        </section>
      </div>
    </div>
  );
}

function KnowledgeBasePanel({
  wsId,
  base,
  knowledgeBases,
  folder,
  canManage,
  onFolder,
}: {
  wsId: string;
  base: KnowledgeBase;
  knowledgeBases: KnowledgeBase[];
  folder: string;
  canManage: boolean;
  onFolder: (folder: string) => void;
}) {
  const { t } = useT("knowledge");
  const paths = useWorkspacePaths();
  const [tab, setTab] = useState<BaseTab>("documents");
  const segments = folder.split("/").filter((segment) => segment.length > 0);

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <header className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="min-w-0 truncate text-lg font-semibold">{base.name}</h2>
          <Badge variant="outline">
            {base.visibility === "restricted"
              ? t(($) => $.explorer.visibility.restricted)
              : t(($) => $.explorer.visibility.workspace)}
          </Badge>
          <Badge variant={base.wiki_enabled ? "secondary" : "outline"}>
            {base.wiki_enabled ? t(($) => $.explorer.wiki_on) : t(($) => $.explorer.wiki_off)}
          </Badge>
          {base.summaries_enabled ? <Badge variant="outline">{t(($) => $.explorer.summaries_on)}</Badge> : null}
          {isProjectBase(base) ? (
            <AppLink
              href={paths.projectDetail(base.project_id ?? "")}
              className="ml-auto inline-flex items-center gap-1 text-caption text-primary hover:underline"
            >
              <ExternalLink className="size-3.5" />
              {t(($) => $.explorer.open_project)}
            </AppLink>
          ) : null}
        </div>
        <p className="text-caption text-muted-foreground">
          {isProjectBase(base) ? t(($) => $.explorer.project_hint) : t(($) => $.explorer.company_hint)}
        </p>
        {segments.length > 0 ? (
          <nav aria-label={t(($) => $.explorer.path_label)} className="flex flex-wrap items-center gap-1 text-caption">
            <button type="button" className="text-primary hover:underline" onClick={() => onFolder("")}>
              {base.name}
            </button>
            {segments.map((segment, index) => {
              const path = segments.slice(0, index + 1).join("/");
              const last = index === segments.length - 1;
              return (
                <span key={path} className="flex items-center gap-1">
                  <span aria-hidden="true" className="text-muted-foreground">
                    /
                  </span>
                  {last ? (
                    <span aria-current="page">{segment}</span>
                  ) : (
                    <button type="button" className="text-primary hover:underline" onClick={() => onFolder(path)}>
                      {segment}
                    </button>
                  )}
                </span>
              );
            })}
          </nav>
        ) : null}
      </header>
      <Tabs value={tab} onValueChange={(value) => setTab(value as BaseTab)}>
        <TabsList variant="line">
          <TabsTrigger value="documents">{t(($) => $.explorer.tabs.documents)}</TabsTrigger>
          <TabsTrigger value="wiki">{t(($) => $.explorer.tabs.wiki)}</TabsTrigger>
          <TabsTrigger value="add">{t(($) => $.explorer.tabs.add)}</TabsTrigger>
          {canManage ? <TabsTrigger value="settings">{t(($) => $.explorer.tabs.settings)}</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="documents" className="pt-4">
          <KnowledgeDocuments wsId={wsId} knowledgeBases={knowledgeBases} scopeKb={base.key} folder={folder} />
        </TabsContent>
        <TabsContent value="wiki" className="pt-4">
          <KnowledgeWikiPanel wsId={wsId} base={base} canManage={canManage} />
        </TabsContent>
        <TabsContent value="add" className="pt-4">
          <KnowledgeUploadPanel wsId={wsId} knowledgeBases={[base]} />
        </TabsContent>
        {canManage ? (
          <TabsContent value="settings" className="pt-4">
            <KnowledgeBaseControls wsId={wsId} base={base} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
