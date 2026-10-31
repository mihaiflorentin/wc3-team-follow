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
	"github.com/mihaiflorentin/wc3-team-follow/domain/mapupdate"
	"github.com/mihaiflorentin/wc3-team-follow/domain/update"
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
	// recentSaveEvery is the shortest time between saves of the recent
	// players after one was added or changed; recentTouchEvery after only
	// last-seen times moved, as they do each second during a match.
	recentSaveEvery  = 5 * time.Second
	recentTouchEvery = time.Minute
)

const (
	clearFailed        = "Recent players couldn't be cleared on this computer. Try again."
	clearRankingFailed = "The scoreboard couldn't be cleared on this computer. Try again."
	noteFailed         = "Your note couldn't be saved on this computer. Try again."
	loadFailed         = "Saved friends couldn't be loaded. Add them again to save a new list."
	saveFailed         = "Your change couldn't be saved on this computer. Try again."
)

// Options tunes the service.
type Options struct {
	// VerifiedBuilds lists the game versions live joining is allowed on.
	VerifiedBuilds    []string
	RefreshInterval   time.Duration
	DiscoveryInterval time.Duration
	// AppVersion is the running build's version; only a clean release
	// version checks for updates.
	AppVersion string
	// UpdateInterval is the time between automatic update checks, and
	// UpdateRetryInterval the wait after a failed one; zero uses 6 hours
	// and 30 minutes.
	UpdateInterval      time.Duration
	UpdateRetryInterval time.Duration
	// ReleasesURL is the releases page, for updating by hand.
	ReleasesURL string
	// System (runtime.GOOS) and Executable (this program's path) say what
	// the player redoes after an update; Updated marks the first start
	// after one.
	System     string
	Executable string
	Updated    bool
	// Changelog is CHANGELOG.md as built into the app, for What's new and
	// the changelog in the Updates section.
	Changelog string
	// RecordRanking turns on recording wins and losses. It is off in the
	// app until results can be trusted: some custom maps report every
	// player as a winner.
	RecordRanking bool
	// TrackedMaps are the custom maps kept current from their GitHub
	// Releases (ADR 0012); MapInterval and MapRetryInterval time their
	// checks, zero using 6 hours and 30 minutes.
	TrackedMaps      []dto.TrackedMap
	MapInterval      time.Duration
	MapRetryInterval time.Duration
	// LeaverMap names the tracked map whose early leavers may be
	// blacklisted ("Legion TD", ADR 0013); LeaverFinalRound is the last
	// part of a match nobody is judged in, zero using 4 minutes.
	LeaverMap        string
	LeaverFinalRound time.Duration
	// ReplayPoll and ReplayWait time the wait for a match's replay; zero
	// uses 2 seconds and 2 minutes.
	ReplayPoll time.Duration
	ReplayWait time.Duration
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
	// Processes closes the game when the player asks for it.
	Processes contract.GameProcesses
	// Keyboard types Game mode's command in the match chat. It is required;
	// on a system that can't type, its Status says so.
	Keyboard contract.GameKeyboard
	// Recent keeps the players seen in a lobby or match with the player.
	Recent contract.RecentPlayers
	// Ranking keeps the wins and losses recorded for the group.
	Ranking contract.Ranking
	// Updater finds a newer release of Team Follow.
	Updater contract.Updater
	// Notes keeps the player's private notes about other players.
	Notes contract.PlayerNotes
	// MapReleases finds and downloads tracked maps' newest releases.
	MapReleases contract.MapReleases
	// Replays reads a match's replay, and ReplayFiles finds it, for
	// blacklisting early leavers (ADR 0013).
	Replays     contract.ReplayReader
	ReplayFiles contract.ReplayLocator
}

type Service struct {
	discovery contract.GameDiscovery
	dialer    contract.GameDialer
	prefs     contract.PreferenceStore
	maps      contract.MapsLocator
	library   contract.MapLibrary
	processes contract.GameProcesses
	keyboard  contract.GameKeyboard
	recentDB  contract.RecentPlayers
	rankingDB contract.Ranking
	updater   contract.Updater
	notesDB   contract.PlayerNotes
	// mapReleases finds and downloads tracked maps' releases.
	mapReleases contract.MapReleases
	replays     contract.ReplayReader
	replayFiles contract.ReplayLocator
	logger      *slog.Logger
	opts        Options

	mu      sync.Mutex
	engine  *follow.Engine
	recent  *follow.Recent
	ranking *follow.Ranking
	// updates decides when to check for a newer release and what to offer.
	updates *update.Checker
	// whatsNew is What's new in the running version.
	whatsNew *update.WhatsNew
	// runCtx is Run's context, for checks the player starts; nil until Run.
	runCtx context.Context
	// checks tracks running update checks and installs; Run waits for them.
	// stopping is set, under s.mu, once Run is shutting down: nothing joins
	// checks after that, so it can't race with Run's Wait.
	checks   sync.WaitGroup
	stopping bool
	// cancelInstall stops the running update's download, or is nil.
	cancelInstall context.CancelFunc
	// mapUpdates decides when to check tracked maps and what to download;
	// cancelMaps stops the running map check and its download, or is nil.
	mapUpdates *mapupdate.Planner
	cancelMaps context.CancelFunc
	// recentSavedAt is when the recent players were last saved.
	recentSavedAt time.Time
	// notes are the player's notes, keyed by follow.NoteKey.
	notes         map[string]dto.PlayerNote
	session       contract.GameSession
	connecting    bool
	nextDiscovery time.Time
	lastRefresh   time.Time
	prefsError    string
	hint          string
	// gameCount is how many copies of the game discovery last found, when
	// more than one.
	gameCount int
	// closing is set while CloseGames runs; discovery waits for it.
	closing bool
	// mapsFolder is the folder the player chose, or "".
	mapsFolder string
	// hostMap is the file Auto-host creates its next lobby on (issue 32);
	// hostMapStale asks for it to be worked out again, and hostMapBusy is
	// set while that runs.
	hostMap      dto.HostMap
	hostMapStale bool
	hostMapBusy  bool
	// matchStarted is when the current match began, or zero outside one;
	// leaversBusy is set while a match's replay is waited for, and
	// lastReplay identifies the last replay read, so each is read once.
	matchStarted time.Time
	leaversBusy  bool
	lastReplay   string
	// lastStatus is the last status line written to the log.
	lastStatus string
	// gamePID is the connected game's process id, for typing.
	gamePID int
	// keyboardStatus is the keyboard's last reported status.
	keyboardStatus dto.KeyboardStatus
	// typing tracks the goroutines typing Game mode's lines; Run waits for
	// them.
	typing sync.WaitGroup

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
		processes:   ports.Processes,
		keyboard:    ports.Keyboard,
		recentDB:    ports.Recent,
		rankingDB:   ports.Ranking,
		updater:     ports.Updater,
		notesDB:     ports.Notes,
		mapReleases: ports.MapReleases,
		replays:     ports.Replays,
		replayFiles: ports.ReplayFiles,
		logger:      logger,
		opts:        opts,
		subscribers: map[int]func(dto.FollowState){},
		// The saved setup's map is worked out once Run starts.
		hostMapStale: true,
	}
	s.engine = follow.NewEngine(s.send, opts.Now, opts.VerifiedBuilds)
	recent, err := s.recentDB.Load()
	if err != nil {
		logger.Warn("recent.load", slog.Any("error", err))
		recent = nil
	}
	s.recent = follow.NewRecent(opts.Now, recent)
	ranked, err := s.rankingDB.Load()
	if err != nil {
		logger.Warn("ranking.load", slog.Any("error", err))
		ranked = nil
	}
	s.ranking = follow.NewRanking(opts.Now, ranked, logger)
	notes, err := s.notesDB.Load()
	if err != nil || notes == nil {
		logger.Warn("notes.load", slog.Any("error", err))
		notes = map[string]dto.PlayerNote{}
	}
	s.notes = notes

	if opts.UpdateInterval <= 0 {
		opts.UpdateInterval = 6 * time.Hour
	}
	if opts.UpdateRetryInterval <= 0 {
		opts.UpdateRetryInterval = 30 * time.Minute
	}
	if opts.LeaverFinalRound <= 0 {
		opts.LeaverFinalRound = defaultFinalRound
	}
	if opts.ReplayPoll <= 0 {
		opts.ReplayPoll = defaultReplayPoll
	}
	if opts.ReplayWait <= 0 {
		opts.ReplayWait = defaultReplayWait
	}
	s.opts = opts

	saved, err := s.prefs.Load()
	checkUpdates, skipped, mapUpdates := true, "", true
	if err == nil {
		checkUpdates, skipped, mapUpdates = saved.CheckUpdates, saved.SkippedVersion, saved.MapUpdates
	}
	s.mapUpdates = mapupdate.New(mapupdate.Options{Interval: opts.MapInterval, RetryInterval: opts.MapRetryInterval, Now: opts.Now},
		opts.TrackedMaps, mapUpdates)
	s.updates = update.New(update.Options{Version: opts.AppVersion, Interval: opts.UpdateInterval, RetryInterval: opts.UpdateRetryInterval,
		ReleasesURL: opts.ReleasesURL, Now: opts.Now, System: opts.System, Executable: opts.Executable, Updated: opts.Updated},
		checkUpdates, skipped)
	if err == nil {
		err = s.engine.Configure(dto.FollowConfig{Tags: &saved.Tags, IncludeTeam: &saved.IncludeTeam, AskPassword: &saved.AskPassword,
			JoinHostTeam: &saved.JoinHostTeam})
	}
	if err == nil {
		// After the group, so a player on both lists leaves the group.
		err = s.engine.Configure(dto.FollowConfig{Blacklist: &saved.Blacklist, AutoBan: &saved.AutoBan,
			BlacklistManualBans: &saved.BlacklistManualBans, BlacklistLeavers: &saved.BlacklistLeavers})
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
	s.startWhatsNew(saved, err == nil)
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
	s.mu.Lock()
	s.runCtx = ctx
	s.mu.Unlock()
	s.cleanupUpdate(ctx)
	s.step(ctx)
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.stopping = true
			off := false
			_ = s.engine.Configure(dto.FollowConfig{Enabled: &off})
			s.logStatus()
			s.logger.Info("companion.stop")
			s.dropSession()
			s.recent.End()
			s.ranking.End()
			s.saveRecent(true)
			s.saveRanking()
			s.mu.Unlock()
			// Typing stops with ctx; wait for it to report, so nothing
			// runs after Run returns.
			s.typing.Wait()
			s.checks.Wait()
			return nil
		case <-ticker.C:
			s.step(ctx)
		}
	}
}

// dropSession closes the game session, if any, and forgets the game's
// process, so nothing is typed into it. It runs with s.mu held.
func (s *Service) dropSession() {
	if s.session != nil {
		_ = s.session.Close()
	}
	s.session, s.gamePID = nil, 0
}

// step runs once a second: timers, roster refresh and discovery.
func (s *Service) step(ctx context.Context) {
	keyboard := s.keyboard.Status(ctx)
	s.mu.Lock()
	s.keyboardStatus = keyboard
	s.engine.Tick()
	s.startHostMap(ctx)
	s.recent.Tick()
	s.saveRanking()
	s.saveRecent(false)
	s.typeChatLines(ctx)
	s.logStatus()
	s.refresh()
	if s.updates.Due() && s.updates.Begin(false) {
		s.startCheck(ctx, false)
	}
	if s.mapUpdates.Due() && s.mapUpdates.Begin(false) {
		s.startMapCheck(ctx, false)
	}
	start := s.session == nil && !s.connecting && !s.closing && !s.opts.Now().Before(s.nextDiscovery)
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
		s.hint, s.gameCount = hint, gameCount(err)
		if s.engine.Connected() {
			s.engine.Connection(false, "")
			s.logStatus()
		}
		s.mu.Unlock()
		s.logger.DebugContext(ctx, "companion.discover", slog.Any("error", err))
		s.notify()
		return
	}
	s.session, s.hint, s.gameCount, s.gamePID = session, "", 0, conn.PID
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
	return playerSentence(err, "Waiting for Warcraft III.")
}

// gameCount returns how many copies of the game a discovery error says are
// running, or 0 when it doesn't say.
func gameCount(err error) int {
	var counter contract.GameCounter
	if errors.As(err, &counter) {
		return counter.GameCount()
	}
	return 0
}

func (s *Service) read(ctx context.Context, session contract.GameSession) {
	defer s.guard("read")
	for {
		// Wake at least every readWake, so a start Auto-host wants to
		// press again goes out from here, in order with the events.
		wait, cancel := context.WithTimeout(ctx, readWake)
		ev, err := session.Receive(wait)
		woke := err != nil && wait.Err() != nil && ctx.Err() == nil
		cancel()
		s.mu.Lock()
		if s.session != session {
			s.mu.Unlock()
			return
		}
		if woke {
			due := s.engine.HasPendingStart()
			if s.sendStart(ctx, session) {
				s.signedOut(ctx)
				return
			}
			if due {
				s.logStatus()
			}
			s.mu.Unlock()
			if due {
				s.notify()
			}
			continue
		}
		if err != nil {
			s.dropSession()
			s.engine.Connection(false, "")
			s.recent.End()
			s.ranking.End()
			s.logStatus()
			s.mu.Unlock()
			s.logger.InfoContext(ctx, "companion.disconnected", slog.Any("error", err))
			s.notify()
			return
		}
		if s.handle(ctx, ev) || s.sendBan(ctx, session) || s.sendStart(ctx, session) {
			s.signedOut(ctx)
			return
		}
		s.logStatus()
		s.mu.Unlock()
		s.notify()
	}
}

// readWake is how long read waits for an event before checking whether
// Auto-host wants to press Start again.
const readWake = time.Second

// signedOut drops a game that signed out, often because the same account
// signed in from another copy, so discovery finds the game again. It is
// called with s.mu held and unlocks it.
func (s *Service) signedOut(ctx context.Context) {
	s.dropSession()
	s.nextDiscovery = time.Time{}
	s.logStatus()
	s.mu.Unlock()
	s.logger.InfoContext(ctx, "companion.disconnected", slog.String("reason", "logged out"))
	s.notify()
}

// handle applies one event from the game. It reports true when the game
// signed out. It runs with s.mu held.
func (s *Service) handle(ctx context.Context, ev dto.GameEvent) (loggedOut bool) {
	if ev.Malformed {
		s.logger.WarnContext(ctx, "companion.event_malformed")
		off := false
		_ = s.engine.Configure(dto.FollowConfig{Enabled: &off})
		s.engine.Note("The game sent something unrecognized. Follow is off.")
		return false
	}
	// The type only: payloads hold friends' details. The custom game list
	// streams several updates a second; diagnostics count them.
	if screen, ok := ev.Payload["screen"].(string); ok && ev.MessageType == "SetGlueScreen" {
		s.logger.InfoContext(ctx, "companion.screen", slog.String("screen", screen))
	} else if !quietEvents[ev.MessageType] {
		s.logger.DebugContext(ctx, "companion.event", slog.String("type", ev.MessageType))
	}
	wasInMatch := s.engine.InMatch()
	s.engine.Receive(ev)
	s.matchSeen(ctx, wasInMatch)
	for _, warning := range s.engine.TakeBanMistakes() {
		s.logger.WarnContext(ctx, "follow.ban_mistake", slog.String("warning", warning))
	}
	for _, sender := range s.engine.TakeIgnoredBanLines() {
		// The sender only (a display name or BattleTag), never the line.
		s.logger.WarnContext(ctx, "follow.ban_line_ignored", slog.String("sender", sender))
	}
	if s.engine.TakeBlacklistChanged() {
		// A player banned by hand joined the blacklist.
		if err := s.save(); err != nil {
			s.logger.ErrorContext(ctx, "companion.preferences_save", slog.Any("error", err))
			s.prefsError = saveFailed
		} else {
			s.prefsError = ""
		}
	}
	s.recent.Receive(ev)
	if s.opts.RecordRanking {
		s.ranking.Receive(ev)
		s.saveRanking()
	}
	s.typeChatLines(ctx)
	if ev.MessageType == "GameLobbySetup" {
		s.logLobby(ctx, ev)
	}
	return ev.MessageType == "LoggedOut"
}

// sendBan sends the ban the engine chose, if any (docs/adr/0008). The
// game's Ban takes only a slot, so every event already waiting on the
// socket is handled first, without waiting for more, and the engine checks
// its choice against the newest lobby update before sending. There is no
// safe way to ask the game for the lobby (GetGameInfo and
// SendGameLobbySetup closed the game), so the waiting events are the
// newest it has. It reports true when the game signed out meanwhile. It
// runs with s.mu held.
func (s *Service) sendBan(ctx context.Context, session contract.GameSession) (loggedOut bool) {
	for s.engine.HasPendingBan() {
		for {
			ev, ok, err := session.TryReceive()
			if err != nil || !ok {
				break
			}
			if s.handle(ctx, ev) {
				return true
			}
		}
		s.engine.SendPendingBan()
	}
	return false
}

// sendStart presses Start again when Auto-host's start went unanswered.
// Like sendBan, it first handles every event already waiting on the
// socket, so the engine checks the newest lobby update: a player who left
// meanwhile stops the start. It reports true when the game signed out
// meanwhile. It runs with s.mu held.
func (s *Service) sendStart(ctx context.Context, session contract.GameSession) (loggedOut bool) {
	if !s.engine.HasPendingStart() {
		return false
	}
	for {
		ev, ok, err := session.TryReceive()
		if err != nil || !ok {
			break
		}
		if s.handle(ctx, ev) || s.sendBan(ctx, session) {
			return true
		}
	}
	s.engine.SendPendingStart()
	return false
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
	state.Keyboard = s.keyboardStatus
	state.PreferencesError = s.prefsError
	state.Recent = s.recent.Players()
	state.Ranking = s.ranking.Entries()
	state.LobbyTags = s.recent.Present()
	state.Update = s.updates.State(update.Activity{InLobby: state.Busy && !state.InMatch, InMatch: state.InMatch,
		AutoJoin: state.Enabled, AutoHost: state.Host.Enabled})
	state.Update.WhatsNew = s.whatsNew.View()
	state.MapUpdates = s.mapUpdates.State()
	state.Host.Map = s.hostMap
	state.Notes = maps.Clone(s.notes)
	if !state.Connected {
		state.ConnectionHint = s.hint
		state.GameCount = s.gameCount
	}
	return state
}

// Configure applies cfg and saves the selection when it changed.
func (s *Service) Configure(cfg dto.FollowConfig) (dto.FollowState, error) {
	s.mu.Lock()
	err := s.engine.Configure(cfg)
	if err == nil && cfg.CheckUpdates != nil {
		s.updates.SetAutomatic(*cfg.CheckUpdates)
	}
	if err == nil && cfg.MapUpdates != nil {
		s.setMapUpdates(*cfg.MapUpdates)
	}
	s.logger.Info("follow.configure", configAttrs(cfg, err)...)
	if err == nil && (cfg.Tags != nil || cfg.CheckUpdates != nil || cfg.MapUpdates != nil || cfg.IncludeTeam != nil || cfg.AskPassword != nil || cfg.JoinHostTeam != nil ||
		cfg.Blacklist != nil || cfg.BlacklistAdd != nil || cfg.BlacklistRemove != nil || cfg.AutoBan != nil ||
		cfg.BlacklistManualBans != nil || cfg.BlacklistLeavers != nil) {
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
	if cfg.AskPassword != nil {
		attrs = append(attrs, slog.Bool("ask_password", *cfg.AskPassword))
	}
	if cfg.JoinHostTeam != nil {
		attrs = append(attrs, slog.Bool("join_host_team", *cfg.JoinHostTeam))
	}
	if cfg.IncludeTeam != nil {
		attrs = append(attrs, slog.Bool("include_team", *cfg.IncludeTeam))
	}
	if cfg.BlacklistAdd != nil {
		attrs = append(attrs, slog.Int("blacklist_add", len(cfg.BlacklistAdd)))
	}
	if cfg.BlacklistRemove != nil {
		attrs = append(attrs, slog.Int("blacklist_remove", len(cfg.BlacklistRemove)))
	}
	if cfg.AutoBan != nil {
		attrs = append(attrs, slog.Bool("auto_ban", *cfg.AutoBan))
	}
	if cfg.BlacklistManualBans != nil {
		attrs = append(attrs, slog.Bool("blacklist_manual_bans", *cfg.BlacklistManualBans))
	}
	if cfg.BlacklistLeavers != nil {
		attrs = append(attrs, slog.Bool("blacklist_leavers", *cfg.BlacklistLeavers))
	}
	if cfg.CheckUpdates != nil {
		attrs = append(attrs, slog.Bool("check_updates", *cfg.CheckUpdates))
	}
	if cfg.MapUpdates != nil {
		attrs = append(attrs, slog.Bool("map_updates", *cfg.MapUpdates))
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
		Tags: s.engine.ManualTags(), IncludeTeam: s.engine.IncludeTeam(), AskPassword: s.engine.AskPassword(),
		JoinHostTeam: s.engine.JoinHostTeam(),
		MapsFolder:   s.mapsFolder, Host: &host,
		Blacklist: s.engine.Blacklist(), AutoBan: s.engine.AutoBan(), BlacklistManualBans: s.engine.BlacklistManualBans(),
		BlacklistLeavers: s.engine.BlacklistLeavers(),
		CheckUpdates:     s.updates.Automatic(), SkippedVersion: s.updates.SkippedVersion(), MapUpdates: s.mapUpdates.Enabled(),
		LastVersion: s.whatsNew.LastVersion(),
	})
}

// ConfigureHost changes the Auto-host setup or turns it on or off. It works
// out the file to host: the newest installed version of a tracked map with
// Use the newest installed version on, otherwise the chosen file (issue
// 32). Turning it on translates that file's path to the game's view (a
// Windows path under Wine or Proton).
func (s *Service) ConfigureHost(ctx context.Context, cfg dto.HostConfig) (dto.FollowState, error) {
	var hostMap *dto.HostMap
	if cfg.Settings != nil || cfg.Enabled != nil && *cfg.Enabled {
		s.mu.Lock()
		settings := s.engine.HostSettings()
		s.mu.Unlock()
		if cfg.Settings != nil {
			settings = *cfg.Settings
		}
		m := s.HostMap(ctx, settings)
		hostMap = &m
		if cfg.Enabled != nil && *cfg.Enabled && m.Path != "" {
			gamePath, err := s.maps.GamePath(ctx, m.Path)
			if err != nil {
				return s.State(), fmt.Errorf("The map's path couldn't be prepared for the game: %w", err)
			}
			cfg.GameMapPath, cfg.MapLabel = gamePath, hostMapLabel(m)
			cfg.MapEachLobby = m.Tracked != "" && settings.UseNewest
		}
	}
	s.mu.Lock()
	err := s.engine.ConfigureHost(cfg)
	if err == nil && hostMap != nil {
		s.hostMap = *hostMap
	}
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
			slog.Int("delay_seconds", st.DelaySeconds), slog.Bool("start_when_full", st.StartWhenFull), slog.Bool("same_team", st.SameTeam),
			slog.Bool("use_newest", st.UseNewest))
	}
	if cfg.Enabled != nil {
		attrs = append(attrs, slog.Bool("enabled", *cfg.Enabled), slog.Bool("from_menus", cfg.FromMenus), slog.String("game_map_path", cfg.GameMapPath),
			slog.String("map_label", cfg.MapLabel), slog.Bool("map_each_lobby", cfg.MapEachLobby))
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
	s.hostMapStale = true
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

// CloseGames closes every copy of the game this user runs, which the player
// asked for and confirmed in the window. It drops the game session and
// looks for the game again straight away.
func (s *Service) CloseGames(ctx context.Context) (dto.FollowState, error) {
	s.mu.Lock()
	if s.closing {
		state := s.snapshot()
		s.mu.Unlock()
		return state, errors.New("Warcraft III is already being closed.")
	}
	// Discovery waits, so it doesn't connect to a game that is closing.
	s.closing = true
	s.mu.Unlock()

	// The lock isn't held here: closing may wait a few seconds for the game.
	count, err := s.processes.CloseAll(ctx)

	s.mu.Lock()
	s.closing = false
	if count > 0 {
		s.dropSession()
		s.recent.End()
		s.ranking.End()
		if s.engine.Connected() {
			s.engine.Connection(false, "")
		}
		s.hint, s.gameCount = "", 0
	}
	switch {
	case count == 1:
		s.engine.Note("Closed 1 copy of Warcraft III.")
	case count > 1:
		s.engine.Note(fmt.Sprintf("Closed %d copies of Warcraft III.", count))
	case err == nil:
		s.engine.Note("No copy of Warcraft III was running.")
	}
	s.nextDiscovery = time.Time{}
	s.logStatus()
	state := s.snapshot()
	s.mu.Unlock()

	if err != nil {
		s.logger.WarnContext(ctx, "companion.close_games", slog.Int("count", count), slog.Any("error", err))
		if count > 0 {
			err = errors.New("Not every copy of Warcraft III could be closed. Close the rest in your task manager.")
		} else {
			err = errors.New("Warcraft III couldn't be closed. Close it in your task manager.")
		}
	} else {
		s.logger.InfoContext(ctx, "companion.close_games", slog.Int("count", count))
	}
	s.notify()
	return state, err
}

// RequestKeyboardPermission asks the system for the permission typing
// needs, which may show a system prompt. The window calls it only when the
// player clicks to allow typing. The companion handles the keyboard's
// error: it logs it here, once, and returns a sentence for the player.
func (s *Service) RequestKeyboardPermission(ctx context.Context) (dto.FollowState, error) {
	err := s.keyboard.RequestPermission(ctx)
	status := s.keyboard.Status(ctx)
	s.mu.Lock()
	s.keyboardStatus = status
	state := s.snapshot()
	s.mu.Unlock()
	attrs := []any{slog.Bool("permitted", status.Permitted)}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
		err = errors.New(playerSentence(err, "The permission to type couldn't be requested. Allow it in your system settings."))
	}
	s.logger.InfoContext(ctx, "keyboard.permission", attrs...)
	s.notify()
	return state, err
}

// errNoGame is why a line isn't typed when no game is connected.
var errNoGame = errors.New("Warcraft III isn't connected.")

// typeChatLines starts typing every line Game mode asked for. Typing takes
// a second or two, so each line is typed in its own goroutine, off s.mu;
// Run waits for them before it returns. A line asked for while no game is
// connected is dropped, so nothing is ever typed into a game that's gone.
// It runs with s.mu held.
func (s *Service) typeChatLines(ctx context.Context) {
	for {
		line, ok := s.engine.TakeChatLine()
		if !ok {
			return
		}
		if s.session == nil {
			s.chatLineTyped(ctx, line, errNoGame)
			continue
		}
		pid := s.gamePID
		s.typing.Go(func() {
			defer s.guard("type")
			err := s.typeChatLine(ctx, pid, line)
			s.mu.Lock()
			s.chatLineTyped(ctx, line, err)
			s.logStatus()
			s.mu.Unlock()
			s.notify()
		})
	}
}

// typeChatLine types one line into the game process pid. Its error is a
// sentence for the player. Nothing is retried.
func (s *Service) typeChatLine(ctx context.Context, pid int, line string) error {
	status := s.keyboard.Status(ctx)
	if !status.Supported || !status.Permitted {
		if status.Reason == "" {
			return errors.New("Team Follow isn't allowed to type on this computer.")
		}
		return errors.New(status.Reason)
	}
	if err := s.keyboard.TypeLine(ctx, pid, line); err != nil {
		return typingError{sentence: playerSentence(err, "Keep Warcraft III in front while Game mode types."), cause: err}
	}
	return nil
}

// typingError is a keyboard failure: its text is the sentence for the
// player, and it unwraps to the keyboard's own error for the log.
type typingError struct {
	sentence string
	cause    error
}

func (e typingError) Error() string { return e.sentence }
func (e typingError) Unwrap() error { return e.cause }

// chatLineTyped logs how typing line went, once, and tells the engine. It
// runs with s.mu held.
func (s *Service) chatLineTyped(ctx context.Context, line string, err error) {
	if err == nil {
		s.logger.InfoContext(ctx, "gamemode.typed", slog.String("line", line))
		s.engine.ChatLineTyped(line, nil)
		return
	}
	attrs := []any{slog.String("line", line), slog.String("reason", err.Error())}
	if cause := errors.Unwrap(err); cause != nil {
		attrs = append(attrs, slog.Any("error", cause))
	}
	s.logger.WarnContext(ctx, "gamemode.not_typed", attrs...)
	s.engine.ChatLineTyped(line, err)
}

// playerSentence returns the outermost error text written for players (a
// sentence starting with a capital letter), or fallback.
func playerSentence(err error, fallback string) string {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if t := e.Error(); t != "" && t[0] >= 'A' && t[0] <= 'Z' {
			return t
		}
	}
	return fallback
}

// saveRecent writes the recent players when they changed: at most every
// recentSaveEvery after a player or map changed, every recentTouchEvery
// when only last-seen times moved, or at once when force is set. A failed
// save is logged and tried again later. It runs with s.mu held.
func (s *Service) saveRecent(force bool) {
	changed, touched := s.recent.Unsaved()
	if !changed && !touched {
		return
	}
	every := recentTouchEvery
	if changed {
		every = recentSaveEvery
	}
	now := s.opts.Now()
	if !force && now.Sub(s.recentSavedAt) < every {
		return
	}
	s.recentSavedAt = now
	players := s.recent.Players()
	if err := s.recentDB.Save(players); err != nil {
		s.logger.Warn("recent.save", slog.Any("error", err))
		return
	}
	s.recent.MarkSaved()
	s.logger.Debug("recent.saved", slog.Int("count", len(players)))
}

// saveRanking writes the ranking when a match was recorded. A failed save
// is logged and tried again on the next step. It runs with s.mu held.
func (s *Service) saveRanking() {
	if !s.ranking.Unsaved() {
		return
	}
	entries := s.ranking.Entries()
	if err := s.rankingDB.Save(entries); err != nil {
		s.logger.Warn("ranking.save", slog.Any("error", err))
		return
	}
	s.ranking.MarkSaved()
	s.logger.Debug("ranking.saved", slog.Int("count", len(entries)))
}

// ClearRanking forgets every recorded win and loss, on this computer too.
// The follow group stays as it is.
func (s *Service) ClearRanking() (dto.FollowState, error) {
	s.mu.Lock()
	err := s.rankingDB.Clear()
	if err != nil {
		s.logger.Error("ranking.clear", slog.Any("error", err))
		err = errors.New(clearRankingFailed)
	} else {
		s.ranking.Clear()
		s.logger.Info("ranking.cleared")
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// ClearRecentPlayers forgets every recent player, on this computer too.
// The follow group stays as it is.
func (s *Service) ClearRecentPlayers() (dto.FollowState, error) {
	s.mu.Lock()
	err := s.recentDB.Clear()
	if err != nil {
		s.logger.Error("recent.clear", slog.Any("error", err))
		err = errors.New(clearFailed)
	} else {
		s.recent.Clear()
		s.logger.Info("recent.cleared")
	}
	state := s.snapshot()
	s.mu.Unlock()
	s.notify()
	return state, err
}

// SetNote saves the player's note about the player with tag, replacing any
// earlier one; empty text deletes it. The note is checked by
// follow.NoteText, kept on this computer only, and never logged.
func (s *Service) SetNote(tag, text string) (dto.FollowState, error) {
	text, err := follow.NoteText(tag, text)
	s.mu.Lock()
	if err == nil {
		notes := maps.Clone(s.notes)
		key := follow.NoteKey(tag)
		if text == "" {
			delete(notes, key)
		} else {
			notes[key] = dto.PlayerNote{Text: text, Updated: s.opts.Now()}
		}
		if saveErr := s.notesDB.Save(notes); saveErr != nil {
			s.logger.Error("notes.save", slog.Any("error", saveErr))
			err = errors.New(noteFailed)
		} else {
			s.notes = notes
			s.logger.Info("notes.saved", slog.Int("count", len(notes)))
		}
	}
	state := s.snapshot()
	s.mu.Unlock()
	if err == nil {
		s.notify()
	}
	return state, err
}
