import { within } from "@testing-library/react";

// Astryx mounts one empty assertive live region (role="alert") for announcements,
// so a page can hold two alert roles. The alert under test is the one with text.
export function findAlert(container: HTMLElement = document.body): HTMLElement {
  const withText = within(container)
    .getAllByRole("alert")
    .filter((el) => !el.hasAttribute("data-astryx-live-region") && (el.textContent ?? "").trim() !== "");
  if (withText.length !== 1) {
    throw new Error(`expected one alert with text, found ${withText.length}`);
  }
  return withText[0] as HTMLElement;
}
