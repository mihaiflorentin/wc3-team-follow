package companion

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/domain/mapupdate"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	// defaultReplayPoll is how often the game's last replay is looked at
	// after a match, and defaultReplayWait how long it is waited for.
	defaultReplayPoll = 2 * time.Second
	defaultReplayWait = 2 * time.Minute
	// defaultFinalRound is the last part of a match nobody is judged in.
	defaultFinalRound = 4 * time.Minute
)

// matchSeen follows the engine's view of the match around one game event:
// it notes when a match began and, when one ends, starts looking for its
// leavers (ADR 0013). It runs with s.mu held.
func (s *Service) matchSeen(ctx context.Context, wasInMatch bool) {
	inMatch := s.engine.InMatch()
	switch {
	case !wasInMatch && inMatch:
		s.matchStarted = s.opts.Now()
	case wasInMatch && !inMatch:
		started := s.matchStarted
		s.matchStarted = time.Time{}
		s.startLeavers(ctx, started)
	}
}

// startLeavers waits for the replay of the match that began at started in
// its own goroutine, off s.mu, when Blacklist players who leave Legion TD
// early is on; Run waits for it. It runs with s.mu held.
func (s *Service) startLeavers(ctx context.Context, started time.Time) {
	if !s.engine.BlacklistLeavers() || started.IsZero() || s.leaversBusy || s.stopping || s.replays == nil || s.replayFiles == nil {
		return
	}
	s.leaversBusy = true
	s.checks.Go(func() {
		defer s.guard("leavers")
		defer func() {
			s.mu.Lock()
			s.leaversBusy = false
			s.mu.Unlock()
		}()
		s.checkLeavers(ctx, started)
	})
}

// checkLeavers finds the match's replay, reads it and blacklists its early
// leavers. Only the newest LastReplay.w3g is read, once, and only when it
// changed after the match began and stopped growing. Nothing about the
// players, the chat or the file's path is logged.
func (s *Service) checkLeavers(ctx context.Context, started time.Time) {
	file, ok := s.waitForReplay(ctx, started)
	if !ok {
		s.logger.InfoContext(ctx, "leavers.no_replay")
		return
	}
	key := fmt.Sprintf("%s|%d|%d", file.Path, file.Modified.UnixNano(), file.Size)
	s.mu.Lock()
	seen := s.lastReplay == key
	s.lastReplay = key
	s.mu.Unlock()
	if seen {
		return
	}
	replay, err := s.replays.ReadFile(ctx, file.Path)
	if err != nil {
		s.logger.WarnContext(ctx, "leavers.read_failed", slog.String("error", withoutPath(err)))
		return
	}
	mapPath := replay.Map.Path
	if mapPath == "" {
		mapPath = replay.Map.File
	}
	i, _ := mapupdate.Match(s.opts.TrackedMaps, mapPath)
	if i < 0 || s.opts.LeaverMap == "" || !strings.EqualFold(s.opts.TrackedMaps[i].Name, s.opts.LeaverMap) {
		s.logger.InfoContext(ctx, "leavers.skipped", slog.String("reason", "not the map"))
		return
	}
	mapName := s.opts.TrackedMaps[i].Name
	report := follow.EarlyLeavers(replay, s.opts.LeaverFinalRound)

	s.mu.Lock()
	if !s.engine.BlacklistLeavers() {
		s.mu.Unlock()
		return
	}
	if !playedIn(replay, s.engine.SelfTag()) {
		s.mu.Unlock()
		s.logger.InfoContext(ctx, "leavers.skipped", slog.String("reason", "not your match"))
		return
	}
	added := s.engine.AddLeavers(report, mapName)
	if s.engine.TakeBlacklistChanged() {
		if err := s.save(); err != nil {
			s.logger.ErrorContext(ctx, "companion.preferences_save", slog.Any("error", err))
			s.prefsError = saveFailed
		} else {
			s.prefsError = ""
		}
	}
	s.noteLeavers(ctx, added)
	s.logStatus()
	s.mu.Unlock()
	s.logger.InfoContext(ctx, "leavers.added", slog.Int("count", len(added)), slog.Int("found", len(report.Leavers)),
		slog.String("ending", report.Ending))
	s.notify()
}

// noteLeavers writes each new leaver's reason into their player note: as
// the note, or on a new line of an existing one when it fits; a note it
// doesn't fit is left alone. It runs with s.mu held.
func (s *Service) noteLeavers(ctx context.Context, added []dto.BlacklistEntry) {
	notes := maps.Clone(s.notes)
	changed := false
	for _, b := range added {
		key := follow.NoteKey(b.Tag)
		if text, ok := follow.AppendNote(notes[key].Text, b.Reason); ok {
			notes[key] = dto.PlayerNote{Text: text, Updated: s.opts.Now()}
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := s.notesDB.Save(notes); err != nil {
		s.logger.ErrorContext(ctx, "notes.save", slog.Any("error", err))
		return
	}
	s.notes = notes
	s.logger.InfoContext(ctx, "notes.saved", slog.Int("count", len(notes)))
}

// waitForReplay waits for the game's last replay to be written after
// started and to stop changing between two looks, polling every
// ReplayPoll for at most ReplayWait.
func (s *Service) waitForReplay(ctx context.Context, started time.Time) (dto.ReplayFile, bool) {
	ticker := time.NewTicker(s.opts.ReplayPoll)
	defer ticker.Stop()
	deadline := time.NewTimer(s.opts.ReplayWait)
	defer deadline.Stop()
	var last dto.ReplayFile
	for {
		select {
		case <-ctx.Done():
			return dto.ReplayFile{}, false
		case <-deadline.C:
			return dto.ReplayFile{}, false
		case <-ticker.C:
		}
		file, err := s.replayFiles.LastReplay(ctx)
		if err != nil || file.Size <= 0 || file.Modified.Before(started) {
			last = dto.ReplayFile{}
			continue
		}
		if file.Path == last.Path && file.Size == last.Size && file.Modified.Equal(last.Modified) {
			return file, true
		}
		last = file
	}
}

// playedIn reports whether self, the player's BattleTag, is a human player
// of the replay.
func playedIn(r dto.Replay, self string) bool {
	self = strings.TrimSpace(self)
	if self == "" {
		return false
	}
	for _, p := range r.Players {
		if !p.Computer && !p.Observer && strings.EqualFold(strings.TrimSpace(p.BattleTag), self) {
			return true
		}
	}
	return false
}

// withoutPath is err's text without a file path, which would show the
// Battle.net account folder.
func withoutPath(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Op + ": " + pathErr.Err.Error()
	}
	return err.Error()
}
