// Тесты на воркер (worker.go) и планировщик (scheduler.go): реальные HTTP-запросы
// через httptest, постановка задач в очередь и обработка граничных случаев.
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"health_checker/internal/domain"

	"github.com/google/uuid"
)

// newCheckService собирает сервис с переданными моками и буферизованным каналом
// результатов, из которого можно синхронно читать результат doCheck.
func newCheckService(repo *mockSiteRepo, limit, queue int) *CheckerService {
	return NewCheckerService(repo, &capturingCheckRepo{}, &capturingSender{}, 1, limit, queue)
}

// doCheck на 2xx-ответе возвращает результат успешной проверки.
func TestDoCheck_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := newCheckService(newMockSiteRepo(), 10, 10)
	svc.doCheck(1, CheckJob{SiteID: uuid.New(), URL: srv.URL})

	select {
	case res := <-svc.resultsChan:
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("statusCode = %d, want 200", res.StatusCode)
		}
		if res.Latency <= 0 {
			t.Errorf("latency = %v, want > 0", res.Latency)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("doCheck не отправил результат")
	}
}

// doCheck на 5xx-ответе всё равно возвращает результат без ошибки запроса
// (код ответа интерпретируется позже, в statusFromResult).
func TestDoCheck_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	svc := newCheckService(newMockSiteRepo(), 10, 10)
	svc.doCheck(1, CheckJob{SiteID: uuid.New(), URL: srv.URL})

	res := <-svc.resultsChan
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("statusCode = %d, want 503", res.StatusCode)
	}
}

// Мёртвый адрес (сервер закрыт) -> результат с ошибкой запроса.
func TestDoCheck_ConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // гарантированно отказываемся соединяться

	svc := newCheckService(newMockSiteRepo(), 10, 10)
	svc.doCheck(1, CheckJob{SiteID: uuid.New(), URL: url})

	select {
	case res := <-svc.resultsChan:
		if res.Err == nil {
			t.Fatal("ожидалась ошибка соединения")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("doCheck не отправил результат")
	}
}

// Некорректный URL -> ошибка создания запроса (http.NewRequestWithContext).
func TestDoCheck_InvalidURL(t *testing.T) {
	svc := newCheckService(newMockSiteRepo(), 10, 10)
	svc.doCheck(1, CheckJob{SiteID: uuid.New(), URL: "://bad-url"})

	select {
	case res := <-svc.resultsChan:
		if res.Err == nil {
			t.Fatal("ожидалась ошибка из-за некорректного URL")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("doCheck не отправил результат")
	}
}

// --- emitResult ---

// emitResult при отменённом контексте не блокируется навсегда и отбрасывает результат.
func TestEmitResult_ContextCancelled(t *testing.T) {
	svc := newCheckService(newMockSiteRepo(), 10, 10) // очередь маленькая и пустая

	// Отменяем контекст сервиса, не заполняя очередь.
	svc.cancel()

	done := make(chan struct{})
	go func() {
		svc.emitResult(CheckResult{SiteID: uuid.New(), StatusCode: 200})
		close(done)
	}()

	select {
	case <-done:
		// ok — emitResult вернулся, не заблокировавшись
	case <-time.After(2 * time.Second):
		t.Fatal("emitResult заблокировался при отменённом контексте")
	}
}

// --- produceJobs ---

// produceJobs кладёт в очередь по одной задаче на каждый сайт, заполняя URL и
// интервал из CheckIntervalSeconds.
func TestProduceJobs_EnqueuesAllSites(t *testing.T) {
	sites := []domain.Site{
		{ID: uuid.New(), Url: "https://a", CheckIntervalSeconds: 30},
		{ID: uuid.New(), Url: "https://b", CheckIntervalSeconds: 60},
	}
	repo := newMockSiteRepo(sites...)
	svc := newCheckService(repo, 10, 10)

	svc.produceJobs()

	if got := len(svc.jobsChan); got != 2 {
		t.Fatalf("jobs в очереди = %d, want 2", got)
	}

	job := <-svc.jobsChan
	if job.Interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", job.Interval)
	}
	if job.URL == "" {
		t.Error("URL не заполнен")
	}
}

// Ошибка выборки сайтов не должна ничего класть в очередь (только логируется).
func TestProduceJobs_RepoError(t *testing.T) {
	repo := newMockSiteRepo()
	repo.getSitesErr = context.DeadlineExceeded
	svc := newCheckService(repo, 10, 10)

	svc.produceJobs()

	if got := len(svc.jobsChan); got != 0 {
		t.Errorf("jobs в очереди = %d, want 0", got)
	}
}

// Переполнение очереди: лишние задачи отбрасываются (с предупреждением в лог),
// но produceJobs не блокируется.
func TestProduceJobs_QueueFullDropsJobs(t *testing.T) {
	sites := []domain.Site{
		{ID: uuid.New(), Url: "https://a"},
		{ID: uuid.New(), Url: "https://b"},
		{ID: uuid.New(), Url: "https://c"},
	}
	repo := newMockSiteRepo(sites...)
	svc := newCheckService(repo, 10, 1) // очередь на 1 задачу

	done := make(chan struct{})
	go func() {
		svc.produceJobs()
		close(done)
	}()

	select {
	case <-done:
		// ok — не заблокировался на переполненной очереди
	case <-time.After(2 * time.Second):
		t.Fatal("produceJobs заблокировался на переполненной очереди")
	}

	if got := len(svc.jobsChan); got != 1 {
		t.Errorf("jobs в очереди = %d, want 1 (остальные дропнуты)", got)
	}
}
