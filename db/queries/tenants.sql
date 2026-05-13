-- name: ListTenants :many
SELECT t.id, t.property_id, t.room_id, t.ktp_number, t.ktp_scan_url,
       t.occupation, t.emergency_name, t.emergency_phone,
       t.is_blacklisted, t.blacklist_reason, t.created_at, t.updated_at,
       p.full_name, p.avatar_url, p.phone, p.role
FROM tenants t
JOIN profiles p ON p.id = t.id
WHERE t.property_id = $1
ORDER BY p.full_name ASC;

-- name: GetTenant :one
SELECT t.id, t.property_id, t.room_id, t.ktp_number, t.ktp_scan_url,
       t.occupation, t.emergency_name, t.emergency_phone,
       t.is_blacklisted, t.blacklist_reason, t.created_at, t.updated_at,
       p.full_name, p.avatar_url, p.phone, p.role
FROM tenants t
JOIN profiles p ON p.id = t.id
WHERE t.id = $1;

-- name: CreateTenant :one
INSERT INTO tenants (id, property_id, room_id, ktp_number, ktp_scan_url, occupation, emergency_name, emergency_phone)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, property_id, room_id, ktp_number, ktp_scan_url, occupation, emergency_name, emergency_phone, is_blacklisted, blacklist_reason, created_at, updated_at;

-- name: UpdateTenant :one
UPDATE tenants
SET property_id     = $2,
    room_id         = $3,
    ktp_number      = $4,
    ktp_scan_url    = $5,
    occupation      = $6,
    emergency_name  = $7,
    emergency_phone = $8,
    updated_at      = NOW()
WHERE id = $1
RETURNING id, property_id, room_id, ktp_number, ktp_scan_url, occupation, emergency_name, emergency_phone, is_blacklisted, blacklist_reason, created_at, updated_at;

-- name: BlacklistTenant :one
UPDATE tenants
SET is_blacklisted   = TRUE,
    blacklist_reason = $2,
    updated_at       = NOW()
WHERE id = $1
RETURNING id, property_id, room_id, ktp_number, ktp_scan_url, occupation, emergency_name, emergency_phone, is_blacklisted, blacklist_reason, created_at, updated_at;
