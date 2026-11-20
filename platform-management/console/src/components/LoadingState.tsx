import { Spinner } from "@multica/ui/components/ui/spinner";
import { t } from "../i18n";

export function LoadingState({ message = t("Loading...") }: { message?: string }) {
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex min-h-48 flex-col items-center justify-center gap-3 p-6 text-center text-muted-foreground"
    >
      <Spinner className="size-6 text-primary" />
      <span className="text-body font-medium">{message}</span>
    </div>
  );
}
