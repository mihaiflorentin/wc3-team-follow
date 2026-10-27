package example

import (
	"context"
	"errors"
	"fmt"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
	"log/slog"
	"strings"
)

// ErrInvalidJob reports an example job the domain refuses to run.
var ErrInvalidJob = errors.New("invalid example job")

// Service is a placeholder domain service. Replace it with your own bounded
// contexts. Its collaborators arrive through the constructor as port/contract
// interfaces, so the domain never imports infrastructure or the container.
type Service struct {
	logger *slog.Logger
}

func NewService(logger *slog.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

// Ping reports the domain status. The placeholder has no storage, so it
// reports no examples.
func (s *Service) Ping(ctx context.Context) (dto.ExampleStatus, error) {
	s.logger.DebugContext(ctx, "example.ping")
	return dto.ExampleStatus{}, nil
}

// RunJob performs one example job: it validates the payload and logs the outcome. Replace it with your own
// job logic; the worker and example-job commands only wire it.
func (s *Service) RunJob(ctx context.Context, job dto.ExampleJob) error {
	if strings.TrimSpace(job.Name) == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidJob)
	}
	s.logger.InfoContext(ctx, "example.job", slog.String("name", job.Name))
	return nil
}
