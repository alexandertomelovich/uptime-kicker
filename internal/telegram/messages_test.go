// Тесты для чистых функций форматирования сообщений (messages.go).
// Все функции не имеют побочных эффектов, поэтому проверяются таблично.
package telegram

import (
	"strings"
	"testing"
	"time"

	"health_checker/internal/domain"

	"github.com/google/uuid"
)

// --- statusEmoji ---

func TestStatusEmoji(t *testing.T) {
	cases := []struct {
		name   string
		status domain.SiteStatus
		want   string
	}{
		{"up", domain.StatusUp, "🟢"},
		{"down", domain.StatusDown, "🔴"},
		{"pending", domain.StatusPending, "🟡"},
		{"maintenance", domain.StatusMaintenance, "🟠"},
		{"unknown", domain.SiteStatus("weird"), "⚪"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusEmoji(tc.status); got != tc.want {
				t.Errorf("statusEmoji(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

// --- formatSiteLine ---

// Активный сайт не должен получать пометку "(неактивен)".
func TestFormatSiteLine_Active(t *testing.T) {
	line := formatSiteLine(domain.SiteResponse{
		Name:     "Example",
		Url:      "https://example.com",
		Status:   domain.StatusUp,
		IsActive: true,
	})

	if !strings.Contains(line, "Example") || !strings.Contains(line, "https://example.com") {
		t.Errorf("строка не содержит имя/URL: %q", line)
	}
	if strings.Contains(line, "неактивен") {
		t.Errorf("активный сайт не должен помечаться неактивным: %q", line)
	}
}

// Неактивный сайт получает пометку "(неактивен)".
func TestFormatSiteLine_Inactive(t *testing.T) {
	line := formatSiteLine(domain.SiteResponse{
		Name:     "Example",
		Url:      "https://example.com",
		Status:   domain.StatusDown,
		IsActive: false,
	})

	if !strings.Contains(line, "(неактивен)") {
		t.Errorf("неактивный сайт должен помечаться: %q", line)
	}
}

// --- formatSiteList ---

// Пустой список -> понятная подсказка, как добавить первый сайт.
func TestFormatSiteList_Empty(t *testing.T) {
	got := formatSiteList(nil)
	if !strings.Contains(got, "/add") {
		t.Errorf("подсказка для пустого списка должна упоминать /add: %q", got)
	}
}

// В списке есть счётчик сайтов и нумерованные строки.
func TestFormatSiteList_WithSites(t *testing.T) {
	sites := []domain.SiteResponse{
		{Name: "First", Url: "https://a.com", Status: domain.StatusUp, IsActive: true},
		{Name: "Second", Url: "https://b.com", Status: domain.StatusDown, IsActive: true},
	}

	got := formatSiteList(sites)

	if !strings.Contains(got, "(2)") {
		t.Errorf("ожидался счётчик сайтов (2): %q", got)
	}
	if !strings.Contains(got, "1.") || !strings.Contains(got, "2.") {
		t.Errorf("строки должны быть пронумерованы: %q", got)
	}
	if !strings.Contains(got, "First") || !strings.Contains(got, "Second") {
		t.Errorf("в списке должны быть имена сайтов: %q", got)
	}
}

// --- formatSiteDetail ---

// Базовая карточка без опциональных полей: счётчик проверок, URL и ID на месте,
// опциональные строки отсутствуют.
func TestFormatSiteDetail_Minimal(t *testing.T) {
	id := uuid.New()
	site := domain.SiteResponse{
		ID:                   id,
		Url:                  "https://example.com",
		Name:                 "Example",
		Status:               domain.StatusUp,
		CheckIntervalSeconds: 60,
		IsActive:             true,
	}

	got := formatSiteDetail(site)

	if !strings.Contains(got, "https://example.com") {
		t.Errorf("нет URL: %q", got)
	}
	if !strings.Contains(got, id.String()) {
		t.Errorf("нет ID: %q", got)
	}
	if !strings.Contains(got, "60 сек") {
		t.Errorf("нет интервала проверки: %q", got)
	}
	// Опциональные поля не заданы -> соответствующих строк быть не должно.
	if strings.Contains(got, "Код ответа") {
		t.Errorf("не должно быть кода ответа: %q", got)
	}
	if strings.Contains(got, "Последняя проверка") {
		t.Errorf("не должно быть последней проверки: %q", got)
	}
}

// При заданных опциональных полях они попадают в карточку.
func TestFormatSiteDetail_WithOptionalFields(t *testing.T) {
	checked := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	code := int32(503)
	rt := int32(150)
	site := domain.SiteResponse{
		ID:             uuid.New(),
		Url:            "https://example.com",
		Name:           "Example",
		Status:         domain.StatusDown,
		IsActive:       true,
		LastCheckedAt:  &checked,
		LastStatusCode: &code,
		ResponseTimeMs: &rt,
	}

	got := formatSiteDetail(site)

	if !strings.Contains(got, "2026-01-02 15:04:05") {
		t.Errorf("нет времени последней проверки: %q", got)
	}
	if !strings.Contains(got, "Код ответа: 503") {
		t.Errorf("нет кода ответа: %q", got)
	}
	if !strings.Contains(got, "Время ответа: 150 мс") {
		t.Errorf("нет времени ответа: %q", got)
	}
}

// --- formatStats ---

func TestFormatStats(t *testing.T) {
	got := formatStats(domain.SiteStats{
		TotalSites:      5,
		UpSites:         3,
		DownSites:       1,
		PendingSites:    1,
		AvgResponseTime: 123.7,
	})

	if !strings.Contains(got, "Всего сайтов: 5") {
		t.Errorf("нет общего числа сайтов: %q", got)
	}
	if !strings.Contains(got, "UP: 3") {
		t.Errorf("нет UP: %q", got)
	}
	if !strings.Contains(got, "DOWN: 1") {
		t.Errorf("нет DOWN: %q", got)
	}
	// %.0f округляет 123.7 до 124.
	if !strings.Contains(got, "124 мс") {
		t.Errorf("среднее время должно округляться: %q", got)
	}
}

// --- Экранирование пользовательских данных (сообщения уходят с ParseMode=HTML) ---

// Имя и URL сайта задаёт пользователь, поэтому в тексте сообщения они должны
// быть экранированы, а не попадать в HTML-разметку как есть.
const (
	injectionName = `<b>hack</b>`
	injectionURL  = `https://x/?q=<script>&y="1"`
)

// Строка списка не содержит «сырых» тегов, но содержит их экранированные версии.
func TestFormatSiteLine_EscapesUserData(t *testing.T) {
	line := formatSiteLine(domain.SiteResponse{
		Name:     injectionName,
		Url:      injectionURL,
		Status:   domain.StatusUp,
		IsActive: true,
	})

	if strings.Contains(line, "<b>") || strings.Contains(line, "<script>") {
		t.Errorf("в строке осталась неэкранированная разметка: %q", line)
	}
	if !strings.Contains(line, "&lt;b&gt;hack&lt;/b&gt;") {
		t.Errorf("имя должно быть экранировано: %q", line)
	}
	if !strings.Contains(line, "&lt;script&gt;&amp;y=") {
		t.Errorf("URL должен быть экранирован: %q", line)
	}
}

// Список сайтов экранирует имена и URL каждой строки.
func TestFormatSiteList_EscapesUserData(t *testing.T) {
	got := formatSiteList([]domain.SiteResponse{
		{Name: injectionName, Url: injectionURL, Status: domain.StatusUp, IsActive: true},
	})

	if strings.Contains(got, "<b>") || strings.Contains(got, "<script>") {
		t.Errorf("в списке осталась неэкранированная разметка: %q", got)
	}
	if !strings.Contains(got, "&lt;b&gt;hack&lt;/b&gt;") {
		t.Errorf("имя в списке должно быть экранировано: %q", got)
	}
}

// Детальная карточка экранирует имя и URL, но не ломает остальные поля.
func TestFormatSiteDetail_EscapesUserData(t *testing.T) {
	site := domain.SiteResponse{
		ID:     uuid.New(),
		Name:   injectionName,
		Url:    injectionURL,
		Status: domain.StatusUp,
	}

	got := formatSiteDetail(site)

	if strings.Contains(got, "<b>") || strings.Contains(got, "<script>") {
		t.Errorf("в карточке осталась неэкранированная разметка: %q", got)
	}
	if !strings.Contains(got, "&lt;b&gt;hack&lt;/b&gt;") {
		t.Errorf("имя должно быть экранировано: %q", got)
	}
	if !strings.Contains(got, "URL: https://x/?q=&lt;script&gt;&amp;y=") {
		t.Errorf("URL должен быть экранирован: %q", got)
	}
	if !strings.Contains(got, site.ID.String()) {
		t.Errorf("ID должен остаться в карточке: %q", got)
	}
}

// Любой текст, уходящий с ParseMode=HTML, не должен содержать «сырых» угловых
// скобок: Telegram парсит их как разметку, а неизвестный тег — это отказ
// отправки целиком. Проверяем и статические подсказки, и функции форматирования.
func TestMessageTexts_HaveNoRawMarkup(t *testing.T) {
	site := domain.SiteResponse{ID: uuid.New(), Name: injectionName, Url: injectionURL, Status: domain.StatusUp}
	inactive := site
	inactive.IsActive = false

	texts := []struct {
		name string
		text string
	}{
		{"welcomeText", welcomeText},
		{"helpText", helpText},
		{"пустой список", formatSiteList(nil)},
		{"список", formatSiteList([]domain.SiteResponse{{Name: injectionName, Url: injectionURL, Status: domain.StatusUp, IsActive: true}})},
		{"карточка", formatSiteDetail(site)},
		{"неактивный сайт", formatSiteDetail(inactive)},
		{"статистика", formatStats(domain.SiteStats{TotalSites: 1})},
	}

	for _, tc := range texts {
		t.Run(tc.name, func(t *testing.T) {
			if strings.ContainsAny(tc.text, "<>") {
				t.Errorf("в тексте остались сырые угловые скобки: %q", tc.text)
			}
		})
	}
}
