import "@testing-library/jest-dom/vitest";
import { configure } from "@testing-library/react";
import "@multica/vitest-setup/web-storage";

configure({ asyncUtilTimeout: 5_000 });

// jsdom lacks these browser APIs, which Base UI dialogs and selects use. Pure
// logic suites opt out of jsdom with `// @vitest-environment node` and share
// this file, so there is no DOM to patch there.
if (typeof window !== "undefined") {
  if (typeof window.matchMedia !== "function") {
    window.matchMedia = (query: string) =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList;
  }

  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }

  if (typeof Element.prototype.scrollIntoView !== "function") {
    Element.prototype.scrollIntoView = () => {};
  }
}
