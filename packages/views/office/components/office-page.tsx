"use client";

import { IssueSurface } from "../../issues/surface/issue-surface";
import { OfficeHeader } from "./office-header";
import { OfficeView } from "./office-view";

/**
 * The Issues board drawn as a pixel office. It runs on the same issue surface,
 * so filters, drag moves, peek, the context menu and create work unchanged. It
 * keeps its own display state and stays out of the saved views of /issues.
 */
export function OfficePage() {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <IssueSurface
        scope={{ type: "workspace" }}
        modes={["board"]}
        surfaceKey="office"
        savedViews={false}
        batchToolbar="never"
        renderHeader={({ controller }) => <OfficeHeader controller={controller} />}
        renderBoard={(props) => <OfficeView {...props} />}
      />
    </div>
  );
}
