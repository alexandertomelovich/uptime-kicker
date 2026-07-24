package repository

import (
	"context"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository/postgres"

	"github.com/google/uuid"
)

type UserRepository struct {
	queries *postgres.Queries
}

func NewUserRepository(queries *postgres.Queries) *UserRepository {
	return &UserRepository{queries: queries}
}

func (r *UserRepository) toDomain(dbUser postgres.User) domain.User {
	return domain.User{
		ID:           dbUser.ID,
		Name:         dbUser.Name,
		Email:        dbUser.Email,
		TelegramID:   dbUser.TelegramID,
		PasswordHash: r.safeString(dbUser.PasswordHash),
		Role:         r.safeString(dbUser.Role),
		CreatedAt:    dbUser.CreatedAt.Time,
		UpdatedAt:    dbUser.UpdatedAt.Time,
	}
}

func (r *UserRepository) fromDomain(user domain.User) postgres.CreateUserParams {
	return postgres.CreateUserParams{
		Email:        user.Email,
		Name:         user.Email,
		PasswordHash: &user.PasswordHash,
		Role:         &user.Role,
	}
}

func (r *UserRepository) safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (uuid.UUID, error) {
	params := r.fromDomain(user)

	id, err := r.queries.CreateUser(ctx, params)
	if err != nil {
		return uuid.Nil, fmt.Errorf("repository.CreateUser: %w", err)
	}
	return id, nil
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("repository.DeleteUser: %w", err)
	}
	return nil
}

func (r *UserRepository) GetAll(ctx context.Context) ([]domain.User, error) {
	usersDB, err := r.queries.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository.GetAllUser: %w", err)
	}
	users := make([]domain.User, len(usersDB))
	for i, userDB := range usersDB {
		users[i] = domain.User{
			ID:           userDB.ID,
			Name:         userDB.Name,
			Email:        userDB.Email,
			TelegramID:   userDB.TelegramID,
			PasswordHash: r.safeString(userDB.PasswordHash),
			Role:         r.safeString(userDB.Role),
			CreatedAt:    userDB.CreatedAt.Time,
			UpdatedAt:    userDB.UpdatedAt.Time,
		}
	}
	return users, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	userDB, err := r.queries.GetByEmail(ctx, email)
	if err != nil {
		return domain.User{}, fmt.Errorf("repository.GetByEmail: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: r.safeString(userDB.PasswordHash),
		Role:         r.safeString(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	userDB, err := r.queries.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("repository.GetByIDUser: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: r.safeString(userDB.PasswordHash),
		Role:         r.safeString(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	userDB, err := r.queries.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return domain.User{}, fmt.Errorf("repository.GetByTelegramID: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: r.safeString(userDB.PasswordHash),
		Role:         r.safeString(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	userDB, err := r.queries.GetByUsername(ctx, username)
	if err != nil {
		return domain.User{}, fmt.Errorf("repository.GetByUsername: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: r.safeString(userDB.PasswordHash),
		Role:         r.safeString(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) Update(ctx context.Context, user domain.User) error {
	params := postgres.UpdateUserParams{
		Email:        user.Email,
		Name:         user.Name,
		PasswordHash: &user.PasswordHash,
		Role:         &user.Role,
		ID:           user.ID,
	}

	_, err := r.queries.UpdateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("repository.UpdateUser: %w", err)
	}
	return nil
}
