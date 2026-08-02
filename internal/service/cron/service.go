package cron

import (
	"context"
	"fmt"
	"health_checker/internal/repository"
	"log"
	"time"

	"github.com/robfig/cron"
)

type DailyStatsService struct {
	repo repository.CheckRepository
	cron *cron.Cron
}

func NewDailyStatsService(repo repository.CheckRepository) *DailyStatsService {
	return &DailyStatsService{
		repo: repo,
		cron: cron.New(), 
	}
}

func (s *DailyStatsService) StartCron() error {
	if s.cron != nil && s.cron.Entries() != nil {
        return fmt.Errorf("cron already started")
    }
	
	if err := s.cron.AddFunc("5 0 * * *", func() {
		s.ExecuteDailyCheck()
	}); err != nil {
		return fmt.Errorf("failed to schedule cron job: %w", err)
	}

	s.cron.Start()
	return nil
}

func (s *DailyStatsService) StopCron(ctx context.Context) {
	s.cron.Stop()
	select {
    case <-ctx.Done():
        log.Println("Cron stopped with context cancellation")
    case <-time.After(5 * time.Second):
        log.Println("Cron stopped with timeout")
    }

}

func (s *DailyStatsService) ExecuteDailyCheck() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Panic in daily check: %v", r)
        }
    }()

    ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Minute)
    defer cancel()

    log.Println("Starting daily check...")
    if err := s.repo.AggregateDailyStats(ctx); err != nil {
        log.Printf("Daily check failed: %v", err)
        return
    }
    log.Println("Daily check completed successfully")
}
