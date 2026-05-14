-- name: ListRoomPhotos :many
SELECT id, room_id, url, order_idx, created_at
FROM room_photos
WHERE room_id = $1
ORDER BY order_idx ASC;

-- name: CreateRoomPhoto :one
INSERT INTO room_photos (room_id, url, order_idx)
VALUES ($1, $2, $3)
RETURNING id, room_id, url, order_idx, created_at;

-- name: DeleteRoomPhoto :exec
DELETE FROM room_photos WHERE id = $1;

-- name: GetRoomPhoto :one
SELECT id, room_id, url, order_idx, created_at
FROM room_photos
WHERE id = $1;
