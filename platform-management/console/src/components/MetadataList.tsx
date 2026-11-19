import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@multica/ui/lib/utils";

const COLUMN_CLASSES: Record<1 | 2 | 3 | 4, string> = {
  1: "grid-cols-1",
  2: "grid-cols-1 sm:grid-cols-2",
  3: "grid-cols-1 sm:grid-cols-2 lg:grid-cols-3",
  4: "grid-cols-1 sm:grid-cols-2 lg:grid-cols-4",
};

interface MetadataListProps extends HTMLAttributes<HTMLDListElement> {
  columns?: 1 | 2 | 3 | 4;
  children: ReactNode;
}

export function MetadataList({
  columns = 2,
  className,
  children,
  ...props
}: MetadataListProps) {
  return (
    <dl
      className={cn("grid gap-x-6 gap-y-4", COLUMN_CLASSES[columns], className)}
      {...props}
    >
      {children}
    </dl>
  );
}

interface MetadataListItemProps extends HTMLAttributes<HTMLDivElement> {
  label: string;
  children: ReactNode;
  mono?: boolean;
  tabular?: boolean;
}

export function MetadataListItem({
  label,
  children,
  mono = false,
  tabular = false,
  className,
  ...props
}: MetadataListItemProps) {
  return (
    <div className={cn("min-w-0", className)} {...props}>
      <dt className="text-label font-medium text-muted-foreground">{label}</dt>
      <dd
        className={cn(
          "mt-1 text-body font-medium text-foreground break-words",
          mono && "font-mono text-caption",
          tabular && "tabular-nums",
        )}
      >
        {children}
      </dd>
    </div>
  );
}
