-- name: CreateInvitation :one
INSERT INTO invitations (owner_id, property_id, email, token, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, owner_id, property_id, email, token, expires_at, used_at, created_at;

-- name: GetInvitationByToken :one
SELECT id, owner_id, property_id, email, token, expires_at, used_at, created_at
FROM invitations
WHERE token = $1;

-- name: MarkInvitationUsed :one
UPDATE invitations
SET used_at = NOW()
WHERE id = $1
RETURNING id, owner_id, property_id, email, token, expires_at, used_at, created_at;
