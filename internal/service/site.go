package service

import (
	"context"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository"
	"time"

	"github.com/google/uuid"
)

type SiteService struct {
	repo repository.SiteRepository
}

func NewSiteService(repo repository.SiteRepository) *SiteService {
	return &SiteService{
		repo: repo,
	}
}

type CreateSiteRequest struct {
	Url                  string            `json:"url"`
	Name                 string            `json:"name"`
	CheckIntervalSeconds int               `json:"check_interval_seconds"`
	UserID               uuid.UUID         `json:"user_id"`
}

func (s *SiteService) Create(ctx context.Context, req CreateSiteRequest) error {
	if req.CheckIntervalSeconds < 30 {
		return fmt.Errorf("minimum value 30 seconds")
	}
	
	//нужно будет сделать верификацию

	site := domain.Site{
		Url: req.Url,
		Name: req.Name,
		CheckIntervalSeconds: req.CheckIntervalSeconds,
		UserID: req.UserID,
		Status: "pending",
		IsActive: true,
	}
	_, err := s.repo.Create(ctx, site)
	if err != nil {
		return fmt.Errorf("service.Create: %w", err)
	}
	return nil
}

func (s *SiteService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("site not found")
	}
	if site.UserID != userID {
		return fmt.Errorf("site does not belong to user")
		//имеет смысл написать список ошибок через var()
	}
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		return fmt.Errorf("service.Delete: %w", err)
	}
	return nil
}

func(s *SiteService) GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
	sites, err := s.repo.GetActiveSitesByStatus(ctx, status)
	if err != nil {
		return nil,  fmt.Errorf("service.GetActiveSitesByStatus: %w", err)
	}
	return sites, nil
}

func(s *SiteService) GetAllSites(ctx context.Context) ([]domain.Site, error) {
	sites, err := s.repo.GetAllSites(ctx)
	if err != nil {
		return nil,  fmt.Errorf("service.GetAllSites: %w", err)
	}
	return sites, nil
}

func(s *SiteService) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
	sites, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("service.GetByUserID: %w", err)
	}
	return sites, nil
}

func(s *SiteService) GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.GetByID: %w", err)
	}
	return site, nil
}

func (s *SiteService) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	stats, err := s.repo.GetSiteStats(ctx, userID)
	if err != nil {
		return domain.SiteStats{}, fmt.Errorf("service.GetSiteStats: %w", err)
	}
	return stats, nil
}

func(s *SiteService) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	sites, err := s.repo.GetSitesNeedingCheck(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("service.GetSitesNeedingCheck: %w", err)
	}
	return sites, nil
}
	// В сервисе:
func (s *SiteService) UpdateStatus(ctx context.Context, siteID, userID uuid.UUID, newStatus domain.SiteStatus, statusCode int32) (domain.Site, error) {
    site, err := s.repo.GetByID(ctx, siteID)
    if err != nil {
        return domain.Site{}, fmt.Errorf("site not found")
    }
    
    if site.VerifiedAt == nil {
        return domain.Site{}, fmt.Errorf("not verified")
    }

	if !site.IsActive {
        return domain.Site{}, fmt.Errorf("not active")
    }

    if site.UserID != userID {
        return domain.Site{}, fmt.Errorf("site does not belong to user")
    }
    now := time.Now()
    site.Status = newStatus
    site.LastCheckedAt = &now
    site.LastStatusCode = &statusCode
    
    updatedSite, err := s.repo.UpdateSiteStatus(ctx, site)
    if err != nil {
        return domain.Site{}, fmt.Errorf("service.UpdateSiteStatus: %w", err)
    }
    
    return updatedSite, nil
}