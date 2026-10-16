// Тесты для HTTP-слоя (пакет httphandler).
//
// Стратегия: сервисы (бизнес-логика) подменяются ручными моками, а сами
// handler-ы вызываются через реальный chi-роутер с помощью httptest. Так мы
// проверяем именно то, за что отвечает этот слой: маршрутизацию, парсинг
// запроса, коды ответов, формат JSON и — главное — маппинг domain-ошибок в
// HTTP-статусы (mapServiceError).
//
// Аутентификация: protected-маршруты оборачиваются тестовым middleware, который
// кладёт в контекст auth.Claims так же, как настоящий JWTManager.AuthMiddleware.
// Это позволяет проверять и сценарии без пользователя в контексте (401).
package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// errBoom — произвольная ошибка, не входящая в набор доменных: сервис оборачивает
// непредвиденные ошибки, и handler должен ответить 500 без утечки текста ошибки.
var errBoom = errors.New("boom")

// --- Мок UserService ---

type mockUserService struct {
	registerFn func(ctx context.Context, req service.RegisterRequest) (domain.User, error)
	loginFn    func(ctx context.Context, email, password string) (*auth.TokenPair, error)
	refreshFn  func(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	logoutFn   func(ctx context.Context, refreshToken string) error
	getAllFn   func(ctx context.Context) ([]domain.User, error)
	getByIDFn  func(ctx context.Context, id uuid.UUID) (domain.User, error)
	updateFn   func(ctx context.Context, params service.UpdateUserParams) error
	deleteFn   func(ctx context.Context, id uuid.UUID) error

	lastUpdate service.UpdateUserParams
}

func (m *mockUserService) Register(ctx context.Context, req service.RegisterRequest) (domain.User, error) {
	if m.registerFn != nil {
		return m.registerFn(ctx, req)
	}
	return domain.User{ID: uuid.New()}, nil
}
func (m *mockUserService) Login(ctx context.Context, email, password string) (*auth.TokenPair, error) {
	if m.loginFn != nil {
		return m.loginFn(ctx, email, password)
	}
	return &auth.TokenPair{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer"}, nil
}
func (m *mockUserService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, refreshToken)
	}
	return &auth.TokenPair{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer"}, nil
}
func (m *mockUserService) Logout(ctx context.Context, refreshToken string) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, refreshToken)
	}
	return nil
}
func (m *mockUserService) GetAll(ctx context.Context) ([]domain.User, error) {
	if m.getAllFn != nil {
		return m.getAllFn(ctx)
	}
	return nil, nil
}
func (m *mockUserService) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return domain.User{ID: id}, nil
}
func (m *mockUserService) Update(ctx context.Context, params service.UpdateUserParams) error {
	m.lastUpdate = params
	if m.updateFn != nil {
		return m.updateFn(ctx, params)
	}
	return nil
}
func (m *mockUserService) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// --- Мок SiteService ---

type mockSiteService struct {
	createFn      func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error)
	verifyFn      func(ctx context.Context, id, userID uuid.UUID, token string) error
	deleteFn      func(ctx context.Context, id, userID uuid.UUID) error
	getAllSitesFn func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error)
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error)
	getByIDFn     func(ctx context.Context, id uuid.UUID) (domain.Site, error)
	getStatsFn    func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error)
	updateFn      func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error)

	lastCreate service.CreateSiteRequest
	lastUpdate domain.SiteUpdate
}

func (m *mockSiteService) Create(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
	m.lastCreate = req
	if m.createFn != nil {
		return m.createFn(ctx, req, userID)
	}
	return domain.Site{ID: uuid.New(), UserID: userID}, nil
}
func (m *mockSiteService) VerifySite(ctx context.Context, id, userID uuid.UUID, token string) error {
	if m.verifyFn != nil {
		return m.verifyFn(ctx, id, userID, token)
	}
	return nil
}
func (m *mockSiteService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}
func (m *mockSiteService) GetAllSites(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
	if m.getAllSitesFn != nil {
		return m.getAllSitesFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockSiteService) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockSiteService) GetByID(ctx context.Context, id uuid.UUID) (domain.Site, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return domain.Site{ID: id}, nil
}
func (m *mockSiteService) GetSiteStats(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, userID)
	}
	return domain.SiteStats{}, nil
}
func (m *mockSiteService) Update(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
	m.lastUpdate = update
	if m.updateFn != nil {
		return m.updateFn(ctx, update)
	}
	return domain.Site{ID: update.ID}, nil
}

// --- Хелперы для сборки роутера и запросов ---

// testUserID — фиксированный id «аутентифицированного» пользователя.
var testUserID = uuid.New()

// authInjector — тестовый middleware, кладущий Claims в контекст. Если role == "",
// claims не добавляются (имитация неаутентифицированного запроса, хотя маршрут
// защищён — тогда getUserID вернёт 401).
func authInjector(role domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if role == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims := &auth.Claims{UserID: testUserID, Role: role}
			ctx := context.WithValue(r.Context(), auth.UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// userRouter собирает роутер только с UserHandler и заданной ролью аутентификации.
func userRouter(svc UserService, role domain.Role) http.Handler {
	r := chi.NewRouter()
	NewUserHandler(svc).Routes(r, authInjector(role))
	return r
}

// userRouterWithMiddleware собирает роутер с произвольным auth-middleware.
// Нужен, чтобы проверить реальную защиту маршрутов (JWTManager.AuthMiddleware).
func userRouterWithMiddleware(svc UserService, mw func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	NewUserHandler(svc).Routes(r, mw)
	return r
}

// siteRouterWithMiddleware — аналогично для SiteHandler.
func siteRouterWithMiddleware(svc SiteService, mw func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	NewSiteHandler(svc).Routes(r, mw)
	return r
}

// siteRouter собирает роутер только с SiteHandler и заданной ролью.
func siteRouter(svc SiteService, role domain.Role) http.Handler {
	r := chi.NewRouter()
	NewSiteHandler(svc).Routes(r, authInjector(role))
	return r
}

// do выполняет запрос к роутеру и возвращает recorder.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// realAuthMiddleware возвращает настоящий JWTManager.AuthMiddleware для проверки
// реальной защиты маршрутов. Также отдаёт сам менеджер, чтобы выпустить валидный
// access-токен в тестах «с токеном».
func realAuthMiddleware() (*auth.JWTManager, func(http.Handler) http.Handler) {
	m := auth.NewJWTManager("access-secret", "refresh-secret", time.Minute, time.Hour)
	return m, m.AuthMiddleware
}

// doWithToken выполняет запрос с заголовком Authorization: Bearer <token>.
func doWithToken(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
