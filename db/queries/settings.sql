-- name: GetSettingsByOwner :one
SELECT id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account,
       due_date_day, grace_period_days, penalty_type, penalty_amount,
       created_at, updated_at
FROM properties
WHERE owner_id = $1
  AND archived_at IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: UpdateProfileSettings :one
UPDATE properties
SET name         = $2,
    address      = $3,
    city         = $4,
    logo_url     = $5,
    phone        = $6,
    bank_name    = $7,
    bank_account = $8,
    updated_at   = NOW()
WHERE id = $1
RETURNING id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account,
          due_date_day, grace_period_days, penalty_type, penalty_amount,
          created_at, updated_at;

-- name: UpdateBillingSettings :one
UPDATE properties
SET due_date_day      = $2,
    grace_period_days = $3,
    penalty_type      = $4,
    penalty_amount    = $5,
    updated_at        = NOW()
WHERE id = $1
RETURNING id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account,
          due_date_day, grace_period_days, penalty_type, penalty_amount,
          created_at, updated_at;

-- name: GetPropertySettings :one
SELECT id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account,
       due_date_day, grace_period_days, penalty_type, penalty_amount,
       created_at, updated_at
FROM properties
WHERE id = $1
  AND archived_at IS NULL;

-- name: ListPropertiesByOwnerForExport :many
SELECT id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account,
       due_date_day, grace_period_days, penalty_type, penalty_amount,
       created_at, updated_at
FROM properties
WHERE owner_id = $1
  AND archived_at IS NULL
ORDER BY created_at ASC;

-- name: ListRoomsByOwnerForExport :many
SELECT r.id, r.property_id, r.number, r.floor, r.status, r.created_at
FROM rooms r
JOIN properties p ON p.id = r.property_id
WHERE p.owner_id = $1
  AND r.archived_at IS NULL
  AND p.archived_at IS NULL
ORDER BY r.property_id, r.number;

-- name: ListTenantsByOwnerForExport :many
SELECT t.id, t.property_id, t.room_id, t.ktp_number, t.occupation,
       t.is_blacklisted, t.created_at,
       pr.full_name, pr.phone
FROM tenants t
JOIN profiles pr ON pr.id = t.id
JOIN properties p ON p.id = t.property_id
WHERE p.owner_id = $1
  AND p.archived_at IS NULL
ORDER BY t.property_id, pr.full_name;

-- name: ListContractsByOwnerForExport :many
SELECT c.id, c.tenant_id, c.room_id, c.property_id, c.start_date, c.end_date,
       c.monthly_price, c.deposit_amount, c.deposit_refunded, c.status, c.created_at
FROM contracts c
JOIN properties p ON p.id = c.property_id
WHERE p.owner_id = $1
  AND p.archived_at IS NULL
ORDER BY c.property_id, c.start_date DESC;

-- name: ListBillsByOwnerForExport :many
SELECT b.id, b.tenant_id, b.property_id, b.room_id, b.period_month, b.period_year,
       b.base_amount, b.utility_amount, b.penalty_amount, b.total_amount,
       b.due_date, b.status, b.created_at
FROM bills b
JOIN properties p ON p.id = b.property_id
WHERE p.owner_id = $1
  AND p.archived_at IS NULL
ORDER BY b.property_id, b.period_year DESC, b.period_month DESC;

-- name: ListPaymentsByOwnerForExport :many
SELECT pay.id, pay.bill_id, pay.tenant_id, pay.amount, pay.status,
       pay.confirmed_at, pay.created_at
FROM payments pay
JOIN bills b ON b.id = pay.bill_id
JOIN properties p ON p.id = b.property_id
WHERE p.owner_id = $1
  AND p.archived_at IS NULL
ORDER BY pay.created_at DESC;
