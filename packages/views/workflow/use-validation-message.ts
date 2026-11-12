import { useT } from "../i18n";
import type { ValidationMessage } from "./types";

/** Renders a validation message in the current locale. */
export function useValidationMessage(): (message: ValidationMessage) => string {
  const { t } = useT("workflow");
  return (message) =>
    t(($) => $.designer.validation.messages[message.key], message.values);
}
