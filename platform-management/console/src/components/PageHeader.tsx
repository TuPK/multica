import type { ReactNode } from "react";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
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
    <header className={cn("space-y-3", className)}>
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
            <Heading level={1}>{title}</Heading>
            {badge}
          </div>
          {meta && (
            <Text type="supporting" as="div" className="block">
              {meta}
            </Text>
          )}
          {description && (
            <Text type="body" color="secondary" as="p" className="block max-w-2xl">
              {description}
            </Text>
          )}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
      <Divider />
    </header>
  );
}
