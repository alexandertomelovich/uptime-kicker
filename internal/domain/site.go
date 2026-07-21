package domain

import (
	"github.com/google/uuid"
	"time"
)

type SiteStatus string

const (
	StatusPending     SiteStatus = "pending"
	StatusUp          SiteStatus = "up"
	StatusDown        SiteStatus = "down"
	StatusMaintenance SiteStatus = "maintenance"
)

type Site struct {
	ID                   uuid.UUID  `json:"id"`
	Url                  string     `json:"url"`
	Name                 string     `json:"name"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
	UserID               uuid.UUID  `json:"user_id"`
	Status               SiteStatus `json:"status"`
	LastStatusCode       *int32     `json:"last_status_code,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	ResponseTimeMs       *int32     `json:"response_time_ms,omitempty"`
	IsActive             bool       `json:"is_active"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	VerificationToken    string     `json:"verification_token"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type SiteStats struct {
	TotalSites      int     `json:"total_sites"`
	UpSites         int     `json:"up_sites"`
	DownSites       int     `json:"down_sites"`
	PendingSites    int     `json:"pending_sites"`
	AvgResponseTime float64 `json:"avg_response_time"`
}
