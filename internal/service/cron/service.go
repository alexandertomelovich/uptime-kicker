package cron

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	cron "github.com/netresearch/go-cron"
)

// dailyStatsSpec — расписание агрегации дневной статистики (каждый день в 00:05).
const dailyStatsSpec = "5 0 * * *"

// CheckRepository описывает операции над статистикой проверок, нужные cron-сервису.
type CheckRepository interface {
	AggregateDailyStats(ctx context.Context) error
}

type DailyStatsService struct {
	repo CheckRepository
	cron *cron.Cron

	// mu защищает started от гонок StartCron/StopCron.
	mu sync.Mutex
	// started отражает, был ли планировщик запущен. Проверять это через
	// cron.Entries() нельзя: реализация возвращает непустой (не-nil) срез
	// даже для пустого планировщика, из-за чего первый же запуск ошибочно
	// считался повторным.
	started bool
}

func NewDailyStatsService(repo CheckRepository) *DailyStatsService {
	return &DailyStatsService{
		repo: repo,
		cron: cron.New(
			// SkipIfStillRunning не даёт перекрываться прогонам одной задачи,
			// если предыдущий ещё не завершился.
			cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		),
	}
}

func (s *DailyStatsService) StartCron() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("cron already started")
	}

	if _, err := s.cron.AddFunc(dailyStatsSpec, s.ExecuteDailyCheck); err != nil {
		return fmt.Errorf("failed to schedule cron job: %w", err)
	}

	s.cron.Start()
	s.started = true
	return nil
}

// StopCron останавливает планировщик и дожидается завершения активных задач.
//
// cron.Stop() возвращает контекст, который отменяется, когда все запущенные
// задачи завершились. Дополнительно уважается переданный ctx (таймаут/отмена),
// чтобы вызов не мог зависнуть навсегда.
func (s *DailyStatsService) StopCron(ctx context.Context) {
	done := s.cron.Stop()

	select {
	case <-done.Done():
		log.Println("Cron stopped gracefully")
	case <-ctx.Done():
		log.Printf("Cron stop interrupted by context: %v", ctx.Err())
	}
}

func (s *DailyStatsService) ExecuteDailyCheck() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in daily check: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Println("Starting daily check...")
	if err := s.repo.AggregateDailyStats(ctx); err != nil {
		log.Printf("Daily check failed: %v", err)
		return
	}
	log.Println("Daily check completed successfully")
}
