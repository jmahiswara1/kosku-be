-- name: ListExpiringContracts :many
SELECT
    c.id,
    c.tenant_id,
    c.room_id,
    c.property_id,
    c.start_date,
    c.end_date,
    c.monthly_price,
    c.deposit_amount,
    c.deposit_refunded,
    c.status,
    c.file_url,
    c.created_at,
    c.updated_at
FROM contracts c
WHERE c.status = 'active'
  AND c.end_date <= NOW() + make_interval(days => $1)
  AND c.end_date >= NOW()
ORDER BY c.end_date ASC;
