"use client";

import { useState } from "react";
import { clientErrorMessage } from "@multica/core/api";
import { useCreateFlowDraft } from "@multica/core/flow-drafts";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Squad } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { AlertCircle, Loader2, Upload } from "@multica/ui/icons";
import { useNavigation } from "../../navigation";
import { useT } from "../../i18n";
import { cutDraftName } from "./draft-name";

export interface ImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squad: Squad;
}

const MANIFEST_FORMAT = "multica.flow-manifest/v1";
// A JSON example for the box, not copy to translate.
const MANIFEST_PLACEHOLDER = `{"format": "${MANIFEST_FORMAT}", ...}`;
// `limits.doc_bytes`: the most a document, and so a manifest, may hold.
const MAX_MANIFEST_BYTES = 512 * 1024;

export function ImportDialog({ open, onOpenChange, squad }: ImportDialogProps) {
  const { t } = useT("squads");
  const nav = useNavigation();
  const paths = useWorkspacePaths();
  const createMut = useCreateFlowDraft();

  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > MAX_MANIFEST_BYTES) {
      setError(t(($) => $.flow_designer.import.too_large));
      return;
    }
    const reader = new FileReader();
    reader.onload = (event) => {
      setContent(String(event.target?.result ?? ""));
      setError(null);
    };
    reader.readAsText(file);
  };

  // The manifest is what Export writes: {format, name, doc}. The server checks everything in it;
  // this only turns away what cannot be one before it is sent.
  const handleImport = async () => {
    setError(null);
    if (!content.trim()) return;
    if (new Blob([content]).size > MAX_MANIFEST_BYTES) {
      setError(t(($) => $.flow_designer.import.too_large));
      return;
    }

    let manifest: unknown;
    try {
      manifest = JSON.parse(content);
    } catch {
      setError(t(($) => $.flow_designer.import.invalid_json));
      return;
    }
    const candidate = manifest as { format?: unknown; name?: unknown; doc?: unknown } | null;
    if (!candidate || typeof candidate !== "object" || candidate.format !== MANIFEST_FORMAT || typeof candidate.doc !== "object" || candidate.doc === null) {
      setError(t(($) => $.flow_designer.import.not_manifest));
      return;
    }

    try {
      const named = typeof candidate.name === "string" ? candidate.name.trim() : "";
      const draft = await createMut.mutateAsync({
        squadId: squad.id,
        data: { name: cutDraftName(named || squad.name), from: { kind: "manifest", manifest } },
      });
      onOpenChange(false);
      nav.push(`${paths.squads()}/${squad.id}/workflows/${draft.id}`);
    } catch (err) {
      setError(clientErrorMessage(err) ?? t(($) => $.flow_designer.import.failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md space-y-4 p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle>{t(($) => $.flow_designer.start.import)}</DialogTitle>
        </DialogHeader>

        {error && (
          <div role="alert" className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
            <AlertCircle className="size-4 shrink-0" aria-hidden />
            <span>{error}</span>
          </div>
        )}

        <label className="block space-y-2">
          <span className="block text-caption font-medium">{t(($) => $.flow_designer.import.select_file)}</span>
          <input
            type="file"
            accept=".json,application/json"
            onChange={handleFileUpload}
            className="w-full text-caption text-muted-foreground file:mr-2 file:rounded file:border file:border-input file:bg-background file:px-3 file:py-1 file:text-caption"
          />
        </label>

        <label className="block space-y-2">
          <span className="block text-caption font-medium">{t(($) => $.flow_designer.import.paste)}</span>
          <Textarea
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              setError(null);
            }}
            placeholder={MANIFEST_PLACEHOLDER}
            className="min-h-32 resize-y font-mono text-micro"
          />
        </label>

        <DialogFooter className="gap-2 border-t border-border pt-2">
          <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t(($) => $.flow_designer.actions.cancel)}
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={!content.trim() || createMut.isPending}
            onClick={() => void handleImport()}
            className="gap-1.5"
          >
            {createMut.isPending ? (
              <Loader2 className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Upload className="size-3.5" aria-hidden />
            )}
            <span>{t(($) => $.flow_designer.import.submit)}</span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
