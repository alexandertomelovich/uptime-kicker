package service

import (
	"context"
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
	Update(ctx context.Context, update domain.SiteUpdate) (domain.Site, error)
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

// Create создаёт сайт и сразу включает его в мониторинг: статус "pending"
// означает лишь, что ни одной проверки ещё не было.
func (s *SiteService) Create(ctx context.Context, req CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
	if req.CheckIntervalSeconds < 30 {
		req.CheckIntervalSeconds = 30
	}

	site := domain.Site{
		Url:                  req.Url,
		Name:                 req.Name,
		CheckIntervalSeconds: req.CheckIntervalSeconds,
		UserID:               userID,
		Status:               "pending",
		IsActive:             true,
	}
	id, err := s.repo.Create(ctx, site)
	if err != nil {
		return domain.Site{}, fmt.Errorf("service.Create: %w", err)
	}
	site.ID = id
	return site, nil
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

// GetByID возвращает сайт по id в виде безопасного SiteResponse (без
// verification-токена). Доступ разрешён только владельцу сайта или администратору:
// для остальных пользователей возвращается ErrSiteNotBelongUser.
func (s *SiteService) GetByID(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.SiteResponse{}, domain.ErrNotFound
		}
		return domain.SiteResponse{}, fmt.Errorf("service.GetByID: %w", err)
	}

	if site.UserID != userID {
		if err := s.requireAdmin(ctx, userID); err != nil {
			return domain.SiteResponse{}, err
		}
	}

	return site.ToResponse(), nil
}

// requireAdmin проверяет, что пользователь userID существует и является
// администратором. Возвращает ErrNotFound, если пользователя нет, и
// ErrSiteNotBelongUser, если он не администратор (чтобы не раскрывать роль).
func (s *SiteService) requireAdmin(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("service.requireAdmin: %w", err)
	}
	if !user.Role.IsAdmin() {
		return domain.ErrSiteNotBelongUser
	}
	return nil
}

func (s *SiteService) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	stats, err := s.repo.GetSiteStats(ctx, userID)
	if err != nil {
		return domain.SiteStats{}, fmt.Errorf("service.GetSiteStats: %w", err)
	}
	return stats, nil
}

// Update частично обновляет сайт и возвращает безопасный SiteResponse (без
// verification-токена). Изменять можно только свой сайт.
func (s *SiteService) Update(ctx context.Context, update domain.SiteUpdate) (domain.SiteResponse, error) {
	site, err := s.repo.GetByID(ctx, update.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.SiteResponse{}, domain.ErrNotFound
		}
		return domain.SiteResponse{}, fmt.Errorf("service.Update: %w", err)
	}

	if site.UserID != update.UserID {
		return domain.SiteResponse{}, domain.ErrSiteNotBelongUser
	}

	updated, err := s.repo.Update(ctx, update)
	if err != nil {
		return domain.SiteResponse{}, fmt.Errorf("service.Update: %w", err)
	}

	return updated.ToResponse(), nil
}

func (s *SiteService) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	sites, err := s.repo.GetSitesNeedingCheck(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("service.GetSitesNeedingCheck: %w", err)
	}
	return sites, nil
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
