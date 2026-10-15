// Тесты для cron-сервиса агрегации дневной статистики (service.go).
// Репозиторий подменяется простым моком (интерфейс из одного метода), сам
// планировщик тестируется косвенно через ExecuteDailyCheck и через проверку
// жизненного цикла StartCron/StopCron.
package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockCheckRepo реализует CheckRepository, запоминая вызовы и умея имитировать
// ошибку (в т.ч. панику) — это нужно, чтобы проверить защиту ExecuteDailyCheck.
// Счётчик защищён мьютексом: задачи могут выполняться из горутины планировщика.
type mockCheckRepo struct {
	mu         sync.Mutex
	calls      int
	err        error
	panicValue any
}

func (m *mockCheckRepo) AggregateDailyStats(ctx context.Context) error {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.panicValue != nil {
		panic(m.panicValue)
	}
	return m.err
}

func (m *mockCheckRepo) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// Успешный запуск: репозиторий вызывается ровно один раз, ошибки нет.
func TestExecuteDailyCheck_Success(t *testing.T) {
	repo := &mockCheckRepo{}
	svc := NewDailyStatsService(repo)

	svc.ExecuteDailyCheck()

	if got := repo.callCount(); got != 1 {
		t.Errorf("AggregateDailyStats вызван %d раз, want 1", got)
	}
}

// Ошибка репозитория не должна ронять сервис (только логируется).
func TestExecuteDailyCheck_RepoError(t *testing.T) {
	repo := &mockCheckRepo{err: errors.New("db down")}
	svc := NewDailyStatsService(repo)

	// Не должно паниковать.
	svc.ExecuteDailyCheck()

	if got := repo.callCount(); got != 1 {
		t.Errorf("AggregateDailyStats вызван %d раз, want 1", got)
	}
}

// Паника внутри репозитория перехватывается recover — сервис не падает.
func TestExecuteDailyCheck_RecoversPanic(t *testing.T) {
	repo := &mockCheckRepo{panicValue: "boom"}
	svc := NewDailyStatsService(repo)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ExecuteDailyCheck не должен пробрасывать панику наверх, got %v", r)
		}
	}()

	svc.ExecuteDailyCheck()
}

// Повторный StartCron является ошибкой (планировщик уже запущен).
func TestStartCron_TwiceReturnsError(t *testing.T) {
	svc := NewDailyStatsService(&mockCheckRepo{})

	if err := svc.StartCron(); err != nil {
		t.Fatalf("первый StartCron: %v", err)
	}
	defer svc.StopCron(context.Background())

	if err := svc.StartCron(); err == nil {
		t.Error("ожидалась ошибка при повторном StartCron")
	}
}

// StopCron завершается при отменённом контексте (не блокируется).
func TestStopCron_CancelledContext(t *testing.T) {
	svc := NewDailyStatsService(&mockCheckRepo{})
	if err := svc.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем сразу

	svc.StopCron(ctx) // не должно зависнуть
}

// StopCron с неотменённым контекстом должен дождаться graceful-остановки
// планировщика и вернуться задолго до истечения контекста.
func TestStopCron_GracefulWithLiveContext(t *testing.T) {
	svc := NewDailyStatsService(&mockCheckRepo{})
	if err := svc.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		svc.StopCron(ctx)
		close(done)
	}()

	select {
	case <-done:
		// ok — graceful-остановка не ждёт весь таймаут контекста
	case <-time.After(2 * time.Second):
		t.Fatal("StopCron не завершился за отведённое время")
	}
}

// StartCron регистрирует ровно одну запись в планировщике с ожидаемым
// расписанием, а StopCron корректно останавливает его.
func TestStartCron_RegistersEntry(t *testing.T) {
	svc := NewDailyStatsService(&mockCheckRepo{})

	if err := svc.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}
	defer svc.StopCron(context.Background())

	entries := svc.cron.Entries()
	if len(entries) != 1 {
		t.Fatalf("записей в планировщике = %d, want 1", len(entries))
	}
	if got := entries[0].Schedule.Next(time.Now()).IsZero(); got {
		t.Error("расписание записи не должно быть пустым")
	}
}
