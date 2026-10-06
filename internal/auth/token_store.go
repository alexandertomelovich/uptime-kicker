package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TokenStore — порт для хранения активных refresh-сессий.
//
// Хранится только идентификатор refresh-токена (jti из RegisteredClaims.ID),
// привязанный к пользователю и времени истечения. Сам подписанный JWT в БД не
// кладётся — он и так проверяется криптографией; БД нужна исключительно для
// возможности ОТОЗВАТЬ токен (logout, ротация, смена пароля/роли, удаление).
//
// Интерфейс объявлен на стороне потребителя (пакета auth), чтобы JWTManager
// не зависел от конкретной реализации хранилища.
type TokenStore interface {
	// Save сохраняет активную refresh-сессию.
	Save(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error

	// Exists сообщает, активна ли сессия с данным jti.
	Exists(ctx context.Context, jti uuid.UUID) (bool, error)

	// Delete удаляет конкретную сессию (ротация при refresh).
	Delete(ctx context.Context, jti uuid.UUID) error

	// DeleteAllForUser удаляет все сессии пользователя
	// (используется при смене пароля/роли и удалении пользователя).
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
}
