package companion

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// mapCheckTimeout bounds one release lookup; a download has no overall
// limit, only the adapter's stall timeout.
const mapCheckTimeout = 30 * time.Second

// startMapCheck checks every tracked map in its own goroutine, off s.mu,
// and downloads what the planner asks for (ADR 0012); Run waits for it.
// The planner has already begun the check. It runs with s.mu held.
func (s *Service) startMapCheck(ctx context.Context, manual bool) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelMaps = cancel
	tracked := s.mapUpdates.Tracked()
	s.checks.Go(func() {
		defer s.guard("maps")
		defer cancel()
		s.checkMaps(ctx, manual, tracked)
	})
}

// checkMaps runs one check of every tracked map, off s.mu.
func (s *Service) checkMaps(ctx context.Context, manual bool, tracked []dto.TrackedMap) {
	root, err := s.currentMapsFolder(ctx)
	if err != nil {
		s.logger.InfoContext(ctx, "maps.check_failed", slog.Bool("manual", manual), slog.Any("error", err))
		s.endMapCheck(func() { s.mapUpdates.Abort(playerSentence(err, "No Warcraft III maps folder was found.")) })
		return
	}
	for i, m := range tracked {
		if ctx.Err() != nil {
			break
		}
		checkCtx, cancel := context.WithTimeout(ctx, mapCheckTimeout)
		rel, err := s.mapReleases.Latest(checkCtx, m)
		cancel()
		var installed []dto.InstalledMap
		if err == nil {
			var findErr error
			installed, findErr = s.library.Installed(ctx, root, m)
			if findErr != nil {
				s.logger.WarnContext(ctx, "maps.installed", slog.String("map", m.Name), slog.Any("error", findErr))
				s.endMapCheck(func() { s.mapUpdates.Abort("Your Maps folder couldn't be read, so maps weren't checked.") })
				return
			}
		}
		s.mu.Lock()
		download := s.mapUpdates.Found(i, rel, installed, err)
		s.takeMapNotes()
		s.mu.Unlock()
		if err != nil {
			s.logger.InfoContext(ctx, "maps.check_failed", slog.String("map", m.Name), slog.Bool("manual", manual), slog.Any("error", err))
		} else {
			s.logger.InfoContext(ctx, "maps.check", slog.String("map", m.Name), slog.Bool("manual", manual), slog.String("latest", rel.Version),
				slog.Int("installed", len(installed)), slog.Bool("download", download))
		}
		s.notify()
		if download {
			s.downloadMap(ctx, i, rel, filepath.Join(root, filepath.FromSlash(m.Folder)))
		}
	}
	s.endMapCheck(s.mapUpdates.Finish)
}

// downloadMap downloads map i's release into dir, reporting progress.
func (s *Service) downloadMap(ctx context.Context, i int, rel dto.MapRelease, dir string) {
	s.logger.InfoContext(ctx, "maps.download_start", slog.String("map", rel.Map.Name), slog.String("version", rel.Version),
		slog.Int64("bytes", rel.Size))
	path, err := s.mapReleases.Download(ctx, rel, dir, func(p dto.MapProgress) {
		s.mu.Lock()
		s.mapUpdates.Progress(i, p)
		s.mu.Unlock()
		s.notify()
	})
	s.mu.Lock()
	switch {
	case err != nil && ctx.Err() != nil:
		// Switched off, or Team Follow is closing; nothing was kept.
		s.mapUpdates.Cancelled(i)
		s.logger.InfoContext(ctx, "maps.download_cancelled", slog.String("map", rel.Map.Name), slog.String("version", rel.Version))
	case err != nil:
		s.mapUpdates.Downloaded(i, "", err)
		s.logger.WarnContext(ctx, "maps.download_failed", slog.String("map", rel.Map.Name), slog.String("version", rel.Version), slog.Any("error", err))
	default:
		s.mapUpdates.Downloaded(i, path, nil)
		// Auto-host's next lobby may use it.
		s.hostMapStale = true
		s.logger.InfoContext(ctx, "maps.download_done", slog.String("map", rel.Map.Name), slog.String("version", rel.Version), slog.String("path", path))
	}
	s.takeMapNotes()
	s.mu.Unlock()
	s.notify()
}

// endMapCheck ends the running check with end, under s.mu.
func (s *Service) endMapCheck(end func()) {
	s.mu.Lock()
	end()
	s.cancelMaps = nil
	s.takeMapNotes()
	s.mu.Unlock()
	s.notify()
}

// takeMapNotes moves the planner's lines into Activity. It runs with s.mu
// held.
func (s *Service) takeMapNotes() {
	for _, note := range s.mapUpdates.TakeNotes() {
		s.engine.Note(note)
	}
}

// CheckMaps checks every tracked map now, after the player clicked Check
// now. With Keep maps up to date on it also downloads what's missing. The
// answer arrives with later states.
func (s *Service) CheckMaps() (dto.FollowState, error) {
	s.mu.Lock()
	var err error
	switch {
	case len(s.mapUpdates.Tracked()) == 0:
		err = errors.New("No maps are tracked.")
	case s.runCtx == nil:
		err = errors.New("Team Follow is still starting. Try again in a moment.")
	case s.stopping:
		err = errClosing
	case s.mapUpdates.Begin(true):
		s.startMapCheck(s.runCtx, true)
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// setMapUpdates switches Keep maps up to date on or off; off stops a
// running download. It runs with s.mu held.
func (s *Service) setMapUpdates(on bool) {
	s.mapUpdates.SetEnabled(on)
	if !on && s.cancelMaps != nil && s.mapUpdates.Downloading() {
		s.cancelMaps()
	}
}
