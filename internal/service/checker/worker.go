package service

import (
	"context"
	"net/http"
	"time"
)

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
