package telegram

import (
	"context"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	"github.com/google/uuid"
)

// UserProvider резолвит пользователя по telegram_id.
// Нужен для привязки Telegram-аккаунта к внутреннему пользователю без
// admin-claims (в отличие от UserService, который требует роль admin).
type UserProvider interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error)
}

// SiteService описывает операции над сайтами, необходимые боту.
// Интерфейс объявлен на стороне потребителя (пакет telegram) и реализуется
// *service.SiteService.
type SiteService interface {
	Create(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error)
	GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	Update(ctx context.Context, update domain.SiteUpdate) (domain.Site, error)
	VerifySite(ctx context.Context, id, userID uuid.UUID, token string) error
}
