-- name: GetStaffPermissions :one
SELECT id, staff_id, owner_id, modules, created_at
FROM staff_permissions
WHERE staff_id = $1
  AND owner_id = $2;

-- name: CreateStaffPermissions :one
INSERT INTO staff_permissions (staff_id, owner_id, modules)
VALUES ($1, $2, $3)
RETURNING id, staff_id, owner_id, modules, created_at;

-- name: DeleteStaffPermissions :exec
DELETE FROM staff_permissions
WHERE staff_id = $1
  AND owner_id = $2;

-- name: ListStaffByOwner :many
SELECT sp.id, sp.staff_id, sp.owner_id, sp.modules, sp.created_at,
       p.full_name, p.avatar_url, p.phone, p.role
FROM staff_permissions sp
JOIN profiles p ON p.id = sp.staff_id
WHERE sp.owner_id = $1
ORDER BY p.full_name ASC;
