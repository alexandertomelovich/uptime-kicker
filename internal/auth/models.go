package auth

import (
	"health_checker/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType различает access- и refresh-токены.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Claims struct {
	UserID     uuid.UUID   `json:"user_id"`
	Email      string      `json:"email"`
	TelegramID int64       `json:"telegram_id,omitempty"`
	Role       domain.Role `json:"role"`
	TokenType  TokenType   `json:"token_type"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}
