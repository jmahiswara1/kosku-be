-- name: ListContractTemplates :many
SELECT id, owner_id, name, content, created_at, updated_at
FROM contract_templates
WHERE owner_id = $1
ORDER BY name ASC;

-- name: GetContractTemplate :one
SELECT id, owner_id, name, content, created_at, updated_at
FROM contract_templates
WHERE id = $1;

-- name: CreateContractTemplate :one
INSERT INTO contract_templates (owner_id, name, content)
VALUES ($1, $2, $3)
RETURNING id, owner_id, name, content, created_at, updated_at;

-- name: UpdateContractTemplate :one
UPDATE contract_templates
SET name       = $2,
    content    = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING id, owner_id, name, content, created_at, updated_at;
