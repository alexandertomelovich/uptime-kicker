package repository

import (
	"context"
	"health_checker/internal/domain"
	"health_checker/internal/repository/postgres"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

)

type UserRepository struct {
	queries *postgres.Queries
}

func NewUserRepository(queries *postgres.Queries) *UserRepository {
	return &UserRepository{queries: queries}
}

func (r *UserRepository) toDomain(dbUser postgres.User) domain.User {
	return domain.User{
		ID: dbUser.ID,
		Name: dbUser.Name,
		Email: dbUser.Email,
		TelegramID: dbUser.TelegramID,
		PasswordHash: r.safeString(dbUser.PasswordHash),
		Role: r.safeString(dbUser.Role),
		CreatedAt: dbUser.CreatedAt.Time,
		UpdatedAt: dbUser.UpdatedAt.Time,
	}
}

func (r *UserRepository) fromDomain(user domain.User) postgres.CreateUserParams {
	return postgres.CreateUserParams{
		Email: user.Email,
		Name: user.Email,
		PasswordHash: &user.PasswordHash,
		Role: &user.Role,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
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
	return r.queries.CreateUser(ctx, params)
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.queries.DeleteUser(ctx, id)
}

func (r *UserRepository) GetAll(ctx context.Context) ([]domain.User, error) {
	usersDB, err := r.queries.GetAllUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]domain.User, len(usersDB))
	for i, userDB := range usersDB {
		users[i] = domain.User{
			ID: userDB.ID,
			Name: userDB.Name,
			Email: userDB.Email,
			TelegramID: userDB.TelegramID,
			PasswordHash: r.safeString(userDB.PasswordHash),
			Role: r.safeString(userDB.Role),
			CreatedAt: userDB.CreatedAt.Time,
			UpdatedAt: userDB.UpdatedAt.Time,
		}
	}
	return users, nil
}

func(r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	userDB, err := r.queries.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}

	return domain.User{
        ID:           userDB.ID,
        Name:         userDB.Name,
        Email:        userDB.Email,
        TelegramID:  userDB.TelegramID,
        PasswordHash: r.safeString(userDB.PasswordHash),
        Role:         r.safeString(userDB.Role),
        CreatedAt:    userDB.CreatedAt.Time,
        UpdatedAt:    userDB.UpdatedAt.Time,
    }, nil
}

func(r *UserRepository) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	userDB, err := r.queries.GetByUsername(ctx, username)
	if err != nil {
		return domain.User{}, err
	}

	return domain.User{
        ID:           userDB.ID,
        Name:         userDB.Name,
        Email:        userDB.Email,
        TelegramID:  userDB.TelegramID,
        PasswordHash: r.safeString(userDB.PasswordHash),
        Role:         r.safeString(userDB.Role),
        CreatedAt:    userDB.CreatedAt.Time,
        UpdatedAt:    userDB.UpdatedAt.Time,
    }, nil
}

func (r *UserRepository) Update(ctx context.Context, user domain.User) error {
	params := postgres.UpdateUserParams{
		Email: user.Email,
		Name: user.Name,
		PasswordHash: &user.PasswordHash,
		Role: &user.Role,
		ID: user.ID,
	}

	_, err := r.queries.UpdateUser(ctx, params)
	if err != nil {
		return err
	}
	return nil
}

