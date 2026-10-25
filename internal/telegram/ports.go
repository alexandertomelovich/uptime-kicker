package telegram

import (
	"context"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// BotAPI описывает методы Telegram-клиента, которые нужны Handler.
//
// Интерфейс объявлен на стороне потребителя (пакет telegram) и полностью
// реализуется *tgbotapi.BotAPI, поэтому в проде передаётся реальный клиент,
// а в тестах — простой мок без сетевых вызовов.
type BotAPI interface {
	// Send отправляет сообщение (или иное действие) в чат.
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	// Request выполняет произвольный API-запрос (например, answerCallbackQuery).
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

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
	// GetByID проверяет владение сайтом внутри сервиса и возвращает безопасный
	// SiteResponse (без verification-токена).
	GetByID(ctx context.Context, id, userID uuid.UUID) (domain.SiteResponse, error)
	GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	Update(ctx context.Context, update domain.SiteUpdate) (domain.SiteResponse, error)
}
