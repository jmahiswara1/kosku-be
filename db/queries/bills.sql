-- name: ListBills :many
SELECT id, tenant_id, property_id, room_id, period_month, period_year,
       base_amount, utility_amount, penalty_amount, total_amount,
       due_date, status, created_at, updated_at
FROM bills
WHERE property_id = $1
ORDER BY period_year DESC, period_month DESC, created_at DESC;

-- name: GetBill :one
SELECT id, tenant_id, property_id, room_id, period_month, period_year,
       base_amount, utility_amount, penalty_amount, total_amount,
       due_date, status, created_at, updated_at
FROM bills
WHERE id = $1;

-- name: CreateBill :one
INSERT INTO bills (tenant_id, property_id, room_id, period_month, period_year, base_amount, utility_amount, penalty_amount, due_date, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, tenant_id, property_id, room_id, period_month, period_year, base_amount, utility_amount, penalty_amount, total_amount, due_date, status, created_at, updated_at;

-- name: UpdateBillStatus :one
UPDATE bills
SET status     = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING id, tenant_id, property_id, room_id, period_month, period_year, base_amount, utility_amount, penalty_amount, total_amount, due_date, status, created_at, updated_at;

-- name: UpdateBillUtilityAmount :one
UPDATE bills
SET utility_amount = $2,
    updated_at     = NOW()
WHERE id = $1
RETURNING id, tenant_id, property_id, room_id, period_month, period_year, base_amount, utility_amount, penalty_amount, total_amount, due_date, status, created_at, updated_at;

-- name: ListBillsByTenant :many
SELECT id, tenant_id, property_id, room_id, period_month, period_year,
       base_amount, utility_amount, penalty_amount, total_amount,
       due_date, status, created_at, updated_at
FROM bills
WHERE tenant_id = $1
ORDER BY period_year DESC, period_month DESC;

-- name: GetFinancialReport :many
SELECT b.property_id,
       b.period_month,
       b.period_year,
       SUM(b.total_amount)   AS total_billed,
       SUM(CASE WHEN b.status = 'paid' THEN b.total_amount ELSE 0 END) AS total_paid,
       COUNT(*)              AS bill_count
FROM bills b
WHERE b.property_id = $1
  AND (b.period_year > $2 OR (b.period_year = $2 AND b.period_month >= $3))
  AND (b.period_year < $4 OR (b.period_year = $4 AND b.period_month <= $5))
GROUP BY b.property_id, b.period_year, b.period_month
ORDER BY b.period_year ASC, b.period_month ASC;
