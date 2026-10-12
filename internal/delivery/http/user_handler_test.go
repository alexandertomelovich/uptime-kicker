package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"health_checker/internal/auth"
	"health_checker/internal/domain"
	"health_checker/internal/service"

	"github.com/google/uuid"
)

// --- Register ---

func TestUserHandler_Register_Success(t *testing.T) {
	svc := &mockUserService{
		registerFn: func(ctx context.Context, req service.RegisterRequest) (domain.User, error) {
			if req.Email != "a@example.com" || req.TelegramID != 5 {
				t.Errorf("неверный запрос в сервис: %+v", req)
			}
			return domain.User{ID: uuid.New(), Email: req.Email}, nil
		},
	}
	h := userRouter(svc, domain.RoleUser) // register — публичный маршрут

	rec := do(h, http.MethodPost, "/auth/register", `{"email":"a@example.com","telegram_id":5,"password":"Passw0rd!"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	var got domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got.Email != "a@example.com" {
		t.Errorf("email = %q", got.Email)
	}
}

func TestUserHandler_Register_InvalidBody(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/register", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_Register_EmailExists(t *testing.T) {
	svc := &mockUserService{
		registerFn: func(ctx context.Context, req service.RegisterRequest) (domain.User, error) {
			return domain.User{}, domain.ErrEmailAlreadyExists
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/register", `{"email":"a@example.com"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_Register_InternalError(t *testing.T) {
	svc := &mockUserService{
		registerFn: func(ctx context.Context, req service.RegisterRequest) (domain.User, error) {
			return domain.User{}, errBoom
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/register", `{"email":"a@example.com"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// --- Login ---

func TestUserHandler_Login_Success(t *testing.T) {
	svc := &mockUserService{
		loginFn: func(ctx context.Context, email, password string) (*auth.TokenPair, error) {
			return &auth.TokenPair{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer"}, nil
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/login", `{"email":"a@example.com","password":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var got auth.TokenPair
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got.AccessToken == "" {
		t.Error("ожидался access-токен")
	}
}

func TestUserHandler_Login_InvalidCredentials(t *testing.T) {
	svc := &mockUserService{
		loginFn: func(ctx context.Context, email, password string) (*auth.TokenPair, error) {
			return nil, domain.ErrInvalidCredentials
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/login", `{"email":"a@example.com","password":"bad"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// --- Refresh ---

func TestUserHandler_Refresh_Success(t *testing.T) {
	svc := &mockUserService{
		refreshFn: func(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
			if refreshToken != "old-refresh" {
				t.Errorf("refreshToken = %q", refreshToken)
			}
			return &auth.TokenPair{AccessToken: "new", RefreshToken: "new-r"}, nil
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/refresh", `{"refresh_token":"old-refresh"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// Ошибка refresh -> 401 (handler не различает причины токена).
func TestUserHandler_Refresh_Invalid(t *testing.T) {
	svc := &mockUserService{
		refreshFn: func(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
			return nil, auth.ErrTokenRevoked
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/refresh", `{"refresh_token":"x"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// --- Logout ---

func TestUserHandler_Logout_Success(t *testing.T) {
	svc := &mockUserService{}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/logout", `{"refresh_token":"x"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestUserHandler_Logout_Invalid(t *testing.T) {
	svc := &mockUserService{
		logoutFn: func(ctx context.Context, refreshToken string) error {
			return errBoom
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/auth/logout", `{"refresh_token":"x"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// --- GetAll ---

func TestUserHandler_GetAll_Success(t *testing.T) {
	svc := &mockUserService{
		getAllFn: func(ctx context.Context) ([]domain.User, error) {
			return []domain.User{{ID: uuid.New()}, {ID: uuid.New()}}, nil
		},
	}
	h := userRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/users", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestUserHandler_GetAll_AccessDenied(t *testing.T) {
	svc := &mockUserService{
		getAllFn: func(ctx context.Context) ([]domain.User, error) {
			return nil, domain.ErrAccessDenied
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/users", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- GetByID ---

func TestUserHandler_GetByID_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockUserService{
		getByIDFn: func(ctx context.Context, got uuid.UUID) (domain.User, error) {
			if got != id {
				t.Errorf("id = %v, want %v", got, id)
			}
			return domain.User{ID: id}, nil
		},
	}
	h := userRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/users/"+id.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// Невалидный UUID в пути -> 400, без обращения к сервису.
func TestUserHandler_GetByID_InvalidUUID(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/users/not-a-uuid", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_GetByID_NotFound(t *testing.T) {
	svc := &mockUserService{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		},
	}
	h := userRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/users/"+uuid.New().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Update ---

func TestUserHandler_Update_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockUserService{}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/users/"+id.String(), `{"name":"New"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUpdate.ID != id {
		t.Errorf("params.ID = %v, want %v", svc.lastUpdate.ID, id)
	}
	if svc.lastUpdate.Name == nil || *svc.lastUpdate.Name != "New" {
		t.Errorf("params.Name = %v, want New", svc.lastUpdate.Name)
	}
}

func TestUserHandler_Update_InvalidUUID(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/users/bad", `{"name":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_Update_InvalidBody(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/users/"+uuid.New().String(), `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_Update_AccessDenied(t *testing.T) {
	svc := &mockUserService{
		updateFn: func(ctx context.Context, params service.UpdateUserParams) error {
			return domain.ErrAccessDenied
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/users/"+uuid.New().String(), `{"name":"x"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- Delete ---

func TestUserHandler_Delete_Success(t *testing.T) {
	id := uuid.New()
	called := false
	svc := &mockUserService{
		deleteFn: func(ctx context.Context, got uuid.UUID) error {
			called = true
			if got != id {
				t.Errorf("id = %v, want %v", got, id)
			}
			return nil
		},
	}
	h := userRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodDelete, "/users/"+id.String(), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !called {
		t.Error("ожидался вызов svc.Delete")
	}
}

func TestUserHandler_Delete_InvalidUUID(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleAdmin)

	rec := do(h, http.MethodDelete, "/users/bad-id", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserHandler_Delete_AccessDenied(t *testing.T) {
	svc := &mockUserService{
		deleteFn: func(ctx context.Context, id uuid.UUID) error {
			return domain.ErrAccessDenied
		},
	}
	h := userRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodDelete, "/users/"+uuid.New().String(), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// Защищённые маршруты обёрнуты реальным JWTMiddleware: без заголовка
// Authorization они должны отвечать 401, не доходя до сервиса.
func TestUserHandler_ProtectedRoutes_Unauthorized(t *testing.T) {
	_, mw := realAuthMiddleware()
	h := userRouterWithMiddleware(&mockUserService{}, mw)

	id := uuid.New().String()
	cases := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/users", ""},
		{http.MethodGet, "/users/" + id, ""},
		{http.MethodPatch, "/users/" + id, `{"name":"x"}`},
		{http.MethodDelete, "/users/" + id, ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := do(h, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// С валидным access-токеном тот же защищённый маршрут проходит (200).
func TestUserHandler_ProtectedRoutes_WithValidToken(t *testing.T) {
	m, mw := realAuthMiddleware()
	h := userRouterWithMiddleware(&mockUserService{
		getAllFn: func(ctx context.Context) ([]domain.User, error) {
			return []domain.User{}, nil
		},
	}, mw)

	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleAdmin, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	rec := doWithToken(h, http.MethodGet, "/users", "", pair.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// Неизвестный маршрут -> 404 (проверяем, что роутер отдаёт NotFound).
func TestUserHandler_NotFoundRoute(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/nonexistent", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Вспомогательный тест-кейс: сервис вернул 500-ошибку на Update.
func TestUserHandler_Update_InternalError(t *testing.T) {
	svc := &mockUserService{
		updateFn: func(ctx context.Context, params service.UpdateUserParams) error {
			return fmt.Errorf("db down: %w", errBoom)
		},
	}
	h := userRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/users/"+uuid.New().String(), `{"name":"x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// Битое тело запроса на каждом публичном маршруте аутентификации -> 400.
func TestUserHandler_AuthRoutes_InvalidBody(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleUser)

	cases := []struct{ name, path string }{
		{"register", "/auth/register"},
		{"login", "/auth/login"},
		{"refresh", "/auth/refresh"},
		{"logout", "/auth/logout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, http.MethodPost, tc.path, `{broken`)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// NewUserHandler паникует на nil-сервисе.
func TestNewUserHandler_NilServicePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ожидалась паника при nil-сервисе")
		}
	}()
	NewUserHandler(nil)
}

// Register с чрезмерно большим телом -> 400 (срабатывает MaxBytesReader).
func TestUserHandler_Register_BodyTooLarge(t *testing.T) {
	h := userRouter(&mockUserService{}, domain.RoleUser)

	big := `{"name":"` + strings.Repeat("a", 1<<20+10) + `"}`
	rec := do(h, http.MethodPost, "/auth/register", big)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
