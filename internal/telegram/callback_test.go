// Тесты для чистых функций (callback.go, handler.go): разбор callback data и
// преобразование доменных ошибок в понятный пользователю текст.
package telegram

import (
	"testing"

	"health_checker/internal/domain"
)

// --- parseCallbackData ---

func TestParseCallbackData(t *testing.T) {
	cases := []struct {
		name       string
		data       string
		wantAction string
		wantSiteID string
		wantOK     bool
	}{
		{"simple", "status:abc", "status", "abc", true},
		{"uuid", "toggle:550e8400-e29b-41d4-a716-446655440000", "toggle", "550e8400-e29b-41d4-a716-446655440000", true},
		{"empty string", "", "", "", false},
		{"no separator", "statusabc", "", "", false},
		{"empty action", ":abc", "", "", false},
		{"empty site id", "status:", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, siteID, ok := parseCallbackData(tc.data)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if action != tc.wantAction {
				t.Errorf("action = %q, want %q", action, tc.wantAction)
			}
			if siteID != tc.wantSiteID {
				t.Errorf("siteID = %q, want %q", siteID, tc.wantSiteID)
			}
		})
	}
}

// Разделитель «:» в самом ID (если появится позже) не должен ломать разбор:
// SplitN(…, 2) сохраняет остаток как siteID.
func TestParseCallbackData_SiteIDWithColon(t *testing.T) {
	action, siteID, ok := parseCallbackData("status:a:b")
	if !ok {
		t.Fatal("ожидался успешный разбор")
	}
	if action != "status" || siteID != "a:b" {
		t.Errorf("получено action=%q siteID=%q", action, siteID)
	}
}

// --- humanizeError ---

func TestHumanizeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"not belong user", domain.ErrSiteNotBelongUser, "сайт вам не принадлежит"},
		{"not found", domain.ErrNotFound, "не найдено"},
		{"unknown", domain.ErrAccessDenied, "внутренняя ошибка"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanizeError(tc.err); got != tc.want {
				t.Errorf("humanizeError = %q, want %q", got, tc.want)
			}
		})
	}
}
