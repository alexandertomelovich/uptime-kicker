package repository

import (
	"context"
	"health_checker/internal/domain"
	"health_checker/internal/repository/converters"
	"health_checker/internal/repository/postgres"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type CheckRepository struct {
	queries *postgres.Queries
}

func NewCheckRepository(queries *postgres.Queries) *CheckRepository {
	return &CheckRepository{queries: queries}
}

func (r *CheckRepository) toDomain(stat postgres.CheckDailyStat) domain.CheckDailyStat {
	var date *time.Time
	if stat.Date.Valid {
		date = &stat.Date.Time
	} else {
		date = nil
	}
	return domain.CheckDailyStat{
		ID:               stat.ID,
		SiteID:           stat.SiteID,
		Date:             date,
		TotalChecks:      int(stat.TotalChecks),
		FailedChecks:     int(stat.FailedChecks),
		AvgLatencyMs:     float64(stat.AvgLatencyMs),
		MaxLatencyMs:     int(stat.MaxLatencyMs),
		UptimePercentage: converters.NumericToFloat64(stat.UptimePercentage),
	}
}

func (r *CheckRepository) fromDomain(stat domain.CheckDailyStat) postgres.CheckDailyStat {
	var date pgtype.Date
	if stat.Date != nil {
		date = pgtype.Date{
			Time:  *stat.Date,
			Valid: true,
		}
	} else {
		date = pgtype.Date{Valid: false}
	}
	return postgres.CheckDailyStat{
		ID:               stat.ID,
		SiteID:           stat.SiteID,
		Date:             date,
		TotalChecks:      int32(stat.TotalChecks),
		FailedChecks:     int32(stat.FailedChecks),
		AvgLatencyMs:     int32(stat.AvgLatencyMs),
		MaxLatencyMs:     int32(stat.MaxLatencyMs),
		UptimePercentage: converters.Float64ToNumeric(stat.UptimePercentage),
	}
}


