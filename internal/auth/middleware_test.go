// Тесты для AuthMiddleware и RequireRole: проверка разбора заголовка
// Authorization, обработки невалидных/истёкших токенов и контроля доступа по ролям.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"health_checker/internal/domain"
)

// okHandler — эндпоинт-заглушка, отвечающий 200 и (опционально) отдающий claims
// из контекста, чтобы проверить, что middleware действительно их положила.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if claims, ok := GetUserFromContext(r.Context()); ok {
			_ = json.NewEncoder(w).Encode(claims)
		}
	})
}

// issueAccess выпускает валидный access-токен заданной роли.
func issueAccess(t *testing.T, m *JWTManager, role domain.Role) string {
	t.Helper()
	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", role, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return pair.AccessToken
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	m := newTestManager()
	token := issueAccess(t, m, domain.RoleUser)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	m.AuthMiddleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Проверяем, что claims попали в контекст с правильными полями.
	var got Claims
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("не удалось разобрать claims: %v", err)
	}
	if got.UserID != testUserID {
		t.Errorf("UserID = %v, want %v", got.UserID, testUserID)
	}
	if got.Role != domain.RoleUser {
		t.Errorf("Role = %q, want user", got.Role)
	}
}

// Регистр схемы "bearer" не важен.
func TestAuthMiddleware_BearerCaseInsensitive(t *testing.T) {
	m := newTestManager()
	token := issueAccess(t, m, domain.RoleUser)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer "+token)

	m.AuthMiddleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_Rejects(t *testing.T) {
	m := newTestManager()
	expiredM := NewJWTManager("access-secret", "refresh-secret", -time.Minute, time.Hour)
	expired := issueAccess(t, expiredM, domain.RoleUser)

	other := NewJWTManager("other", "other", time.Minute, time.Hour)
	foreign := issueAccess(t, other, domain.RoleUser)

	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"no scheme", "justtoken"},
		{"wrong scheme", "Token abc"},
		{"empty token after scheme", "Bearer "},
		{"garbage token", "Bearer not-a-jwt"},
		{"expired token", "Bearer " + expired},
		{"foreign secret", "Bearer " + foreign},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			m.AuthMiddleware(okHandler()).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// Refresh-токен не должен приниматься как access в middleware.
func TestAuthMiddleware_RejectsRefreshToken(t *testing.T) {
	m := newTestManager()
	pair, err := m.GenerateTokenPair(context.Background(), testUserID, "a@example.com", domain.RoleUser, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)

	m.AuthMiddleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (refresh не должен проходить как access)", rec.Code)
	}
}

// --- GetUserFromContext ---

func TestGetUserFromContext_Absent(t *testing.T) {
	if _, ok := GetUserFromContext(context.Background()); ok {
		t.Error("в пустом контексте не должно быть claims")
	}
}

// --- RequireRole ---

func TestRequireRole(t *testing.T) {
	user := &Claims{UserID: testUserID, Role: domain.RoleUser}
	admin := &Claims{UserID: testUserID, Role: domain.RoleAdmin}

	cases := []struct {
		name     string
		claims   *Claims // nil — нет claims в контексте
		allowed  []domain.Role
		wantCode int
	}{
		{"admin allowed", admin, []domain.Role{domain.RoleAdmin}, http.StatusOK},
		{"user not allowed", user, []domain.Role{domain.RoleAdmin}, http.StatusForbidden},
		{"user allowed", user, []domain.Role{domain.RoleUser, domain.RoleAdmin}, http.StatusOK},
		{"no claims", nil, []domain.Role{domain.RoleUser}, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var h http.Handler = next
			h = RequireRole(tc.allowed...)(h)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			// Имитируем AuthMiddleware: кладём claims в контекст.
			if tc.claims != nil {
				req = req.WithContext(context.WithValue(req.Context(), UserContextKey, tc.claims))
			}

			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}
