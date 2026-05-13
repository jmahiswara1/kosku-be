-- name: ListUtilityCharges :many
SELECT id, bill_id, type, amount, note, created_at
FROM utility_charges
WHERE bill_id = $1
ORDER BY created_at ASC;

-- name: UpsertUtilityCharge :one
INSERT INTO utility_charges (bill_id, type, amount, note)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING
RETURNING id, bill_id, type, amount, note, created_at;

-- name: CreateUtilityCharge :one
INSERT INTO utility_charges (bill_id, type, amount, note)
VALUES ($1, $2, $3, $4)
RETURNING id, bill_id, type, amount, note, created_at;

-- name: DeleteUtilityCharge :exec
DELETE FROM utility_charges WHERE id = $1;
