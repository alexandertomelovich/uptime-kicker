// Тесты для пакета auth: выпуск/валидация JWT, ротация refresh-сессий, отзыв
// (logout) и защита от типичных атак на JWT.
//
// Пакет отвечает за безопасность, поэтому кроме «счастливого пути» отдельно
// проверяются негативные сценарии: подмена алгоритма подписи, использование
// refresh-токена как access (и наоборот), истёкшие токены, подпись чужим секретом
// и отозванные сессии.
package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"health_checker/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// --- Тестовые зависимости ---

// mockTokenStore — in-memory реализация TokenStore с возможностью имитировать
// ошибки каждого метода.
type mockTokenStore struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]uuid.UUID // jti -> userID

	saveErr      error
	existsErr    error
	deleteErr    error
	deleteAllErr error

	deleted []uuid.UUID
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{sessions: make(map[uuid.UUID]uuid.UUID)}
}

func (m *mockTokenStore) Save(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[jti] = userID
	return nil
}

func (m *mockTokenStore) Exists(ctx context.Context, jti uuid.UUID) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[jti]
	return ok, nil
}

func (m *mockTokenStore) Delete(ctx context.Context, jti uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, jti)
	m.deleted = append(m.deleted, jti)
	return nil
}

func (m *mockTokenStore) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	if m.deleteAllErr != nil {
		return m.deleteAllErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for jti, uid := range m.sessions {
		if uid == userID {
			delete(m.sessions, jti)
		}
	}
	return nil
}

func (m *mockTokenStore) has(jti uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[jti]
	return ok
}

func (m *mockTokenStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// mockUserProvider возвращает заранее заданные данные или ошибку.
type mockUserProvider struct {
	email      string
	role       domain.Role
	telegramID int64
	err        error
}

func (m *mockUserProvider) GetAuthData(ctx context.Context, userID uuid.UUID) (string, domain.Role, int64, error) {
	if m.err != nil {
		return "", "", 0, m.err
	}
	return m.email, m.role, m.telegramID, nil
}

// newTestManager создаёт менеджер с короткими, но не мгновенными TTL.
func newTestManager(opts ...Option) *JWTManager {
	return NewJWTManager("access-secret", "refresh-secret", time.Minute, time.Hour, opts...)
}

// testUserID — произвольный фиксированный идентификатор пользователя.
var testUserID = uuid.New()

// --- GenerateTokenPair / валидация ---

// Успешный выпуск пары: access валидируется access-секретом, refresh — refresh-секретом.
func TestGenerateTokenPair_AndValidate(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	pair, err := m.GenerateTokenPair(ctx, testUserID, "a@example.com", domain.RoleUser, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("токены должны быть непустыми")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", pair.TokenType)
	}
	if pair.ExpiresIn != 60 {
		t.Errorf("ExpiresIn = %d, want 60", pair.ExpiresIn)
	}

	accessClaims, err := m.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if accessClaims.UserID != testUserID || accessClaims.Email != "a@example.com" {
		t.Errorf("claims = %+v", accessClaims)
	}
	if accessClaims.TelegramID != 99 {
		t.Errorf("TelegramID = %d, want 99", accessClaims.TelegramID)
	}
	if accessClaims.Role != domain.RoleUser {
		t.Errorf("Role = %q, want user", accessClaims.Role)
	}
	if accessClaims.TokenType != TokenTypeAccess {
		t.Errorf("TokenType = %q, want access", accessClaims.TokenType)
	}

	if _, err := m.ValidateRefreshToken(pair.RefreshToken); err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
}

// Если подключён tokenStore, refresh-сессия сохраняется.
func TestGenerateTokenPair_SavesSession(t *testing.T) {
	store := newMockTokenStore()
	m := newTestManager(WithTokenStore(store))

	if _, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.count() != 1 {
		t.Errorf("сессий сохранено %d, want 1", store.count())
	}
}

// Ошибка сохранения сессии -> GenerateTokenPair возвращает ошибку.
func TestGenerateTokenPair_SaveError(t *testing.T) {
	store := newMockTokenStore()
	store.saveErr = errors.New("db down")
	m := newTestManager(WithTokenStore(store))

	_, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка сохранения сессии")
	}
	if store.count() != 0 {
		t.Errorf("сессий сохранено %d, want 0", store.count())
	}
}

// --- Защита от подмены алгоритма ---

// Токен, подписанный другим алгоритмом (HS512), должен отвергаться: менеджер
// требует строго HS256.
func TestValidate_RejectsNonHS256(t *testing.T) {
	m := newTestManager()

	claims := &Claims{
		UserID:    testUserID,
		Role:      domain.RoleUser,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := token.SignedString([]byte("access-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := m.ValidateAccessToken(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// Токен с алгоритмом "none" (без подписи) должен отвергаться.
func TestValidate_RejectsNoneAlg(t *testing.T) {
	m := newTestManager()

	claims := &Claims{
		UserID:    testUserID,
		Role:      domain.RoleUser,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := m.ValidateAccessToken(signed); err == nil {
		t.Fatal("токен с alg=none должен быть отвергнут")
	}
}

// --- Подпись и секреты ---

// Токен, подписанный чужим секретом, отвергается.
func TestValidate_WrongSecret(t *testing.T) {
	other := NewJWTManager("other-secret", "other-refresh", time.Minute, time.Hour)
	pair, err := other.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	m := newTestManager()
	if _, err := m.ValidateAccessToken(pair.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// Access-токен нельзя проверить как refresh и наоборот — разные секреты.
func TestValidate_CrossTypeSecret(t *testing.T) {
	m := newTestManager()
	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Refresh-токен, поданный в ValidateAccessToken, не пройдёт по секрету.
	if _, err := m.ValidateAccessToken(pair.RefreshToken); err == nil {
		t.Error("refresh-токен не должен проходить ValidateAccessToken")
	}
	if _, err := m.ValidateRefreshToken(pair.AccessToken); err == nil {
		t.Error("access-токен не должен проходить ValidateRefreshToken")
	}
}

// --- Путаница типов токенов ---

// Токен с корректной подписью, но неверным полем token_type, отвергается
// ErrInvalidTokenType. Здесь подписываем access-секретом claims с типом refresh.
func TestValidate_TokenTypeMismatch(t *testing.T) {
	m := newTestManager()

	claims := &Claims{
		UserID:    testUserID,
		Role:      domain.RoleUser,
		TokenType: TokenTypeRefresh, // подписываем, но ждём access
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.accessSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := m.ValidateAccessToken(signed); !errors.Is(err, ErrInvalidTokenType) {
		t.Fatalf("err = %v, want ErrInvalidTokenType", err)
	}
}

// --- Истечение срока ---

// Истёкший токен даёт ErrExpiredToken.
func TestValidate_Expired(t *testing.T) {
	m := NewJWTManager("access-secret", "refresh-secret", -time.Minute, time.Hour)

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := m.ValidateAccessToken(pair.AccessToken); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("err = %v, want ErrExpiredToken", err)
	}
}

// Мусорная строка -> ErrInvalidToken.
func TestValidate_Garbage(t *testing.T) {
	m := newTestManager()
	if _, err := m.ValidateAccessToken("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// --- RefreshAccessToken: ротация ---

// Успешное обновление: старая сессия удаляется, выпускается новая пара,
// и она валидна.
func TestRefreshAccessToken_Rotation(t *testing.T) {
	store := newMockTokenStore()
	m := newTestManager(WithTokenStore(store))

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if store.count() != 1 {
		t.Fatalf("сессий = %d, want 1", store.count())
	}

	newPair, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}

	// Старая сессия удалена, добавлена новая — итого всё ещё одна.
	if store.count() != 1 {
		t.Errorf("сессий после ротации = %d, want 1", store.count())
	}
	// Старый refresh-токен теперь отозван.
	if _, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("старый refresh err = %v, want ErrTokenRevoked", err)
	}
	// Новая пара валидна.
	if _, err := m.ValidateAccessToken(newPair.AccessToken); err != nil {
		t.Errorf("новый access: %v", err)
	}
}

// При обновлении актуальные данные берутся из userProvider, а не из старого токена.
func TestRefreshAccessToken_UsesUserProvider(t *testing.T) {
	store := newMockTokenStore()
	provider := &mockUserProvider{email: "new@example.com", role: domain.RoleAdmin, telegramID: 555}
	m := newTestManager(WithTokenStore(store), WithUserProvider(provider))

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "old@example.com", domain.RoleUser, 1)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	newPair, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}

	claims, err := m.ValidateAccessToken(newPair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Email != "new@example.com" {
		t.Errorf("Email = %q, want new@example.com", claims.Email)
	}
	if claims.Role != domain.RoleAdmin {
		t.Errorf("Role = %q, want admin", claims.Role)
	}
	if claims.TelegramID != 555 {
		t.Errorf("TelegramID = %d, want 555", claims.TelegramID)
	}
}

// Отозванный refresh-токен (сессии нет в store) -> ErrTokenRevoked.
func TestRefreshAccessToken_Revoked(t *testing.T) {
	store := newMockTokenStore()
	m := newTestManager(WithTokenStore(store))

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Логинимся (удаляем сессию), не трогая сам токен.
	if err := m.RevokeRefreshToken(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	if _, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("err = %v, want ErrTokenRevoked", err)
	}
}

// Удалённый пользователь (provider вернул ErrNotFound) не получает новую пару.
func TestRefreshAccessToken_UserDeleted(t *testing.T) {
	store := newMockTokenStore()
	provider := &mockUserProvider{err: domain.ErrNotFound}
	m := newTestManager(WithTokenStore(store), WithUserProvider(provider))

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Невалидный refresh-токен -> ошибка (обёрнутая ErrInvalidToken).
func TestRefreshAccessToken_InvalidToken(t *testing.T) {
	m := newTestManager(WithTokenStore(newMockTokenStore()))

	if _, err := m.RefreshAccessToken(context.Background(), "garbage"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// Ошибки tokenStore прокидываются наверх с оборачиванием.
func TestRefreshAccessToken_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("exists error", func(t *testing.T) {
		store := newMockTokenStore()
		store.existsErr = errors.New("boom")
		m := newTestManager(WithTokenStore(store))
		pair, _ := m.GenerateTokenPair(ctx, testUserID, "a@example.com", domain.RoleUser, 0)

		if _, err := m.RefreshAccessToken(ctx, pair.RefreshToken); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("delete error", func(t *testing.T) {
		store := newMockTokenStore()
		m := newTestManager(WithTokenStore(store))
		pair, _ := m.GenerateTokenPair(ctx, testUserID, "a@example.com", domain.RoleUser, 0)
		store.deleteErr = errors.New("boom")

		if _, err := m.RefreshAccessToken(ctx, pair.RefreshToken); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

// Обновление без tokenStore и без provider: работает чисто на криптографии.
func TestRefreshAccessToken_NoStoreNoProvider(t *testing.T) {
	m := newTestManager()
	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 7)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	newPair, err := m.RefreshAccessToken(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	claims, err := m.ValidateAccessToken(newPair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	// Данные сохраняются из исходного refresh-токена.
	if claims.Email != "a@example.com" || claims.TelegramID != 7 {
		t.Errorf("claims = %+v", claims)
	}
}

// --- RevokeRefreshToken (logout) ---

func TestRevokeRefreshToken_Success(t *testing.T) {
	store := newMockTokenStore()
	m := newTestManager(WithTokenStore(store))

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := m.RevokeRefreshToken(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}
	if store.count() != 0 {
		t.Errorf("сессий осталось %d, want 0", store.count())
	}
}

// Без tokenStore отзыв — no-op без ошибки (токен всё равно должен быть валиден).
func TestRevokeRefreshToken_NoStore(t *testing.T) {
	m := newTestManager()
	pair, _ := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)

	if err := m.RevokeRefreshToken(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Невалидный токен при отзыве -> ошибка.
func TestRevokeRefreshToken_Invalid(t *testing.T) {
	m := newTestManager(WithTokenStore(newMockTokenStore()))
	if err := m.RevokeRefreshToken(context.Background(), "garbage"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

// Ошибка удаления сессии прокидывается.
func TestRevokeRefreshToken_DeleteError(t *testing.T) {
	store := newMockTokenStore()
	m := newTestManager(WithTokenStore(store))
	pair, _ := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	store.deleteErr = errors.New("boom")

	if err := m.RevokeRefreshToken(context.Background(), pair.RefreshToken); err == nil {
		t.Fatal("ожидалась ошибка удаления")
	}
}

// Криптографически валидный refresh-токен с некоректным (не-UUID) jti
// отвергается как ErrInvalidToken.
func craftRefreshTokenWithJTI(t *testing.T, m *JWTManager, jti string) string {
	t.Helper()
	claims := &Claims{
		UserID:    testUserID,
		Role:      domain.RoleUser,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			ID:        jti,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.refreshSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func TestRefreshAccessToken_MalformedJTI(t *testing.T) {
	m := newTestManager(WithTokenStore(newMockTokenStore()))
	token := craftRefreshTokenWithJTI(t, m, "not-a-uuid")

	if _, err := m.RefreshAccessToken(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestRevokeRefreshToken_MalformedJTI(t *testing.T) {
	m := newTestManager(WithTokenStore(newMockTokenStore()))
	token := craftRefreshTokenWithJTI(t, m, "not-a-uuid")

	if err := m.RevokeRefreshToken(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}
