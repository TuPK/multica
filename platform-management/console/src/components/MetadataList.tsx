import type { HTMLAttributes, ReactNode } from "react";
import {
  MetadataList as AstryxMetadataList,
  MetadataListItem as AstryxMetadataListItem,
} from "@astryxdesign/core/MetadataList";
import { Text } from "@astryxdesign/core/Text";
import { cn } from "@multica/ui/lib/utils";

interface MetadataListProps extends HTMLAttributes<HTMLDivElement> {
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
    <AstryxMetadataList
      columns={columns === 1 ? "single" : columns}
      label={{ position: "top" }}
      className={className}
      {...props}
    >
      {children}
    </AstryxMetadataList>
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
    <AstryxMetadataListItem label={label} className={cn("min-w-0", className)} {...props}>
      {mono || tabular ? (
        <Text
          type={mono ? "code" : "body"}
          hasTabularNumbers={tabular}
          className="break-words"
        >
          {children}
        </Text>
      ) : (
        children
      )}
    </AstryxMetadataListItem>
  );
}
