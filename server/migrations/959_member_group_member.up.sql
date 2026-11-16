-- member_group_member: one person in one group (platform management spec
-- 7b.2). No PRIMARY KEY, for the reason given on member_group; the unique index
-- on (workspace_id, user_id, group_id) is migration 960 and its prefix serves
-- the permission lookup of one person. There are no foreign keys: removing a
-- member or a group removes these rows in the same transaction.
CREATE TABLE IF NOT EXISTS member_group_member (
    workspace_id uuid NOT NULL,
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    added_by uuid,
    added_at timestamptz NOT NULL DEFAULT now()
);
