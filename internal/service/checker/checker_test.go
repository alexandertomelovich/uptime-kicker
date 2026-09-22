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
	if len(m.sites) > limit {
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
	if params.Status != nil {
		m.statuses[params.ID] = domain.SiteStatus(*params.Status)
	}
	return domain.Site{ID: params.ID}, nil
}

type mockCheckRepo struct {
	inserts int64
}

func (m *mockCheckRepo) InsertLog(ctx context.Context, log domain.CheckLogsRaw) error {
	atomic.AddInt64(&m.inserts, 1)
	return nil
}

type mockSender struct {
	sent int64
}

func (m *mockSender) Send(ctx context.Context, message notifier.Message) error {
	atomic.AddInt64(&m.sent, 1)
	return nil
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
