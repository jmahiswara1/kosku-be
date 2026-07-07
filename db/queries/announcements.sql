-- name: CreateAnnouncement :one
INSERT INTO announcements (owner_id, property_id, title, body, send_email)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, owner_id, property_id, title, body, send_email, created_at;

-- name: ListAnnouncements :many
SELECT id, owner_id, property_id, title, body, send_email, created_at
FROM announcements
WHERE owner_id = $1
  AND ($2::uuid IS NULL OR property_id = $2)
ORDER BY created_at DESC;

-- name: GetActiveTenantsByProperty :many
SELECT t.id, p.full_name, p.email
FROM tenants t
JOIN profiles p ON p.id = t.id
WHERE t.property_id = $1
  AND t.archived_at IS NULL
  AND (t.is_blacklisted IS NULL OR t.is_blacklisted = FALSE);

-- name: GetActiveTenantsByOwner :many
SELECT t.id, p.full_name, p.email
FROM tenants t
JOIN profiles p ON p.id = t.id
JOIN properties pr ON pr.id = t.property_id
WHERE pr.owner_id = $1
  AND t.archived_at IS NULL
  AND pr.archived_at IS NULL
  AND (t.is_blacklisted IS NULL OR t.is_blacklisted = FALSE);
