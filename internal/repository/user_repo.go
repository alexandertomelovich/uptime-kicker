package repository

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/repository/converters"
	"health_checker/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type UserRepository struct {
	queries *postgres.Queries
}

func NewUserRepository(queries *postgres.Queries) *UserRepository {
	return &UserRepository{queries: queries}
}

func (r *UserRepository) fromDomain(user domain.User) postgres.CreateUserParams {
	role := string(user.Role)
	return postgres.CreateUserParams{
		Email:        user.Email,
		Name:         user.Name,
		TelegramID:   user.TelegramID,
		PasswordHash: &user.PasswordHash,
		Role:         &role,
	}
}

func toDomainRole(roleDB *string) domain.Role {
	role := domain.Role(converters.SafeString(roleDB))
	if !role.IsValid() {
		// Не доверяем некорректным данным из БД: безопасный дефолт — обычный пользователь.
		return domain.RoleUser
	}
	return role
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
	_, err := r.queries.DeleteUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("repository.DeleteUser: %w", err)
	}
	return nil
}

func (r *UserRepository) GetAll(ctx context.Context) ([]domain.User, error) {
	usersDB, err := r.queries.GetAllUsers(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("repository.GetAllUser: %w", err)
	}
	users := make([]domain.User, len(usersDB))
	for i, userDB := range usersDB {
		users[i] = domain.User{
			ID:           userDB.ID,
			Name:         userDB.Name,
			Email:        userDB.Email,
			TelegramID:   userDB.TelegramID,
			PasswordHash: converters.SafeString(userDB.PasswordHash),
			Role:         toDomainRole(userDB.Role),
			CreatedAt:    userDB.CreatedAt.Time,
			UpdatedAt:    userDB.UpdatedAt.Time,
		}
	}
	return users, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	userDB, err := r.queries.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("repository.GetByEmail: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: converters.SafeString(userDB.PasswordHash),
		Role:         toDomainRole(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	userDB, err := r.queries.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("repository.GetByIDUser: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: converters.SafeString(userDB.PasswordHash),
		Role:         toDomainRole(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	userDB, err := r.queries.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, fmt.Errorf("repository.GetByTelegramID: %w", err)
	}

	return domain.User{
		ID:           userDB.ID,
		Name:         userDB.Name,
		Email:        userDB.Email,
		TelegramID:   userDB.TelegramID,
		PasswordHash: converters.SafeString(userDB.PasswordHash),
		Role:         toDomainRole(userDB.Role),
		CreatedAt:    userDB.CreatedAt.Time,
		UpdatedAt:    userDB.UpdatedAt.Time,
	}, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) ([]domain.User, error) {
	usersDB, err := r.queries.GetUsersByName(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("repository.GetByUsername: %w", err)
	}
	users := make([]domain.User, len(usersDB))

	for i, userDB := range usersDB {
		users[i] = domain.User{
			ID:           userDB.ID,
			Name:         userDB.Name,
			Email:        userDB.Email,
			TelegramID:   userDB.TelegramID,
			PasswordHash: converters.SafeString(userDB.PasswordHash),
			Role:         toDomainRole(userDB.Role),
			CreatedAt:    userDB.CreatedAt.Time,
			UpdatedAt:    userDB.UpdatedAt.Time,
		}
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, update domain.UserUpdate) error {
	params := postgres.UpdateUserParams{
		Email:        update.Email,
		Name:         update.Name,
		PasswordHash: update.PasswordHash,
		Role:         roleToDB(update.Role),
		ID:           update.ID,
	}

	_, err := r.queries.UpdateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("repository.UpdateUser: %w", err)
	}
	return nil
}

// roleToDB превращает *domain.Role в *string для pgx, сохраняя nil (не менять поле).
func roleToDB(role *domain.Role) *string {
	if role == nil {
		return nil
	}
	s := string(*role)
	return &s
}
