import { useParams } from "react-router-dom";
import { FlowDraftEditorPage } from "@multica/views/squads/flow-designer";

export function DesktopFlowDraftEditorPage() {
  const { id = "", draftId = "" } = useParams<{ id: string; draftId: string }>();
  return <FlowDraftEditorPage squadId={id} draftId={draftId} />;
}
