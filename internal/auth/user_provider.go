package auth

import (
	"context"

	"health_checker/internal/domain"

	"github.com/google/uuid"
)

// UserProvider — порт для загрузки актуальных данных пользователя.
//
// Нужен при обновлении access-токена: роль и прочие атрибуты обязаны браться
// из БД, а не из тела старого refresh-токена. Иначе понижение/повышение роли
// или удаление пользователя не отразится на уже выданных токенах
// (privilege escalation).
type UserProvider interface {
	// GetAuthData возвращает актуальные email/роль/telegram_id пользователя.
	// Если пользователь удалён — возвращает domain.ErrNotFound.
	GetAuthData(ctx context.Context, userID uuid.UUID) (email string, role domain.Role, telegramID int64, err error)
}
