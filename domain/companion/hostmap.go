package companion

import (
	"context"
	"log/slog"

	"github.com/mihaiflorentin/wc3-team-follow/domain/mapupdate"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// HostMap works out the file Auto-host creates its lobbies on with
// settings (issue 32): for a tracked map with UseNewest its newest
// installed version, otherwise the chosen file. It reads the Maps folder,
// off s.mu.
func (s *Service) HostMap(ctx context.Context, settings dto.HostSettings) dto.HostMap {
	m := dto.HostMap{Path: settings.MapPath}
	i, version := mapupdate.Match(s.opts.TrackedMaps, settings.MapPath)
	if settings.MapPath == "" || i < 0 {
		return m
	}
	tracked := s.opts.TrackedMaps[i]
	m.Tracked, m.Version, m.ChosenVersion, m.NewestVersion = tracked.Name, version, version, version
	root, err := s.currentMapsFolder(ctx)
	var installed []dto.InstalledMap
	if err == nil {
		installed, err = s.library.Installed(ctx, root, tracked)
	}
	if err != nil {
		// The chosen file is hosted; the next lobby looks again.
		s.logger.WarnContext(ctx, "host.newest_map", slog.String("map", tracked.Name), slog.Any("error", err))
		m.NewestVersion = ""
		return m
	}
	newest, ok := mapupdate.Newest(installed)
	if !ok || mapupdate.CompareVersions(newest.Version, version) <= 0 {
		// The chosen file is the newest: host it, not another copy.
		m.Newest = settings.UseNewest
		return m
	}
	m.NewestVersion = newest.Version
	if settings.UseNewest {
		m.Path, m.Version, m.Newest = newest.Path, newest.Version, true
	}
	return m
}

// hostMapLabel names a resolved map in Activity, like "Legion TD
// 11.4c-hf9"; "" for an untracked map, which keeps the settings' name.
func hostMapLabel(m dto.HostMap) string {
	if m.Tracked == "" {
		return ""
	}
	return m.Tracked + " " + m.Version
}

// startHostMap works the hosted map out again in its own goroutine when
// Auto-host's next lobby waits for it, or when something may have changed
// it (a download, another Maps folder, start-up). It runs with s.mu held.
func (s *Service) startHostMap(ctx context.Context) {
	due := s.engine.HostMapDue()
	if (!due && !s.hostMapStale) || s.hostMapBusy || s.stopping {
		return
	}
	s.hostMapStale, s.hostMapBusy = false, true
	settings := s.engine.HostSettings()
	s.checks.Go(func() {
		defer s.guard("host_map")
		m := s.HostMap(ctx, settings)
		gamePath, err := "", error(nil)
		if m.Path != "" {
			gamePath, err = s.maps.GamePath(ctx, m.Path)
		}
		s.mu.Lock()
		s.hostMapBusy = false
		if err == nil {
			s.hostMap = m
		}
		if s.engine.HostMapDue() {
			if err != nil {
				// The last file is hosted again.
				s.logger.WarnContext(ctx, "host.map_path", slog.String("path", m.Path), slog.Any("error", err))
				gamePath = ""
			}
			s.logger.InfoContext(ctx, "host.map", slog.String("map", m.Tracked), slog.String("version", m.Version),
				slog.Bool("newest", m.Newest), slog.String("game_map_path", gamePath))
			s.engine.SetHostMap(gamePath, hostMapLabel(m))
			s.logStatus()
		}
		s.mu.Unlock()
		s.notify()
	})
}
