import { AlertCircle } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";

export function ErrorState({
  title = "Something went wrong",
  message,
  onRetry,
}: {
  title?: string;
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-lg border border-destructive/20 bg-destructive/5 p-6 text-center text-destructive"
    >
      <AlertCircle className="size-6 shrink-0" />
      <div className="space-y-1">
        <h3 className="font-heading text-title-sm font-semibold">{title}</h3>
        {message && <p className="text-body text-muted-foreground">{message}</p>}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}
