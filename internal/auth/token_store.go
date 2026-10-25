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

	// Take атомарно изымает сессию при ротации: удаляет её и возвращает
	// привязанного пользователя. Если сессии уже нет (logout, ротация уже
	// прошла), возвращает ErrTokenRevoked. Атомарность критична: проверка
	// «существует?» отдельным запросом открывала бы гонку, при которой один
	// refresh-токен дважды обменивался на две новые пары.
	Take(ctx context.Context, jti uuid.UUID) (uuid.UUID, error)

	// Delete удаляет конкретную сессию (logout с этим токеном).
	Delete(ctx context.Context, jti uuid.UUID) error

	// DeleteAllForUser удаляет все сессии пользователя
	// (используется при смене пароля/роли и удалении пользователя).
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
}
