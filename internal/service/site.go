package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSiteNotFound        = errors.New("site not found")
	ErrSiteNotBelongUser   = errors.New("site does not belong to user")
	ErrInvalidToken        = errors.New("invalid verification token")
	ErrSiteAlreadyVerified = errors.New("site already verified")
	ErrSiteNotVerified     = errors.New("site not verified")
	ErrSiteNotActive       = errors.New("site not active")
	ErrTokenGeneration     = errors.New("failed to generate verification token")
	ErrAccessDenied        = errors.New("access denied: insufficient permissions")
)

type SiteService struct {
	repo     repository.SiteRepository
	userRepo repository.UserRepository
}

func NewSiteService(repo repository.SiteRepository, userRepo repository.UserRepository) *SiteService {
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
		return domain.Site{}, fmt.Errorf("%w: %v", ErrTokenGeneration, err)
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
	_, err = s.repo.Create(ctx, site)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.Create: %w", err)
	}
	return site, nil
}

func (s *SiteService) VerifySite(ctx context.Context, id, userID uuid.UUID, token string) error {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrSiteNotFound
	}

	if site.UserID != userID {
		return ErrSiteNotBelongUser
	}

	if site.VerifiedAt != nil {
		return ErrSiteAlreadyVerified
	}

	_, err = s.repo.VerifySite(ctx, id, userID, token)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidVerificationToken) {
			return ErrInvalidToken
		}
		return fmt.Errorf("service.VerifySite: %w", err)
	}
	return nil
}

func (s *SiteService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrSiteNotFound
	}
	if site.UserID != userID {
		return ErrSiteNotBelongUser
	}
	if err := s.repo.Delete(ctx, id, userID); err != nil {
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
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user.Role != "admin" {
		return nil, ErrAccessDenied
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
		return domain.Site{}, ErrSiteNotFound
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

func (s *SiteService) UpdateStatus(ctx context.Context, siteID, userID uuid.UUID, newStatus domain.SiteStatus, statusCode int32) (domain.Site, error) {
	site, err := s.repo.GetByID(ctx, siteID)
	if err != nil {
		return domain.Site{}, ErrSiteNotFound
	}

	if site.VerifiedAt == nil {
		return domain.Site{}, ErrSiteNotVerified
	}

	if !site.IsActive {
		return domain.Site{}, ErrSiteNotActive
	}

	if site.UserID != userID {
		return domain.Site{}, ErrSiteNotBelongUser
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

func generateVerificationToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
