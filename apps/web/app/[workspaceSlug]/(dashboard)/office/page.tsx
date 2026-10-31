"use client";

import { OfficePage } from "@multica/views/office";
import { ErrorBoundary } from "@multica/ui/components/common/error-boundary";

export default function Page() {
  return (
    <ErrorBoundary>
      <OfficePage />
    </ErrorBoundary>
  );
}
