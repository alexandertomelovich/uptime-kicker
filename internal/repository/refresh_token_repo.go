package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"health_checker/internal/auth"
	"health_checker/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RefreshTokenRepository реализует auth.TokenStore поверх sqlc-запросов:
// хранит только jti активных refresh-сессий, привязанный к пользователю.
type RefreshTokenRepository struct {
	queries *postgres.Queries
}

func NewRefreshTokenRepository(queries *postgres.Queries) *RefreshTokenRepository {
	return &RefreshTokenRepository{queries: queries}
}

func (r *RefreshTokenRepository) Save(ctx context.Context, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error {
	err := r.queries.CreateRefreshToken(ctx, postgres.CreateRefreshTokenParams{
		Jti:       jti,
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("repository.SaveRefreshToken: %w", err)
	}
	return nil
}

// Take атомарно изымает refresh-сессию: DELETE ... RETURNING выполняется
// одним оператором, поэтому при гонке двух refresh с одним токеном новую
// пару сможет выпустить только первый (второй получит ErrTokenRevoked).
func (r *RefreshTokenRepository) Take(ctx context.Context, jti uuid.UUID) (uuid.UUID, error) {
	userID, err := r.queries.TakeRefreshToken(ctx, jti)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, auth.ErrTokenRevoked
		}
		return uuid.Nil, fmt.Errorf("repository.TakeRefreshToken: %w", err)
	}
	return userID, nil
}

func (r *RefreshTokenRepository) Delete(ctx context.Context, jti uuid.UUID) error {
	if err := r.queries.DeleteRefreshToken(ctx, jti); err != nil {
		return fmt.Errorf("repository.DeleteRefreshToken: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	if err := r.queries.DeleteUserRefreshTokens(ctx, userID); err != nil {
		return fmt.Errorf("repository.DeleteUserRefreshTokens: %w", err)
	}
	return nil
}
