-- name: CreateAuditLog :one
INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, actor_id, action, entity_type, entity_id, metadata, created_at;

-- name: ListAuditLogs :many
SELECT id, actor_id, action, entity_type, entity_id, metadata, created_at
FROM audit_logs
WHERE ($1::uuid IS NULL OR actor_id = $1)
  AND ($2::text IS NULL OR action = $2)
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at <= $4)
ORDER BY created_at DESC
LIMIT $5 OFFSET $6;
