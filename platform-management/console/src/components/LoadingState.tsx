import { Spinner } from "@astryxdesign/core/Spinner";
import { t } from "../i18n";

export function LoadingState({ message = t("Loading...") }: { message?: string }) {
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex min-h-48 flex-col items-center justify-center gap-3 p-6 text-center text-muted-foreground"
    >
      <Spinner size="xl" shade="default" />
      <span className="text-body font-medium">{message}</span>
    </div>
  );
}
