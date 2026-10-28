package container

import (
	"errors"
	"fmt"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

type DomainContainer struct {
	companion lazy[contract.Companion]
}

// Companion returns the Team Follow service, built once with its adapters
// injected and tuned by [follow].
func (s *ServiceContainer) Companion() (contract.Companion, error) {
	return s.domain.companion.get(func() (contract.Companion, error) {
		cfg := s.config.Follow
		if cfg == nil {
			return nil, errors.New("missing [follow] config section")
		}
		refresh, err := time.ParseDuration(cfg.RefreshInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid follow.refresh_interval %q: %w", cfg.RefreshInterval, err)
		}
		discoveryEvery, err := time.ParseDuration(cfg.DiscoveryInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid follow.discovery_interval %q: %w", cfg.DiscoveryInterval, err)
		}
		discovery, err := s.GameDiscovery()
		if err != nil {
			return nil, fmt.Errorf("resolve game discovery: %w", err)
		}
		dialer, err := s.GameDialer()
		if err != nil {
			return nil, fmt.Errorf("resolve game dialer: %w", err)
		}
		prefs, err := s.PreferenceStore()
		if err != nil {
			return nil, fmt.Errorf("resolve preference store: %w", err)
		}
		locator, err := s.MapsLocator()
		if err != nil {
			return nil, fmt.Errorf("resolve maps locator: %w", err)
		}
		library, err := s.MapLibrary()
		if err != nil {
			return nil, fmt.Errorf("resolve map library: %w", err)
		}
		processes, err := s.GameProcesses()
		if err != nil {
			return nil, fmt.Errorf("resolve game processes: %w", err)
		}
		keyboard, err := s.GameKeyboard()
		if err != nil {
			return nil, fmt.Errorf("resolve game keyboard: %w", err)
		}
		ports := companion.Ports{Discovery: discovery, Dialer: dialer, Prefs: prefs, Maps: locator, Library: library, Processes: processes,
			Keyboard: keyboard}
		return companion.New(ports, s.logger, companion.Options{
			VerifiedBuilds:    cfg.VerifiedBuilds,
			RefreshInterval:   refresh,
			DiscoveryInterval: discoveryEvery,
		}), nil
	})
}
