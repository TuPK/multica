import type { ReactNode } from "react";
import { Card } from "@astryxdesign/core/Card";
import { EmptyState as AstryxEmptyState } from "@astryxdesign/core/EmptyState";
import { FileText } from "@multica/ui/icons";

interface EmptyStateProps {
  title?: string;
  description?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  isCompact?: boolean;
}

export function EmptyState({
  title = "No items found",
  description,
  icon,
  actions,
  isCompact = false,
}: EmptyStateProps) {
  return (
    <Card variant="default" padding={isCompact ? 3 : 6}>
      <AstryxEmptyState
        title={title}
        description={description}
        icon={icon ?? <FileText className="size-5" aria-hidden="true" />}
        actions={actions}
        isCompact={isCompact}
      />
    </Card>
  );
}
