package service

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("user with this email already exists")
)

type UserService struct {
	repo       repository.UserRepository
	notif      notifier.Sender
	jwtManager *auth.JWTManager
}

func NewUserService(repo repository.UserRepository, notif notifier.Sender, jwtManager *auth.JWTManager) *UserService {
	return &UserService{
		repo:       repo,
		notif:      notif,
		jwtManager: jwtManager,
	}
}

type RegisterRequest struct {
    Name       string `json:"name" validate:"required"`    
    Email      string `json:"email" validate:"required"`     
    TelegramID int64  `json:"telegram_id" validate:"required"`
    Password   string `json:"password" validate:"required"` 
    Role       string `json:"role,omitempty"`                
}

type UpdateUserParams struct {
	ID           uuid.UUID
	Email        *string `json:"email,omitempty"`
	Name         *string `json:"name,omitempty"`
	Password     *string `json:"password,omitempty"`
	PasswordHash *string `json:"-"`
	Role         *string `json:"role,omitempty"`
}

func (s *UserService) Register(ctx context.Context, req RegisterRequest) (domain.User, error) {
	_, err := s.repo.GetByEmail(ctx, strings.ToLower(req.Email))
	if err == nil {
		return domain.User{}, fmt.Errorf("email %s already registered", req.Email)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, fmt.Errorf("failed to check email: %w", err)
	}

	_, err = s.repo.GetByTelegramID(ctx, req.TelegramID)
	if err == nil {
		return domain.User{}, fmt.Errorf("this telegram already registered")
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, fmt.Errorf("failed to check telegram: %w", err)
	}

	if req.Role == "" {
		req.Role = "user"
	}

	if req.Role == "admin" {
		return domain.User{}, errors.New("cannot create a user with administrator rights")
	}

	passwordHash, err := s.hashPassword(req.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.Register %w", err)
	}

	user := domain.User{
		Name:         req.Name,
		Email:        strings.ToLower(req.Email),
		TelegramID:   req.TelegramID,
		PasswordHash: string(passwordHash),
		Role:         req.Role,
	}

	userID, err := s.repo.Create(ctx, user)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.Register: %w", err)
	}

	user.ID = userID

	go s.sendWelcome(&user)

	return user, nil
}

func (s *UserService) Login(ctx context.Context, email, password string) (*auth.TokenPair, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	tokens, err := s.jwtManager.GenerateTokenPair(
		user.ID,
		user.Email,
		user.Role,
		user.TelegramID,
	)
	if err != nil {
		return nil, fmt.Errorf("service.Login: %w", err)
	}
	return tokens, nil
}

func (s *UserService) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrUserNotFound
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		return fmt.Errorf("service.Delete: %w", err)
	}
	return nil
}

func (s *UserService) GetAll(ctx context.Context) ([]domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.GetAll: %w", err)
	}

	if claims.Role != "admin" {
		return nil, fmt.Errorf("access denied: admin role required")
	}

	users, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.GetAll: %w", err)
	}

	return users, nil
}

func (s *UserService) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByEmail: %w", err)
	}

	return user, nil
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByID: %w", err)
	}

	if claims.Role != "admin" {
		return domain.User{}, fmt.Errorf("access denied: admin role required")
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, ErrUserNotFound
	}

	return user, nil
}

func (s *UserService) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByTelegramID: %w", err)
	}

	if claims.Role != "admin" {
		return domain.User{}, fmt.Errorf("access denied: admin role required")
	}

	user, err := s.repo.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return domain.User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *UserService) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return domain.User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *UserService) Update(ctx context.Context, params UpdateUserParams) error {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return err
	}

	if claims.Role != "admin" && claims.UserID != params.ID {
		return ErrAccessDenied
	}

	user, err := s.repo.GetByID(ctx, params.ID)
	if err != nil {
		return ErrUserNotFound
	}
	_, err = s.repo.GetByEmail(ctx, *params.Email)
	if err == nil {
		return ErrEmailAlreadyExists
	}

	if params.Password != nil {
		passwordHash, err := s.hashPassword(*params.Password)
		if err != nil {
			return fmt.Errorf("service.Update: %w", err)
		}
		params.PasswordHash = &passwordHash
	}

	updatedUser := user

	if params.Email != nil {
		updatedUser.Email = *params.Email
	}
	if params.Name != nil {
		updatedUser.Name = *params.Name
	}
	if params.PasswordHash != nil {
		updatedUser.PasswordHash = *params.PasswordHash
	}
	if params.Role != nil {
		updatedUser.Role = *params.Role
	}
	updatedUser.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, updatedUser); err != nil {
		return fmt.Errorf("service.Update: %w", err)
	}

	return nil
}

func (s *UserService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	tokenPair, err := s.jwtManager.RefreshAccessToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("service.RefreshToken: %w", err)
	}
	return tokenPair, nil
}

func (s *UserService) GetUserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetUserByID: %w", err)
	}
	return user, nil
}

func (s *UserService) sendWelcome(user *domain.User) {
	msg := notifier.Message{
		ChatID: user.TelegramID,
		Text: fmt.Sprintf(
			"Добро пожаловать, %s!\n\n"+
				"Ты успешно зарегистрирован в Uptime Kicker.\n"+
				"Теперь ты можешь:\n"+
				"• Добавлять сайты для мониторинга\n"+
				"• Получать уведомления о сбоях\n"+
				"• Настраивать интервалы проверки\n\n"+
				"Начни с команды /add <url>",
			user.Name,
		),
		ParseMode: "HTML",
	}
	_ = s.notif.Send(context.Background(), msg)
}

func (s *UserService) checkAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, errors.New("unauthorized")
	}
	return claims, nil
}

func (s *UserService) hashPassword(password string) (string, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash error %w", err)
	}
	return string(passwordHash), nil
}
