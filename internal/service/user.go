package service

import (
	"context"
	"errors"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository"

	"github.com/google/uuid"
)

type UserService struct {
	repo repository.UserRepository
	notif notifier.Sender
}

func NewUserService(repo repository.UserRepository, notif notifier.Sender) *UserService {
	return &UserService{
		repo: repo,
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

func (s *UserService) Register(ctx context.Context, user domain.User) (uuid.UUID, error) {
	
}
	


