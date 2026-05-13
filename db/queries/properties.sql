-- name: ListPropertiesByOwner :many
SELECT id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account, created_at, updated_at
FROM properties
WHERE owner_id = $1
  AND archived_at IS NULL
ORDER BY created_at DESC;

-- name: ListPropertiesWithStatsByOwner :many
SELECT
  p.id,
  p.owner_id,
  p.name,
  p.address,
  p.city,
  p.logo_url,
  p.phone,
  p.bank_name,
  p.bank_account,
  p.created_at,
  p.updated_at,
  COUNT(r.id)                                                    AS total_rooms,
  COUNT(r.id) FILTER (WHERE r.status = 'occupied')              AS occupied_rooms
FROM properties p
LEFT JOIN rooms r ON r.property_id = p.id AND r.archived_at IS NULL
WHERE p.owner_id = $1
  AND p.archived_at IS NULL
GROUP BY p.id
ORDER BY p.created_at DESC;

-- name: GetProperty :one
SELECT id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account, created_at, updated_at
FROM properties
WHERE id = $1
  AND archived_at IS NULL;

-- name: GetPropertyWithStats :one
SELECT
  p.id,
  p.owner_id,
  p.name,
  p.address,
  p.city,
  p.logo_url,
  p.phone,
  p.bank_name,
  p.bank_account,
  p.created_at,
  p.updated_at,
  COUNT(r.id)                                                    AS total_rooms,
  COUNT(r.id) FILTER (WHERE r.status = 'occupied')              AS occupied_rooms
FROM properties p
LEFT JOIN rooms r ON r.property_id = p.id AND r.archived_at IS NULL
WHERE p.id = $1
  AND p.archived_at IS NULL
GROUP BY p.id;

-- name: CreateProperty :one
INSERT INTO properties (owner_id, name, address, city, logo_url, phone, bank_name, bank_account)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account, created_at, updated_at;

-- name: UpdateProperty :one
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
RETURNING id, owner_id, name, address, city, logo_url, phone, bank_name, bank_account, created_at, updated_at;

-- name: DeleteProperty :exec
DELETE FROM properties WHERE id = $1;

-- name: ArchiveProperty :exec
UPDATE properties SET archived_at = NOW() WHERE id = $1;

-- name: ArchiveRoomsByProperty :exec
UPDATE rooms SET archived_at = NOW() WHERE property_id = $1 AND archived_at IS NULL;

-- name: ArchiveTenantsByProperty :exec
UPDATE tenants SET archived_at = NOW() WHERE property_id = $1 AND archived_at IS NULL;
