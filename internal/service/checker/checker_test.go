package service

import (
	"context"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository/postgres"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Моки ---

type mockSiteRepo struct {
	mu       sync.Mutex
	sites    []domain.Site
	statuses map[uuid.UUID]domain.SiteStatus

	// lastParams — аргументы последнего UpdateSiteStatus (проверяется в тестах).
	lastParams postgres.UpdateSiteStatusParams
	// updateErr позволяет смоделировать ошибку БД при обновлении статуса.
	updateErr error
	// getSitesErr позволяет смоделировать ошибку при выборке сайтов.
	getSitesErr error
}

func newMockSiteRepo(sites ...domain.Site) *mockSiteRepo {
	m := &mockSiteRepo{
		sites:    sites,
		statuses: make(map[uuid.UUID]domain.SiteStatus),
	}
	for _, s := range sites {
		m.statuses[s.ID] = s.Status
	}
	return m
}

func (m *mockSiteRepo) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getSitesErr != nil {
		return nil, m.getSitesErr
	}
	if limit > 0 && len(m.sites) > limit {
		return m.sites[:limit], nil
	}
	return m.sites, nil
}

func (m *mockSiteRepo) GetByIDWithOwner(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sites {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Site{}, domain.ErrNotFound
}

func (m *mockSiteRepo) UpdateSiteStatus(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastParams = params
	if m.updateErr != nil {
		return domain.Site{}, m.updateErr
	}
	if params.Status != nil {
		m.statuses[params.ID] = domain.SiteStatus(*params.Status)
	}
	return domain.Site{ID: params.ID}, nil
}

// statusOf возвращает текущий «сохранённый» статус сайта.
func (m *mockSiteRepo) statusOf(id uuid.UUID) domain.SiteStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statuses[id]
}

type mockCheckRepo struct {
	inserts int64
}

func (m *mockCheckRepo) InsertLog(ctx context.Context, log domain.CheckLogsRaw) error {
	atomic.AddInt64(&m.inserts, 1)
	return nil
}

func (m *mockCheckRepo) count() int64 {
	return atomic.LoadInt64(&m.inserts)
}

// capturingCheckRepo — CheckRepository, который запоминает вставленные логи и
// умеет имитировать ошибку вставки.
type capturingCheckRepo struct {
	mu   sync.Mutex
	logs []domain.CheckLogsRaw
	err  error
}

func (m *capturingCheckRepo) InsertLog(ctx context.Context, log domain.CheckLogsRaw) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.logs = append(m.logs, log)
	return nil
}

type mockSender struct {
	sent int64
}

func (m *mockSender) Send(ctx context.Context, message notifier.Message) error {
	atomic.AddInt64(&m.sent, 1)
	return nil
}

// capturingSender — notifier.Sender, который запоминает сообщения и умеет
// имитировать ошибку отправки.
type capturingSender struct {
	mu       sync.Mutex
	messages []notifier.Message
	err      error
}

func (m *capturingSender) Send(ctx context.Context, message notifier.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, message)
	return nil
}

func (m *capturingSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// --- Тесты ---

// TestStopDoesNotPanicAndReturns быстро останавливает сервис с активными
// воркерами и убеждается, что Stop не паникует (send on closed channel)
// и возвращается без дедлока.
func TestStopDoesNotPanicAndReturns(t *testing.T) {
	sites := []domain.Site{
		{ID: uuid.New(), Url: "https://example.com", Status: domain.StatusUp, OwnerTelegramID: 1},
		{ID: uuid.New(), Url: "https://example.org", Status: domain.StatusUp, OwnerTelegramID: 2},
	}

	svc := NewCheckerService(
		newMockSiteRepo(sites...),
		&mockCheckRepo{},
		&mockSender{},
		4,  // воркеров
		10, // лимит
		4,  // размер очереди
	)

	svc.Start()

	// Дадим сервису немного поработать, затем остановим.
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within timeout (possible deadlock)")
	}
}

// TestStopIsIdempotent проверяет, что повторный Stop не паникует.
func TestStopIsIdempotent(t *testing.T) {
	svc := NewCheckerService(
		newMockSiteRepo(),
		&mockCheckRepo{},
		&mockSender{},
		2, 2, 2,
	)

	svc.Start()
	svc.Stop()
	svc.Stop() // не должно паниковать
}

// TestStartIsIdempotent проверяет, что повторный Start не удваивает воркеров
// (иначе Stop может зависнуть или сервис будет работать дважды).
func TestStartIsIdempotent(t *testing.T) {
	svc := NewCheckerService(
		newMockSiteRepo(),
		&mockCheckRepo{},
		&mockSender{},
		2, 2, 2,
	)

	svc.Start()
	svc.Start() // не должно создавать дублирующих горутин
	svc.Stop()
}
