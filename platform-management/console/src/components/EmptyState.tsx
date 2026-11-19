import type { ReactNode } from "react";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@multica/ui/components/ui/empty";
import { FileText } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";

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
    <Empty
      className={cn(
        "border border-dashed border-surface-border bg-surface/50 text-muted-foreground",
        isCompact ? "min-h-28 gap-2 p-4" : "min-h-48 gap-3 p-8",
      )}
    >
      <EmptyHeader className="gap-1.5">
        <EmptyMedia
          variant="icon"
          aria-hidden="true"
          className={cn("mb-1 text-faint-foreground", isCompact ? "size-7" : "size-9")}
        >
          {icon ?? <FileText className="size-4" />}
        </EmptyMedia>
        <EmptyTitle className="font-heading text-title-sm font-medium text-foreground">
          {title}
        </EmptyTitle>
        {description && (
          <EmptyDescription className="text-caption text-muted-foreground">
            {description}
          </EmptyDescription>
        )}
      </EmptyHeader>
      {actions && <EmptyContent className="flex-row justify-center">{actions}</EmptyContent>}
    </Empty>
  );
}
