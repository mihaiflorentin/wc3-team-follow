package container

import (
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/discovery"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/gamesocket"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/keyboard"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/maps"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/settings"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/updater"
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
	gameKeyboard    lazy[contract.GameKeyboard]
	recentPlayers   lazy[contract.RecentPlayers]
	playerNotes     lazy[contract.PlayerNotes]
	updater         lazy[contract.Updater]
}

// Updater returns the GitHub Releases client named by [updates]. It talks
// only to GitHub's hosts over HTTPS (docs/adr/0009).
func (s *ServiceContainer) Updater() (contract.Updater, error) {
	return s.infrastructure.updater.get(func() (contract.Updater, error) {
		cfg := s.config.Updates
		if cfg == nil {
			return nil, errors.New("missing [updates] config section")
		}
		if u, err := url.Parse(cfg.APIURL); err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("invalid updates.api_url %q: it must be an HTTPS address", cfg.APIURL)
		}
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid updates.timeout %q: %w", cfg.Timeout, err)
		}
		return updater.New(updater.NewClient(timeout), updater.Options{
			APIURL: cfg.APIURL, Repository: cfg.Repository, Asset: updater.AssetName(runtime.GOOS, runtime.GOARCH),
		}, s.logger), nil
	})
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

// GameKeyboard returns the keystroke input for this operating system, used
// only to type a line in the match chat (docs/adr/0007). It needs no config.
func (s *ServiceContainer) GameKeyboard() (contract.GameKeyboard, error) {
	return s.infrastructure.gameKeyboard.get(func() (contract.GameKeyboard, error) {
		return keyboard.New(), nil
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

// RecentPlayers returns the recent players store, next to the friend
// selection in the per-user data folder named by [storage].
func (s *ServiceContainer) RecentPlayers() (contract.RecentPlayers, error) {
	return s.infrastructure.recentPlayers.get(func() (contract.RecentPlayers, error) {
		dir, err := config.DataDir(s.config.Storage)
		if err != nil {
			return nil, fmt.Errorf("resolve data folder: %w", err)
		}
		return settings.NewRecentPlayers(dir), nil
	})
}

// PlayerNotes returns the player notes store, next to the friend selection
// in the per-user data folder named by [storage].
func (s *ServiceContainer) PlayerNotes() (contract.PlayerNotes, error) {
	return s.infrastructure.playerNotes.get(func() (contract.PlayerNotes, error) {
		dir, err := config.DataDir(s.config.Storage)
		if err != nil {
			return nil, fmt.Errorf("resolve data folder: %w", err)
		}
		return settings.NewPlayerNotes(dir), nil
	})
}
