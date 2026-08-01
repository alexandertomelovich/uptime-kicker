package domain

import (
	"time"

	"github.com/google/uuid"
)

type CheckDailyStat struct {
	ID               int64     `json:"id"`
	SiteID           uuid.UUID `json:"site_id"`
	Date             *time.Time `json:"date"`
	TotalChecks      int       `json:"total_checks"`
	FailedChecks     int       `json:"failed_checks"`
	AvgLatencyMs     float64   `json:"avg_latency_ms"`
	MaxLatencyMs     int       `json:"max_latency_ms"`
	UptimePercentage float64   `json:"uptime_percentage"`
}

type CheckLogsRaw struct {
	ID           int64     `json:"id"`
	SiteID       uuid.UUID `json:"site_id"`
	StatusCode   int       `json:"status_code"`
	LatencyMs    int       `json:"latency_ms"`
	ErrorMessage string    `json:"error_message"`
	CheckedAt    time.Time `json:"checked_at"`
}
