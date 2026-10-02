package auth

import (
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
)

type JWTManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewJWTManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *JWTManager {
	return &JWTManager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (m *JWTManager) GenerateTokenPair(userID uuid.UUID, email string, role domain.Role, telegramID int64) (*TokenPair, error) {
	accessToken, err := m.generateToken(userID, email, role, telegramID, TokenTypeAccess, m.accessTTL, m.accessSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := m.generateToken(userID, email, role, telegramID, TokenTypeRefresh, m.refreshTTL, m.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.accessTTL.Seconds()),
	}, nil
}

func (m *JWTManager) generateToken(
	userID uuid.UUID,
	email string,
	role domain.Role,
	telegramID int64,
	tokenType TokenType,
	ttl time.Duration,
	secret []byte,
) (string, error) {
	now := time.Now()
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
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
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
		// подмену alg на \"none\") считается недействительным.
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

func (m *JWTManager) RefreshAccessToken(refreshToken string) (*TokenPair, error) {
	claims, err := m.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	return m.GenerateTokenPair(
		claims.UserID,
		claims.Email,
		claims.Role,
		claims.TelegramID,
	)
}
