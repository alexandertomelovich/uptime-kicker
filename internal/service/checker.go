package service

import (
	"context"
	"health_checker/internal/repository"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type CheckJob struct {
	SiteID   uuid.UUID
	URL      string
	Interval time.Duration
}

type CheckResult struct {
	SiteID     uuid.UUID
	StatusCode int
	Latency    time.Duration
	Err        error
}

type CheckerService struct {
	repo repository.SiteRepository
	//нужно будет добавить для отправки уведомлений
	numWorkers  int
	limit       int
	jobsChan    chan CheckJob
	resultsChan chan CheckResult

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCheckerService(
	repo repository.SiteRepository,
	numWorkers int,
	limit int,
	queueSize int,
) *CheckerService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CheckerService{
		repo:        repo,
		numWorkers:  numWorkers,
		limit:       limit,
		jobsChan:    make(chan CheckJob, queueSize),
		resultsChan: make(chan CheckResult, queueSize),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *CheckerService) Start() {
	for i := 1; i <= s.numWorkers; i++ {
		go s.worker(i)
	}

	go s.resultProcessor()

	go scheduler()
}

func (s *CheckerService) Stop() {
	s.cancel()

	close(s.jobsChan)
	close(s.resultsChan)
}

func (s *CheckerService) worker(id int) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case job, ok := <-s.jobsChan:
			if !ok {
				return
			}
			s.doCheck(id, job)
		}
	}
}

func (s *CheckerService) doCheck(id int, job CheckJob) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", job.URL, nil)
	if err != nil {
		s.resultsChan <- CheckResult{
			SiteID: job.SiteID,
			Err:    err,
		}
		return
	}

	client := &http.Client{}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		s.resultsChan <- CheckResult{
			SiteID:  job.SiteID,
			Latency: time.Since(start),
			Err:     err,
		}
		return
	}
	defer resp.Body.Close()

	s.resultsChan <- CheckResult{
		SiteID:     job.SiteID,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Err:        nil,
	}
}

func (s *CheckerService) scheduler() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.produceJobs()
		}
	}
}

func (s *CheckerService) produceJobs() {
	sites, err := s.repo.GetSitesNeedingCheck(s.ctx, s.limit)
	if err != nil {
		return
	}
	jobs := make([]CheckJob, len(sites))

	for i, site := range sites {
		jobs[i] = CheckJob{
			SiteID: site.ID,
			URL: site.Url,
			Interval: time.Duration(site.CheckIntervalSeconds),
		}
	}

	for _, job := range jobs {
		select {
		case <-s.ctx.Done():
			return
		case s.jobsChan <- job:
		default:
			//...
		}
	}
}

func (s *CheckerService) resultProcessor() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case result, ok := <-s.resultsChan:
			if !- ok {
				return
			}
			s.processResult(result)
		}
	}
}

func (s *CheckerService) processResult(result CheckResult) {

}

func (s *CheckerService) sendStatusChangeAlert