package service

import (
	"health_checker/internal/notifier"
	"health_checker/internal/repository"
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
