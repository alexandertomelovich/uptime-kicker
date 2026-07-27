package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	repo  repository.UserRepository
	notif notifier.Sender
}

func NewUserService(repo repository.UserRepository, notif notifier.Sender) *UserService {
	return &UserService{
		repo:  repo,
		notif: notif,
	}
}

type RegisterRequest struct {
	Name       string
	Email      string
	TelegramID int64
	Password   string
	Role       string
}

type LoginResponse struct {
	User  domain.User  `json:"user"`
	Token domain.Token `json:"token"`
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
		return domain.User{}, fmt.Errorf("this telegram: %s already registered", req.TelegramID)
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

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash error %w", err)
	}

	user := domain.User{
		Name:         req.Name,
		Email:        strings.ToLower(req.Email),
		TelegramID:   req.TelegramID,
		PasswordHash: string(passwordHash),
		Role:         req.Role,
		//НУЖНО РАЗОБРАТЬСЯ С CREATED and UPDATED TIME!!!!!!!!!!!!!!!
	}

	userID, err := s.repo.Create(ctx, user)
	if err != nil {
		return domain.User{}, fmt.Errorf("service.Register: %w", err)
	}
	
	user.ID = userID

	go s.sendWelcome(&user)

	return user, nil
}

func (s *UserService) Login(ctx context.Context, email, password string) (domain.User, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return domain.User{}, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return domain.User{}, fmt.Errorf("invalid credentials")
	}
	//......
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
