-- name: ListConversations :many
SELECT DISTINCT ON (LEAST(sender_id, receiver_id), GREATEST(sender_id, receiver_id))
       id, sender_id, receiver_id, property_id, body, is_read, created_at
FROM messages
WHERE sender_id = $1
   OR receiver_id = $1
ORDER BY LEAST(sender_id, receiver_id),
         GREATEST(sender_id, receiver_id),
         created_at DESC;

-- name: GetMessageThread :many
SELECT id, sender_id, receiver_id, property_id, body, is_read, created_at
FROM messages
WHERE (sender_id = $1 AND receiver_id = $2)
   OR (sender_id = $2 AND receiver_id = $1)
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO messages (sender_id, receiver_id, property_id, body)
VALUES ($1, $2, $3, $4)
RETURNING id, sender_id, receiver_id, property_id, body, is_read, created_at;

-- name: GetUnreadMessages :many
SELECT id, sender_id, receiver_id, property_id, body, is_read, created_at
FROM messages
WHERE receiver_id = $1
  AND is_read = FALSE
ORDER BY created_at DESC;
