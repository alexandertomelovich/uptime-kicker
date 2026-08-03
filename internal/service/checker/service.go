package service

import (
	"context"
	"fmt"
	"health_checker/internal/notifier"
	"health_checker/internal/repository"
	"health_checker/internal/telegram"
	"time"

	"github.com/google/uuid"
)

type CheckJob struct {
	SiteID   uuid.UUID
	URL      string
	Interval time.Duration
}

type CheckResult struct {
	SiteID     uuid.UUID
	StatusCode int
	Latency    time.Duration
	Err        error
}

type CheckerService struct {
	repo        repository.SiteRepository
	sender      telegram.Sender
	numWorkers  int
	limit       int
	jobsChan    chan CheckJob
	resultsChan chan CheckResult

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCheckerService(
	repo repository.SiteRepository,
	sender telegram.Sender,
	numWorkers int,
	limit int,
	queueSize int,
) *CheckerService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CheckerService{
		repo:        repo,
		sender:      sender,
		numWorkers:  numWorkers,
		limit:       limit,
		jobsChan:    make(chan CheckJob, queueSize),
		resultsChan: make(chan CheckResult, queueSize),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *CheckerService) Start() {
	for i := 1; i <= s.numWorkers; i++ {
		go s.worker(i)
	}

	go s.resultProcessor()

	go s.scheduler()
}

func (s *CheckerService) Stop() {
	s.cancel()

	close(s.jobsChan)
	close(s.resultsChan)
}

func (s *CheckerService) sendStatusChangeAlert(ctx context.Context, message notifier.Message) error {
	if err := s.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("sendStatusChangeAlert: %w", err)
	}
	return nil
}
