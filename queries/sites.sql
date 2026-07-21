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
    is_active,
    verified_at,
    verification_token,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING id;

-- name: DeleteSite :exec
DELETE FROM sites WHERE id = $1 AND user_id = $2;

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
    verified_at,
    verification_token,
    created_at,
    updated_at
FROM sites ORDER BY created_at DESC;

-- name: GetByID :one
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
    verified_at,
    verification_token,
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
    verified_at,
    verification_token,
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
    verified_at,
    verification_token,
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
    verified_at,
    verification_token,
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
    last_status_code = COALESCE($2, last_status_code),
    last_checked_at = COALESCE($3, last_checked_at),
    response_time_ms = COALESCE($4, response_time_ms),
    updated_at = NOW()
WHERE id = $5
RETURNING 
    id,
    status,
    last_status_code,
    last_checked_at,
    response_time_ms,
    updated_at;

-- name: GetSiteStats :one
SELECT 
    COUNT(*) as total_sites,
    COUNT(CASE WHEN status = 'up' THEN 1 END) as up_sites,
    COUNT(CASE WHEN status = 'down' THEN 1 END) as down_sites,
    COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_sites,
    AVG(response_time_ms) as avg_response_time
FROM sites 
WHERE user_id = $1;

