package container

import (
	"fmt"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/discovery"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/gamesocket"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// InfrastructureContainer holds the lazily built outbound adapters. Every
// accessor returns (T, error): a missing config section is reported to the
// caller instead of surfacing later as a nil pointer. Errors are wrapped
// with the step that failed, so a chain reads like a path.
type InfrastructureContainer struct {
	gameDiscovery   lazy[contract.GameDiscovery]
	gameDialer      lazy[contract.GameDialer]
	preferenceStore lazy[contract.PreferenceStore]
}

// GameDiscovery returns the discovery for this operating system.
func (s *ServiceContainer) GameDiscovery() (contract.GameDiscovery, error) {
	return s.infrastructure.gameDiscovery.get(func() (contract.GameDiscovery, error) {
		return discovery.New(s.logger), nil
	})
}

// GameDialer returns the dialer for the game's menu socket.
func (s *ServiceContainer) GameDialer() (contract.GameDialer, error) {
	return s.infrastructure.gameDialer.get(func() (contract.GameDialer, error) {
		return gamesocket.NewDialer(), nil
	})
}

// PreferenceStore returns the friend-selection store in the per-user data
// folder named by [storage] (config.DataDir).
func (s *ServiceContainer) PreferenceStore() (contract.PreferenceStore, error) {
	return s.infrastructure.preferenceStore.get(func() (contract.PreferenceStore, error) {
		dir, err := config.DataDir(s.config.Storage)
		if err != nil {
			return nil, fmt.Errorf("resolve data folder: %w", err)
		}
		return settings.New(dir), nil
	})
}
