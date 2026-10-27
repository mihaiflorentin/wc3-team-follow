package container

import (
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/sys"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// InfrastructureContainer holds the lazily built outbound adapters. Every
// accessor returns (T, error): a missing config section or an unreachable
// backend is reported to the caller instead of surfacing later as a nil
// pointer. Adapters that hold connections register their Close(ctx) method
// as a close hook and their Ping method as a readiness check (onReady), so
// GET /health/ready covers every backend the process uses. Errors are
// wrapped with the step that failed ("create redis client", "resolve
// mysql"), so a chain reads like a path.
type InfrastructureContainer struct {
	sys lazy[contract.Sys]
}

func (s *ServiceContainer) Sys() (contract.Sys, error) {
	return s.infrastructure.sys.get(func() (contract.Sys, error) {
		return sys.New(), nil
	})
}
