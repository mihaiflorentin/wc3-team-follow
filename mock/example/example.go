package mockexample

import (
	"context"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.ExampleService = (*Service)(nil)

// Service is a test double for the example service contract. Ping returns
// PingStatus and PingErr and counts the calls; RunJob records each job and
// returns RunJobErr. It is safe for concurrent use, so a worker goroutine can
// call it while the test reads Jobs.
type Service struct {
	PingStatus dto.ExampleStatus
	PingErr    error
	PingCalls  int
	RunJobErr  error

	mu   sync.Mutex
	jobs []dto.ExampleJob
}

func (s *Service) Ping(ctx context.Context) (dto.ExampleStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PingCalls++
	return s.PingStatus, s.PingErr
}

func (s *Service) RunJob(ctx context.Context, job dto.ExampleJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
	return s.RunJobErr
}

// Jobs returns a copy of the jobs RunJob received, in call order.
func (s *Service) Jobs() []dto.ExampleJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]dto.ExampleJob(nil), s.jobs...)
}
