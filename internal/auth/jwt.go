package auth

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidTokenType = errors.New("invalid token type")
	// ErrTokenRevoked — refresh-токен криптографически валиден, но его сессия
	// отсутствует в хранилище (logout, ротация, смена пароля/роли, удаление).
	ErrTokenRevoked = errors.New("token has been revoked")
)

type JWTManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration

	// tokenStore хранит активные refresh-сессии для их отзыва/ротации.
	// Может быть nil — тогда работают только криптографические проверки,
	// без возможности отзыва (полезно в тестах).
	tokenStore TokenStore
	// userProvider загружает актуальные атрибуты пользователя при обновлении
	// токена, чтобы изменения роли/удаление отражались немедленно.
	userProvider UserProvider
}

// Option — функциональная опция для NewJWTManager.
type Option func(*JWTManager)

// WithTokenStore подключает хранилище refresh-сессий (включает отзыв/ротацию).
func WithTokenStore(store TokenStore) Option {
	return func(m *JWTManager) { m.tokenStore = store }
}

// WithUserProvider подключает источник актуальных данных пользователя.
func WithUserProvider(provider UserProvider) Option {
	return func(m *JWTManager) { m.userProvider = provider }
}

func NewJWTManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration, opts ...Option) *JWTManager {
	m := &JWTManager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// GenerateTokenPair создаёт пару токенов и (если подключён tokenStore)
// сохраняет refresh-сессию как активную.
func (m *JWTManager) GenerateTokenPair(ctx context.Context, userID uuid.UUID, email string, role domain.Role, telegramID int64) (*TokenPair, error) {
	accessToken, _, err := m.generateToken(userID, email, role, telegramID, TokenTypeAccess, m.accessTTL, m.accessSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, jti, err := m.generateToken(userID, email, role, telegramID, TokenTypeRefresh, m.refreshTTL, m.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	if m.tokenStore != nil {
		expiresAt := time.Now().Add(m.refreshTTL)
		if err := m.tokenStore.Save(ctx, jti, userID, expiresAt); err != nil {
			return nil, fmt.Errorf("failed to persist refresh session: %w", err)
		}
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.accessTTL.Seconds()),
	}, nil
}

// generateToken выпускает токен и возвращает его строковое представление и jti.
func (m *JWTManager) generateToken(
	userID uuid.UUID,
	email string,
	role domain.Role,
	telegramID int64,
	tokenType TokenType,
	ttl time.Duration,
	secret []byte,
) (string, uuid.UUID, error) {
	now := time.Now()
	jti := uuid.New()
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		TelegramID: telegramID,
		Role:       role,
		TokenType:  tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", uuid.Nil, err
	}
	return signed, jti, nil
}

func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	return m.validateToken(tokenString, m.accessSecret, TokenTypeAccess)
}

func (m *JWTManager) ValidateRefreshToken(tokenString string) (*Claims, error) {
	return m.validateToken(tokenString, m.refreshSecret, TokenTypeRefresh)
}

func (m *JWTManager) validateToken(tokenString string, secret []byte, expected TokenType) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Явно требуем HS256: любой другой алгоритм (включая HS384/HS512 и
		// подмену alg на "none") считается недействительным.
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.TokenType != expected {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// RefreshAccessToken валидирует refresh-токен, проверяет активность его сессии,
// загружает актуальные данные пользователя и выполняет РОТАЦИЮ: старая сессия
// удаляется, выпускается и сохраняется новая пара токенов.
func (m *JWTManager) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := m.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, fmt.Errorf("malformed token id: %w", ErrInvalidToken)
	}

	// Проверяем, что сессия ещё активна (не отозвана).
	if m.tokenStore != nil {
		exists, err := m.tokenStore.Exists(ctx, jti)
		if err != nil {
			return nil, fmt.Errorf("failed to check refresh session: %w", err)
		}
		if !exists {
			return nil, ErrTokenRevoked
		}
	}

	// Загружаем актуальные атрибуты: роль/email/telegram могут измениться,
	// а удалённый пользователь не должен получать новые токены.
	email, role, telegramID := claims.Email, claims.Role, claims.TelegramID
	if m.userProvider != nil {
		email, role, telegramID, err = m.userProvider.GetAuthData(ctx, claims.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to load user data: %w", err)
		}
	}

	// Ротация: старую сессию удаляем ДО выдачи новой пары.
	if m.tokenStore != nil {
		if err := m.tokenStore.Delete(ctx, jti); err != nil {
			return nil, fmt.Errorf("failed to revoke refresh session: %w", err)
		}
	}

	return m.GenerateTokenPair(ctx, claims.UserID, email, role, telegramID)
}

// RevokeRefreshToken отзывает сессию, связанную с переданным refresh-токеном
// (logout). Токен предварительно валидируется.
func (m *JWTManager) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	claims, err := m.ValidateRefreshToken(refreshToken)
	if err != nil {
		return fmt.Errorf("invalid refresh token: %w", err)
	}
	if m.tokenStore == nil {
		return nil
	}

	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return fmt.Errorf("malformed token id: %w", ErrInvalidToken)
	}

	if err := m.tokenStore.Delete(ctx, jti); err != nil {
		return fmt.Errorf("failed to revoke refresh session: %w", err)
	}
	return nil
}
