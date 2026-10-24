package service

import (
	"fmt"
	"health_checker/internal/domain"
	"health_checker/internal/notifier"
	"health_checker/internal/repository/converters"
	"health_checker/internal/repository/postgres"
	"log"
	"time"
	"unicode/utf8"
)

// maxErrorMessageLen ограничивает длину текста ошибки, записываемого в БД,
// чтобы не раздувать строку непредвиденно длинным сообщением от http/dns.
const maxErrorMessageLen = 500

func (s *CheckerService) resultProcessor() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case result, ok := <-s.resultsChan:
			if !ok {
				return
			}
			s.processResult(result)
		}
	}
}

func (s *CheckerService) processResult(result CheckResult) {
	newStatus := statusFromResult(result)

	// Читаем сайт вместе с telegram_id владельца, чтобы:
	// 1) узнать предыдущий статус (для уведомления о смене);
	// 2) знать, кому отправлять алерт.
	site, err := s.repo.GetByIDWithOwner(s.ctx, result.SiteID)
	if err != nil {
		log.Printf("processResult: failed to load site %s: %v", result.SiteID, err)
		return
	}

	// 1. Логируем каждую проверку в check_logs_raw.
	s.saveLog(result)

	// 2. Обновляем статус сайта.
	prevStatus := site.Status
	if _, err := s.updateSiteStatus(site, result, newStatus); err != nil {
		log.Printf("processResult: failed to update status for site %s: %v", result.SiteID, err)
		return
	}

	// 3. Уведомляем владельца только при смене статуса.
	if prevStatus != newStatus {
		s.notifyStatusChange(site, prevStatus, newStatus, result)
	}
}

// statusFromResult применяет единое правило: сайт считается "up",
// только если запрос прошёл без ошибки и вернул 2xx. Всё остальное — "down".
func statusFromResult(result CheckResult) domain.SiteStatus {
	if result.Err == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
		return domain.StatusUp
	}
	return domain.StatusDown
}

func (s *CheckerService) saveLog(result CheckResult) {
	errMsg := ""
	if result.Err != nil {
		errMsg = truncate(result.Err.Error(), maxErrorMessageLen)
	}

	logEntry := domain.CheckLogsRaw{
		SiteID:       result.SiteID,
		StatusCode:   result.StatusCode,
		LatencyMs:    int(result.Latency.Milliseconds()),
		ErrorMessage: errMsg,
	}

	if err := s.checkRepo.InsertLog(s.ctx, logEntry); err != nil {
		log.Printf("processResult: failed to insert log for site %s: %v", result.SiteID, err)
	}
}

func (s *CheckerService) updateSiteStatus(
	site domain.Site,
	result CheckResult,
	newStatus domain.SiteStatus,
) (domain.Site, error) {
	now := time.Now()
	statusStr := string(newStatus)
	statusCode := int32(result.StatusCode)
	latencyMs := int32(result.Latency.Milliseconds())

	params := postgres.UpdateSiteStatusParams{
		Status:         &statusStr,
		LastStatusCode: &statusCode,
		LastCheckedAt:  converters.TimeToPgTimestamp(&now),
		ResponseTimeMs: &latencyMs,
		ID:             site.ID,
	}

	return s.repo.UpdateSiteStatus(s.ctx, params)
}

func (s *CheckerService) notifyStatusChange(
	site domain.Site,
	prevStatus, newStatus domain.SiteStatus,
	result CheckResult,
) {
	if site.OwnerTelegramID == 0 {
		return
	}

	text := buildStatusChangeText(site, prevStatus, newStatus, result)

	msg := notifier.Message{
		ChatID:    site.OwnerTelegramID,
		Text:      text,
		ParseMode: notifier.ParseModeHTML,
	}

	if err := s.sendStatusChangeAlert(s.ctx, msg); err != nil {
		log.Printf("processResult: failed to notify owner of site %s: %v", site.ID, err)
	}
}

func buildStatusChangeText(
	site domain.Site,
	prevStatus, newStatus domain.SiteStatus,
	result CheckResult,
) string {
	// Имя и URL сайта задаёт пользователь, а текст ошибки приходит от удалённого
	// сервера. Сообщение уходит с ParseMode=HTML, поэтому все эти значения
	// экранируются: иначе имя вида "<b>" сломает отправку или добавит разметку.
	name := notifier.EscapeHTML(site.Name)
	url := notifier.EscapeHTML(site.Url)

	switch newStatus {
	case domain.StatusDown:
		detail := fmt.Sprintf("HTTP %d", result.StatusCode)
		if result.Err != nil {
			// Сначала обрезаем, потом экранируем: иначе обрезка могла бы
			// разорвать HTML-сущность пополам.
			detail = notifier.EscapeHTML(truncate(result.Err.Error(), maxErrorMessageLen))
		}
		return fmt.Sprintf(
			"🔴 Сайт недоступен: %s (%s)\nСтатус: %s → %s\nОшибка: %s",
			name, url, prevStatus, newStatus, detail,
		)
	case domain.StatusUp:
		return fmt.Sprintf(
			"🟢 Сайт снова доступен: %s (%s)\nСтатус: %s → %s\nКод ответа: %d, время: %d мс",
			name, url, prevStatus, newStatus,
			result.StatusCode, result.Latency.Milliseconds(),
		)
	default:
		return fmt.Sprintf(
			"Статус сайта %s (%s) изменился: %s → %s",
			name, url, prevStatus, newStatus,
		)
	}
}

// truncate обрезает строку до max байт, не разрывая многобайтовый символ:
// разрезанная руна — невалидный UTF-8, который Telegram отвергает целиком.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
