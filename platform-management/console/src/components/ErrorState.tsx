import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Heading } from "@astryxdesign/core/Heading";
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
    <Banner
      role="alert"
      status="error"
      container="card"
      collapsible={false}
      title={<Heading level={3}>{title}</Heading>}
      description={message}
    >
      {onRetry && (
        <Button label={t("Try again")} variant="secondary" size="sm" onClick={onRetry} />
      )}
    </Banner>
  );
}
