"use client";

import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { Download, Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { fetchHubBundleDownload, getHubExportBlob, type HubItemKind } from "@multica/core/hub";
import { useT } from "../i18n";

/** Opens a fresh bundle link for one item. The link is presigned, so it is never reused. */
export function HubDownloadBundleButton({
  wsId,
  kind,
  refId,
}: {
  wsId: string;
  kind: HubItemKind;
  refId: string;
}) {
  const { t } = useT("hub");
  const download = useMutation({
    mutationFn: () => fetchHubBundleDownload(wsId, kind, refId),
    onSuccess: (download) => {
      if (download.kind === "url") {
        window.open(download.url, "_blank", "noopener,noreferrer");
      } else {
        saveBlob(download.blob, `${kind}-${refId}-${download.version || "bundle"}.zip`);
      }
    },
    onError: () => {
      toast.error(t(($) => $.export.bundle_failed));
    },
  });

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={download.isPending}
      aria-busy={download.isPending}
      onClick={() => download.mutate()}
    >
      {download.isPending ? (
        <Loader2 className="animate-spin" aria-hidden="true" />
      ) : (
        <Download aria-hidden="true" />
      )}
      {t(($) => $.export.bundle)}
    </Button>
  );
}

/** Admins and owners only: saves the zip of every item's bundle, with manifest.json. */
export function HubExportAllButton({ wsId }: { wsId: string }) {
  const { t } = useT("hub");
  const exportAll = useMutation({
    mutationFn: () => getHubExportBlob(wsId),
    onSuccess: (blob) => {
      saveBlob(blob, `multica-hub-${new Date().toISOString().slice(0, 10)}.zip`);
    },
    onError: () => {
      toast.error(t(($) => $.export.all_failed));
    },
  });

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={exportAll.isPending}
      aria-busy={exportAll.isPending}
      onClick={() => exportAll.mutate()}
    >
      {exportAll.isPending ? (
        <Loader2 className="animate-spin" aria-hidden="true" />
      ) : (
        <Download aria-hidden="true" />
      )}
      {t(($) => $.export.all)}
    </Button>
  );
}

// The export needs the bearer token, so it is saved from memory, not opened as a link.
function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
