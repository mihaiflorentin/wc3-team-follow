package container

import (
	"fmt"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/discovery"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/gamesocket"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/maps"
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
	mapsLocator     lazy[contract.MapsLocator]
	mapLibrary      lazy[contract.MapLibrary]
	gameProcesses   lazy[contract.GameProcesses]
}

// MapsLocator returns the Maps folder locator for this operating system.
func (s *ServiceContainer) MapsLocator() (contract.MapsLocator, error) {
	return s.infrastructure.mapsLocator.get(func() (contract.MapsLocator, error) {
		return discovery.NewMapsLocator(s.logger), nil
	})
}

// MapLibrary returns the map listing and search adapter.
func (s *ServiceContainer) MapLibrary() (contract.MapLibrary, error) {
	return s.infrastructure.mapLibrary.get(func() (contract.MapLibrary, error) {
		return maps.New(), nil
	})
}

// GameDiscovery returns the discovery for this operating system.
func (s *ServiceContainer) GameDiscovery() (contract.GameDiscovery, error) {
	return s.infrastructure.gameDiscovery.get(func() (contract.GameDiscovery, error) {
		return discovery.New(s.logger), nil
	})
}

// GameProcesses returns the closer for the game's processes on this
// operating system, used only when the player asks to close the game.
func (s *ServiceContainer) GameProcesses() (contract.GameProcesses, error) {
	return s.infrastructure.gameProcesses.get(func() (contract.GameProcesses, error) {
		return discovery.NewProcesses(s.logger), nil
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

// GameKeyboard returns the keystroke input Game mode types with. Until the
// keystroke adapter (issue 12) is merged there is none, and Game mode says
// typing isn't supported on this system.
func (s *ServiceContainer) GameKeyboard() (contract.GameKeyboard, error) {
	return nil, nil
}
