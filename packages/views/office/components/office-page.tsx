"use client";

import { IssueSurface } from "../../issues/surface/issue-surface";
import { OfficeHeader } from "./office-header";
import { OfficeView } from "./office-view";

/**
 * The Issues board drawn as the pixtuoid office. It runs on the same issue
 * surface, so filters and peek work as they do on the board. It keeps its own
 * display state and stays out of the saved views of /issues.
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
