// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatusBadge } from "./StatusBadge";

describe("StatusBadge", () => {
  it("renders operational statuses with a hidden status dot and semantic tokens (no hardcoded emerald/amber/red)", () => {
    const cases: Array<{ status: string; label: string }> = [
      { status: "active", label: "Active" },
      { status: "read_only", label: "Read-only" },
      { status: "suspended", label: "Suspended" },
      { status: "pending_deletion", label: "Pending Deletion" },
      { status: "deleted", label: "Deleted" },
    ];

    for (const { status, label } of cases) {
      const { container, unmount } = render(<StatusBadge status={status} />);
      const badge = container.querySelector("[data-slot='badge']")!;
      expect(badge).toHaveTextContent(label);
      const dot = badge.querySelector("span[aria-hidden='true']");
      expect(dot).not.toBeNull();
      expect(badge.className).not.toMatch(/emerald-|amber-|red-/);
      unmount();
    }
  });

  it("renders order, subscription, and request states with readable labels", () => {
    const cases: Array<{ status: string; label: string }> = [
      { status: "pending", label: "Pending" },
      { status: "paid", label: "Paid" },
      { status: "expired", label: "Expired" },
      { status: "canceled", label: "Canceled" },
      { status: "refunded", label: "Refunded" },
      { status: "trialing", label: "Trialing" },
      { status: "past_due", label: "Past due" },
      { status: "provisioned", label: "Provisioned" },
      { status: "completed", label: "Completed" },
      { status: "failed", label: "Failed" },
      { status: "needs_info", label: "Needs info" },
      { status: "issued", label: "Issued" },
      { status: "cancelled", label: "Cancelled" },
      { status: "received", label: "Received" },
      { status: "needs_review", label: "Needs review" },
      { status: "applied", label: "Applied" },
      { status: "resolved", label: "Resolved" },
      { status: "dismissed", label: "Dismissed" },
      { status: "scheduled", label: "Scheduled" },
      { status: "verified", label: "Verified" },
      { status: "unverified", label: "Unverified" },
    ];

    for (const { status, label } of cases) {
      const { container, unmount } = render(<StatusBadge status={status} />);
      expect(screen.getByText(label)).toBeInTheDocument();
      const badge = container.querySelector("[data-slot='badge']")!;
      expect(badge.querySelector("span[aria-hidden='true']")).not.toBeNull();
      unmount();
    }
  });

  it("renders staff and workspace roles without a status dot", () => {
    for (const [role, label] of [
      ["admin", "Admin"],
      ["support", "Support"],
      ["billing", "Billing"],
      ["owner", "Owner"],
      ["member", "Member"],
    ] as const) {
      const { container, unmount } = render(<StatusBadge status={role} />);
      expect(screen.getByText(label)).toBeInTheDocument();
      const badge = container.querySelector("[data-slot='badge']")!;
      expect(badge.querySelector("span[aria-hidden='true']")).toBeNull();
      unmount();
    }
  });

  it("supports custom label overrides and falls back safely on unrecognized statuses", () => {
    const { rerender } = render(<StatusBadge status="needs_info" label="Needs buyer details" />);
    expect(screen.getByText("Needs buyer details")).toBeInTheDocument();

    rerender(<StatusBadge status="unknown" />);
    expect(screen.getByText("Unknown")).toBeInTheDocument();

    rerender(<StatusBadge status="custom_future_state" />);
    expect(screen.getByText("custom_future_state")).toBeInTheDocument();
  });
});
