import { Alert, AlertDescription, AlertTitle } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { AlertCircle } from "@multica/ui/icons";
import { t } from "../i18n";

export function ErrorState({
  title = t("Something went wrong"),
  message,
  onRetry,
}: {
  title?: string;
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 p-4">
      <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
      <AlertTitle>
        <h3 className="font-heading text-title-sm font-semibold">{title}</h3>
      </AlertTitle>
      {(message || onRetry) && (
        <AlertDescription className="col-start-2 space-y-3 text-body text-muted-foreground">
          {message && <p>{message}</p>}
          {onRetry && (
            <div>
              <Button variant="outline" size="sm" onClick={onRetry}>
                {t("Try again")}
              </Button>
            </div>
          )}
        </AlertDescription>
      )}
    </Alert>
  );
}
