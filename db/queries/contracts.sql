-- name: GetActiveContract :one
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE tenant_id = $1
  AND status = 'active'
LIMIT 1;

-- name: ListContractsByTenant :many
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE tenant_id = $1
ORDER BY start_date DESC;

-- name: ListContractsByRoom :many
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE room_id = $1
ORDER BY start_date DESC;

-- name: CreateContract :one
INSERT INTO contracts (tenant_id, room_id, property_id, start_date, end_date, monthly_price, deposit_amount, deposit_refunded, status, file_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, tenant_id, room_id, property_id, start_date, end_date, monthly_price, deposit_amount, deposit_refunded, status, file_url, created_at, updated_at;

-- name: UpdateContractStatus :one
UPDATE contracts
SET status     = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING id, tenant_id, room_id, property_id, start_date, end_date, monthly_price, deposit_amount, deposit_refunded, status, file_url, created_at, updated_at;

-- name: ListExpiringContracts :many
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE status = 'active'
  AND end_date <= CURRENT_DATE + INTERVAL '30 days'
  AND end_date >= CURRENT_DATE
ORDER BY end_date ASC;
