import "@testing-library/jest-dom/vitest";
import { configure } from "@testing-library/react";
import "@multica/vitest-setup/web-storage";
import { vi } from "vitest";

// Row actions render as inline buttons while the sidebar is collapsed and as a
// "..." menu while it is expanded. Screen tests address the buttons directly,
// so they run collapsed; RowActions.test switches to the expanded menu.
vi.mock("../components/sidebar-state", () => ({
  SidebarCollapsedProvider: ({ children }: { children: unknown }) => children,
  useSidebarCollapsed: vi.fn(() => true),
}));

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
      private callback: ResizeObserverCallback;
      constructor(callback: ResizeObserverCallback) {
        this.callback = callback;
      }
      observe(target: Element) {
        const rect = {
          x: 0,
          y: 0,
          width: 640,
          height: 240,
          top: 0,
          right: 640,
          bottom: 240,
          left: 0,
          toJSON: () => ({}),
        } as DOMRectReadOnly;
        this.callback(
          [
            {
              target,
              contentRect: rect,
              borderBoxSize: [],
              contentBoxSize: [],
              devicePixelContentBoxSize: [],
            } as ResizeObserverEntry,
          ],
          this as unknown as globalThis.ResizeObserver,
        );
      }
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }

  if (typeof Element.prototype.scrollIntoView !== "function") {
    Element.prototype.scrollIntoView = () => {};
  }

  // jsdom lacks native <dialog> showModal/close used by Astryx MobileNav.
  if (typeof HTMLDialogElement !== "undefined") {
    if (typeof HTMLDialogElement.prototype.showModal !== "function") {
      HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
        this.setAttribute("open", "");
      };
    }
    if (typeof HTMLDialogElement.prototype.close !== "function") {
      HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
        this.removeAttribute("open");
      };
    }
  }
}
