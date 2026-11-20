import type { ReactNode } from "react";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { t } from "../i18n";
import { ArrowLeft } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { HeaderControls } from "./header-controls";

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

// One header for every page: the title and right-hand controls share a fixed-height
// row, the meta line always reserves its height, and the divider closes the header.
// Descriptions sit below the divider so the header height does not change per page.
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
    <header className={cn("space-y-2 pb-4", className)}>
      <div className="flex min-h-9 flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 items-center gap-2">
          {backAction && (
            <Button
              label={t(backAction.label)}
              icon={<ArrowLeft className="size-4" aria-hidden="true" />}
              isIconOnly
              tooltip={t(backAction.label)}
              variant="ghost"
              size="sm"
              onClick={backAction.onClick}
              className="-ml-2"
            />
          )}
          <Heading level={1}>{title}</Heading>
          {badge}
          {meta && (
            <Text type="supporting" as="span" className="whitespace-nowrap">
              {meta}
            </Text>
          )}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {actions}
          <HeaderControls />
        </div>
      </div>
      <Divider />
      {description && (
        <Text type="body" color="secondary" as="p" className="block max-w-2xl pt-1">
          {description}
        </Text>
      )}
    </header>
  );
}
