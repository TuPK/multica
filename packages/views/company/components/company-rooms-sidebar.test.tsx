/**
 * @vitest-environment jsdom
 */
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { CompanyRoomsSidebar } from "./company-rooms-sidebar";

describe("CompanyRoomsSidebar", () => {
  it("renders collapsed and inert, so the floor list is out of reach until it opens", () => {
    const { container } = render(<CompanyRoomsSidebar open={false} slotRef={() => {}} />);
    const aside = container.querySelector('[data-slot="company-rooms"]');
    expect(aside?.getAttribute("data-state")).toBe("collapsed");
    expect(aside?.hasAttribute("inert")).toBe(true);
  });

  it("expands when open and keeps the list slot mounted while collapsed", () => {
    let slot: HTMLDivElement | null = null;
    const ref = (el: HTMLDivElement | null) => {
      slot = el;
    };
    const { container, rerender } = render(<CompanyRoomsSidebar open={false} slotRef={ref} />);
    expect(slot).not.toBeNull();
    rerender(<CompanyRoomsSidebar open slotRef={ref} />);
    const aside = container.querySelector('[data-slot="company-rooms"]');
    expect(aside?.getAttribute("data-state")).toBe("expanded");
    expect(aside?.hasAttribute("inert")).toBe(false);
  });
});
