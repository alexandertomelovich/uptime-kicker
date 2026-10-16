// Тесты на обработку результатов проверки (processor.go): определение статуса,
// запись лога, обновление статуса сайта и уведомление владельца при смене статуса.
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"health_checker/internal/domain"
	"health_checker/internal/notifier"

	"github.com/google/uuid"
)

// --- statusFromResult: чистая функция ---

// Единое правило: "up" только при отсутствии ошибки и коде 2xx, иначе "down".
func TestStatusFromResult(t *testing.T) {
	cases := []struct {
		name   string
		result CheckResult
		want   domain.SiteStatus
	}{
		{"200 ok", CheckResult{StatusCode: 200}, domain.StatusUp},
		{"201 created", CheckResult{StatusCode: 201}, domain.StatusUp},
		{"299 boundary", CheckResult{StatusCode: 299}, domain.StatusUp},
		{"300 redirect is down", CheckResult{StatusCode: 300}, domain.StatusDown},
		{"199 boundary is down", CheckResult{StatusCode: 199}, domain.StatusDown},
		{"404 is down", CheckResult{StatusCode: 404}, domain.StatusDown},
		{"500 is down", CheckResult{StatusCode: 500}, domain.StatusDown},
		{"network error is down even with 200", CheckResult{StatusCode: 200, Err: errors.New("dial")}, domain.StatusDown},
		{"zero code is down", CheckResult{}, domain.StatusDown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusFromResult(tc.result); got != tc.want {
				t.Errorf("statusFromResult = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- truncate ---

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate короткой строки = %q, want hello", got)
	}
	if got := truncate("hello", 5); got != "hello" {
		t.Errorf("truncate на границе = %q, want hello", got)
	}
	if got := truncate("hello world", 5); got != "hello" {
		t.Errorf("truncate длинной строки = %q, want hello", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate пустой = %q, want empty", got)
	}
}

// --- buildStatusChangeText ---

func TestBuildStatusChangeText_Down_UsesHTTPCode(t *testing.T) {
	site := domain.Site{Name: "Site", Url: "https://x"}
	text := buildStatusChangeText(site, domain.StatusUp, domain.StatusDown, CheckResult{StatusCode: 503})

	if !strings.Contains(text, "недоступен") {
		t.Errorf("текст не содержит 'недоступен': %q", text)
	}
	if !strings.Contains(text, "HTTP 503") {
		t.Errorf("текст не содержит HTTP-код: %q", text)
	}
	if !strings.Contains(text, "up → down") {
		t.Errorf("текст не содержит перехода статусов: %q", text)
	}
}

// При ошибке сети в тексте должен быть текст ошибки, а не HTTP-код.
func TestBuildStatusChangeText_Down_UsesError(t *testing.T) {
	site := domain.Site{Name: "Site", Url: "https://x"}
	text := buildStatusChangeText(site, domain.StatusUp, domain.StatusDown, CheckResult{Err: errors.New("dial tcp: timeout")})

	if !strings.Contains(text, "dial tcp: timeout") {
		t.Errorf("текст не содержит ошибку: %q", text)
	}
}

// Длинная ошибка в тексте алерта обрезается до maxErrorMessageLen.
func TestBuildStatusChangeText_Down_TruncatesError(t *testing.T) {
	site := domain.Site{Name: "Site", Url: "https://x"}
	long := strings.Repeat("e", maxErrorMessageLen+50)
	text := buildStatusChangeText(site, domain.StatusUp, domain.StatusDown, CheckResult{Err: errors.New(long)})

	if strings.Contains(text, strings.Repeat("e", maxErrorMessageLen+1)) {
		t.Error("ошибка в тексте не была обрезана")
	}
}

func TestBuildStatusChangeText_Up(t *testing.T) {
	site := domain.Site{Name: "Site", Url: "https://x"}
	text := buildStatusChangeText(site, domain.StatusDown, domain.StatusUp, CheckResult{
		StatusCode: 200,
		Latency:    150 * time.Millisecond,
	})

	if !strings.Contains(text, "снова доступен") {
		t.Errorf("текст не содержит 'снова доступен': %q", text)
	}
	if !strings.Contains(text, "Код ответа: 200") {
		t.Errorf("текст не содержит код ответа: %q", text)
	}
	if !strings.Contains(text, "время: 150 мс") {
		t.Errorf("текст не содержит время: %q", text)
	}
}

func TestBuildStatusChangeText_Default(t *testing.T) {
	site := domain.Site{Name: "Site", Url: "https://x"}
	text := buildStatusChangeText(site, domain.StatusUp, domain.StatusMaintenance, CheckResult{})

	if !strings.Contains(text, "изменился") {
		t.Errorf("ожидался общий текст смены статуса: %q", text)
	}
}

// --- processResult: интеграция обработки одного результата ---

// Полный успешный путь: сайт найден, лог записан, статус обновлён, а при смене
// статуса владельцу ушёл алерт.
func TestProcessResult_StatusChange_NotifiesOwner(t *testing.T) {
	siteID := uuid.New()
	site := domain.Site{ID: siteID, Url: "https://x", Status: domain.StatusDown, OwnerTelegramID: 42}

	repo := newMockSiteRepo(site)
	checkRepo := &mockCheckRepo{}
	sender := &capturingSender{}
	svc := NewCheckerService(repo, checkRepo, sender, 1, 10, 10)

	svc.processResult(CheckResult{SiteID: siteID, StatusCode: 200, Latency: 10 * time.Millisecond})

	if got := checkRepo.count(); got != 1 {
		t.Errorf("inserts = %d, want 1", got)
	}
	if got := repo.statusOf(siteID); got != domain.StatusUp {
		t.Errorf("status = %q, want up", got)
	}
	if got := sender.count(); got != 1 {
		t.Fatalf("уведомлений = %d, want 1", got)
	}
	if sender.messages[0].ChatID != 42 {
		t.Errorf("chatID = %d, want 42", sender.messages[0].ChatID)
	}
}

// Если статус не изменился — уведомление не отправляется, но лог и обновление тех
// же статусов всё равно выполняются.
func TestProcessResult_NoStatusChange_NoNotification(t *testing.T) {
	siteID := uuid.New()
	site := domain.Site{ID: siteID, Url: "https://x", Status: domain.StatusUp, OwnerTelegramID: 42}

	repo := newMockSiteRepo(site)
	checkRepo := &mockCheckRepo{}
	sender := &capturingSender{}
	svc := NewCheckerService(repo, checkRepo, sender, 1, 10, 10)

	svc.processResult(CheckResult{SiteID: siteID, StatusCode: 200})

	if got := sender.count(); got != 0 {
		t.Errorf("уведомлений = %d, want 0", got)
	}
	if got := checkRepo.count(); got != 1 {
		t.Errorf("inserts = %d, want 1", got)
	}
}

// Если у сайта нет telegram-владельца (ID == 0), уведомление не шлётся даже при
// смене статуса.
func TestProcessResult_NoOwner_NoNotification(t *testing.T) {
	siteID := uuid.New()
	site := domain.Site{ID: siteID, Url: "https://x", Status: domain.StatusUp, OwnerTelegramID: 0}

	sender := &capturingSender{}
	svc := NewCheckerService(newMockSiteRepo(site), &mockCheckRepo{}, sender, 1, 10, 10)

	svc.processResult(CheckResult{SiteID: siteID, StatusCode: 500}) // up -> down

	if got := sender.count(); got != 0 {
		t.Errorf("уведомлений = %d, want 0 (нет владельца)", got)
	}
}

// Сайт не найден при загрузке -> обработка прерывается, лог не пишется.
func TestProcessResult_SiteLoadError(t *testing.T) {
	checkRepo := &mockCheckRepo{}
	repo := newMockSiteRepo() // пусто -> GetByIDWithOwner вернёт ErrNotFound
	svc := NewCheckerService(repo, checkRepo, &capturingSender{}, 1, 10, 10)

	svc.processResult(CheckResult{SiteID: uuid.New(), StatusCode: 200})

	if got := checkRepo.count(); got != 0 {
		t.Errorf("inserts = %d, want 0 (сайт не найден)", got)
	}
}

// Ошибка отправки алерта не должна ломать обработку (просто логируется).
func TestProcessResult_NotifyError_DoesNotPanic(t *testing.T) {
	siteID := uuid.New()
	site := domain.Site{ID: siteID, Url: "https://x", Status: domain.StatusUp, OwnerTelegramID: 7}

	sender := &capturingSender{err: errors.New("telegram down")}
	svc := NewCheckerService(newMockSiteRepo(site), &mockCheckRepo{}, sender, 1, 10, 10)

	// Статус меняется up -> down, алерт упадёт — но паники быть не должно.
	svc.processResult(CheckResult{SiteID: siteID, StatusCode: 500})
}

// --- saveLog: запись лога проверки ---

// Успешная проверка: лог без текста ошибки, latency в миллисекундах.
func TestSaveLog_Success(t *testing.T) {
	siteID := uuid.New()
	checkRepo := &capturingCheckRepo{}
	svc := NewCheckerService(newMockSiteRepo(), checkRepo, &capturingSender{}, 1, 10, 10)

	svc.saveLog(CheckResult{SiteID: siteID, StatusCode: 200, Latency: 123 * time.Millisecond})

	if len(checkRepo.logs) != 1 {
		t.Fatalf("логов = %d, want 1", len(checkRepo.logs))
	}
	got := checkRepo.logs[0]
	if got.SiteID != siteID {
		t.Errorf("siteID = %v, want %v", got.SiteID, siteID)
	}
	if got.StatusCode != 200 {
		t.Errorf("statusCode = %d, want 200", got.StatusCode)
	}
	if got.LatencyMs != 123 {
		t.Errorf("latencyMs = %d, want 123", got.LatencyMs)
	}
	if got.ErrorMessage != "" {
		t.Errorf("errorMessage = %q, want empty", got.ErrorMessage)
	}
}

// Проверка с ошибкой: текст ошибки попадает в лог.
func TestSaveLog_WithError(t *testing.T) {
	checkRepo := &capturingCheckRepo{}
	svc := NewCheckerService(newMockSiteRepo(), checkRepo, &capturingSender{}, 1, 10, 10)

	svc.saveLog(CheckResult{SiteID: uuid.New(), Err: errors.New("connection refused")})

	if len(checkRepo.logs) != 1 {
		t.Fatalf("логов = %d, want 1", len(checkRepo.logs))
	}
	if checkRepo.logs[0].ErrorMessage != "connection refused" {
		t.Errorf("errorMessage = %q", checkRepo.logs[0].ErrorMessage)
	}
}

// Слишком длинная ошибка в логе обрезается до maxErrorMessageLen.
func TestSaveLog_TruncatesLongError(t *testing.T) {
	checkRepo := &capturingCheckRepo{}
	svc := NewCheckerService(newMockSiteRepo(), checkRepo, &capturingSender{}, 1, 10, 10)

	long := strings.Repeat("x", maxErrorMessageLen+100)
	svc.saveLog(CheckResult{SiteID: uuid.New(), Err: errors.New(long)})

	if len(checkRepo.logs[0].ErrorMessage) != maxErrorMessageLen {
		t.Errorf("len(errorMessage) = %d, want %d", len(checkRepo.logs[0].ErrorMessage), maxErrorMessageLen)
	}
}

// Ошибка вставки лога не прерывает процесс (только логируется).
func TestSaveLog_InsertError_DoesNotPanic(t *testing.T) {
	checkRepo := &capturingCheckRepo{err: errors.New("db down")}
	svc := NewCheckerService(newMockSiteRepo(), checkRepo, &capturingSender{}, 1, 10, 10)

	svc.saveLog(CheckResult{SiteID: uuid.New(), StatusCode: 200})
}

// --- updateSiteStatus: сборка параметров для БД ---

// Проверяем, что сервис правильно маппит результат в UpdateSiteStatusParams.
func TestUpdateSiteStatus_MapsParams(t *testing.T) {
	siteID := uuid.New()
	site := domain.Site{ID: siteID}
	repo := newMockSiteRepo(site)
	svc := NewCheckerService(repo, &mockCheckRepo{}, &capturingSender{}, 1, 10, 10)

	result := CheckResult{SiteID: siteID, StatusCode: 503, Latency: 80 * time.Millisecond}

	_, err := svc.updateSiteStatus(site, result, domain.StatusDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := repo.statusOf(siteID); got != domain.StatusDown {
		t.Errorf("status в репозитории = %q, want down", got)
	}
	p := repo.lastParams
	if p.Status == nil || *p.Status != string(domain.StatusDown) {
		t.Errorf("params.Status = %v, want down", p.Status)
	}
	if p.LastStatusCode == nil || *p.LastStatusCode != 503 {
		t.Errorf("params.LastStatusCode = %v, want 503", p.LastStatusCode)
	}
	if p.ResponseTimeMs == nil || *p.ResponseTimeMs != 80 {
		t.Errorf("params.ResponseTimeMs = %v, want 80", p.ResponseTimeMs)
	}
	if !p.LastCheckedAt.Valid {
		t.Error("params.LastCheckedAt должен быть валидным")
	}
}

// --- sendStatusChangeAlert ---

func TestSendStatusChangeAlert_Error(t *testing.T) {
	sender := &capturingSender{err: errors.New("boom")}
	svc := NewCheckerService(newMockSiteRepo(), &mockCheckRepo{}, sender, 1, 10, 10)

	err := svc.sendStatusChangeAlert(context.Background(), notifier.Message{ChatID: 1, Text: "x"})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "sendStatusChangeAlert") {
		t.Errorf("ошибка должна быть обёрнута: %v", err)
	}
}

// Проверяем также, что updateSiteStatus прокидывает ошибку репозитория.
func TestUpdateSiteStatus_RepoError(t *testing.T) {
	site := domain.Site{ID: uuid.New()}
	repo := newMockSiteRepo()
	repo.updateErr = errors.New("update failed")
	svc := NewCheckerService(repo, &mockCheckRepo{}, &capturingSender{}, 1, 10, 10)

	_, err := svc.updateSiteStatus(site, CheckResult{StatusCode: 200}, domain.StatusUp)
	if err == nil {
		t.Fatal("ожидалась ошибка от репозитория")
	}
}
