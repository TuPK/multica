-- name: InsertAuditEvent :exec
INSERT INTO audit_event (
    workspace_id, actor_type, actor_id, actor_label, action,
    target_type, target_id, target_label, detail, ip, user_agent, request_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
);
