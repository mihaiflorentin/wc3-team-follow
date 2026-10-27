package contract

import "context"

// ReadinessCheck probes one dependency the process created, such as a
// database ping or a broker connection. GET /health/ready runs every check.
type ReadinessCheck interface {
	// Name identifies the dependency in the /health/ready response.
	Name() string
	// Check returns an error when the dependency cannot serve traffic. It
	// must return once ctx is done.
	Check(ctx context.Context) error
}

// ReadinessChecks lists the readiness checks registered so far. The
// container implements it: each adapter registers its check when the
// container creates it, alongside its close hook.
type ReadinessChecks interface {
	ReadinessChecks() []ReadinessCheck
}
