-- name: GetProfile :one
SELECT id, full_name, avatar_url, phone, email, role, created_at, updated_at
FROM profiles
WHERE id = $1;

-- name: UpsertProfile :one
INSERT INTO profiles (id, full_name, avatar_url, phone, email, role)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE
  SET full_name  = EXCLUDED.full_name,
      avatar_url = EXCLUDED.avatar_url,
      phone      = EXCLUDED.phone,
      email      = EXCLUDED.email,
      updated_at = NOW()
RETURNING id, full_name, avatar_url, phone, email, role, created_at, updated_at;

-- name: UpdateProfile :one
UPDATE profiles
SET full_name  = $2,
    avatar_url = $3,
    phone      = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING id, full_name, avatar_url, phone, email, role, created_at, updated_at;

-- name: DeleteProfile :exec
DELETE FROM profiles WHERE id = $1;
