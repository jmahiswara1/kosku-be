-- name: ListNotifications :many
SELECT id, user_id, type, title, body, entity_id, is_read, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, title, body, entity_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, type, title, body, entity_id, is_read, created_at;

-- name: MarkNotificationRead :exec
UPDATE notifications
SET is_read = TRUE
WHERE id = $1
  AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET is_read = TRUE
WHERE user_id = $1
  AND is_read = FALSE;
