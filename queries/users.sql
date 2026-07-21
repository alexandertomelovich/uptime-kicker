-- name: CreateUser :one
INSERT INTO users (
    email,
    name,
    telegram_id,
    password_hash,
    role,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id;

-- name: GetAllUsers :many
SELECT id,
    email,
    name,
    telegram_id,
    password_hash,
    role,
    created_at,
    updated_at
FROM users ORDER BY created_at DESC;

-- name: GetByUsername :one
SELECT id,
    email,
    name,
    telegram_id,
    password_hash,
    role,
    created_at,
    updated_at
FROM users WHERE name = $1 ORDER BY created_at DESC;

-- name: GetByID :one
SELECT id,
    email,
    name,
    telegram_id,
    password_hash,
    role,
    created_at,
    updated_at
FROM users WHERE id = $1 ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users
SET 
    email = COALESCE($1, email),
    name = COALESCE($2, name),
    password_hash = COALESCE($3, password_hash),
    role = COALESCE($4, role),
    updated_at = NOW()
WHERE id = $5
RETURNING id;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;