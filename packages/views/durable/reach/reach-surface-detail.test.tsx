import { cleanup, fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReachSurface } from "@multica/core/durable/reach";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { renderWithI18n } from "../../test/i18n";
import { ReachSurfaceDetail } from "./reach-surface-detail";

vi.mock("../../navigation", () => ({
  AppLink: ({ children, ...props }: React.ComponentProps<"a">) => <a {...props}>{children}</a>,
}));

const sampleSurface: ReachSurface = {
  surface: "external:stripe.com",
  state: "declared_only",
  effects_30d: 14,
  agent_effects_30d: 3,
  human_minutes_30d: 95,
  gap_asks_30d: 6,
  top_ask_ids: ["ask-1", "ask-2", "ask-3"],
  gap_issue_id: "issue-77",
};

function renderDetail(props: Partial<React.ComponentProps<typeof ReachSurfaceDetail>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <WorkspaceSlugProvider slug="acme">
        <ReachSurfaceDetail
          wsId="ws-1"
          surface={sampleSurface}
          canCloseGap={true}
          closing={false}
          onCloseGap={vi.fn()}
          onDismiss={vi.fn()}
          {...props}
        />
      </WorkspaceSlugProvider>
    </QueryClientProvider>,
  );
}

afterEach(cleanup);

describe("ReachSurfaceDetail", () => {
  it("renders all snapshot metrics and sample ask count without fabricating a time series", () => {
    renderDetail();

    expect(screen.getByText("stripe.com")).toBeInTheDocument();
    expect(screen.getByText("external:stripe.com")).toBeInTheDocument();
    expect(screen.getByText("Declared only")).toBeInTheDocument();
    expect(screen.getByText("14")).toBeInTheDocument();
    expect(screen.getByText("95m")).toBeInTheDocument();
    expect(screen.getByText("6")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View issue" })).toHaveAttribute(
      "href",
      "/acme/issues/issue-77",
    );
    expect(screen.queryByText(/time series|history|trend/i)).not.toBeInTheDocument();
  });

  it("hides Close gap when canCloseGap is false and triggers callback when true", () => {
    const onCloseGap = vi.fn();
    const { unmount } = renderDetail({ canCloseGap: false, onCloseGap });
    expect(screen.queryByRole("button", { name: "Close gap" })).not.toBeInTheDocument();
    unmount();

    renderDetail({ canCloseGap: true, onCloseGap });
    fireEvent.click(screen.getByRole("button", { name: "Close gap" }));
    expect(onCloseGap).toHaveBeenCalledWith("external:stripe.com");
  });
});
