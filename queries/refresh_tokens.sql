-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (
    jti,
    user_id,
    expires_at
) VALUES (
    $1, $2, $3
);

-- name: GetRefreshToken :one
SELECT jti,
    user_id,
    expires_at,
    created_at
FROM refresh_tokens
WHERE jti = $1;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens
WHERE jti = $1;

-- name: DeleteUserRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE user_id = $1;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW();