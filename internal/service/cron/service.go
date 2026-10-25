package cron

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	cron "github.com/netresearch/go-cron"
)

// dailyStatsSpec — расписание ежедневного обслуживания (каждый день в 00:05).
const dailyStatsSpec = "5 0 * * *"

// CheckRepository описывает операции над логами проверок, нужные cron-сервису.
type CheckRepository interface {
	AggregateDailyStats(ctx context.Context) error
	ClearLogs(ctx context.Context) error
}

// TokenCleaner описывает очистку истёкших refresh-сессий.
type TokenCleaner interface {
	DeleteExpired(ctx context.Context) error
}

// MaintenanceService ежедневно агрегирует дневную статистику и подчищает
// устаревшие данные: сырые логи проверок старше суток и истёкшие
// refresh-сессии. Без очистки обе таблицы росли бы бесконечно.
type MaintenanceService struct {
	repo   CheckRepository
	tokens TokenCleaner
	cron   *cron.Cron

	// mu защищает started от гонок StartCron/StopCron.
	mu sync.Mutex
	// started отражает, был ли планировщик запущен. Проверять это через
	// cron.Entries() нельзя: реализация возвращает непустой (не-nil) срез
	// даже для пустого планировщика, из-за чего первый же запуск ошибочно
	// считался повторным.
	started bool
}

func NewMaintenanceService(repo CheckRepository, tokens TokenCleaner) *MaintenanceService {
	return &MaintenanceService{
		repo:   repo,
		tokens: tokens,
		cron: cron.New(
			// SkipIfStillRunning не даёт перекрываться прогонам одной задачи,
			// если предыдущий ещё не завершился.
			cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		),
	}
}

func (s *MaintenanceService) StartCron() error {
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
func (s *MaintenanceService) StopCron(ctx context.Context) {
	done := s.cron.Stop()

	select {
	case <-done.Done():
		log.Println("Cron stopped gracefully")
	case <-ctx.Done():
		log.Printf("Cron stop interrupted by context: %v", ctx.Err())
	}
}

// ExecuteDailyCheck прогоняет полный цикл обслуживания. Порядок важен:
// сырые логи удаляются только после успешной агрегации — иначе при сбое БД
// можно было бы удалить данные, которые ещё не попали в дневную статистику.
func (s *MaintenanceService) ExecuteDailyCheck() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in daily maintenance: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Println("Starting daily maintenance...")

	if err := s.repo.AggregateDailyStats(ctx); err != nil {
		log.Printf("Daily stats aggregation failed, skipping log cleanup: %v", err)
	} else if err := s.repo.ClearLogs(ctx); err != nil {
		log.Printf("Log cleanup failed: %v", err)
	}

	// Очистка refresh-токенов не зависит от агрегации логов.
	if s.tokens != nil {
		if err := s.tokens.DeleteExpired(ctx); err != nil {
			log.Printf("Expired refresh token cleanup failed: %v", err)
		}
	}

	log.Println("Daily maintenance completed")
}
