-- name: ListRoomsByProperty :many
SELECT r.id, r.property_id, r.room_type_id, r.number, r.floor, r.status,
       r.grid_x, r.grid_y, r.facilities, r.created_at, r.updated_at,
       rt.name AS room_type_name, rt.monthly_price
FROM rooms r
LEFT JOIN room_types rt ON rt.id = r.room_type_id
WHERE r.property_id = $1
ORDER BY r.number ASC;

-- name: GetRoom :one
SELECT r.id, r.property_id, r.room_type_id, r.number, r.floor, r.status,
       r.grid_x, r.grid_y, r.facilities, r.created_at, r.updated_at,
       rt.name AS room_type_name, rt.monthly_price
FROM rooms r
LEFT JOIN room_types rt ON rt.id = r.room_type_id
WHERE r.id = $1;

-- name: CreateRoom :one
INSERT INTO rooms (property_id, room_type_id, number, floor, status, grid_x, grid_y, facilities)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, property_id, room_type_id, number, floor, status, grid_x, grid_y, facilities, created_at, updated_at;

-- name: UpdateRoom :one
UPDATE rooms
SET room_type_id = $2,
    number       = $3,
    floor        = $4,
    status       = $5,
    grid_x       = $6,
    grid_y       = $7,
    facilities   = $8,
    updated_at   = NOW()
WHERE id = $1
RETURNING id, property_id, room_type_id, number, floor, status, grid_x, grid_y, facilities, created_at, updated_at;

-- name: DeleteRoom :exec
DELETE FROM rooms WHERE id = $1;

-- name: UpdateRoomLayout :exec
UPDATE rooms
SET grid_x     = $2,
    grid_y     = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: GetRoomHistory :many
SELECT c.id, c.tenant_id, c.room_id, c.property_id, c.start_date, c.end_date,
       c.monthly_price, c.deposit_amount, c.deposit_refunded, c.status,
       c.file_url, c.created_at, c.updated_at,
       p.full_name AS tenant_name
FROM contracts c
JOIN tenants t ON t.id = c.tenant_id
JOIN profiles p ON p.id = t.id
WHERE c.room_id = $1
ORDER BY c.start_date DESC;
