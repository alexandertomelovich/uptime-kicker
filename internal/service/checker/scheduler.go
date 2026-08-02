package service

import (
	"log"
	"time"
)

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
		log.Printf(err.Error())
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

