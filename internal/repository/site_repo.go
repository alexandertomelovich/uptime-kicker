package repository

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository/converters"
	"health_checker/internal/repository/postgres"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidVerificationToken = errors.New("invalid verification token")
	ErrSiteNotFound             = errors.New("site not found")
)

type SiteRepository struct {
	queries *postgres.Queries
}

func NewSiteRepository(queries *postgres.Queries) *SiteRepository {
	return &SiteRepository{queries: queries}
}

func (r *SiteRepository) toDomain(siteDB postgres.Site) domain.Site {
	return domain.Site{
		ID:                   siteDB.ID,
		Url:                  siteDB.Url,
		Name:                 siteDB.Name,
		CheckIntervalSeconds: int(siteDB.CheckIntervalSeconds),
		UserID:               siteDB.UserID,
		Status:               domain.SiteStatus(converters.SafeString(siteDB.Status)),
		LastStatusCode:       siteDB.LastStatusCode,
		LastCheckedAt:        r.pgTimestampToPtr(siteDB.LastCheckedAt),
		ResponseTimeMs:       siteDB.ResponseTimeMs,
		IsActive:             converters.SafeBool(siteDB.IsActive),
		VerifiedAt:           r.pgTimestampToPtr(siteDB.VerifiedAt),
		VerificationToken:    converters.SafeString(siteDB.VerificationToken),
		CreatedAt:            siteDB.CreatedAt.Time,
		UpdatedAt:            siteDB.UpdatedAt.Time,
	}
}

func (r *SiteRepository) fromDomain(site domain.Site) postgres.CreateSiteParams {
	return postgres.CreateSiteParams{
		Url:                  site.Url,
		Name:                 site.Name,
		CheckIntervalSeconds: int32(site.CheckIntervalSeconds),
		UserID:               site.UserID,
		Status:               (*string)(&site.Status),
		LastStatusCode:       site.LastStatusCode,
		LastCheckedAt:        r.timeToPgTimestamp(site.LastCheckedAt),
		ResponseTimeMs:       site.ResponseTimeMs,
		IsActive:             &site.IsActive,
		VerificationToken:    &site.VerificationToken,
		VerifiedAt:           r.timeToPgTimestamp(site.VerifiedAt),
	}
}

func (r *SiteRepository) Create(ctx context.Context, site domain.Site) (uuid.UUID, error) {
	params := r.fromDomain(site)
	id, err := r.queries.CreateSite(ctx, params)
	if err != nil {
		return uuid.Nil, fmt.Errorf("repository.CreateSite: %w", err)
	}
	return id, nil
}

func (r *SiteRepository) Delete(ctx context.Context, id, user_id uuid.UUID) error {
	params := postgres.DeleteSiteParams{
		ID:     id,
		UserID: user_id,
	}
	_, err := r.queries.DeleteSite(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSiteNotFound
		}
		return fmt.Errorf("repository.DeleteSite: %w", err)
	}

	return nil
}

func (r *SiteRepository) GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetActiveSitesByStatus(ctx, (*string)(&status))
	if err != nil {
		return nil, fmt.Errorf("repository.GetActiveSitesByStatus: %w", err)
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
}

func (r *SiteRepository) GetAllSites(ctx context.Context) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetAllSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository.GetAllSites: %w", err)
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
}

func (r *SiteRepository) GetByUserID(ctx context.Context, user_id uuid.UUID) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetByUserID(ctx, user_id)
	if err != nil {
		return nil, fmt.Errorf("repository.GetByUserID: %w", err)
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
}

func (r *SiteRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	site, err := r.queries.GetSiteByID(ctx, id)
	if err != nil {
		return domain.Site{}, fmt.Errorf("repository.GetByID: %w", err)
	}
	return r.toDomain(site), nil
}

func (r *SiteRepository) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	statsDB, err := r.queries.GetSiteStats(ctx, userID)
	if err != nil {
		return domain.SiteStats{}, fmt.Errorf("repository.GetSiteStats: %w", err)
	}

	return domain.SiteStats{
		TotalSites:      int(statsDB.TotalSites),
		UpSites:         int(statsDB.UpSites),
		DownSites:       int(statsDB.DownSites),
		PendingSites:    int(statsDB.PendingSites),
		AvgResponseTime: statsDB.AvgResponseTime,
	}, nil
}

func (r *SiteRepository) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetSitesNeedingCheck(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("repository.GetSitesNeedingCheck: %w", err)
	}
	return r.toDomainSlice(sitesDB), nil
}

func (r *SiteRepository) UpdateSiteStatus(ctx context.Context, site domain.Site) (domain.Site, error) {
	params := postgres.UpdateSiteStatusParams{
		Status:         (*string)(&site.Status),
		LastStatusCode: site.LastStatusCode,
		LastCheckedAt:  r.timeToPgTimestamp(site.LastCheckedAt),
		ResponseTimeMs: site.ResponseTimeMs,
		ID:             site.ID,
	}

	updated, err := r.queries.UpdateSiteStatus(ctx, params)
	if err != nil {
		return domain.Site{}, fmt.Errorf("repository.UpdateSiteStatus: %w", err)
	}

	site.Status = domain.SiteStatus(converters.SafeString(updated.Status))
	site.LastStatusCode = updated.LastStatusCode
	site.LastCheckedAt = r.pgTimestampToPtr(updated.LastCheckedAt)
	site.ResponseTimeMs = updated.ResponseTimeMs
	site.UpdatedAt = updated.UpdatedAt.Time

	return site, nil
}

func (r *SiteRepository) Update(ctx context.Context, site domain.Site) (domain.Site, error) {
	params := postgres.UpdateSiteParams{
		ID:     site.ID,
		UserID: site.UserID,
	}

	if site.Url != "" {
		params.Url = &site.Url
	}
	if site.Name != "" {
		params.Name = &site.Name
	}
	if site.CheckIntervalSeconds > 0 {
		v := int32(site.CheckIntervalSeconds)
		params.CheckIntervalSeconds = &v
	}

	updated, err := r.queries.UpdateSite(ctx, params)
	if err != nil {
		return domain.Site{}, err
	}

	return r.toDomain(updated), nil
}

func (r *SiteRepository) VerifySite(ctx context.Context, id, userID uuid.UUID, token string) (domain.Site, error) {
	params := postgres.VerifySiteParams{
		ID:                id,
		UserID:            userID,
		VerificationToken: &token,
	}

	site, err := r.queries.VerifySite(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Site{}, ErrInvalidVerificationToken
		}
		return domain.Site{}, fmt.Errorf("repository.VerifySite: %w", err)
	}

	return r.toDomain(site), nil
}

func (r *SiteRepository) toDomainSlice(sitesDB []postgres.Site) []domain.Site {
	sites := make([]domain.Site, len(sitesDB))
	for i, site := range sitesDB {
		sites[i] = r.toDomain(site)
	}
	return sites
}

func (r *SiteRepository) pgTimestampToPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func (r *SiteRepository) timeToPgTimestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
