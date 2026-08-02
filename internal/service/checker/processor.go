package service


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

}