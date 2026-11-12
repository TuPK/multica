"use client";

import { use } from "react";
import { WorkflowDesignerPage } from "@multica/views/workflow";

export default function WorkflowDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  return <WorkflowDesignerPage workflowId={id} />;
}
