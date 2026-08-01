-- name: AggregateDailyStats :exec
INSERT INTO check_daily_stats (
    site_id, date, 
    total_checks, 
    failed_checks, 
    avg_latency_ms, 
    max_latency_ms, 
    uptime_percentage
    )
SELECT 
    site_id,
    DATE(checked_at) as log_date,
    COUNT(*) as total_checks,
    COUNT(*) FILTER (WHERE status_code != 200) as failed_checks,
    AVG(latency_ms)::INT as avg_latency,
    MAX(latency_ms) as max_latency,
    ROUND(((COUNT(*) - COUNT(*) FILTER (WHERE status_code != 200))::NUMERIC / COUNT(*)) * 100, 2) as uptime
FROM check_logs_raw
WHERE checked_at >= CURRENT_DATE - INTERVAL '1 day' AND checked_at < CURRENT_DATE
GROUP BY site_id, log_date
ON CONFLICT (site_id, date) DO NOTHING;

-- name: ClearLogs :exec
DELETE FROM check_logs_raw 
WHERE checked_at < CURRENT_DATE - INTERVAL '1 day';