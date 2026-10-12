package httphandler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"health_checker/internal/domain"
	"health_checker/internal/service"

	"github.com/google/uuid"
)

// --- Create ---

func TestSiteHandler_Create_Success(t *testing.T) {
	svc := &mockSiteService{
		createFn: func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
			if req.Url != "https://example.com" {
				t.Errorf("url = %q", req.Url)
			}
			if userID != testUserID {
				t.Errorf("userID = %v, want %v", userID, testUserID)
			}
			return domain.Site{ID: uuid.New(), Url: req.Url, UserID: userID}, nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites", `{"url":"https://example.com","name":"Ex","check_interval_seconds":60}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	var got domain.Site
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got.Url != "https://example.com" {
		t.Errorf("url = %q", got.Url)
	}
}

func TestSiteHandler_Create_InvalidBody(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites", `{nope`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSiteHandler_Create_TokenGenerationFails(t *testing.T) {
	svc := &mockSiteService{
		createFn: func(ctx context.Context, req service.CreateSiteRequest, userID uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrTokenGeneration
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites", `{"url":"https://example.com"}`)
	// ErrTokenGeneration не в списке маппинга -> 500.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// --- Verify ---

func TestSiteHandler_Verify_Success(t *testing.T) {
	siteID := uuid.New()
	svc := &mockSiteService{
		verifyFn: func(ctx context.Context, id, userID uuid.UUID, token string) error {
			if id != siteID || userID != testUserID || token != "abc" {
				t.Errorf("неверные аргументы: id=%v userID=%v token=%q", id, userID, token)
			}
			return nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites/"+siteID.String()+"/verify", `{"token":"abc"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestSiteHandler_Verify_InvalidSiteID(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites/bad-id/verify", `{"token":"abc"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSiteHandler_Verify_AlreadyVerified(t *testing.T) {
	svc := &mockSiteService{
		verifyFn: func(ctx context.Context, id, userID uuid.UUID, token string) error {
			return domain.ErrSiteAlreadyVerified
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites/"+uuid.New().String()+"/verify", `{"token":"abc"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestSiteHandler_Verify_NotBelongUser(t *testing.T) {
	svc := &mockSiteService{
		verifyFn: func(ctx context.Context, id, userID uuid.UUID, token string) error {
			return domain.ErrSiteNotBelongUser
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites/"+uuid.New().String()+"/verify", `{"token":"abc"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// Verify с битым телом запроса -> 400.
func TestSiteHandler_Verify_InvalidBody(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodPost, "/sites/"+uuid.New().String()+"/verify", `{broken`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- GetAllSites ---

func TestSiteHandler_GetAllSites_Success(t *testing.T) {
	svc := &mockSiteService{
		getAllSitesFn: func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
			return []domain.Site{{ID: uuid.New()}}, nil
		},
	}
	h := siteRouter(svc, domain.RoleAdmin)

	rec := do(h, http.MethodGet, "/sites", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []domain.Site
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestSiteHandler_GetAllSites_AccessDenied(t *testing.T) {
	svc := &mockSiteService{
		getAllSitesFn: func(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
			return nil, domain.ErrAccessDenied
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- GetByUserID (mine) ---

func TestSiteHandler_GetByUserID_Success(t *testing.T) {
	svc := &mockSiteService{
		getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
			if userID != testUserID {
				t.Errorf("userID = %v, want %v", userID, testUserID)
			}
			return []domain.SiteResponse{{ID: uuid.New(), Url: "https://a.com"}}, nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/mine", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []domain.SiteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if len(got) != 1 || got[0].Url != "https://a.com" {
		t.Errorf("некорректный ответ: %+v", got)
	}
}

// Ошибка сервиса при получении своих сайтов -> 500.
func TestSiteHandler_GetByUserID_InternalError(t *testing.T) {
	svc := &mockSiteService{
		getByUserIDFn: func(ctx context.Context, userID uuid.UUID) ([]domain.SiteResponse, error) {
			return nil, domain.ErrSiteNotActive // домен. ошибка без явного маппинга -> 400
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/mine", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// --- GetByID ---

func TestSiteHandler_GetByID_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockSiteService{
		getByIDFn: func(ctx context.Context, got uuid.UUID) (domain.Site, error) {
			return domain.Site{ID: got}, nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/"+id.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestSiteHandler_GetByID_InvalidID(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/not-a-uuid", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSiteHandler_GetByID_NotFound(t *testing.T) {
	svc := &mockSiteService{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (domain.Site, error) {
			return domain.Site{}, domain.ErrNotFound
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/"+uuid.New().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- GetStats ---

func TestSiteHandler_GetStats_Success(t *testing.T) {
	svc := &mockSiteService{
		getStatsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return domain.SiteStats{TotalSites: 3, UpSites: 2, DownSites: 1}, nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got domain.SiteStats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got.TotalSites != 3 {
		t.Errorf("total = %d, want 3", got.TotalSites)
	}
}

// Ошибка сервиса при получении статистики -> 500.
func TestSiteHandler_GetStats_InternalError(t *testing.T) {
	svc := &mockSiteService{
		getStatsFn: func(ctx context.Context, userID uuid.UUID) (domain.SiteStats, error) {
			return domain.SiteStats{}, errBoom
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodGet, "/sites/stats", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// --- Update ---

func TestSiteHandler_Update_Success(t *testing.T) {
	id := uuid.New()
	name := "New"
	svc := &mockSiteService{
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			if update.ID != id || update.UserID != testUserID {
				t.Errorf("неверный update: %+v", update)
			}
			return domain.Site{ID: id, Name: name}, nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/sites/"+id.String(), `{"name":"New"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUpdate.Name == nil || *svc.lastUpdate.Name != name {
		t.Errorf("update.Name = %v, want %q", svc.lastUpdate.Name, name)
	}
}

func TestSiteHandler_Update_InvalidID(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/sites/bad", `{"name":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSiteHandler_Update_NotBelongUser(t *testing.T) {
	svc := &mockSiteService{
		updateFn: func(ctx context.Context, update domain.SiteUpdate) (domain.Site, error) {
			return domain.Site{}, domain.ErrSiteNotBelongUser
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/sites/"+uuid.New().String(), `{"name":"x"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// Update с битым телом -> 400.
func TestSiteHandler_Update_InvalidBody(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodPatch, "/sites/"+uuid.New().String(), `{broken`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- Delete ---

func TestSiteHandler_Delete_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockSiteService{
		deleteFn: func(ctx context.Context, got, userID uuid.UUID) error {
			if got != id || userID != testUserID {
				t.Errorf("неверные аргументы: id=%v userID=%v", got, userID)
			}
			return nil
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodDelete, "/sites/"+id.String(), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestSiteHandler_Delete_InvalidID(t *testing.T) {
	h := siteRouter(&mockSiteService{}, domain.RoleUser)

	rec := do(h, http.MethodDelete, "/sites/bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSiteHandler_Delete_NotFound(t *testing.T) {
	svc := &mockSiteService{
		deleteFn: func(ctx context.Context, id, userID uuid.UUID) error {
			return domain.ErrNotFound
		},
	}
	h := siteRouter(svc, domain.RoleUser)

	rec := do(h, http.MethodDelete, "/sites/"+uuid.New().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Защищённые маршруты сайтов без токена -> 401.
func TestSiteHandler_ProtectedRoutes_Unauthorized(t *testing.T) {
	_, mw := realAuthMiddleware()
	h := siteRouterWithMiddleware(&mockSiteService{}, mw)

	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/sites", `{"url":"https://x"}`},
		{http.MethodGet, "/sites", ""},
		{http.MethodGet, "/sites/mine", ""},
		{http.MethodGet, "/sites/stats", ""},
		{http.MethodDelete, "/sites/" + uuid.New().String(), ""},
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

// getUserID возвращает 401, если в контексте нет Claims. Здесь middleware
// пропускает запрос как есть (role == ""), эмулируя отсутствие пользователя,
// и сам handler обязан ответить 401 — это отдельная от middleware защита.
func TestSiteHandler_GetUserID_NoClaims(t *testing.T) {
	h := siteRouter(&mockSiteService{}, "") // claims не кладутся

	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/sites", `{"url":"https://x"}`},
		{http.MethodGet, "/sites", ""},
		{http.MethodGet, "/sites/mine", ""},
		{http.MethodGet, "/sites/stats", ""},
		{http.MethodPost, "/sites/" + uuid.New().String() + "/verify", `{"token":"t"}`},
		{http.MethodPatch, "/sites/" + uuid.New().String(), `{"name":"x"}`},
		{http.MethodDelete, "/sites/" + uuid.New().String(), ""},
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

// NewSiteHandler паникует на nil-сервисе (защита от неверной сборки приложения).
func TestNewSiteHandler_NilServicePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ожидалась паника при nil-сервисе")
		}
	}()
	NewSiteHandler(nil)
}
