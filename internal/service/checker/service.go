package service

import (
	"context"
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository/postgres"
	"sync"
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

// SiteRepository описывает операции над сайтами, нужные checker-сервису.
type SiteRepository interface {
	GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error)
	GetByIDWithOwner(ctx context.Context, id uuid.UUID) (domain.Site, error)
	UpdateSiteStatus(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error)
}

// CheckRepository описывает операции над логами проверок, нужные checker-сервису.
type CheckRepository interface {
	InsertLog(ctx context.Context, log domain.CheckLogsRaw) error
}

type CheckerService struct {
	repo        SiteRepository
	checkRepo   CheckRepository
	sender      notifier.Sender
	numWorkers  int
	limit       int
	jobsChan    chan CheckJob
	resultsChan chan CheckResult

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewCheckerService(
	repo SiteRepository,
	checkRepo CheckRepository,
	sender notifier.Sender,
	numWorkers int,
	limit int,
	queueSize int,
) *CheckerService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CheckerService{
		repo:        repo,
		checkRepo:   checkRepo,
		sender:      sender,
		numWorkers:  numWorkers,
		limit:       limit,
		jobsChan:    make(chan CheckJob, queueSize),
		resultsChan: make(chan CheckResult, queueSize),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start запускает воркеры, процессор результатов и планировщик.
// Идемпотентен: повторный вызов не создаёт дублирующих горутин.
func (s *CheckerService) Start() {
	s.startOnce.Do(func() {
		for i := 1; i <= s.numWorkers; i++ {
			s.wg.Add(1)
			go func(id int) {
				defer s.wg.Done()
				s.worker(id)
			}(i)
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.resultProcessor()
		}()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.scheduler()
		}()
	})
}

// Stop сигнализирует о завершении всем горутинам и дожидается их выхода.
// Каналы намеренно НЕ закрываются: закрытие канала со стороны, отличной от
// отправителя, приводило бы к панике "send on closed channel". Остановка
// реализована исключительно через отмену контекста. Идемпотентен.
func (s *CheckerService) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
	})
}

func (s *CheckerService) sendStatusChangeAlert(ctx context.Context, message notifier.Message) error {
	if err := s.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("sendStatusChangeAlert: %w", err)
	}
	return nil
}
