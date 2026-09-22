package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository/converters"
	"health_checker/internal/repository/postgres"
	"time"

	"github.com/google/uuid"
)

type SiteRepository interface {
	Create(ctx context.Context, site domain.Site) (uuid.UUID, error)
	Delete(ctx context.Context, id, user_id uuid.UUID) error
	GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error)
	GetAllSites(ctx context.Context) ([]domain.Site, error)
	GetByUserID(ctx context.Context, user_id uuid.UUID) ([]domain.Site, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error)
	GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error)
	UpdateSiteStatus(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error)
	Update(ctx context.Context, site domain.Site) (domain.Site, error)
	VerifySite(ctx context.Context, id, userID uuid.UUID, token string) (domain.Site, error)
}

type SiteService struct {
	repo     SiteRepository
	userRepo UserRepository
}

func NewSiteService(repo SiteRepository, userRepo UserRepository) *SiteService {
	return &SiteService{
		repo:     repo,
		userRepo: userRepo,
	}
}

type CreateSiteRequest struct {
	Url                  string `json:"url"`
	Name                 string `json:"name"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
}

func (s *SiteService) Create(ctx context.Context, req CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
	if req.CheckIntervalSeconds < 30 {
		req.CheckIntervalSeconds = 30
	}

	token, err := generateVerificationToken()
	if err != nil {
		return domain.Site{}, fmt.Errorf("%w: %w", domain.ErrTokenGeneration, err)
	}

	site := domain.Site{
		Url:                  req.Url,
		Name:                 req.Name,
		CheckIntervalSeconds: req.CheckIntervalSeconds,
		UserID:               userID,
		Status:               "pending",
		VerificationToken:    token,
		IsActive:             false,
	}
	id, err := s.repo.Create(ctx, site)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.Create: %w", err)
	}
	site.ID = id
	return site, nil
}

func (s *SiteService) VerifySite(ctx context.Context, id, userID uuid.UUID, token string) error {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("service.VerifySite: %w", err)
	}

	if site.UserID != userID {
		return domain.ErrSiteNotBelongUser
	}

	if site.VerifiedAt != nil {
		return domain.ErrSiteAlreadyVerified
	}

	_, err = s.repo.VerifySite(ctx, id, userID, token)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidToken) {
			return domain.ErrInvalidToken
		}
		return fmt.Errorf("service.VerifySite: %w", err)
	}
	return nil
}

func (s *SiteService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("service.Delete: %w", err)
	}
	if site.UserID != userID {
		return domain.ErrSiteNotBelongUser
	}
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("service.Delete: %w", err)
	}
	return nil
}

func (s *SiteService) GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
	sites, err := s.repo.GetActiveSitesByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("service.GetActiveSitesByStatus: %w", err)
	}
	return sites, nil
}

func (s *SiteService) GetAllSites(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("service.GetAllSites: %w", err)
	}
	if !user.Role.IsAdmin() {
		return nil, domain.ErrAccessDenied
	}

	sites, err := s.repo.GetAllSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.GetAllSites: %w", err)
	}
	return sites, nil
}

func (s *SiteService) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
	sites, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("service.GetByUserID: %w", err)
	}

	responses := make([]domain.SiteResponse, len(sites))
	for i, site := range sites {
		responses[i] = site.ToResponse()
	}
	return responses, nil
}

func (s *SiteService) GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Site{}, domain.ErrNotFound
		}
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

func (s *SiteService) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	sites, err := s.repo.GetSitesNeedingCheck(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("service.GetSitesNeedingCheck: %w", err)
	}
	return sites, nil
}

func (s *SiteService) UpdateStatus(
	ctx context.Context,
	siteID, userID uuid.UUID,
	newStatus domain.SiteStatus,
	statusCode int32,
	responseTimeMs *int,
) (domain.Site, error) {
	site, err := s.repo.GetByID(ctx, siteID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Site{}, domain.ErrNotFound
		}
		return domain.Site{}, fmt.Errorf("service.UpdateStatus: %w", err)
	}

	if site.UserID != userID {
		return domain.Site{}, domain.ErrSiteNotBelongUser
	}

	if site.VerifiedAt == nil {
		return domain.Site{}, domain.ErrSiteNotVerified
	}

	if !site.IsActive {
		return domain.Site{}, domain.ErrSiteNotActive
	}

	return s.updateStatusInternal(ctx, site, newStatus, statusCode, responseTimeMs)
}

func (s *SiteService) updateStatusInternal(
	ctx context.Context,
	site domain.Site,
	newStatus domain.SiteStatus,
	statusCode int32,
	responseTimeMs *int,
) (domain.Site, error) {
	now := time.Now()
	statusStr := string(newStatus)

	params := postgres.UpdateSiteStatusParams{
		Status:         &statusStr,
		LastStatusCode: &statusCode,
		LastCheckedAt:  converters.TimeToPgTimestamp(&now),
		ResponseTimeMs: converters.IntPtrToInt32Ptr(responseTimeMs),
		ID:             site.ID,
	}

	updatedSite, err := s.repo.UpdateSiteStatus(ctx, params)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.updateStatusInternal: %w", err)
	}

	return updatedSite, nil
}

func (s *SiteService) UpdateStatusByID(
	ctx context.Context,
	siteID uuid.UUID,
	newStatus domain.SiteStatus,
	statusCode int32, responseTimeMs *int,
) (domain.Site, error) {
	site, err := s.repo.GetByID(ctx, siteID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Site{}, domain.ErrNotFound
		}
		return domain.Site{}, fmt.Errorf("service.UpdateStatusByID: %w", err)
	}

	now := time.Now()
	statusStr := string(newStatus)

	params := postgres.UpdateSiteStatusParams{
		Status:         &statusStr,
		LastStatusCode: &statusCode,
		LastCheckedAt:  converters.TimeToPgTimestamp(&now),
		ResponseTimeMs: converters.IntPtrToInt32Ptr(responseTimeMs),
		ID:             site.ID,
	}

	updatedSite, err := s.repo.UpdateSiteStatus(ctx, params)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.UpdateStatusByID: %w", err)
	}

	return updatedSite, nil
}

func generateVerificationToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
