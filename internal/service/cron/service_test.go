// Тесты для cron-сервиса ежедневного обслуживания (service.go):
// агрегация дневной статистики + очистка устаревших логов и истёкших
// refresh-сессий. Репозитории подменяются простыми моками, сам планировщик
// тестируется косвенно через ExecuteDailyCheck и через проверку жизненного
// цикла StartCron/StopCron.
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
// Счётчики защищены мьютексом: задачи могут выполняться из горутины планировщика.
type mockCheckRepo struct {
	mu         sync.Mutex
	aggCalls   int
	clearCalls int

	aggErr      error
	clearErr    error
	panicValue  any
	clearShouldPanic bool
}

func (m *mockCheckRepo) AggregateDailyStats(ctx context.Context) error {
	m.mu.Lock()
	m.aggCalls++
	m.mu.Unlock()
	if m.panicValue != nil {
		panic(m.panicValue)
	}
	return m.aggErr
}

func (m *mockCheckRepo) ClearLogs(ctx context.Context) error {
	m.mu.Lock()
	m.clearCalls++
	m.mu.Unlock()
	if m.clearShouldPanic {
		panic("clear boom")
	}
	return m.clearErr
}

func (m *mockCheckRepo) counts() (agg, clear int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.aggCalls, m.clearCalls
}

// mockTokenCleaner реализует TokenCleaner.
type mockTokenCleaner struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (m *mockTokenCleaner) DeleteExpired(ctx context.Context) error {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.err
}

func (m *mockTokenCleaner) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// Успешный запуск: агрегация, очистка логов и очистка токенов — по одному разу.
func TestExecuteDailyCheck_Success(t *testing.T) {
	repo := &mockCheckRepo{}
	tokens := &mockTokenCleaner{}
	svc := NewMaintenanceService(repo, tokens)

	svc.ExecuteDailyCheck()

	agg, clear := repo.counts()
	if agg != 1 {
		t.Errorf("AggregateDailyStats вызван %d раз, want 1", agg)
	}
	if clear != 1 {
		t.Errorf("ClearLogs вызван %d раз, want 1", clear)
	}
	if tokens.callCount() != 1 {
		t.Errorf("DeleteExpired вызван %d раз, want 1", tokens.callCount())
	}
}

// Ошибка агрегации => очистка логов НЕ выполняется (нельзя удалять сырые
// данные, не попавшие в дневную статистику), но refresh-токены чистятся —
// это независимая таблица.
func TestExecuteDailyCheck_AggErrorSkipsLogCleanup(t *testing.T) {
	repo := &mockCheckRepo{aggErr: errors.New("db down")}
	tokens := &mockTokenCleaner{}
	svc := NewMaintenanceService(repo, tokens)

	svc.ExecuteDailyCheck() // не должен паниковать

	agg, clear := repo.counts()
	if agg != 1 {
		t.Errorf("AggregateDailyStats вызван %d раз, want 1", agg)
	}
	if clear != 0 {
		t.Errorf("ClearLogs вызван %d раз, want 0 (агрегация не прошла)", clear)
	}
	if tokens.callCount() != 1 {
		t.Errorf("DeleteExpired вызван %d раз, want 1", tokens.callCount())
	}
}

// Ошибка агрегации не должна ронять сервис (только логируется).
func TestExecuteDailyCheck_RepoError(t *testing.T) {
	repo := &mockCheckRepo{aggErr: errors.New("db down")}
	svc := NewMaintenanceService(repo, &mockTokenCleaner{})

	svc.ExecuteDailyCheck()

	if agg, _ := repo.counts(); agg != 1 {
		t.Errorf("AggregateDailyStats вызван %d раз, want 1", agg)
	}
}

// Паника внутри репозитория перехватывается recover — сервис не падает.
func TestExecuteDailyCheck_RecoversPanic(t *testing.T) {
	repo := &mockCheckRepo{panicValue: "boom"}
	svc := NewMaintenanceService(repo, &mockTokenCleaner{})

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ExecuteDailyCheck не должен пробрасывать панику наверх, got %v", r)
		}
	}()

	svc.ExecuteDailyCheck()
}

// nil-очиститель токенов допустим — эта часть обслуживания просто пропускается.
func TestExecuteDailyCheck_NilTokenCleaner(t *testing.T) {
	repo := &mockCheckRepo{}
	svc := NewMaintenanceService(repo, nil)

	svc.ExecuteDailyCheck()

	if agg, clear := repo.counts(); agg != 1 || clear != 1 {
		t.Errorf("agg=%d clear=%d, want 1/1", agg, clear)
	}
}

// Повторный StartCron является ошибкой (планировщик уже запущен).
func TestStartCron_TwiceReturnsError(t *testing.T) {
	svc := NewMaintenanceService(&mockCheckRepo{}, &mockTokenCleaner{})

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
	svc := NewMaintenanceService(&mockCheckRepo{}, &mockTokenCleaner{})
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
	svc := NewMaintenanceService(&mockCheckRepo{}, &mockTokenCleaner{})
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
	svc := NewMaintenanceService(&mockCheckRepo{}, &mockTokenCleaner{})

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
