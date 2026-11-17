import { z } from "zod";

// Account groups and the permissions they grant (platform spec 7b.2). The
// schemas are lenient: a field a newer server adds is optional here, and a
// permission key this client does not know stays a string, so it grants
// nothing (see `can` in permissions/rules.ts).

export const MemberGroupSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().default(""),
    name: z.string().default(""),
    description: z.string().default(""),
    permissions: z.array(z.string()).default([]),
    member_count: z.number().default(0),
    created_by: z.string().nullish().transform((v) => v ?? null),
    created_at: z.string().default(""),
    updated_at: z.string().default(""),
  })
  .loose();

export const MemberGroupMemberSchema = z
  .object({
    user_id: z.string().min(1),
    added_by: z.string().nullish().transform((v) => v ?? null),
    added_at: z.string().default(""),
  })
  .loose();

export const MemberGroupDetailSchema = MemberGroupSchema.extend({
  members: z.array(MemberGroupMemberSchema).default([]),
}).loose();

export const MemberGroupListSchema = z.array(MemberGroupSchema);

export const PermissionCatalogueSchema = z
  .object({ permissions: z.array(z.string()).default([]) })
  .loose();

export const MyPermissionsSchema = z
  .object({
    role: z.string().default(""),
    permissions: z.array(z.string()).default([]),
  })
  .loose();

export const AddMemberGroupMembersResponseSchema = z
  .object({ added: z.array(z.string()).default([]) })
  .loose();

export type MemberGroup = z.infer<typeof MemberGroupSchema>;
export type MemberGroupMember = z.infer<typeof MemberGroupMemberSchema>;
export type MemberGroupDetail = z.infer<typeof MemberGroupDetailSchema>;
export type MyPermissions = z.infer<typeof MyPermissionsSchema>;

/** The answer for a group whose response failed to parse: no keys, no members. */
export function emptyGroup(): MemberGroup {
  return {
    id: "",
    workspace_id: "",
    name: "",
    description: "",
    permissions: [],
    member_count: 0,
    created_by: null,
    created_at: "",
    updated_at: "",
  };
}
