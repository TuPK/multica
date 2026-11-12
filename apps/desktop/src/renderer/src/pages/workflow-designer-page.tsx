import { useParams } from "react-router-dom";
import { WorkflowDesignerPage as SharedWorkflowDesignerPage } from "@multica/views/workflow";

export function DesktopWorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <SharedWorkflowDesignerPage workflowId={id} />;
}
