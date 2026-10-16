// Тесты для чистых функций и построения клавиатур (keyboards.go).
package telegram

import (
	"testing"
)

// --- callbackData ---

func TestCallbackData(t *testing.T) {
	if got := callbackData("status", "abc-123"); got != "status:abc-123" {
		t.Errorf("callbackData = %q, want status:abc-123", got)
	}
}

// --- statusEmojiText ---

func TestStatusEmojiText(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"up", "🟢"},
		{"down", "🔴"},
		{"pending", "🟡"},
		{"maintenance", "⚪"},
		{"unknown", "⚪"},
		{"", "⚪"},
	}

	for _, tc := range cases {
		if got := statusEmojiText(tc.status); got != tc.want {
			t.Errorf("statusEmojiText(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// --- shorten ---

// Короткая строка возвращается без изменений.
func TestShorten_ShortString(t *testing.T) {
	if got := shorten("hello", 10); got != "hello" {
		t.Errorf("shorten = %q, want hello", got)
	}
}

// Строка ровно по границе не обрезается.
func TestShorten_ExactBoundary(t *testing.T) {
	if got := shorten("hello", 5); got != "hello" {
		t.Errorf("shorten на границе = %q, want hello", got)
	}
}

// Длинная строка обрезается до max рун с многоточием в конце.
func TestShorten_LongString(t *testing.T) {
	// "hello world" (11 рун), max=5 -> 4 руны + "…" = "hell…"
	got := shorten("hello world", 5)
	if got != "hell…" {
		t.Errorf("shorten = %q, want hell…", got)
	}
}

// Обрезка считает руны, а не байты: многобайтовые символы не должны ломаться.
func TestShorten_Unicode(t *testing.T) {
	// 6 рун (русские буквы), max=4 -> первые 3 руны + "…".
	got := shorten("привет", 4)
	if got != "при…" {
		t.Errorf("shorten юникода = %q, want при…", got)
	}
}

// max <= 1 -> обрезка без многоточия.
func TestShorten_MaxOneOrLess(t *testing.T) {
	if got := shorten("hello", 1); got != "h" {
		t.Errorf("shorten(max=1) = %q, want h", got)
	}
	if got := shorten("hello", 0); got != "" {
		t.Errorf("shorten(max=0) = %q, want empty", got)
	}
}

// --- siteListKeyboard ---

// Пустой список сайтов даёт пустую клавиатуру (без строк).
func TestSiteListKeyboard_Empty(t *testing.T) {
	kb := siteListKeyboard(nil)
	if len(kb.InlineKeyboard) != 0 {
		t.Errorf("для пустого списка ожидалась пустая клавиатура, got %d строк", len(kb.InlineKeyboard))
	}
}

// На каждый сайт создаётся строка с одной кнопкой и правильным callback data.
func TestSiteListKeyboard_WithSites(t *testing.T) {
	sites := []siteSummary{
		{ID: "id-1", Name: "First", Status: "up"},
		{ID: "id-2", Name: "Second", Status: "down"},
	}

	kb := siteListKeyboard(sites)

	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("строк = %d, want 2", len(kb.InlineKeyboard))
	}
	if got := kb.InlineKeyboard[0][0].CallbackData; got == nil || *got != "status:id-1" {
		t.Errorf("callback первой кнопки = %v, want status:id-1", got)
	}
	if got := kb.InlineKeyboard[1][0].CallbackData; got == nil || *got != "status:id-2" {
		t.Errorf("callback второй кнопки = %v, want status:id-2", got)
	}
}

// --- siteDetailKeyboard ---

// Для активного сайта кнопка предлагает «Остановить».
func TestSiteDetailKeyboard_Active(t *testing.T) {
	kb := siteDetailKeyboard("site-1", true)

	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("строк = %d, want 2", len(kb.InlineKeyboard))
	}
	toggle := kb.InlineKeyboard[0][0]
	if toggle.Text != "⏸ Остановить" {
		t.Errorf("текст кнопки = %q, want «⏸ Остановить»", toggle.Text)
	}
	if toggle.CallbackData == nil || *toggle.CallbackData != "toggle:site-1" {
		t.Errorf("callback = %v, want toggle:site-1", toggle.CallbackData)
	}
}

// Для неактивного сайта кнопка предлагает «Запустить».
func TestSiteDetailKeyboard_Inactive(t *testing.T) {
	kb := siteDetailKeyboard("site-1", false)

	toggle := kb.InlineKeyboard[0][0]
	if toggle.Text != "▶️ Запустить" {
		t.Errorf("текст кнопки = %q, want «▶️ Запустить»", toggle.Text)
	}
}

// --- confirmRemoveKeyboard ---

// Клавиатура подтверждения содержит кнопку удаления (confirm) и отмены (status).
func TestConfirmRemoveKeyboard(t *testing.T) {
	kb := confirmRemoveKeyboard("site-9")

	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("ожидалась одна строка с двумя кнопками, got %+v", kb.InlineKeyboard)
	}
	if got := kb.InlineKeyboard[0][0].CallbackData; got == nil || *got != "confirm:site-9" {
		t.Errorf("кнопка подтверждения = %v, want confirm:site-9", got)
	}
	if got := kb.InlineKeyboard[0][1].CallbackData; got == nil || *got != "status:site-9" {
		t.Errorf("кнопка отмены = %v, want status:site-9", got)
	}
}
