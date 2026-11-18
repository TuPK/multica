"use client";

import { useQuery } from "@tanstack/react-query";
import { myPermissionsOptions } from "../groups/queries";
import { can, type PermissionKey } from "./rules";
import type { Decision } from "./types";
import { useCurrentMember } from "./use-current-member";

/**
 * Whether the caller holds `key` in workspace `wsId`, from `/permissions/me`.
 * While the answer loads, every key is refused: the server stays the decider,
 * the client only hides what it cannot use.
 */
export function useCan(wsId: string, key: PermissionKey, opts: { write?: boolean } = {}): Decision {
  const { data } = useQuery({ ...myPermissionsOptions(wsId), enabled: wsId !== "" });
  const { isGuest } = useCurrentMember(wsId);
  // An empty role is the failed-call answer: it grants nothing.
  const grant =
    data && data.role !== ""
      ? { role: data.role, permissions: data.permissions, isGuest }
      : null;
  return can(key, grant, opts);
}
