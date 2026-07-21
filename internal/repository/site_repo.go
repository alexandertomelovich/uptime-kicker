package repository

import (
	"context"
	"health_checker/internal/domain"
	"health_checker/internal/repository/postgres"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type SiteRepository struct {
	queries *postgres.Queries
}

func NewSiteRepository(queries *postgres.Queries) *SiteRepository {
	return &SiteRepository{queries: queries}
}

func (r *SiteRepository) toDomain(siteDB postgres.Site) domain.Site {
	return domain.Site{
		ID: siteDB.ID,
		Url: siteDB.Url,
		Name: siteDB.Name,
		CheckIntervalSeconds: int(siteDB.CheckIntervalSeconds),
		UserID: siteDB.UserID,
		Status: domain.SiteStatus(r.safeString(siteDB.Status)),
		LastStatusCode: siteDB.LastStatusCode,
		LastCheckedAt: r.pgTimestampToPtr(siteDB.LastCheckedAt),
		ResponseTimeMs: siteDB.ResponseTimeMs,
		IsActive: r.safeBool(siteDB.IsActive),
		VerifiedAt: r.pgTimestampToPtr(siteDB.VerifiedAt),
		VerificationToken: r.safeString(siteDB.VerificationToken),
		CreatedAt: siteDB.CreatedAt.Time,
		UpdatedAt: siteDB.UpdatedAt.Time,

	}
}

func (r *SiteRepository) fromDomain(site domain.Site) postgres.CreateSiteParams {
	return postgres.CreateSiteParams{
		Url: site.Url,
		Name: site.Name,
		CheckIntervalSeconds: int32(site.CheckIntervalSeconds),
		UserID: site.UserID,
		Status: (*string)(&site.Status),
		LastStatusCode: site.LastStatusCode,
		LastCheckedAt: r.timeToPgTimestamp(site.LastCheckedAt),
		ResponseTimeMs: site.ResponseTimeMs,
		IsActive: &site.IsActive,
		VerifiedAt: r.timeToPgTimestamp(site.VerifiedAt),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
}

func (r *SiteRepository) Create(ctx context.Context, site domain.Site) (uuid.UUID, error) {
	params := r.fromDomain(site)
	return r.queries.CreateSite(ctx, params)
}

func (r *SiteRepository) Delete(ctx context.Context, id, user_id uuid.UUID) error {
	params := postgres.DeleteSiteParams{
		ID: id,
		UserID: user_id,
	}
	return r.queries.DeleteSite(ctx, params)
}

func(r *SiteRepository) GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetActiveSitesByStatus(ctx, (*string)(&status))
	if err != nil {
		return nil, err
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
} 

func (r *SiteRepository) GetAllSites(ctx context.Context) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetAllSites(ctx)
	if err != nil {
		return nil, err
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
}

func (r *SiteRepository) GetByUserID(ctx context.Context, user_id uuid.UUID) ([]domain.Site, error) {
	sitesDB, err := r.queries.GetByUserID(ctx, user_id)
	if err != nil {
		return nil, err
	}

	sites := r.toDomainSlice(sitesDB)
	return sites, nil
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
	return pgtype.Timestamptz{Valid: true}
}

func (r *SiteRepository) safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *SiteRepository) safeBool(b *bool) bool {
    if b == nil {
        return true
    }
    return *b
}