package container

import (
	"errors"
	"fmt"
	"runtime"
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
		updatesCfg := s.config.Updates
		if updatesCfg == nil {
			return nil, errors.New("missing [updates] config section")
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
		recent, err := s.RecentPlayers()
		if err != nil {
			return nil, fmt.Errorf("resolve recent players: %w", err)
		}
		notes, err := s.PlayerNotes()
		if err != nil {
			return nil, fmt.Errorf("resolve player notes: %w", err)
		}
		ranking, err := s.Ranking()
		if err != nil {
			return nil, fmt.Errorf("resolve ranking: %w", err)
		}
		updates, err := s.Updater()
		if err != nil {
			return nil, fmt.Errorf("resolve updater: %w", err)
		}
		mapsCfg := s.config.Maps
		if mapsCfg == nil {
			return nil, errors.New("missing [maps] config section")
		}
		tracked, err := trackedMaps(mapsCfg)
		if err != nil {
			return nil, err
		}
		mapEvery, err := time.ParseDuration(mapsCfg.CheckInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid maps.check_interval %q: %w", mapsCfg.CheckInterval, err)
		}
		mapRetry, err := time.ParseDuration(mapsCfg.RetryInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid maps.retry_interval %q: %w", mapsCfg.RetryInterval, err)
		}
		mapReleases, err := s.MapReleases()
		if err != nil {
			return nil, fmt.Errorf("resolve map releases: %w", err)
		}
		checkEvery, err := time.ParseDuration(updatesCfg.CheckInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid updates.check_interval %q: %w", updatesCfg.CheckInterval, err)
		}
		retryEvery, err := time.ParseDuration(updatesCfg.RetryInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid updates.retry_interval %q: %w", updatesCfg.RetryInterval, err)
		}
		ports := companion.Ports{Discovery: discovery, Dialer: dialer, Prefs: prefs, Maps: locator, Library: library, Processes: processes,
			Keyboard: keyboard, Recent: recent, Notes: notes, Ranking: ranking, Updater: updates,
			MapReleases: mapReleases}
		return companion.New(ports, s.logger, companion.Options{
			VerifiedBuilds:      cfg.VerifiedBuilds,
			RefreshInterval:     refresh,
			DiscoveryInterval:   discoveryEvery,
			AppVersion:          s.config.App.Version,
			UpdateInterval:      checkEvery,
			UpdateRetryInterval: retryEvery,
			ReleasesURL:         "https://github.com/" + updatesCfg.Repository + "/releases",
			System:              runtime.GOOS,
			Executable:          s.config.App.Executable,
			Updated:             s.config.App.Updated,
			Changelog:           s.config.App.Changelog,
			TrackedMaps:         tracked,
			MapInterval:         mapEvery,
			MapRetryInterval:    mapRetry,
		}), nil
	})
}
