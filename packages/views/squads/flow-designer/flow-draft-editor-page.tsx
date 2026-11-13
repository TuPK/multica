"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import {
  useExportFlowDraft,
  useFlowCapabilities,
  useFlowDraft,
  useValidateFlowDraft,
} from "@multica/core/flow-drafts";
import type { FlowCapabilitiesResponse, FlowDocStep, FlowDraft, FlowIssue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  AlertCircle,
  ArrowLeft,
  Check,
  Download,
  Loader2,
  Play,
  RotateCcw,
  Upload,
} from "@multica/ui/icons";
import { useT } from "../../i18n";
import { AppLink, useNavigation } from "../../navigation";
import { useDraftBuffer } from "./use-draft-buffer";
import { DraftStatus } from "./draft-status";
import { Outline } from "./outline";
import { StepInspector } from "./step-inspector";
import { RolesPanel } from "./roles-panel";
import { VarsPanel } from "./vars-panel";
import { VerdictsPanel } from "./verdicts-panel";
import { LoopsPanel } from "./loops-panel";
import { StopsPanel } from "./stops-panel";
import { TerminalsPanel } from "./terminals-panel";
import { ProblemsPanel } from "./problems-panel";
import { SimulateDialog } from "./simulate-dialog";
import { PublishDialog } from "./publish-dialog";
import { FlowGraph } from "./flow-graph";
import { downloadJson, manifestFileName } from "./download-json";

export interface FlowDraftEditorPageProps {
  squadId: string;
  draftId: string;
}

// What this build of the editor can draw. A server that lists more is ahead of the app: the page says so
// instead of offering controls that would save a document it cannot show.
const KNOWN_STEP_KINDS = ["tasks", "lead", "manual"];
const KNOWN_TASK_KINDS = ["work", "gate"];

function hasUnsupportedKinds(capabilities: FlowCapabilitiesResponse | undefined): boolean {
  if (!capabilities) return false;
  return (
    capabilities.step_kinds.some((kind) => !KNOWN_STEP_KINDS.includes(kind)) ||
    capabilities.task_kinds.some((kind) => !KNOWN_TASK_KINDS.includes(kind))
  );
}

/**
 * The page of one draft. It loads the draft and mounts the editor only with a draft in hand: the
 * editor keeps its own buffer, started from the draft it is given, and a buffer started from a
 * placeholder would save the placeholder over the real document.
 */
export function FlowDraftEditorPage({ squadId, draftId }: FlowDraftEditorPageProps) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const nav = useNavigation();
  const { t } = useT("squads");

  const capabilitiesQuery = useFlowCapabilities(wsId);
  const draftQuery = useFlowDraft(squadId, draftId, wsId);
  const draft = draftQuery.data;

  const backHref = `${paths.squads()}/${squadId}`;
  const status = (draftQuery.error as { status?: number } | null)?.status;

  useEffect(() => {
    if (draftQuery.isError && status === 404) nav.replace(backHref);
  }, [draftQuery.isError, status, nav, backHref]);

  if (capabilitiesQuery.data && !capabilitiesQuery.data.features.authoring) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 p-12 text-center">
        <AlertCircle className="size-8 text-muted-foreground" aria-hidden />
        <h2 className="text-body font-semibold">{t(($) => $.flow_designer.editor.disabled)}</h2>
        <AppLink href={backHref}>
          <Button variant="outline" size="sm">
            {t(($) => $.flow_designer.editor.back)}
          </Button>
        </AppLink>
      </div>
    );
  }

  // The empty draft the client falls back to for an answer it cannot read has no id: it is not a
  // draft of this squad, and editing it would save a blank document over the real one.
  const unreadable = draftQuery.isSuccess && !draft?.id;
  if ((draftQuery.isError && status !== 404) || unreadable) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 p-12 text-center">
        <AlertCircle className="size-8 text-destructive" aria-hidden />
        <h2 className="text-body font-semibold text-destructive">{t(($) => $.flow_designer.load_failed)}</h2>
        <Button type="button" variant="outline" size="sm" onClick={() => void draftQuery.refetch()} className="gap-1.5">
          <RotateCcw className="size-3.5" aria-hidden />
          <span>{t(($) => $.flow_designer.retry)}</span>
        </Button>
      </div>
    );
  }

  if (!draft) {
    return (
      <div className="flex h-96 items-center justify-center">
        <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden />
      </div>
    );
  }

  return (
    <FlowDraftEditor
      key={draft.id}
      squadId={squadId}
      draftId={draftId}
      draft={draft}
      capabilities={capabilitiesQuery.data}
      onReload={async () => {
        const latest = await draftQuery.refetch();
        return latest.data?.id ? latest.data : null;
      }}
    />
  );
}

type EditorTab =
  | "editor"
  | "graph"
  | "roles"
  | "vars"
  | "verdicts"
  | "loops"
  | "stops"
  | "terminals"
  | "problems";

interface FlowDraftEditorProps {
  squadId: string;
  draftId: string;
  draft: FlowDraft;
  capabilities: FlowCapabilitiesResponse | undefined;
  /** Reads the draft again from the server; null when it could not be read. */
  onReload: () => Promise<FlowDraft | null>;
}

function FlowDraftEditor({ squadId, draftId, draft, capabilities, onReload }: FlowDraftEditorProps) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const { t } = useT("squads");
  const { role } = useCurrentMember(wsId);
  const isAdmin = role === "owner" || role === "admin";

  const [activeTab, setActiveTab] = useState<EditorTab>("editor");
  const [selectedStepKey, setSelectedStepKey] = useState<string | null>(null);
  const [simulateOpen, setSimulateOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [issues, setIssues] = useState<FlowIssue[]>([]);

  const validateMut = useValidateFlowDraft();
  const exportMut = useExportFlowDraft();
  const buffer = useDraftBuffer({ wsId, squadId, draftId, initialDraft: draft });

  const currentStep = useMemo(() => {
    const steps = buffer.doc.steps ?? [];
    return steps.find((s) => s.key === selectedStepKey) ?? steps[0] ?? null;
  }, [buffer.doc.steps, selectedStepKey]);

  // Checks what is in the buffer, saved or not.
  const handleValidate = async () => {
    setActiveTab("problems");
    try {
      const res = await validateMut.mutateAsync({ squadId, draftId, doc: buffer.doc });
      setIssues(res.issues);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.editor.check_failed));
    }
  };

  // The export is the server's manifest of the saved draft, without the people bound to its roles.
  const handleExport = async () => {
    try {
      if ((await buffer.saveNow()) === null) {
        toast.error(t(($) => $.flow_designer.editor.not_saved));
        return;
      }
      const manifest = await exportMut.mutateAsync({ squadId, draftId });
      downloadJson(manifestFileName(buffer.name), manifest);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.editor.export_failed));
    }
  };

  const handleReload = async () => {
    const latest = await onReload();
    if (latest) buffer.reload(latest);
  };

  const tabs: { id: EditorTab; label: string }[] = [
    { id: "editor", label: t(($) => $.flow_designer.editor.tab_editor) },
    { id: "graph", label: t(($) => $.flow_designer.editor.tab_graph) },
    { id: "roles", label: t(($) => $.flow_designer.roles.title) },
    { id: "vars", label: t(($) => $.flow_designer.vars.title) },
    { id: "verdicts", label: t(($) => $.flow_designer.verdicts.title) },
    { id: "loops", label: t(($) => $.flow_designer.loop.title) },
    { id: "stops", label: t(($) => $.flow_designer.stops.declared) },
    { id: "terminals", label: t(($) => $.flow_designer.editor.tab_terminals) },
    { id: "problems", label: t(($) => $.flow_designer.editor.tab_problems) },
  ];

  return (
    <div className="flex h-[calc(100vh-4rem)] w-full flex-col overflow-hidden bg-background">
      <header className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-card px-4 py-2.5">
        <div className="flex items-center gap-3">
          <AppLink href={`${paths.squads()}/${squadId}`} aria-label={t(($) => $.flow_designer.editor.back)}>
            <Button variant="ghost" size="xs" className="h-8 w-8 p-0">
              <ArrowLeft className="size-4" aria-hidden />
            </Button>
          </AppLink>

          <Input
            value={buffer.name}
            onChange={(e) => buffer.setName(e.target.value)}
            className="h-8 max-w-xs border-transparent px-2 text-body font-semibold hover:border-input focus:border-input"
            placeholder={t(($) => $.flow_designer.editor.name_placeholder)}
          />

          <DraftStatus
            isDirty={buffer.isDirty}
            isSaving={buffer.isSaving}
            saveError={buffer.saveError}
            isOffline={buffer.isOffline}
            onRetry={() => void buffer.saveNow()}
          />

          {hasUnsupportedKinds(capabilities) && (
            <Badge variant="outline" className="text-micro text-amber-600">
              {t(($) => $.flow_designer.unsupported)}
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={validateMut.isPending}
            onClick={() => void handleValidate()}
            className="h-8 gap-1.5 text-caption"
          >
            {validateMut.isPending ? (
              <Loader2 className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            <span>{t(($) => $.flow_designer.actions.check)}</span>
          </Button>

          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setSimulateOpen(true)}
            className="h-8 gap-1.5 text-caption"
          >
            <Play className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow_designer.actions.simulate)}</span>
          </Button>

          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={exportMut.isPending}
            onClick={() => void handleExport()}
            className="h-8 gap-1.5 text-caption"
          >
            <Download className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow_designer.export)}</span>
          </Button>

          <Button
            type="button"
            size="sm"
            disabled={issues.some((i) => i.severity === "error" && i.code !== "ack_required")}
            onClick={() => setPublishOpen(true)}
            className="h-8 gap-1.5 text-caption"
          >
            <Upload className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow_designer.actions.publish)}</span>
          </Button>
        </div>
      </header>

      <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border bg-muted/30 px-4 py-1.5 text-caption">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setActiveTab(tab.id)}
            className={`flex items-center gap-1 rounded px-2.5 py-1 font-medium transition-colors ${
              activeTab === tab.id
                ? "bg-background text-foreground shadow-xs"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <span>{tab.label}</span>
            {tab.id === "problems" && issues.length > 0 && (
              <Badge variant="destructive" className="h-4 px-1 text-micro">
                {issues.length}
              </Badge>
            )}
          </button>
        ))}
      </div>

      <div className="flex flex-1 overflow-hidden">
        {activeTab === "editor" && (
          <>
            <Outline
              doc={buffer.doc}
              selectedStepKey={currentStep?.key ?? null}
              onSelectStep={(key) => setSelectedStepKey(key)}
              onAddStep={(kind) => {
                const count = (buffer.doc.steps?.length ?? 0) + 1;
                const newStep: FlowDocStep = {
                  key: `step_${count}`,
                  title: t(($) => $.flow_designer.editor.step_title, { n: count }),
                  kind,
                  tasks: [],
                  edges: [],
                };
                buffer.setDoc((prev) => ({ ...prev, steps: [...(prev.steps ?? []), newStep] }));
                setSelectedStepKey(newStep.key);
              }}
              issues={issues}
              stepLimit={capabilities?.limits.steps ?? 40}
            />

            {currentStep ? (
              <StepInspector
                step={currentStep}
                onChangeStep={(updated) => {
                  buffer.setDoc((prev) => ({
                    ...prev,
                    steps: (prev.steps ?? []).map((s) => (s.key === updated.key ? updated : s)),
                  }));
                }}
                doc={buffer.doc}
                taskLimit={capabilities?.limits.tasks_per_step ?? 12}
              />
            ) : (
              <div className="flex flex-1 items-center justify-center text-caption text-muted-foreground">
                {t(($) => $.flow_designer.editor.select_step)}
              </div>
            )}
          </>
        )}

        {activeTab === "graph" && (
          <FlowGraph
            doc={buffer.doc}
            selectedStepKey={currentStep?.key ?? null}
            onSelectStep={(key) => {
              setSelectedStepKey(key);
              setActiveTab("editor");
            }}
            issues={issues}
          />
        )}

        {activeTab === "roles" && (
          <div className="flex-1 overflow-y-auto">
            <RolesPanel
              doc={buffer.doc}
              onChangeRoles={(roles) => buffer.setDoc((prev) => ({ ...prev, roles }))}
              squadId={squadId}
              roleLimit={capabilities?.limits.roles ?? 16}
            />
          </div>
        )}

        {activeTab === "vars" && (
          <div className="flex-1 overflow-y-auto">
            <VarsPanel
              doc={buffer.doc}
              onChangeVars={(vars) => buffer.setDoc((prev) => ({ ...prev, vars }))}
              varLimit={capabilities?.limits.vars ?? 16}
            />
          </div>
        )}

        {activeTab === "verdicts" && (
          <div className="flex-1 overflow-y-auto">
            <VerdictsPanel
              doc={buffer.doc}
              onChangeVerdicts={(verdicts) => buffer.setDoc((prev) => ({ ...prev, verdicts }))}
              verdictLimit={capabilities?.limits.verdict_sets ?? 8}
            />
          </div>
        )}

        {activeTab === "loops" && (
          <div className="flex-1 overflow-y-auto">
            <LoopsPanel
              doc={buffer.doc}
              onChangeLoops={(loops) => buffer.setDoc((prev) => ({ ...prev, loops }))}
              loopLimit={capabilities?.limits.loops ?? 8}
            />
          </div>
        )}

        {activeTab === "stops" && (
          <div className="flex-1 overflow-y-auto">
            <StopsPanel doc={buffer.doc} onChangeStops={(stops) => buffer.setDoc((prev) => ({ ...prev, stops }))} />
          </div>
        )}

        {activeTab === "terminals" && (
          <div className="flex-1 overflow-y-auto">
            <TerminalsPanel
              doc={buffer.doc}
              onChangeTerminals={(terminals) => buffer.setDoc((prev) => ({ ...prev, terminals }))}
              onChangePolicy={(policy) => buffer.setDoc((prev) => ({ ...prev, policy }))}
              isAdmin={isAdmin}
            />
          </div>
        )}

        {activeTab === "problems" && (
          <div className="flex-1 overflow-y-auto p-4">
            <ProblemsPanel issues={issues} onSelectPath={() => setActiveTab("editor")} />
          </div>
        )}
      </div>

      <Dialog open={buffer.isConflict}>
        <DialogContent className="max-w-md space-y-4 p-6">
          <DialogHeader className="space-y-1">
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertCircle className="size-5 shrink-0" aria-hidden />
              <span>{t(($) => $.flow_designer.conflict.title)}</span>
            </DialogTitle>
          </DialogHeader>
          <DialogFooter className="gap-2 pt-2">
            <Button type="button" variant="outline" size="sm" onClick={() => void handleReload()}>
              {t(($) => $.flow_designer.conflict.reload)}
            </Button>
            <Button type="button" size="sm" onClick={() => void buffer.keepAsCopy()}>
              {t(($) => $.flow_designer.conflict.keep_copy)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SimulateDialog
        open={simulateOpen}
        onOpenChange={setSimulateOpen}
        squadId={squadId}
        draftId={draftId}
        doc={buffer.doc}
      />

      <PublishDialog
        open={publishOpen}
        onOpenChange={setPublishOpen}
        squadId={squadId}
        draftId={draftId}
        doc={buffer.doc}
        isAdmin={isAdmin}
        saveDraft={buffer.saveNow}
      />
    </div>
  );
}
