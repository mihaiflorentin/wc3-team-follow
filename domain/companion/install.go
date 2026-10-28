package companion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// InstallUpdate downloads, verifies and installs the release on offer,
// after the player clicked Update (ADR 0009). It returns at once; each step
// arrives with a later state, ending in restarting, which tells the window
// to close so Team Follow starts again in the new version, or in failed,
// with the running version untouched.
func (s *Service) InstallUpdate() (dto.FollowState, error) {
	s.mu.Lock()
	var err error
	var rel dto.Release
	if s.runCtx == nil {
		err = errors.New("Team Follow is still starting. Try again in a moment.")
	} else if s.stopping {
		err = errClosing
	} else {
		rel, err = s.updates.StartInstall()
	}
	if err == nil {
		ctx, cancel := context.WithCancel(s.runCtx)
		s.cancelInstall = cancel
		s.logger.Info("update.install_start", slog.String("tag", rel.Tag), slog.String("asset", rel.AssetName))
		s.checks.Go(func() {
			defer s.guard("update_install")
			defer cancel()
			s.install(ctx, rel)
		})
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// install runs the update's steps off s.mu.
func (s *Service) install(ctx context.Context, rel dto.Release) {
	path, err := s.updater.Download(ctx, rel, func(p dto.UpdateProgress) {
		s.mu.Lock()
		s.updates.Progress(p)
		s.mu.Unlock()
		s.notify()
	})
	if err != nil {
		s.mu.Lock()
		s.cancelInstall = nil
		if ctx.Err() != nil {
			// The player cancelled, or the window is closing.
			s.updates.Cancelled()
			s.logger.Info("update.cancelled", slog.String("tag", rel.Tag))
		} else {
			s.updates.Fail(err)
			s.logger.Warn("update.failed", slog.String("step", "download"), slog.String("tag", rel.Tag), slog.Any("error", err))
		}
		s.mu.Unlock()
		s.notify()
		return
	}

	// Past the download, the update can't be cancelled: replacing the
	// files takes a moment and must not stop half-way.
	s.mu.Lock()
	s.cancelInstall = nil
	s.updates.Installing()
	s.mu.Unlock()
	s.notify()
	err = s.updater.Install(context.WithoutCancel(ctx), path)

	s.mu.Lock()
	if err != nil {
		s.updates.Fail(err)
		s.logger.Warn("update.failed", slog.String("step", "install"), slog.String("tag", rel.Tag), slog.Any("error", err))
	} else {
		s.updates.Restarting()
		s.logger.Info("update.installed", slog.String("tag", rel.Tag))
	}
	s.mu.Unlock()
	s.notify()
}

// CancelUpdate stops the update's download; it can't be cancelled once it
// is installing. The offer stays.
func (s *Service) CancelUpdate() dto.FollowState {
	s.mu.Lock()
	if s.cancelInstall != nil && s.updates.Busy() {
		s.cancelInstall()
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state
}

// DismissUpdateNotes hides what the player had to redo after an update,
// such as the Linux setcap step.
func (s *Service) DismissUpdateNotes() dto.FollowState {
	s.mu.Lock()
	s.updates.DismissAfter()
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state
}

// cleanupUpdate removes what an earlier update left behind, at every start
// of a release build. Development builds never touch their files.
func (s *Service) cleanupUpdate(ctx context.Context) {
	s.mu.Lock()
	supported := s.updates.Supported()
	s.mu.Unlock()
	if !supported {
		return
	}
	if err := s.updater.Cleanup(ctx); err != nil {
		s.logger.WarnContext(ctx, "update.cleanup", slog.Any("error", err))
	}
}
