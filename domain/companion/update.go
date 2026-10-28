package companion

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// checkTimeout bounds one update check.
const checkTimeout = 30 * time.Second

// startCheck asks the Updater for the newest release in its own goroutine,
// off s.mu; Run waits for it. The checker has already begun the check. It
// runs with s.mu held.
func (s *Service) startCheck(ctx context.Context, manual bool) {
	s.checks.Go(func() {
		defer s.guard("update")
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		rel, err := s.updater.Latest(checkCtx)

		s.mu.Lock()
		s.updates.Finish(rel, err)
		offered, ok := s.updates.Release()
		s.mu.Unlock()

		if err != nil {
			// A check that fails stays quiet in the window; the log keeps why.
			s.logger.InfoContext(ctx, "update.check_failed", slog.Bool("manual", manual), slog.Any("error", err))
		} else {
			attrs := []any{slog.Bool("manual", manual), slog.String("latest", rel.Tag), slog.Bool("asset", rel.AssetURL != ""),
				slog.Bool("changelog", rel.Changelog != "")}
			if ok {
				attrs = append(attrs, slog.String("offered", offered.Version))
			}
			s.logger.InfoContext(ctx, "update.check", attrs...)
		}
		s.notify()
	})
}

// CheckForUpdates starts a check the player asked for. The answer arrives
// with a later state: a banner, or a message in the Updates section.
func (s *Service) CheckForUpdates() (dto.FollowState, error) {
	s.mu.Lock()
	var err error
	switch {
	case !s.updates.Supported():
		err = errors.New("Updates are off in development builds.")
	case s.runCtx == nil:
		err = errors.New("Team Follow is still starting. Try again in a moment.")
	case s.updates.Begin(true):
		s.startCheck(s.runCtx, true)
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// startWhatsNew decides What's new from the built-in changelog and the
// last version run, and saves the running version. With settings that
// couldn't be loaded it shows nothing and saves nothing, so the file is
// left for the player to fix. It runs from New.
func (s *Service) startWhatsNew(saved dto.Preferences, loaded bool) {
	changelog := update.ParseChangelog(s.opts.Changelog)
	if !loaded {
		s.whatsNew = update.NewWhatsNew(changelog, "", "", false)
		return
	}
	s.whatsNew = update.NewWhatsNew(changelog, s.opts.AppVersion, saved.LastVersion, saved.Stored)
	if view := s.whatsNew.View(); view != nil && view.Show {
		s.logger.Info("update.whats_new", slog.String("version", view.Version), slog.String("since", view.Since),
			slog.Int("sections", len(view.Changes)))
	}
	if !s.whatsNew.NeedsSave() {
		return
	}
	if err := s.save(); err != nil {
		s.logger.Warn("companion.preferences_save", slog.Any("error", err))
	}
}

// DismissWhatsNew closes What's new until the next update.
func (s *Service) DismissWhatsNew() dto.FollowState {
	s.mu.Lock()
	s.whatsNew.Dismiss()
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state
}

// Changelog is every released version in the built-in changelog, newest
// first.
func (s *Service) Changelog() []dto.ChangelogSection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.whatsNew.Changelog()
}

// UpdateLater hides the banner until a newer release is found or the
// player checks again. Nothing is saved.
func (s *Service) UpdateLater() dto.FollowState {
	s.mu.Lock()
	s.updates.Later()
	s.logger.Info("update.later")
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state
}

// SkipUpdate silences the release on offer for good and saves that, so it
// stays quiet after a restart; the next release is offered again.
func (s *Service) SkipUpdate() (dto.FollowState, error) {
	s.mu.Lock()
	version, err := s.updates.Skip()
	if err == nil {
		s.logger.Info("update.skip", slog.String("version", version))
		if saveErr := s.save(); saveErr != nil {
			s.logger.Error("companion.preferences_save", slog.Any("error", saveErr))
			s.prefsError = saveFailed
			err = errors.New(saveFailed)
		} else {
			s.prefsError = ""
		}
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}
