"use client";

import { use } from "react";
import { HubItemPage } from "@multica/views/hub";

export default function HubItemRoute({
  params,
}: {
  params: Promise<{ kind: string; id: string }>;
}) {
  const { kind, id } = use(params);
  return <HubItemPage kind={kind} refId={id} />;
}
