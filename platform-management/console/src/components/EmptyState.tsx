import { FileText } from "@multica/ui/icons";

export function EmptyState({
  title = "No items found",
  description,
}: {
  title?: string;
  description?: string;
}) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-surface-border p-8 text-center text-muted-foreground">
      <FileText className="size-8 stroke-[1.5]" />
      <h3 className="font-heading text-title-sm font-medium text-foreground">{title}</h3>
      {description && <p className="max-w-sm text-caption text-muted-foreground">{description}</p>}
    </div>
  );
}
