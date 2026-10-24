-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (
    jti,
    user_id,
    expires_at
) VALUES (
    $1, $2, $3
);

-- name: TakeRefreshToken :one
-- Атомарное изъятие refresh-сессии: один DELETE сразу удаляет строку и
-- возвращает её user_id. Второй параллельный запрос с тем же токеном строку
-- уже не найдёт — так закрывается гонка Exists-then-Delete, при которой
-- один refresh-токен можно было дважды обменять на две новые пары.
DELETE FROM refresh_tokens
WHERE jti = $1
RETURNING user_id;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens
WHERE jti = $1;

-- name: DeleteUserRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE user_id = $1;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW();
