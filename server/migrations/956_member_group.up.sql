-- member_group: a named set of workspace members that holds extra permission
-- keys (platform management spec 7b.2). `permissions` is a JSON array of keys
-- from the fixed catalogue in server/internal/membergroup. The table declares
-- no PRIMARY KEY: that would build its index inside CREATE TABLE, not
-- concurrently. The unique indexes on (workspace_id, id) and
-- (workspace_id, lower(name)) are migrations 957 and 958. There are no foreign
-- keys; the application removes a group's member rows with the group.
CREATE TABLE IF NOT EXISTS member_group (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions jsonb NOT NULL DEFAULT '[]',
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
