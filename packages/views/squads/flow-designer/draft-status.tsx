import { Button } from "@multica/ui/components/ui/button";
import { Check, Loader2, RotateCcw } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface DraftStatusProps {
  isDirty: boolean;
  isSaving: boolean;
  saveError: unknown | null;
  isOffline: boolean;
  onRetry: () => void;
}

export function DraftStatus({
  isDirty,
  isSaving,
  saveError,
  isOffline,
  onRetry,
}: DraftStatusProps) {
  const { t } = useT("squads");

  if (isSaving) {
    return (
      <div className="flex items-center gap-1.5 text-caption text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" aria-hidden />
        <span>{t(($) => $.flow_designer.save.saving)}</span>
      </div>
    );
  }

  if (saveError) {
    return (
      <div className="flex items-center gap-2 text-caption text-destructive">
        <span>{isOffline ? t(($) => $.flow_designer.save.unsaved) : t(($) => $.flow_designer.save.failed)}</span>
        <Button
          variant="outline"
          size="xs"
          onClick={onRetry}
          className="h-6 gap-1 text-micro px-2"
        >
          <RotateCcw className="size-3" aria-hidden />
          <span>{t(($) => $.workflows.retry)}</span>
        </Button>
      </div>
    );
  }

  if (isDirty) {
    return (
      <div className="text-caption text-amber-600 dark:text-amber-400 font-medium">
        <span>{t(($) => $.flow_designer.save.unsaved)}</span>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-1.5 text-caption text-muted-foreground">
      <Check className="size-3.5 text-emerald-500" aria-hidden />
      <span>{t(($) => $.flow_designer.save.saved)}</span>
    </div>
  );
}
