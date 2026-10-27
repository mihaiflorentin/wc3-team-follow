package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// ExampleService is the inbound port of the example domain. HTTP handlers
// depend on this interface, never on the concrete domain/example.Service.
type ExampleService interface {
	// Ping reports the state of the example domain.
	Ping(ctx context.Context) (dto.ExampleStatus, error)
	// RunJob performs one example job. The worker calls it per consumed
	// payload and the example-job command calls it once.
	RunJob(ctx context.Context, job dto.ExampleJob) error
}
