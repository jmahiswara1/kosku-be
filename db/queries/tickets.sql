-- name: ListTickets :many
SELECT id, tenant_id, property_id, room_id, title, description,
       priority, status, resolution, created_at, updated_at
FROM tickets
WHERE property_id = $1
ORDER BY created_at DESC;

-- name: GetTicket :one
SELECT id, tenant_id, property_id, room_id, title, description,
       priority, status, resolution, created_at, updated_at
FROM tickets
WHERE id = $1;

-- name: CreateTicket :one
INSERT INTO tickets (tenant_id, property_id, room_id, title, description, priority, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, tenant_id, property_id, room_id, title, description, priority, status, resolution, created_at, updated_at;

-- name: UpdateTicket :one
UPDATE tickets
SET priority   = $2,
    status     = $3,
    resolution = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING id, tenant_id, property_id, room_id, title, description, priority, status, resolution, created_at, updated_at;
