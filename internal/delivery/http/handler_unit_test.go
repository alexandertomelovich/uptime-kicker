package httphandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"health_checker/internal/domain"
)

// --- mapServiceError: domain-ошибка -> HTTP-статус ---

// Маппинг доменных ошибок в HTTP-статусы — ключевая логика этого слоя.
// Проверяем каждую пару «ошибка -> статус» таблично.
func TestMapServiceError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", domain.ErrNotFound, http.StatusNotFound},
		{"unauthorized", domain.ErrUnauthorized, http.StatusUnauthorized},
		{"invalid credentials", domain.ErrInvalidCredentials, http.StatusUnauthorized},
		{"access denied", domain.ErrAccessDenied, http.StatusForbidden},
		{"role change forbidden", domain.ErrRoleChangeForbidden, http.StatusForbidden},
		{"site not belong user", domain.ErrSiteNotBelongUser, http.StatusForbidden},
		{"email exists", domain.ErrEmailAlreadyExists, http.StatusBadRequest},
		{"telegram exists", domain.ErrTelegramAlreadyExists, http.StatusBadRequest},
		{"password policy", domain.ErrPasswordPolicy, http.StatusBadRequest},
		{"invalid role", domain.ErrInvalidRole, http.StatusBadRequest},
		{"site not active", domain.ErrSiteNotActive, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := mapServiceError(tc.err)
			if status != tc.wantStatus {
				t.Errorf("status = %d, want %d", status, tc.wantStatus)
			}
		})
	}
}

// Обёрнутая доменная ошибка тоже должна распознаваться (errors.Is).
func TestMapServiceError_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("service.GetByID: %w", domain.ErrNotFound)
	status, _ := mapServiceError(wrapped)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

// Неизвестная ошибка -> 500 и без утечки оригинала в тело ответа.
func TestMapServiceError_Unknown(t *testing.T) {
	status, msg := mapServiceError(errBoom)
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
	if strings.Contains(msg, errBoom.Error()) {
		t.Errorf("текст ответа не должен раскрывать внутреннюю ошибку: %q", msg)
	}
}

// --- respondJSON / respondError ---

func TestRespondJSON_ContentTypeAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	respondJSON(rec, http.StatusCreated, map[string]string{"hello": "world"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("body = %v, want hello=world", got)
	}
}

// nil-payload пишет только статус без тела.
func TestRespondJSON_NilPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	respondJSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body должен быть пустым, got %q", rec.Body.String())
	}
}

func TestRespondError_Format(t *testing.T) {
	rec := httptest.NewRecorder()
	respondError(rec, http.StatusBadRequest, "bad thing")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var got APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v", err)
	}
	if got.Error != "bad thing" {
		t.Errorf("error = %q, want %q", got.Error, "bad thing")
	}
}

// --- decodeJSON ---

type samplePayload struct {
	A string `json:"a"`
	B int    `json:"b"`
}

func TestDecodeJSON_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"x","b":7}`))

	var dst samplePayload
	if err := decodeJSON(rec, req, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.A != "x" || dst.B != 7 {
		t.Errorf("decoded = %+v, want {x 7}", dst)
	}
}

// Неизвестные поля запрещены (DisallowUnknownFields).
func TestDecodeJSON_UnknownField(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"x","c":true}`))

	var dst samplePayload
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatal("ожидалась ошибка из-за неизвестного поля")
	}
}

// Битый JSON -> ошибка.
func TestDecodeJSON_Malformed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":`))

	var dst samplePayload
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatal("ожидалась ошибка для битого JSON")
	}
}

// Тело больше 1 МиБ должно отклоняться (защита MaxBytesReader).
func TestDecodeJSON_TooLarge(t *testing.T) {
	big := `{"a":"` + strings.Repeat("x", 1<<20+10) + `"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))

	var dst samplePayload
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatal("ожидалась ошибка для слишком большого тела")
	}
}

// Проверяем, что ErrAccessDenied (обёрнутый) не превращается в 500.
func TestMapServiceError_WrappedAccessDenied(t *testing.T) {
	status, _ := mapServiceError(fmt.Errorf("x: %w", domain.ErrAccessDenied))
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", status)
	}
}
