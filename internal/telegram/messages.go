package telegram

import (
	"fmt"
	"strings"

	"health_checker/internal/domain"
)

// statusEmoji возвращает эмодзи для статуса сайта.
func statusEmoji(s domain.SiteStatus) string {
	switch s {
	case domain.StatusUp:
		return "🟢"
	case domain.StatusDown:
		return "🔴"
	case domain.StatusPending:
		return "🟡"
	case domain.StatusMaintenance:
		return "🟠"
	default:
		return "⚪"
	}
}

// formatSiteLine форматирует одну строку со сайтом для списка.
func formatSiteLine(s domain.SiteResponse) string {
	line := fmt.Sprintf("%s %s — %s", statusEmoji(s.Status), s.Name, s.Url)
	if !s.IsActive {
		line += " (неактивен)"
	}
	return line
}

// formatSiteList формирует текстовый список сайтов пользователя.
func formatSiteList(sites []domain.SiteResponse) string {
	if len(sites) == 0 {
		return "У вас пока нет сайтов.\n\nДобавьте первый: /add <url>"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📋 Ваши сайты (%d):\n\n", len(sites))
	for i, s := range sites {
		fmt.Fprintf(&b, "%d. %s\n", i+1, formatSiteLine(s))
	}
	b.WriteString("\nПодробнее о сайте: /status <id>")
	return b.String()
}

// formatSiteDetail формирует детальную карточку сайта.
func formatSiteDetail(s domain.Site) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", statusEmoji(s.Status), s.Name)
	fmt.Fprintf(&b, "URL: %s\n", s.Url)
	fmt.Fprintf(&b, "Статус: %s\n", s.Status)
	fmt.Fprintf(&b, "Интервал проверки: %d сек\n", s.CheckIntervalSeconds)
	fmt.Fprintf(&b, "Активен: %t\n", s.IsActive)
	fmt.Fprintf(&b, "ID: %s\n", s.ID)

	if s.LastCheckedAt != nil {
		fmt.Fprintf(&b, "Последняя проверка: %s\n", s.LastCheckedAt.Format("2006-01-02 15:04:05"))
	}
	if s.LastStatusCode != nil {
		fmt.Fprintf(&b, "Код ответа: %d\n", *s.LastStatusCode)
	}
	if s.ResponseTimeMs != nil {
		fmt.Fprintf(&b, "Время ответа: %d мс\n", *s.ResponseTimeMs)
	}
	if !s.IsActive && s.VerifiedAt == nil {
		b.WriteString("\n⚠️ Сайт не подтверждён. Используйте /verify <id> <token>")
	}
	return b.String()
}

// formatStats формирует карточку статистики.
func formatStats(s domain.SiteStats) string {
	return fmt.Sprintf(
		"📊 Статистика\n\n"+
			"Всего сайтов: %d\n"+
			"🟢 UP: %d\n"+
			"🔴 DOWN: %d\n"+
			"🟡 PENDING: %d\n"+
			"⏱ Среднее время ответа: %.0f мс",
		s.TotalSites, s.UpSites, s.DownSites, s.PendingSites, s.AvgResponseTime,
	)
}

const helpText = `🤖 Uptime Kicker — бот мониторинга сайтов.

Доступные команды:
/start — приветствие
/add <url> [name] — добавить сайт
/list — список ваших сайтов
/status <id> — подробная информация о сайте
/remove <id> — удалить сайт
/verify <id> <token> — подтвердить владение сайтом
/stats — статистика
/help — эта справка

Инлайн-кнопки доступны в ответах на /list и /status.`

const welcomeText = `👋 Добро пожаловать в Uptime Kicker!

Я слежу за доступностью ваших сайтов и пришлю уведомление, если что-то упадёт.

Добавьте первый сайт командой:
/add https://example.com
`
