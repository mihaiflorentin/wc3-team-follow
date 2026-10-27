// Package container is the composition root. main builds one ServiceContainer
// and hands it to the CLI command tree, which resolves dependencies from it and
// passes them on through constructors. Only main and cmd/cli import container
// (docs/adr/0002).
package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// closeTimeout bounds how long Close waits for resources that shut down with a context.
const closeTimeout = 10 * time.Second

type ServiceContainer struct {
	config *config.Config
	logger *slog.Logger

	infrastructure InfrastructureContainer
	domain         DomainContainer

	closeMu sync.Mutex
	closers []closer

	readyMu sync.Mutex
	checks  []contract.ReadinessCheck
}

type closer struct {
	name  string
	close func(ctx context.Context) error
}

// New returns a container that builds services lazily from cfg and logs
// through logger, which must not be nil. Call Close once the process no
// longer needs them.
func New(cfg *config.Config, logger *slog.Logger) *ServiceContainer {
	return &ServiceContainer{config: cfg, logger: logger}
}

// Config returns the configuration the container was built with.
func (s *ServiceContainer) Config() *config.Config {
	return s.config
}

// Logger returns the application logger.
func (s *ServiceContainer) Logger() *slog.Logger {
	return s.logger
}

// Close shuts down every resource the container created, in reverse order of
// creation, and returns all errors joined. It is safe to call more than once.
func (s *ServiceContainer) Close() error {
	s.closeMu.Lock()
	closers := s.closers
	s.closers = nil
	s.closeMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	var errs []error
	for i := len(closers) - 1; i >= 0; i-- {
		c := closers[i]
		if err := c.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", c.name, err))
			continue
		}
		s.logger.Debug("container.close", slog.String("resource", c.name))
	}
	return errors.Join(errs...)
}

// onClose registers a shutdown hook for a resource the container created.
func (s *ServiceContainer) onClose(name string, fn func(ctx context.Context) error) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.closers = append(s.closers, closer{name: name, close: fn})
}

// readinessCheck adapts a named probe function to contract.ReadinessCheck.
type readinessCheck struct {
	name  string
	check func(ctx context.Context) error
}

func (c readinessCheck) Name() string                    { return c.name }
func (c readinessCheck) Check(ctx context.Context) error { return c.check(ctx) }

// onReady registers a readiness check for a resource the container created.
// Adapters register it next to their onClose hook, typically with the
// adapter's Ping method, so GET /health/ready covers every dependency.
func (s *ServiceContainer) onReady(name string, fn func(ctx context.Context) error) {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()
	s.checks = append(s.checks, readinessCheck{name: name, check: fn})
}

// ReadinessChecks returns the readiness checks registered so far, in
// registration order. It implements contract.ReadinessChecks, so the HTTP
// layer sees adapters created after the server started too.
func (s *ServiceContainer) ReadinessChecks() []contract.ReadinessCheck {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()
	return slices.Clone(s.checks)
}

// lazy holds a singleton that is built on first successful use. Concurrent
// first calls build it once; a failed build is not cached, so a later call
// retries (for example once a database becomes reachable).
type lazy[T any] struct {
	mu    sync.Mutex
	built bool
	value T
}

func (l *lazy[T]) get(build func() (T, error)) (T, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.built {
		return l.value, nil
	}
	value, err := build()
	if err != nil {
		var zero T
		return zero, err
	}
	l.value, l.built = value, true
	return value, nil
}
