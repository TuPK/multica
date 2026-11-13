"use client";

import { use } from "react";
import { FlowDraftEditorPage } from "@multica/views/squads/flow-designer";

export default function FlowDraftEditorRoute({
  params,
}: {
  params: Promise<{ id: string; draftId: string }>;
}) {
  const { id, draftId } = use(params);
  return <FlowDraftEditorPage squadId={id} draftId={draftId} />;
}
