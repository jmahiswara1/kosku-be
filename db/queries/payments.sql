-- name: CreatePayment :one
INSERT INTO payments (bill_id, tenant_id, amount, proof_url, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, bill_id, tenant_id, amount, proof_url, status, rejection_reason, confirmed_by, confirmed_at, created_at;

-- name: GetPayment :one
SELECT id, bill_id, tenant_id, amount, proof_url, status, rejection_reason, confirmed_by, confirmed_at, created_at
FROM payments
WHERE id = $1;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status           = $2,
    rejection_reason = $3,
    confirmed_by     = $4,
    confirmed_at     = $5
WHERE id = $1
RETURNING id, bill_id, tenant_id, amount, proof_url, status, rejection_reason, confirmed_by, confirmed_at, created_at;

-- name: ListPaymentsByBill :many
SELECT id, bill_id, tenant_id, amount, proof_url, status, rejection_reason, confirmed_by, confirmed_at, created_at
FROM payments
WHERE bill_id = $1
ORDER BY created_at DESC;
