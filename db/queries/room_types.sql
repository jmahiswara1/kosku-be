-- name: ListRoomTypesByProperty :many
SELECT id, property_id, name, monthly_price, created_at
FROM room_types
WHERE property_id = $1
ORDER BY name ASC;

-- name: GetRoomType :one
SELECT id, property_id, name, monthly_price, created_at
FROM room_types
WHERE id = $1;

-- name: CreateRoomType :one
INSERT INTO room_types (property_id, name, monthly_price)
VALUES ($1, $2, $3)
RETURNING id, property_id, name, monthly_price, created_at;

-- name: UpdateRoomType :one
UPDATE room_types
SET name          = $2,
    monthly_price = $3
WHERE id = $1
RETURNING id, property_id, name, monthly_price, created_at;
