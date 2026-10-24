package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"health_checker/internal/domain"
	"health_checker/internal/repository/postgres"

	"github.com/google/uuid"
)

type mockSiteRepo struct {
	// sites — общее «дефолтное» хранилище для методов, у которых не задан хук.
	sites []domain.Site
	// stats — значение, возвращаемое GetSiteStats по умолчанию.
	stats domain.SiteStats

	// Хуки конкретных методов. nil означает «использовать поведение по умолчанию».
	getByIDFn         func(ctx context.Context, id uuid.UUID) (domain.Site, error)
	createFn          func(ctx context.Context, site domain.Site) (uuid.UUID, error)
	deleteFn          func(ctx context.Context, id, userID uuid.UUID) error
	updateFn          func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error)
	updateStatusFn    func(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error)
	getByUserIDFn     func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error)
	getAllSitesFn     func(ctx context.Context) ([]domain.Site, error)
	getActiveFn       func(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error)
	getStatsFn        func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	getNeedingCheckFn func(ctx context.Context, limit int) ([]domain.Site, error)

	// lastCreated — аргумент, переданный в последний вызов Create. Нужен, чтобы
	// проверить, какие именно поля сервис кладёт в сайт (статус, токен, интервал).
	lastCreated domain.Site
	// lastParams — аргумент, переданный в последний вызов UpdateSiteStatus.
	// Позволяет проверить маппинг (status code, response time, last_checked_at).
	lastParams postgres.UpdateSiteStatusParams
}

// Create фиксирует переданный сайт в lastCreated и либо вызывает хук, либо
// возвращает случайный новый ID (имитируя автоинкремент БД).
func (m *mockSiteRepo) Create(ctx context.Context, site domain.Site) (uuid.UUID, error) {
	m.lastCreated = site
	if m.createFn != nil {
		return m.createFn(ctx, site)
	}
	return uuid.New(), nil
}

// Delete по умолчанию успешен; ошибки задаются через deleteFn.
func (m *mockSiteRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}

// GetActiveSitesByStatus/GetAllSites/GetByUserID по умолчанию возвращают sites.
func (m *mockSiteRepo) GetActiveSitesByStatus(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
	if m.getActiveFn != nil {
		return m.getActiveFn(ctx, status)
	}
	return m.sites, nil
}

func (m *mockSiteRepo) GetAllSites(ctx context.Context) ([]domain.Site, error) {
	if m.getAllSitesFn != nil {
		return m.getAllSitesFn(ctx)
	}
	return m.sites, nil
}

func (m *mockSiteRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return m.sites, nil
}

// GetByID по умолчанию ищет сайт среди sites; если не найден — возвращает
// ErrNotFound, как это сделал бы настоящий репозиторий.
func (m *mockSiteRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	for _, s := range m.sites {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Site{}, domain.ErrNotFound
}

func (m *mockSiteRepo) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, userID)
	}
	return m.stats, nil
}

func (m *mockSiteRepo) GetSitesNeedingCheck(ctx context.Context, limit int) ([]domain.Site, error) {
	if m.getNeedingCheckFn != nil {
		return m.getNeedingCheckFn(ctx, limit)
	}
	return m.sites, nil
}

// UpdateSiteStatus фиксирует params в lastParams и возвращает минимальный сайт.
// Именно lastParams проверяется в тестах, чтобы убедиться, что сервис правильно
// собрал аргументы для БД.
func (m *mockSiteRepo) UpdateSiteStatus(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error) {
	m.lastParams = params
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, params)
	}
	return domain.Site{ID: params.ID}, nil
}

func (m *mockSiteRepo) Update(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, update)
	}
	return domain.Site{ID: update.ID}, nil
}

// mockUserRepo — реализация UserRepository для тестов. Сервис сайтов
// использует только GetByID (для проверки роли в GetAllSites), а UserService
// задействует практически все методы, поэтому каждому из них соответствует
// необязательный хук: если он задан — вызывается он, иначе возвращается
// простое поведение по умолчанию.
type mockUserRepo struct {
	// getByIDFn позволяет задать роль/ошибку пользователя в GetByID.
	getByIDFn func(ctx context.Context, id uuid.UUID) (domain.User, error)
	// user — что вернуть по умолчанию, если getByIDFn не задан.
	user domain.User

	createFn          func(ctx context.Context, user domain.User) (uuid.UUID, error)
	deleteFn          func(ctx context.Context, id uuid.UUID) error
	getAllFn          func(ctx context.Context) ([]domain.User, error)
	getByEmailFn      func(ctx context.Context, email string) (domain.User, error)
	getByTelegramIDFn func(ctx context.Context, telegramID int64) (domain.User, error)
	getByUsernameFn   func(ctx context.Context, username string) ([]domain.User, error)
	updateFn          func(ctx context.Context, update domain.UserUpdate) error

	// lastCreated — аргумент последнего вызова Create (проверяем хеш/роль/email).
	lastCreated domain.User
	// lastUpdate — аргумент последнего вызова Update.
	lastUpdate domain.UserUpdate
}

func (m *mockUserRepo) Create(ctx context.Context, user domain.User) (uuid.UUID, error) {
	m.lastCreated = user
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return uuid.New(), nil
}
func (m *mockUserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockUserRepo) GetAll(ctx context.Context) ([]domain.User, error) {
	if m.getAllFn != nil {
		return m.getAllFn(ctx)
	}
	return nil, nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return domain.User{}, domain.ErrNotFound
}
func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return m.user, nil
}
func (m *mockUserRepo) GetByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	if m.getByTelegramIDFn != nil {
		return m.getByTelegramIDFn(ctx, telegramID)
	}
	return domain.User{}, domain.ErrNotFound
}
func (m *mockUserRepo) GetByUsername(ctx context.Context, username string) ([]domain.User, error) {
	if m.getByUsernameFn != nil {
		return m.getByUsernameFn(ctx, username)
	}
	return nil, nil
}
func (m *mockUserRepo) Update(ctx context.Context, update domain.UserUpdate) error {
	m.lastUpdate = update
	if m.updateFn != nil {
		return m.updateFn(ctx, update)
	}
	return nil
}

// errBoom — универсальная «произвольная» ошибка для проверки того, что сервис
// оборачивает ошибки репозитория, сохраняя их через errors.Is (fmt.Errorf с %w).
var errBoom = errors.New("universal")

// newTestSiteService создаёт SiteService с моками. Если userRepo не передан,
// подставляется пустой mockUserRepo (в большинстве тестов он не нужен).
func newTestSiteService(repo SiteRepository, userRepo UserRepository) *SiteService {
	if userRepo == nil {
		userRepo = &mockUserRepo{}
	}
	return NewSiteService(repo, userRepo)
}

// --- Create ---

// Проверяем бизнес-правило: слишком маленький интервал проверки принудительно
// поднимается до минимального (30 секунд) — и в репозиторий, и в ответ.
func TestSiteService_Create_DefaultIntervalClamped(t *testing.T) {
	repo := &mockSiteRepo{}
	svc := newTestSiteService(repo, nil)

	userID := uuid.New()
	site, err := svc.Create(context.Background(), CreateSiteRequest{
		Url:                  "https://example.com",
		Name:                 "Example",
		CheckIntervalSeconds: 5, // меньше минимума
	}, userID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastCreated.CheckIntervalSeconds != 30 {
		t.Errorf("interval = %d, want 30", repo.lastCreated.CheckIntervalSeconds)
	}
	if site.CheckIntervalSeconds != 30 {
		t.Errorf("returned interval = %d, want 30", site.CheckIntervalSeconds)
	}
}

// Корректный (>= 30) интервал не меняется.
func TestSiteService_Create_KeepsValidInterval(t *testing.T) {
	repo := &mockSiteRepo{}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Create(context.Background(), CreateSiteRequest{
		Url:                  "https://example.com",
		CheckIntervalSeconds: 120,
	}, uuid.New())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastCreated.CheckIntervalSeconds != 120 {
		t.Errorf("interval = %d, want 120", repo.lastCreated.CheckIntervalSeconds)
	}
}

// Новый сайт создаётся в статусе pending (ни одной проверки ещё не было),
// сразу активным и привязанным к создателю. Сервис обязан вернуть сайт с ID,
// который присвоил репозиторий.
func TestSiteService_Create_SetsDefaults(t *testing.T) {
	repo := &mockSiteRepo{}
	svc := newTestSiteService(repo, nil)

	userID := uuid.New()
	site, err := svc.Create(context.Background(), CreateSiteRequest{
		Url:  "https://example.com",
		Name: "Example",
	}, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastCreated.Status != "pending" {
		t.Errorf("status = %q, want pending", repo.lastCreated.Status)
	}
	if !repo.lastCreated.IsActive {
		t.Error("is_active должен быть true: сайт сразу включается в мониторинг")
	}
	if repo.lastCreated.UserID != userID {
		t.Errorf("userID = %v, want %v", repo.lastCreated.UserID, userID)
	}
	if site.ID == uuid.Nil {
		t.Error("Create должен вернуть сайт с присвоенным ID")
	}
}

// Ошибка репозитория при создании пробрасывается наверх обёрнутой (%w),
// чтобы вызывающий код мог распознать её через errors.Is.
func TestSiteService_Create_RepoError(t *testing.T) {
	repo := &mockSiteRepo{
		createFn: func(ctx context.Context, site domain.Site) (uuid.UUID, error) {
			return uuid.Nil, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Create(context.Background(), CreateSiteRequest{Url: "https://example.com"}, uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- Delete ---

// Успешное удаление своего сайта доходит до репозитория.
func TestSiteService_Delete_Success(t *testing.T) {
	userID := uuid.New()
	siteID := uuid.New()
	deleted := false
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID, UserID: userID}, nil
		},
		deleteFn: func(ctx context.Context, id, uid uuid.UUID) error {
			deleted = true
			return nil
		},
	}
	svc := newTestSiteService(repo, nil)

	if err := svc.Delete(context.Background(), siteID, userID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("ожидался вызов repo.Delete")
	}
}

// Несуществующий сайт -> ErrNotFound (до вызова Delete).
func TestSiteService_Delete_NotFound(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, nil)

	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Прочие ошибки GetByID оборачиваются.
func TestSiteService_Delete_GetErrorWrapped(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// Удалять можно только свой сайт.
func TestSiteService_Delete_NotBelongUser(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: uuid.New()}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrSiteNotBelongUser) {
		t.Fatalf("err = %v, want ErrSiteNotBelongUser", err)
	}
}

// Гонка: сайт был, но между GetByID и Delete его удалили -> ErrNotFound.
func TestSiteService_Delete_RepoNotFound(t *testing.T) {
	userID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: userID}, nil
		},
		deleteFn: func(ctx context.Context, id, uid uuid.UUID) error {
			return domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, nil)

	err := svc.Delete(context.Background(), uuid.New(), userID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Прочие ошибки Delete оборачиваются.
func TestSiteService_Delete_RepoErrorWrapped(t *testing.T) {
	userID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: userID}, nil
		},
		deleteFn: func(ctx context.Context, id, uid uuid.UUID) error {
			return errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	err := svc.Delete(context.Background(), uuid.New(), userID)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetAllSites ---

// Просматривать все сайты может только администратор.
func TestSiteService_GetAllSites_AdminSuccess(t *testing.T) {
	want := []domain.Site{{ID: uuid.New()}, {ID: uuid.New()}}
	repo := &mockSiteRepo{
		getAllSitesFn: func(ctx context.Context) ([]domain.Site, error) {
			return want, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Role: domain.RoleAdmin}, nil
		},
	}
	svc := newTestSiteService(repo, userRepo)

	got, err := svc.GetAllSites(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
}

// Обычный пользователь получает ErrAccessDenied.
func TestSiteService_GetAllSites_AccessDenied(t *testing.T) {
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Role: domain.RoleUser}, nil
		},
	}
	svc := newTestSiteService(&mockSiteRepo{}, userRepo)

	_, err := svc.GetAllSites(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

// Если пользователь не найден, роль проверить нельзя -> ErrNotFound.
func TestSiteService_GetAllSites_UserNotFound(t *testing.T) {
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(&mockSiteRepo{}, userRepo)

	_, err := svc.GetAllSites(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Прочие ошибки проверки пользователя оборачиваются.
func TestSiteService_GetAllSites_UserRepoErrorWrapped(t *testing.T) {
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	svc := newTestSiteService(&mockSiteRepo{}, userRepo)

	_, err := svc.GetAllSites(context.Background(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// Админ авторизован, но репозиторий сайтов падает -> ошибка оборачивается.
func TestSiteService_GetAllSites_RepoErrorWrapped(t *testing.T) {
	repo := &mockSiteRepo{
		getAllSitesFn: func(ctx context.Context) ([]domain.Site, error) {
			return nil, errBoom
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Role: domain.RoleAdmin}, nil
		},
	}
	svc := newTestSiteService(repo, userRepo)

	_, err := svc.GetAllSites(context.Background(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetByUserID ---

// Список сайтов пользователя конвертируется в SiteResponse: проверяем, что
// ключевые поля (id/url/name/status/is_active) переносятся без потерь.
func TestSiteService_GetByUserID_MapsToResponse(t *testing.T) {
	now := time.Now()
	code := int32(200)
	sites := []domain.Site{
		{
			ID:                   uuid.New(),
			Url:                  "https://a.com",
			Name:                 "A",
			CheckIntervalSeconds: 60,
			Status:               domain.StatusUp,
			IsActive:             true,
			LastStatusCode:       &code,
			LastCheckedAt:        &now,
		},
	}
	repo := &mockSiteRepo{
		getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
			return sites, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.GetByUserID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != sites[0].ID || got[0].Url != "https://a.com" || got[0].Name != "A" {
		t.Errorf("некорректное преобразование: %+v", got[0])
	}
	if got[0].Status != domain.StatusUp || !got[0].IsActive {
		t.Errorf("некорректный статус/активность: %+v", got[0])
	}
}

func TestSiteService_GetByUserID_EmptyReturnsEmpty(t *testing.T) {
	repo := &mockSiteRepo{
		getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
			return nil, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.GetByUserID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestSiteService_GetByUserID_RepoError(t *testing.T) {
	repo := &mockSiteRepo{
		getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
			return nil, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetByUserID(context.Background(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetByID ---

// Владелец получает свой сайт в виде SiteResponse.
func TestSiteService_GetByID_OwnerSuccess(t *testing.T) {
	siteID := uuid.New()
	userID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID, UserID: userID}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	site, err := svc.GetByID(context.Background(), siteID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != siteID {
		t.Errorf("ID = %v, want %v", site.ID, siteID)
	}
}

// Чужой сайт для обычного пользователя -> ErrSiteNotBelongUser (403 на HTTP-слое).
func TestSiteService_GetByID_NotBelongUser(t *testing.T) {
	siteID := uuid.New()
	ownerID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID, UserID: ownerID}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Role: domain.RoleUser}, nil
		},
	}
	svc := newTestSiteService(repo, userRepo)

	_, err := svc.GetByID(context.Background(), siteID, uuid.New()) // не владелец
	if !errors.Is(err, domain.ErrSiteNotBelongUser) {
		t.Fatalf("err = %v, want ErrSiteNotBelongUser", err)
	}
}

// Администратор может читать любой сайт.
func TestSiteService_GetByID_AdminSuccess(t *testing.T) {
	siteID := uuid.New()
	ownerID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID, UserID: ownerID}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{ID: id, Role: domain.RoleAdmin}, nil
		},
	}
	svc := newTestSiteService(repo, userRepo)

	site, err := svc.GetByID(context.Background(), siteID, uuid.New()) // админ
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != siteID {
		t.Errorf("ID = %v, want %v", site.ID, siteID)
	}
}

// Если пользователя нет в БД при проверке админа -> ErrNotFound.
func TestSiteService_GetByID_CallerNotFound(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: uuid.New()}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, userRepo)

	_, err := svc.GetByID(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSiteService_GetByID_NotFound(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetByID(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSiteService_GetByID_ErrorWrapped(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetByID(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetSiteStats ---

func TestSiteService_GetSiteStats(t *testing.T) {
	want := domain.SiteStats{
		TotalSites:      3,
		UpSites:         2,
		DownSites:       1,
		AvgResponseTime: 123.5,
	}
	repo := &mockSiteRepo{
		getStatsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return want, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.GetSiteStats(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("stats = %+v, want %+v", got, want)
	}
}

func TestSiteService_GetSiteStats_Error(t *testing.T) {
	repo := &mockSiteRepo{
		getStatsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return domain.SiteStats{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetSiteStats(context.Background(), uuid.New())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetActiveSitesByStatus ---

func TestSiteService_GetActiveSitesByStatus(t *testing.T) {
	repo := &mockSiteRepo{
		getActiveFn: func(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
			if status != domain.StatusDown {
				t.Errorf("status = %v, want down", status)
			}
			return []domain.Site{{ID: uuid.New()}}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.GetActiveSitesByStatus(context.Background(), domain.StatusDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestSiteService_GetActiveSitesByStatus_Error(t *testing.T) {
	repo := &mockSiteRepo{
		getActiveFn: func(ctx context.Context, status domain.SiteStatus) ([]domain.Site, error) {
			return nil, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetActiveSitesByStatus(context.Background(), domain.StatusDown)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- GetSitesNeedingCheck ---

func TestSiteService_GetSitesNeedingCheck(t *testing.T) {
	repo := &mockSiteRepo{
		getNeedingCheckFn: func(ctx context.Context, limit int) ([]domain.Site, error) {
			if limit != 10 {
				t.Errorf("limit = %d, want 10", limit)
			}
			return []domain.Site{{ID: uuid.New()}}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.GetSitesNeedingCheck(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestSiteService_GetSitesNeedingCheck_Error(t *testing.T) {
	repo := &mockSiteRepo{
		getNeedingCheckFn: func(ctx context.Context, limit int) ([]domain.Site, error) {
			return nil, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.GetSitesNeedingCheck(context.Background(), 10)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- Update ---

// Успешное обновление возвращает SiteResponse.
func TestSiteService_Update_Success(t *testing.T) {
	userID := uuid.New()
	siteID := uuid.New()
	newName := "New name"
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID, UserID: userID}, nil
		},
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			return domain.Site{ID: update.ID, Name: *update.Name}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.Update(context.Background(), domain.SiteUpdate{
		ID:     siteID,
		UserID: userID,
		Name:   &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != newName {
		t.Errorf("name = %q, want %q", got.Name, newName)
	}
}

func TestSiteService_Update_NotFound(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{ID: uuid.New(), UserID: uuid.New()})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSiteService_Update_GetErrorWrapped(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{ID: uuid.New(), UserID: uuid.New()})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

func TestSiteService_Update_NotBelongUser(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: uuid.New()}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{ID: uuid.New(), UserID: uuid.New()})
	if !errors.Is(err, domain.ErrSiteNotBelongUser) {
		t.Fatalf("err = %v, want ErrSiteNotBelongUser", err)
	}
}

func TestSiteService_Update_RepoError(t *testing.T) {
	userID := uuid.New()
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id, UserID: userID}, nil
		},
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			return domain.Site{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.Update(context.Background(), domain.SiteUpdate{ID: uuid.New(), UserID: userID})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}

// --- UpdateStatusByID ---

func TestSiteService_UpdateStatusByID_Success(t *testing.T) {
	siteID := uuid.New()
	code := int32(200)
	rt := 42
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: siteID}, nil
		},
	}
	svc := newTestSiteService(repo, nil)

	got, err := svc.UpdateStatusByID(context.Background(), siteID, domain.StatusUp, code, &rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != siteID {
		t.Errorf("ID = %v, want %v", got.ID, siteID)
	}
	p := repo.lastParams
	if p.Status == nil || *p.Status != string(domain.StatusUp) {
		t.Errorf("params.Status = %v, want up", p.Status)
	}
	if p.ResponseTimeMs == nil || *p.ResponseTimeMs != int32(rt) {
		t.Errorf("params.ResponseTimeMs = %v, want %d", p.ResponseTimeMs, rt)
	}
}

func TestSiteService_UpdateStatusByID_NotFound(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.UpdateStatusByID(context.Background(), uuid.New(), domain.StatusUp, 200, nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSiteService_UpdateStatusByID_RepoError(t *testing.T) {
	repo := &mockSiteRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: id}, nil
		},
		updateStatusFn: func(ctx context.Context, params postgres.UpdateSiteStatusParams) (domain.Site, error) {
			return domain.Site{}, errBoom
		},
	}
	svc := newTestSiteService(repo, nil)

	_, err := svc.UpdateStatusByID(context.Background(), uuid.New(), domain.StatusUp, 200, nil)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want wrapped errBoom", err)
	}
}
