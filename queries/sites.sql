-- name: CreateSite :one
INSERT INTO sites (
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id;

-- name: DeleteSite :one
DELETE FROM sites WHERE id = $1 AND user_id = $2
RETURNING id;

-- name: GetAllSites :many
SELECT id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at
FROM sites ORDER BY created_at DESC;

-- name: GetSiteByID :one
SELECT id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at
FROM sites WHERE id = $1;

-- name: GetByUserID :many
SELECT id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at
FROM sites WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetActiveSitesByStatus :many
SELECT
    id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at
FROM sites
WHERE is_active = true AND status = $1;

-- name: GetSitesNeedingCheck :many
SELECT
    id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at
FROM sites
WHERE is_active = true
  AND (
    last_checked_at IS NULL
    OR last_checked_at < NOW() - (check_interval_seconds * INTERVAL '1 second')
  )
ORDER BY last_checked_at NULLS FIRST
LIMIT $1;

-- name: UpdateSiteStatus :one
UPDATE sites
SET
    status = COALESCE($1, status),
    last_status_code = $2,
    last_checked_at = $3,
    response_time_ms = $4,
    updated_at = NOW()
WHERE id = $5
RETURNING
    id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at;

-- name: UpdateSite :one
UPDATE sites
SET
    url = COALESCE(sqlc.narg('url'), url),
    name = COALESCE(sqlc.narg('name'), name),
    check_interval_seconds = COALESCE(sqlc.narg('check_interval_seconds'), check_interval_seconds),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at = NOW()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING
    id,
    url,
    name,
    check_interval_seconds,
    user_id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    is_active,
    created_at,
    updated_at;

-- name: GetSiteStats :one
SELECT
    COUNT(*) as total_sites,
    COUNT(CASE WHEN status = 'up' THEN 1 END) as up_sites,
    COUNT(CASE WHEN status = 'down' THEN 1 END) as down_sites,
    COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_sites,
    -- COALESCE обязателен: без сайтов (или без проверок) AVG даёт NULL,
    -- который pgx не может отсканировать в не-указатель float64.
    -- ::FLOAT8 фиксирует тип, иначе sqlc выводит interface{}.
    COALESCE(AVG(response_time_ms), 0)::FLOAT8 as avg_response_time
FROM sites
WHERE user_id = $1;

-- name: GetSiteWithOwner :one
SELECT
    s.id,
    s.url,
    s.name,
    s.check_interval_seconds,
    s.user_id,
    s.status,
    s.last_status_code,
    s.last_checked_at,
    s.response_time_ms,
    s.is_active,
    s.created_at,
    s.updated_at,
    u.telegram_id AS owner_telegram_id
FROM sites s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1;
