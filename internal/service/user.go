package service

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserRepository interface {
	Create(ctx context.Context, user domain.User) (uuid.UUID, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetAll(ctx context.Context) ([]domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error)
	GetByUsername(ctx context.Context, username string) ([]domain.User, error)
	Update(ctx context.Context, update domain.UserUpdate) error
}

type UserService struct {
	repo       UserRepository
	notif      notifier.Sender
	jwtManager *auth.JWTManager
}

func NewUserService(repo UserRepository, notif notifier.Sender, jwtManager *auth.JWTManager) *UserService {
	if repo == nil {
		panic("UserService: repo is nil")
	}
	if jwtManager == nil {
		panic("UserService: jwtManager is nill")
	}

	return &UserService{
		repo:       repo,
		notif:      notif,
		jwtManager: jwtManager,
	}
}

type RegisterRequest struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	TelegramID int64  `json:"telegram_id"`
	Password   string `json:"password"`
}

type UpdateUserParams struct {
	ID       uuid.UUID
	Email    *string      `json:"email,omitempty"`
	Name     *string      `json:"name,omitempty"`
	Password *string      `json:"password,omitempty"`
	Role     *domain.Role `json:"role,omitempty"`
}

type PasswordPolicy struct {
	MinLength int
	MaxLength int
}

var DefaultPolicy = PasswordPolicy{
	MinLength: 8,
	MaxLength: 100,
}

func (s *UserService) Register(ctx context.Context, req RegisterRequest) (domain.User, error) {
	_, err := s.repo.GetByEmail(ctx, strings.ToLower(req.Email))
	if err == nil {
		return domain.User{}, domain.ErrEmailAlreadyExists
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, fmt.Errorf("failed to check email: %w", err)
	}

	_, err = s.repo.GetByTelegramID(ctx, req.TelegramID)
	if err == nil {
		return domain.User{}, domain.ErrTelegramAlreadyExists
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, fmt.Errorf("failed to check telegram: %w", err)
	}

	validate, problems := s.validatePassword(req.Password)
	if !validate {
		return domain.User{}, fmt.Errorf("%w: %s", domain.ErrPasswordPolicy, strings.Join(problems, ", "))
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
		Role:         domain.RoleUser,
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
	user, err := s.repo.GetByEmail(ctx, strings.ToLower(email))
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
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
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return fmt.Errorf("service.Delete: %w", err)
	}

	if !claims.Role.IsAdmin() {
		return domain.ErrAccessDenied
	}

	_, err = s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("service.Delete: %w", err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return err
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

	if !claims.Role.IsAdmin() {
		return nil, domain.ErrAccessDenied
	}

	users, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.GetAll: %w", err)
	}

	return users, nil
}

func (s *UserService) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByEmail: %w", err)
	}

	if !claims.Role.IsAdmin() {
		return domain.User{}, domain.ErrAccessDenied
	}

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

	if !claims.Role.IsAdmin() {
		return domain.User{}, domain.ErrAccessDenied
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, domain.ErrNotFound
	}

	return user, nil
}

func (s *UserService) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByTelegramID: %w", err)
	}

	if !claims.Role.IsAdmin() {
		return domain.User{}, domain.ErrAccessDenied
	}

	user, err := s.repo.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetByTelegramID: %w", err)
	}
	return user, nil
}

func (s *UserService) GetByUsername(ctx context.Context, username string) ([]domain.User, error) {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.GetByUsername: %w", err)
	}

	if !claims.Role.IsAdmin() {
		return nil, domain.ErrAccessDenied
	}
	users, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("service.GetByUsername: %w", err)
	}
	return users, nil
}

func (s *UserService) Update(ctx context.Context, params UpdateUserParams) error {
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return err
	}

	if params.Email != nil {
		lower := strings.ToLower(*params.Email)
		params.Email = &lower
	}

	if !claims.Role.IsAdmin() && claims.UserID != params.ID {
		return domain.ErrAccessDenied
	}

	if params.Role != nil {
		if !claims.Role.IsAdmin() {
			return domain.ErrRoleChangeForbidden
		}
		if !params.Role.IsValid() {
			return domain.ErrInvalidRole
		}
	}

	update := domain.UserUpdate{
		ID:    params.ID,
		Role:  params.Role,
		Name:  params.Name,
		Email: params.Email,
	}

	if params.Email != nil {
		user, err := s.repo.GetByID(ctx, params.ID)
		if err != nil {
			return fmt.Errorf("service.GetByID: %w", err)
		}

		if *params.Email != user.Email {
			existingUser, err := s.repo.GetByEmail(ctx, *params.Email)
			if err == nil && existingUser.ID != params.ID {
				return domain.ErrEmailAlreadyExists
			}
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("service.Update: failed to check email: %w", err)
			}
		}
	}

	if params.Password != nil {
		validate, problems := s.validatePassword(*params.Password)
		if !validate {
			return fmt.Errorf("%w: %s", domain.ErrPasswordPolicy, strings.Join(problems, ", "))
		}

		passwordHash, err := s.hashPassword(*params.Password)
		if err != nil {
			return fmt.Errorf("service.Update: %w", err)
		}
		update.PasswordHash = &passwordHash
	}

	if err := s.repo.Update(ctx, update); err != nil {
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
	claims, err := s.checkAuth(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetUserByID: %w", err)
	}

	if !claims.Role.IsAdmin() {
		return domain.User{}, domain.ErrAccessDenied
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.GetUserByID: %w", err)
	}
	return user, nil
}

func (s *UserService) sendWelcome(user *domain.User) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	if err := s.notif.Send(ctx, msg); err != nil {
		log.Printf("service.sendWelcome: failed to notify user %s: %v", user.ID, err)
	}
}

func (s *UserService) checkAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}

func (s *UserService) validatePassword(password string) (bool, []string) {
	var errs []string

	if len(password) < DefaultPolicy.MinLength {
		errs = append(errs, "minimum length required")
	}

	if len(password) > DefaultPolicy.MaxLength {
		errs = append(errs, "too long")
	}

	var (
		hasDigit   bool
		hasUpper   bool
		hasLower   bool
		hasSpecial bool
	)

	for _, symbol := range password {
		switch {
		case unicode.IsDigit(symbol):
			hasDigit = true
		case unicode.IsUpper(symbol):
			hasUpper = true
		case unicode.IsLower(symbol):
			hasLower = true
		case unicode.IsPunct(symbol) || unicode.IsSymbol(symbol):
			hasSpecial = true
		}
	}

	if !hasDigit {
		errs = append(errs, "need at least one digit")
	}
	if !hasUpper {
		errs = append(errs, "need uppercase letter")
	}
	if !hasLower {
		errs = append(errs, "need lowercase letter")
	}
	if !hasSpecial {
		errs = append(errs, "need special character")
	}

	return len(errs) == 0, errs
}

func (s *UserService) hashPassword(password string) (string, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash error %w", err)
	}
	return string(passwordHash), nil
}
