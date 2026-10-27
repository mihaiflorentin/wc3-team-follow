// Package companion runs Team Follow: it finds the game, keeps one session
// with its menu service, feeds game events to the follow engine and saves
// the friend selection.
package companion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
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
// Ports are the adapters the service drives.
type Ports struct {
	Discovery contract.GameDiscovery
	Dialer    contract.GameDialer
	Prefs     contract.PreferenceStore
	Maps      contract.MapsLocator
	Library   contract.MapLibrary
}

type Service struct {
	discovery contract.GameDiscovery
	dialer    contract.GameDialer
	prefs     contract.PreferenceStore
	maps      contract.MapsLocator
	library   contract.MapLibrary
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
	// mapsFolder is the folder the player chose, or "".
	mapsFolder string
	// lastStatus is the last status line written to the log.
	lastStatus string

	subMu       sync.Mutex
	subscribers map[int]func(dto.FollowState)
	nextSub     int
}

// New returns the service with the saved selection loaded. Follow starts
// off and preview mode on.
func New(ports Ports, logger *slog.Logger, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Service{
		discovery:   ports.Discovery,
		dialer:      ports.Dialer,
		prefs:       ports.Prefs,
		maps:        ports.Maps,
		library:     ports.Library,
		logger:      logger,
		opts:        opts,
		subscribers: map[int]func(dto.FollowState){},
	}
	s.engine = follow.NewEngine(s.send, opts.Now, opts.VerifiedBuilds)

	saved, err := s.prefs.Load()
	if err == nil {
		err = s.engine.Configure(dto.FollowConfig{Tags: &saved.Tags, IncludeTeam: &saved.IncludeTeam})
	}
	if err == nil && saved.Host != nil {
		err = s.engine.ConfigureHost(dto.HostConfig{Settings: saved.Host})
	}
	if err == nil {
		s.mapsFolder = saved.MapsFolder
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
			if ev.MessageType == "GameLobbySetup" {
				s.logLobby(ctx, ev)
			}
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
	state.MapsFolder = s.mapsFolder
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
		if saveErr := s.save(); saveErr != nil {
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

// save writes the friend selection, Maps folder and Auto-host setup. It
// runs with s.mu held.
func (s *Service) save() error {
	host := s.engine.HostSettings()
	return s.prefs.Save(dto.Preferences{
		Tags: s.engine.ManualTags(), IncludeTeam: s.engine.IncludeTeam(),
		MapsFolder: s.mapsFolder, Host: &host,
	})
}

// ConfigureHost changes the Auto-host setup or turns it on or off. Turning
// it on translates the map's path to the game's view (a Windows path under
// Wine or Proton).
func (s *Service) ConfigureHost(ctx context.Context, cfg dto.HostConfig) (dto.FollowState, error) {
	if cfg.Enabled != nil && *cfg.Enabled {
		s.mu.Lock()
		mapPath := s.engine.HostSettings().MapPath
		s.mu.Unlock()
		if cfg.Settings != nil {
			mapPath = cfg.Settings.MapPath
		}
		if mapPath != "" {
			gamePath, err := s.maps.GamePath(ctx, mapPath)
			if err != nil {
				return s.State(), fmt.Errorf("The map's path couldn't be prepared for the game: %w", err)
			}
			cfg.GameMapPath = gamePath
		}
	}
	s.mu.Lock()
	err := s.engine.ConfigureHost(cfg)
	s.logger.Info("host.configure", hostAttrs(cfg, err)...)
	if err == nil && cfg.Settings != nil {
		if saveErr := s.save(); saveErr != nil {
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

// hostAttrs describes an Auto-host change for the log.
func hostAttrs(cfg dto.HostConfig, err error) []any {
	var attrs []any
	if st := cfg.Settings; st != nil {
		attrs = append(attrs, slog.String("map", st.MapName), slog.String("map_path", st.MapPath), slog.String("game_name", st.GameName),
			slog.Bool("private", st.Private), slog.Int("visibility", st.Visibility), slog.Int("observers", st.Observers),
			slog.Int("delay_seconds", st.DelaySeconds), slog.Bool("start_when_full", st.StartWhenFull), slog.Bool("same_team", st.SameTeam))
	}
	if cfg.Enabled != nil {
		attrs = append(attrs, slog.Bool("enabled", *cfg.Enabled), slog.Bool("from_menus", cfg.FromMenus), slog.String("game_map_path", cfg.GameMapPath))
	}
	if err != nil {
		attrs = append(attrs, slog.String("refused", err.Error()))
	}
	return attrs
}

// MapsFolders returns the Maps folder the player chose, if any, then the
// ones found on this computer.
func (s *Service) MapsFolders(ctx context.Context) []dto.MapsFolder {
	s.mu.Lock()
	chosen := s.mapsFolder
	s.mu.Unlock()
	var out []dto.MapsFolder
	if chosen != "" {
		out = append(out, dto.MapsFolder{Path: chosen, Source: "Chosen by you"})
	}
	for _, f := range s.maps.MapsFolders(ctx) {
		if f.Path != chosen {
			out = append(out, f)
		}
	}
	return out
}

// SetMapsFolder makes path the Maps folder; "" goes back to detecting it.
func (s *Service) SetMapsFolder(path string) (dto.FollowState, error) {
	if path != "" {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return s.State(), errors.New("That folder doesn't exist on this computer.")
		}
	}
	s.mu.Lock()
	s.mapsFolder = path
	err := s.save()
	if err != nil {
		s.logger.Error("companion.preferences_save", slog.Any("error", err))
		err = errors.New(saveFailed)
	}
	s.logger.Info("maps.folder", slog.String("path", path))
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// currentMapsFolder is the chosen folder or the most likely detected one.
func (s *Service) currentMapsFolder(ctx context.Context) (string, error) {
	if folders := s.MapsFolders(ctx); len(folders) > 0 {
		return folders[0].Path, nil
	}
	return "", errors.New("No Warcraft III maps folder was found. Choose it yourself.")
}

// BrowseMaps lists one folder of the Maps folder.
func (s *Service) BrowseMaps(ctx context.Context, dir string) (dto.MapListing, error) {
	root, err := s.currentMapsFolder(ctx)
	if err != nil {
		return dto.MapListing{}, err
	}
	listing, err := s.library.Browse(ctx, root, dir)
	if err != nil {
		s.logger.WarnContext(ctx, "maps.browse", slog.String("root", root), slog.String("dir", dir), slog.Any("error", err))
		return listing, errors.New("That folder couldn't be read.")
	}
	return listing, nil
}

// SearchMaps finds maps by name in the whole Maps folder.
func (s *Service) SearchMaps(ctx context.Context, query string) (dto.MapListing, error) {
	root, err := s.currentMapsFolder(ctx)
	if err != nil {
		return dto.MapListing{}, err
	}
	listing, err := s.library.Search(ctx, root, query)
	if err != nil {
		s.logger.WarnContext(ctx, "maps.search", slog.String("root", root), slog.Any("error", err))
		return listing, errors.New("The search couldn't finish.")
	}
	return listing, nil
}

// logLobby writes Auto-host's view of the lobby, and the lobby update's
// field names, so the start rules can be checked from a log. It runs with
// s.mu held.
func (s *Service) logLobby(ctx context.Context, ev dto.GameEvent) {
	host := s.engine.Snapshot().Host
	if !host.Enabled || host.Phase != "lobby" {
		return
	}
	var playerFields []string
	if players, ok := ev.Payload["players"].([]any); ok && len(players) > 0 {
		if first, ok := players[0].(map[string]any); ok {
			playerFields = slices.Sorted(maps.Keys(first))
		}
	}
	s.logger.DebugContext(ctx, "host.lobby",
		slog.Int("players", host.Players), slog.Int("slots", host.Slots), slog.Bool("full", host.Full),
		slog.Any("missing", host.Missing), slog.Any("off_team", host.OffTeam),
		slog.Any("fields", slices.Sorted(maps.Keys(ev.Payload))), slog.Any("player_fields", playerFields))
}
