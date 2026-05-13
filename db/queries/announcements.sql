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
