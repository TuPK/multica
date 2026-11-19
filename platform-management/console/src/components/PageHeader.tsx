import type { ReactNode } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { ArrowLeft } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";

interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  backAction?: {
    label: string;
    onClick: () => void;
  };
  badge?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
  className?: string;
}

export function PageHeader({
  title,
  description,
  backAction,
  badge,
  actions,
  meta,
  className,
}: PageHeaderProps) {
  return (
    <header className={cn("space-y-3 border-b border-surface-border pb-4", className)}>
      {backAction && (
        <div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={backAction.onClick}
            className="-ml-2.5 gap-1.5 text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="size-4" aria-hidden="true" />
            <span>{backAction.label}</span>
          </Button>
        </div>
      )}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="font-heading text-title-lg font-semibold tracking-tight text-foreground">
              {title}
            </h1>
            {badge}
          </div>
          {meta && <div className="text-caption text-muted-foreground">{meta}</div>}
          {description && (
            <p className="max-w-2xl text-body text-muted-foreground">{description}</p>
          )}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </header>
  );
}
