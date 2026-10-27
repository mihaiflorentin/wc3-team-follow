// Package companion runs Team Follow: it finds the game, keeps one session
// with its menu service, feeds game events to the follow engine and saves
// the friend selection.
package companion

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	tickInterval = time.Second
	sendTimeout  = 2 * time.Second
)

// quietEvents arrive several times a second while Custom Games is open and
// are left out of the log.
var quietEvents = map[string]bool{"GameListUpdate": true, "GameListRemove": true}

const (
	loadFailed = "Saved friends couldn't be loaded. Add them again to save a new list."
	saveFailed = "Your change couldn't be saved on this computer. Try again."
)

// Options tunes the service.
type Options struct {
	// VerifiedBuilds lists the game versions live joining is allowed on.
	VerifiedBuilds    []string
	RefreshInterval   time.Duration
	DiscoveryInterval time.Duration
	// Now reads the clock; nil uses time.Now.
	Now func() time.Time
}

// Service implements contract.Companion.
type Service struct {
	discovery contract.GameDiscovery
	dialer    contract.GameDialer
	prefs     contract.PreferenceStore
	logger    *slog.Logger
	opts      Options

	mu            sync.Mutex
	engine        *follow.Engine
	session       contract.GameSession
	connecting    bool
	nextDiscovery time.Time
	lastRefresh   time.Time
	prefsError    string
	hint          string
	// lastStatus is the last status line written to the log.
	lastStatus string

	subMu       sync.Mutex
	subscribers map[int]func(dto.FollowState)
	nextSub     int
}

// New returns the service with the saved selection loaded. Follow starts
// off and preview mode on.
func New(discovery contract.GameDiscovery, dialer contract.GameDialer, prefs contract.PreferenceStore, logger *slog.Logger, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Service{
		discovery:   discovery,
		dialer:      dialer,
		prefs:       prefs,
		logger:      logger,
		opts:        opts,
		subscribers: map[int]func(dto.FollowState){},
	}
	s.engine = follow.NewEngine(s.send, opts.Now, opts.VerifiedBuilds)

	saved, err := prefs.Load()
	if err == nil {
		err = s.engine.Configure(dto.FollowConfig{Tags: &saved.Tags, IncludeTeam: &saved.IncludeTeam})
	}
	if err != nil {
		logger.Warn("companion.preferences_load", slog.Any("error", err))
		s.prefsError = loadFailed
	}
	return s
}

// send is the engine's Sender. It runs with s.mu held.
func (s *Service) send(message string, payload map[string]any) bool {
	if s.session == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	// Only the command name is logged: payloads can hold a lobby password.
	if err := s.session.Send(ctx, message, payload); err != nil {
		s.logger.Warn("companion.send", slog.String("command", message), slog.Any("error", err))
		return false
	}
	s.logger.Debug("companion.send", slog.String("command", message))
	return true
}

// logStatus writes the engine's status line to the log when it changed. It
// runs with s.mu held.
func (s *Service) logStatus() {
	if status := s.engine.Status(); status != s.lastStatus {
		s.lastStatus = status
		s.logger.Info("follow.status", slog.String("status", status))
	}
}

// guard logs a panic in a background goroutine with its stack, then lets
// it crash the app as it would have anyway, so the log explains the crash.
func (s *Service) guard(where string) {
	if r := recover(); r != nil {
		s.logger.Error("companion.panic", slog.String("where", where), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
		panic(r)
	}
}

// Run drives the companion until ctx is cancelled, then switches Follow off
// and closes the game session.
func (s *Service) Run(ctx context.Context) error {
	defer s.guard("run")
	s.logger.InfoContext(ctx, "companion.run", slog.Any("verified_builds", s.opts.VerifiedBuilds),
		slog.String("refresh_interval", s.opts.RefreshInterval.String()), slog.String("discovery_interval", s.opts.DiscoveryInterval.String()))
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	s.step(ctx)
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			off := false
			_ = s.engine.Configure(dto.FollowConfig{Enabled: &off})
			s.logStatus()
			s.logger.Info("companion.stop")
			if s.session != nil {
				_ = s.session.Close()
				s.session = nil
			}
			s.mu.Unlock()
			return nil
		case <-ticker.C:
			s.step(ctx)
		}
	}
}

// step runs once a second: timers, roster refresh and discovery.
func (s *Service) step(ctx context.Context) {
	s.mu.Lock()
	s.engine.Tick()
	s.logStatus()
	s.refresh()
	start := s.session == nil && !s.connecting && !s.opts.Now().Before(s.nextDiscovery)
	if start {
		s.connecting = true
		s.nextDiscovery = s.opts.Now().Add(s.opts.DiscoveryInterval)
	}
	s.mu.Unlock()
	if start {
		go s.connect(ctx)
	}
	s.notify()
}

// refresh asks the game for the team and friends. It runs with s.mu held.
func (s *Service) refresh() {
	if s.session == nil || s.opts.Now().Sub(s.lastRefresh) < s.opts.RefreshInterval {
		return
	}
	s.lastRefresh = s.opts.Now()
	s.send(follow.CmdTeamGetInfo, map[string]any{})
	s.send(follow.CmdFriendsGetFriends, map[string]any{})
}

func (s *Service) connect(ctx context.Context) {
	defer s.guard("connect")
	conn, err := s.discovery.Discover(ctx)
	var session contract.GameSession
	if err == nil {
		session, err = s.dialer.Dial(ctx, conn)
	}
	conn.Session = ""

	s.mu.Lock()
	s.connecting = false
	if err != nil || ctx.Err() != nil {
		if session != nil {
			_ = session.Close()
		}
		hint := playerMessage(err)
		if hint != s.hint {
			// Log each new reason once; the same reason repeats every few seconds.
			s.logger.InfoContext(ctx, "companion.waiting", slog.String("hint", hint), slog.Any("error", err))
		}
		s.hint = hint
		if s.engine.Connected() {
			s.engine.Connection(false, "")
			s.logStatus()
		}
		s.mu.Unlock()
		s.logger.DebugContext(ctx, "companion.discover", slog.Any("error", err))
		s.notify()
		return
	}
	s.session, s.hint = session, ""
	s.engine.Connection(true, conn.Version)
	s.logStatus()
	s.lastRefresh = time.Time{}
	s.refresh()
	s.mu.Unlock()
	s.logger.InfoContext(ctx, "companion.connected", slog.String("version", conn.Version), slog.Int("pid", conn.PID),
		slog.Int("port", conn.Port), slog.Bool("verified_build", slices.Contains(s.opts.VerifiedBuilds, conn.Version)))
	s.notify()
	go s.read(ctx, session)
}

// playerMessage returns the outermost error text written for players
// (discovery's errors are sentences starting with a capital letter), or a
// generic hint for internal errors.
func playerMessage(err error) string {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if t := e.Error(); t != "" && t[0] >= 'A' && t[0] <= 'Z' {
			return t
		}
	}
	return "Waiting for Warcraft III."
}

func (s *Service) read(ctx context.Context, session contract.GameSession) {
	defer s.guard("read")
	for {
		ev, err := session.Receive(ctx)
		s.mu.Lock()
		if s.session != session {
			s.mu.Unlock()
			return
		}
		if err != nil {
			_ = session.Close()
			s.session = nil
			s.engine.Connection(false, "")
			s.logStatus()
			s.mu.Unlock()
			s.logger.InfoContext(ctx, "companion.disconnected", slog.Any("error", err))
			s.notify()
			return
		}
		if ev.Malformed {
			s.logger.WarnContext(ctx, "companion.event_malformed")
			off := false
			_ = s.engine.Configure(dto.FollowConfig{Enabled: &off})
			s.engine.Note("The game sent something unrecognized. Follow is off.")
		} else {
			// The type only: payloads hold friends' details. The custom game
			// list streams several updates a second; diagnostics count them.
			if screen, ok := ev.Payload["screen"].(string); ok && ev.MessageType == "SetGlueScreen" {
				s.logger.InfoContext(ctx, "companion.screen", slog.String("screen", screen))
			} else if !quietEvents[ev.MessageType] {
				s.logger.DebugContext(ctx, "companion.event", slog.String("type", ev.MessageType))
			}
			s.engine.Receive(ev)
		}
		s.logStatus()
		s.mu.Unlock()
		s.notify()
	}
}

// State returns the current snapshot.
func (s *Service) State() dto.FollowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Service) snapshot() dto.FollowState {
	state := s.engine.Snapshot()
	state.PreferencesError = s.prefsError
	if !state.Connected {
		state.ConnectionHint = s.hint
	}
	return state
}

// Configure applies cfg and saves the selection when it changed.
func (s *Service) Configure(cfg dto.FollowConfig) (dto.FollowState, error) {
	s.mu.Lock()
	err := s.engine.Configure(cfg)
	s.logger.Info("follow.configure", configAttrs(cfg, err)...)
	if err == nil && (cfg.Tags != nil || cfg.IncludeTeam != nil) {
		if saveErr := s.prefs.Save(dto.Preferences{Tags: s.engine.ManualTags(), IncludeTeam: s.engine.IncludeTeam()}); saveErr != nil {
			s.logger.Error("companion.preferences_save", slog.Any("error", saveErr))
			s.prefsError = saveFailed
			err = errors.New(saveFailed)
		} else {
			s.prefsError = ""
		}
	}
	s.logStatus()
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// configAttrs describes a configuration change for the log.
func configAttrs(cfg dto.FollowConfig, err error) []any {
	var attrs []any
	if cfg.Tags != nil {
		attrs = append(attrs, slog.Any("tags", *cfg.Tags))
	}
	if cfg.IncludeTeam != nil {
		attrs = append(attrs, slog.Bool("include_team", *cfg.IncludeTeam))
	}
	if cfg.Diagnostic != nil {
		attrs = append(attrs, slog.Bool("preview", *cfg.Diagnostic))
	}
	if cfg.Enabled != nil {
		attrs = append(attrs, slog.Bool("enabled", *cfg.Enabled), slog.Bool("from_menus", cfg.FromMenus))
	}
	if err != nil {
		attrs = append(attrs, slog.String("refused", err.Error()))
	}
	return attrs
}

// SubmitPassword answers the game's lobby password request. The password
// is never logged.
func (s *Service) SubmitPassword(password string) (dto.FollowState, error) {
	s.mu.Lock()
	err := s.engine.SubmitPassword(password)
	if err != nil {
		s.logger.Info("follow.password", slog.String("refused", err.Error()))
	} else {
		s.logger.Info("follow.password", slog.String("result", "sent to the game"))
	}
	s.logStatus()
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// Diagnostics returns the shareable report.
func (s *Service) Diagnostics() dto.Diagnostics {
	s.mu.Lock()
	report := s.engine.Diagnostics()
	s.mu.Unlock()
	report.Platform = runtime.GOOS + "/" + runtime.GOARCH
	return report
}

// Subscribe registers fn for state changes.
func (s *Service) Subscribe(fn func(dto.FollowState)) func() {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	id := s.nextSub
	s.nextSub++
	s.subscribers[id] = fn
	return func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		delete(s.subscribers, id)
	}
}

func (s *Service) notify() {
	state := s.State()
	s.subMu.Lock()
	fns := make([]func(dto.FollowState), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		fns = append(fns, fn)
	}
	s.subMu.Unlock()
	for _, fn := range fns {
		fn(state)
	}
}
