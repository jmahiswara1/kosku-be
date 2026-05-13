-- name: CreateTicketAttachment :one
INSERT INTO ticket_attachments (ticket_id, url)
VALUES ($1, $2)
RETURNING id, ticket_id, url, created_at;

-- name: ListTicketAttachments :many
SELECT id, ticket_id, url, created_at
FROM ticket_attachments
WHERE ticket_id = $1
ORDER BY created_at ASC;
