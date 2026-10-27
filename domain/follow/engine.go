// Package follow is the team-follow state machine: it reads the game's team
// and friend presence events and decides when to join a friend's lobby.
//
// The engine is not safe for concurrent use; its owner serializes calls.
package follow

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	freshFor        = 20 * time.Second
	joinTimeout     = 15 * time.Second
	passwordTTL     = 2 * time.Minute
	attemptCooldown = 10 * time.Second
	resumeDelay     = 2 * time.Second
	maxLog          = 40
	maxEventKinds   = 120
	maxAttempts     = 256
)

// Game commands the engine sends.
const (
	CmdTeamGetInfo           = "TeamGetInfo"
	CmdFriendsGetFriends     = "FriendsGetFriends"
	CmdCancelTeamInvitations = "RequestCancelTeamInvitations"
	CmdTeamLeave             = "TeamLeave"
	CmdJoinFriendGameLobby   = "JoinFriendGameLobby"
	CmdJoinGameByGameName    = "JoinGameByGameName"
)

// Sender delivers one command to the game. It reports false when the game
// connection is not open.
type Sender func(message string, payload map[string]any) bool

var (
	busyScreens  = []string{"GAME_LOBBY", "CUSTOM_GAME_LOBBY", "LOADING_SCREEN", "DISABLED_SCREEN", "OUT_OF_MENUS", "SCORE_SCREEN"}
	matchScreens = []string{"LOADING_SCREEN", "DISABLED_SCREEN", "OUT_OF_MENUS"}
	menuScreens  = []string{"MAIN_MENU", "MULTIPLAYER", "CUSTOM_LOBBIES", "CHAT", "VERSUS"}
)

type member struct {
	battleTag string
	isSelf    bool
	isFriend  bool
	isOwner   bool
}

type friendRecord struct {
	data  map[string]any
	seen  time.Time
	epoch int
}

type presenceEpoch struct {
	open      bool
	signature string
	epoch     int
}

type pendingJoin struct {
	started               time.Time
	friend                string
	lobby                 string
	emptyConfirmationSent bool
}

type passwordRequest struct {
	lobby   string
	friend  string
	started time.Time
}

type presence struct {
	label    string
	open     bool
	auroraID any
	lobby    string
	epoch    int
}

// Engine is the follow state machine.
type Engine struct {
	send     Sender
	now      func() time.Time
	verified []string

	connected  bool
	version    string
	enabled    bool
	diagnostic bool

	team    []member
	teamAt  time.Time
	watched []member
	friends map[string]friendRecord

	status      string
	busy        bool
	queued      bool
	screen      string
	pending     *pendingJoin
	announced   map[string]bool
	counts      map[string]int
	log         []dto.LogEntry
	lastJoin    *dto.JoinRecord
	manualTags  []string
	includeTeam bool
	password    *passwordRequest

	inMatch      bool
	awaitingMenu bool
	menuReady    bool
	resumeAt     time.Time

	attempted      []string
	presenceEpochs map[string]presenceEpoch
	nextAttemptAt  time.Time
}

// NewEngine returns an engine that sends commands through send, reads the
// time from now and allows live joining only on the verified game builds.
// Follow starts off and preview mode starts on.
func NewEngine(send Sender, now func() time.Time, verified []string) *Engine {
	return &Engine{
		send:           send,
		now:            now,
		verified:       slices.Clone(verified),
		diagnostic:     true,
		includeTeam:    true,
		status:         "Waiting for Warcraft III.",
		friends:        map[string]friendRecord{},
		announced:      map[string]bool{},
		counts:         map[string]int{},
		presenceEpochs: map[string]presenceEpoch{},
	}
}

func tagKey(tag string) string { return strings.ToLower(strings.TrimSpace(tag)) }

// Note records message as the current status and in the activity log.
func (e *Engine) Note(message string) {
	e.status = message
	if n := len(e.log); n > 0 && e.log[n-1].Message == message {
		return
	}
	e.log = append(e.log, dto.LogEntry{Time: e.now().UTC(), Message: message})
	if len(e.log) > maxLog {
		e.log = e.log[1:]
	}
}

// Connected reports whether the game connection is open.
func (e *Engine) Connected() bool { return e.connected }

// Connection resets everything learned from the game. Follow always ends up
// off; the friend selection and mode are kept.
func (e *Engine) Connection(connected bool, version string) {
	e.connected, e.version = connected, version
	e.enabled, e.pending, e.password = false, nil, nil
	e.team, e.teamAt = nil, time.Time{}
	clear(e.friends)
	e.watched = nil
	e.busy, e.queued, e.screen = false, false, ""
	e.inMatch, e.awaitingMenu, e.menuReady, e.resumeAt = false, false, false, time.Time{}
	e.attempted = nil
	clear(e.presenceEpochs)
	e.nextAttemptAt = time.Time{}
	if connected {
		e.Note("Connected to Warcraft III. Form a team in the game, or add friends here.")
	} else {
		e.Note("Waiting for Warcraft III. Follow is off.")
	}
}

func (e *Engine) disarm() { e.enabled, e.pending, e.password = false, nil, nil }

// Configure applies a selection or mode change, or switches Follow on or
// off. Any selection or mode change switches Follow off.
func (e *Engine) Configure(cfg dto.FollowConfig) error {
	if cfg.Tags != nil {
		tags, err := NormalizeTags(*cfg.Tags)
		if err != nil {
			return err
		}
		e.manualTags = tags
		e.disarm()
	}
	if cfg.IncludeTeam != nil {
		e.includeTeam = *cfg.IncludeTeam
		e.disarm()
	}
	if cfg.Diagnostic != nil && *cfg.Diagnostic != e.diagnostic {
		e.disarm()
		e.diagnostic = *cfg.Diagnostic
		clear(e.announced)
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		e.disarm()
		e.Note("Follow is off.")
		return nil
	}
	if cfg.Enabled != nil && *cfg.Enabled {
		if err := e.arm(cfg.FromMenus); err != nil {
			return err
		}
	}
	e.evaluate()
	return nil
}

func (e *Engine) arm(fromMenus bool) error {
	now := e.now()
	switch {
	case e.pending != nil:
		return errors.New("A join request is still pending. Check Warcraft III before trying again.")
	case !e.connected:
		return errors.New("Start Warcraft III and wait for it to connect first.")
	case !e.VerifiedBuild():
		return errors.New("This game build has not been verified. Preview works; joining is disabled.")
	case e.includeTeam && slices.ContainsFunc(e.team, func(m member) bool { return !m.isSelf }) && now.Sub(e.teamAt) > freshFor:
		return errors.New("Waiting for a fresh team list from the game. Try again in a few seconds.")
	case e.busy || e.queued || e.inMatch || e.awaitingMenu:
		return errors.New("Return to Custom Games before starting.")
	case !fromMenus:
		return errors.New("Start Follow while you are in the game menus.")
	}

	var members []member
	if e.includeTeam {
		for _, m := range e.team {
			if !m.isSelf && m.battleTag != "" {
				members = append(members, m)
			}
		}
	}
	for _, tag := range e.manualTags {
		if slices.ContainsFunc(e.team, func(m member) bool { return m.isSelf && tagKey(m.battleTag) == tagKey(tag) }) {
			return errors.New("Choose a friend's BattleTag, not your own.")
		}
		record, ok := e.friends[tagKey(tag)]
		if !ok || now.Sub(record.seen) > freshFor {
			return errors.New("No activity yet for " + tag + ". Check the spelling and that they're on your Battle.net friends list.")
		}
		if !isTrue(record.data["isFriend"]) {
			return errors.New(tag + " must already be on your Battle.net friends list.")
		}
		if !slices.ContainsFunc(members, func(m member) bool { return tagKey(m.battleTag) == tagKey(tag) }) {
			name, _ := record.data["battleTag"].(string)
			members = append(members, member{battleTag: name, isFriend: true})
		}
	}
	if len(members) == 0 {
		return errors.New("Invite a friend to your team or add a friend's BattleTag first.")
	}

	e.watched = members
	e.enabled, e.pending, e.password = true, nil, nil
	clear(e.announced)
	e.attempted, e.nextAttemptAt = nil, time.Time{}
	// Demand a new presence response after arming. Never act on a cached
	// friend snapshot.
	clear(e.friends)
	if e.diagnostic {
		e.Note("Preview is on. You'll see when a friend's lobby opens; nothing will be joined.")
	} else {
		e.Note("Auto-join is on. It pauses in lobbies, matches and queues.")
	}
	e.send(CmdFriendsGetFriends, map[string]any{})
	return nil
}

// VerifiedBuild reports whether live joining is allowed on the connected
// game build.
func (e *Engine) VerifiedBuild() bool { return slices.Contains(e.verified, e.version) }

// Receive applies one event from the game.
func (e *Engine) Receive(event dto.GameEvent) {
	if event.MessageType == "" {
		return
	}
	kind, p := event.MessageType, event.Payload
	if p == nil {
		p = map[string]any{}
	}
	if _, seen := e.counts[kind]; seen || len(e.counts) < maxEventKinds {
		e.counts[kind]++
	}

	switch kind {
	case "TeamsInformation":
		if members, ok := list(field(p, "message", "partyMembers")); ok {
			e.team = e.team[:0:0]
			for _, raw := range members {
				x := object(raw)
				tag, ok := x["battleTag"].(string)
				if !ok {
					continue
				}
				e.team = append(e.team, member{battleTag: tag, isSelf: isTrue(x["isSelf"]), isFriend: isTrue(x["isFriend"]), isOwner: isTrue(x["isOwner"])})
			}
			e.teamAt = e.now()
		}
	case "TeamsLeft", "TeamsDisbanded":
		e.team, e.teamAt = nil, time.Time{}
	case "FriendsFriendData":
		if friends, ok := list(field(p, "data", "friends")); ok {
			clear(e.friends)
			for _, f := range friends {
				e.friend(object(f))
			}
		}
	case "FriendsFriendUpdated":
		e.friend(object(field(p, "data", "friend")))
	case "FriendsFriendRemoved":
		clear(e.friends)
	case "QueueEntered":
		e.queued = true
		e.pause("Follow is on but paused for matchmaking.")
	case "QueueExited":
		e.queued = false
		clear(e.friends)
		e.send(CmdFriendsGetFriends, map[string]any{})
	case "RequestForPassword":
		e.confirmLobbyRequest(p)
	case "GameLobbySetup":
		players, _ := list(p["players"])
		inSlot := slices.ContainsFunc(players, func(raw any) bool {
			x := object(raw)
			return isTrue(x["isSelf"]) && numberIs(x["slotStatus"], 2)
		})
		name, _ := p["lobbyName"].(string)
		if e.lastJoin != nil && inSlot && (e.lastJoin.Lobby == "" || name == e.lastJoin.Lobby) {
			if !e.lastJoin.Confirmed {
				e.lastJoin.Confirmed = true
				e.Note("Joined " + e.lastJoin.Friend + "'s lobby. Follow stays on and is paused here.")
			}
		} else if e.pending != nil {
			e.Note("A lobby update arrived after the join request. Check the lobby in Warcraft III.")
		}
		e.pending, e.password = nil, nil
		e.busy, e.awaitingMenu, e.menuReady = true, false, false
	case "IsGameUIActive":
		switch p["isActive"] {
		case "true", true:
			e.inMatch, e.busy, e.menuReady = true, true, false
			e.pause("Follow is on but paused during the match.")
		case "false", false:
			e.inMatch = false
		}
	case "MultiplayerGameJoinStarted":
		e.busy, e.menuReady = true, false
		e.pause("Follow is on but paused while entering a lobby.")
	case "PauseUI", "GameFound", "LoadingScreenUpdate":
		e.inMatch, e.busy, e.menuReady = true, true, false
		e.pause("Follow is on but paused while the game enters a match.")
	case "MultiplayerGameLeave", "ExitedGame":
		if kind == "ExitedGame" {
			e.inMatch = false
		}
		e.busy, e.awaitingMenu, e.menuReady = true, true, false
		e.resumeAt = e.now().Add(resumeDelay)
		e.pending, e.password = nil, nil
		clear(e.friends)
		if e.enabled {
			e.Note("Follow is still on. Return to Custom Games to resume automatically.")
		} else {
			e.Note("Returned from a game or lobby. Follow is off.")
		}
	case "GameList":
		// Only the game's own interface requests listings; this companion
		// never does. A listing after leaving shows Custom Games is open.
		if _, ok := list(p["games"]); ok && e.awaitingMenu && !e.inMatch && !e.queued {
			e.menuReady = true
		}
	case "SetGlueScreen":
		e.screen, _ = p["screen"].(string)
		switch {
		case slices.Contains(busyScreens, e.screen):
			e.busy, e.menuReady = true, false
			if slices.Contains(matchScreens, e.screen) {
				e.inMatch = true
			}
			if e.screen == "SCORE_SCREEN" {
				e.inMatch, e.awaitingMenu = false, true
				e.resumeAt = e.now().Add(resumeDelay)
			}
			e.pause("Follow is on but paused in this lobby or game screen.")
		case slices.Contains(menuScreens, e.screen):
			e.inMatch = false
			if e.busy || e.awaitingMenu {
				e.awaitingMenu, e.menuReady = true, true
				if next := e.now().Add(resumeDelay); next.After(e.resumeAt) {
					e.resumeAt = next
				}
			}
		}
	case "LoggedOut":
		e.Connection(false, "")
	case "GameSearchFailedToJoinGame", "GameSearchGameNotFound":
		e.pending, e.password = nil, nil
		e.Note("The game could not join that lobby. Follow stays on and waits for a new lobby.")
	}
	e.resumeIfReady()
	e.evaluate()
}

func (e *Engine) friend(f map[string]any) {
	tag, ok := f["battleTag"].(string)
	if !ok {
		return
	}
	key := tagKey(tag)
	open := isTrue(f["isOnline"]) && isTrue(f["isFriend"]) && f["currentProgram"] == "W3" && attribute(f, "currentGameState") == "open"
	signature := attribute(f, "currentGameName")
	epoch := 0
	if previous, ok := e.presenceEpochs[key]; ok {
		epoch = previous.epoch
		if (open && !previous.open) || (open && signature != previous.signature) {
			epoch++
		}
	}
	e.presenceEpochs[key] = presenceEpoch{open: open, signature: signature, epoch: epoch}
	e.friends[key] = friendRecord{data: f, seen: e.now(), epoch: epoch}
}

func (e *Engine) pause(message string) {
	e.pending, e.password = nil, nil
	if e.enabled {
		e.Note(message)
	}
}

func (e *Engine) resumeIfReady() {
	if !e.awaitingMenu || !e.menuReady || e.inMatch || e.queued || e.now().Before(e.resumeAt) {
		return
	}
	e.busy, e.awaitingMenu, e.menuReady = false, false, false
	clear(e.friends)
	e.send(CmdFriendsGetFriends, map[string]any{})
	if e.enabled {
		e.Note("Custom Games is open again. Follow has resumed.")
	}
}

func (e *Engine) confirmLobbyRequest(p map[string]any) {
	if !e.enabled || e.pending == nil || e.diagnostic || !e.connected || e.busy || e.queued || e.inMatch {
		return
	}
	name, _ := field(p, "message", "matchJoinGameByNameText").(string)
	if name == "" || len(name) > 128 || (e.pending.lobby != "" && name != e.pending.lobby) {
		e.pending = nil
		e.Note("The lobby confirmation did not match the requested friend. Waiting for a new lobby.")
		return
	}
	if e.pending.emptyConfirmationSent {
		e.password = &passwordRequest{lobby: name, friend: e.pending.friend, started: e.now()}
		e.pending = nil
		e.Note("The game is asking for a lobby password.")
		return
	}
	e.pending.emptyConfirmationSent = true
	e.pending.started = e.now()
	e.pending.lobby = name
	if e.lastJoin != nil {
		e.lastJoin.Lobby = name
	}
	if !e.send(CmdJoinGameByGameName, map[string]any{"gameName": name, "gamePass": ""}) {
		e.pending = nil
		e.Note("The lobby confirmation could not be sent. Waiting for a new lobby.")
		return
	}
	e.Note("Confirming the lobby with no password. Waiting for the lobby.")
}

// SubmitPassword answers the game's password request once. The password is
// passed to the game and never kept.
func (e *Engine) SubmitPassword(password string) error {
	if e.password == nil || e.now().Sub(e.password.started) > passwordTTL {
		return errors.New("The password request has expired. Start a new follow attempt.")
	}
	if !e.enabled || !e.connected || e.busy || e.queued || e.inMatch || e.diagnostic {
		return errors.New("Return to the game menus before joining.")
	}
	if len(password) > 128 {
		return errors.New("Enter a valid lobby password.")
	}
	request := e.password
	e.password = nil
	e.pending = &pendingJoin{friend: request.friend, lobby: request.lobby, started: e.now(), emptyConfirmationSent: true}
	if !e.send(CmdJoinGameByGameName, map[string]any{"gameName": request.lobby, "gamePass": password}) {
		e.pending = nil
		return errors.New("The game connection failed.")
	}
	e.Note("Password sent to the game. It was not saved.")
	return nil
}

func (e *Engine) presence(m member) presence {
	record, ok := e.friends[tagKey(m.battleTag)]
	if !ok {
		if m.isFriend {
			return presence{label: "Waiting for activity"}
		}
		return presence{label: "Not on your friends list"}
	}
	f := record.data
	fresh := e.now().Sub(record.seen) < freshFor
	online := isTrue(f["isOnline"])
	inGame := f["currentProgram"] == "W3"
	open := fresh && online && isTrue(f["isFriend"]) && inGame && attribute(f, "currentGameState") == "open" && validAccountID(f["accountId"])
	label := "Online in Battle.net"
	switch {
	case !fresh:
		label = "Activity is out of date"
	case !online:
		label = "Offline"
	case open:
		label = "Lobby open"
	case inGame:
		label = "In Warcraft III"
	}
	return presence{label: label, open: open, auroraID: f["accountId"], lobby: attribute(f, "currentGameName"), epoch: record.epoch}
}

func attemptKey(tag string, p presence) string {
	return tagKey(tag) + ":" + itoa(p.epoch) + ":" + p.lobby
}

func (e *Engine) evaluate() {
	if !e.enabled || !e.connected || e.busy || e.queued || e.inMatch || e.awaitingMenu || e.pending != nil || e.password != nil || e.now().Before(e.nextAttemptAt) {
		return
	}
	// Watched order is team order, so the choice is deterministic when more
	// than one member becomes joinable together.
	var (
		target member
		p      presence
		found  bool
	)
	for _, m := range e.watched {
		candidate := e.presence(m)
		if candidate.open && !slices.Contains(e.attempted, attemptKey(m.battleTag, candidate)) {
			target, p, found = m, candidate, true
			break
		}
	}
	if !found {
		return
	}
	if e.diagnostic {
		marker := tagKey(target.battleTag) + ":" + p.lobby
		if !e.announced[marker] {
			e.announced[marker] = true
			e.Note(target.battleTag + " has an open lobby. Preview mode didn't join it.")
		}
		return
	}
	// Attempt each observed lobby once. A closed and reopened, or renamed,
	// lobby is a new opportunity.
	e.attempted = append(e.attempted, attemptKey(target.battleTag, p))
	if len(e.attempted) > maxAttempts {
		e.attempted = e.attempted[1:]
	}
	now := e.now()
	e.nextAttemptAt = now.Add(attemptCooldown)
	e.pending = &pendingJoin{started: now, friend: target.battleTag, lobby: p.lobby}
	e.lastJoin = &dto.JoinRecord{Friend: target.battleTag, Lobby: p.lobby, Time: now.UTC()}

	ok := e.send(CmdCancelTeamInvitations, map[string]any{})
	if ok && slices.ContainsFunc(e.team, func(m member) bool { return !m.isSelf }) {
		ok = e.send(CmdTeamLeave, map[string]any{})
	}
	if ok {
		ok = e.send(CmdJoinFriendGameLobby, map[string]any{"auroraId": p.auroraID})
	}
	if !ok {
		e.pending = nil
		e.Note("The join request could not be sent. Waiting for a new lobby.")
		return
	}
	e.Note("Asked to join " + target.battleTag + "'s lobby. Follow stays on.")
}

// Tick runs the time-based transitions; call it about once a second.
func (e *Engine) Tick() {
	e.resumeIfReady()
	if e.password != nil && e.now().Sub(e.password.started) > passwordTTL {
		e.password = nil
		e.Note("The lobby password request expired. Waiting for a new lobby.")
	}
	if e.pending != nil && e.now().Sub(e.pending.started) > joinTimeout {
		e.pending = nil
		e.Note("The game didn't confirm the join. Check Warcraft III before trying again.")
	}
	e.evaluate()
}

// ManualTags returns the manually added friend BattleTags.
func (e *Engine) ManualTags() []string { return slices.Clone(e.manualTags) }

// IncludeTeam reports whether the current team is part of the selection.
func (e *Engine) IncludeTeam() bool { return e.includeTeam }

// Snapshot returns the state for the interface. It never includes account
// identifiers or passwords.
func (e *Engine) Snapshot() dto.FollowState {
	s := dto.FollowState{
		Connected:     e.connected,
		Version:       e.version,
		VerifiedBuild: e.VerifiedBuild(),
		Enabled:       e.enabled,
		Diagnostic:    e.diagnostic,
		ManualTags:    e.ManualTags(),
		IncludeTeam:   e.includeTeam,
		Status:        e.status,
		Screen:        e.screen,
		Busy:          e.busy,
		Queued:        e.queued,
		InMatch:       e.inMatch,
		Paused:        e.enabled && (e.busy || e.queued || e.inMatch || e.awaitingMenu),
		InMenus:       e.connected && slices.Contains(menuScreens, e.screen) && !e.busy && !e.queued && !e.inMatch && !e.awaitingMenu,
		JoinPending:   e.pending != nil,
		Team:          []dto.TeamMember{},
		Watched:       []dto.WatchedMember{},
		Friends:       []dto.WatchedMember{},
		Log:           slices.Clone(e.log),
	}
	if s.ManualTags == nil {
		s.ManualTags = []string{}
	}
	if s.Log == nil {
		s.Log = []dto.LogEntry{}
	}
	if e.password != nil {
		s.PasswordRequest = &dto.PasswordRequest{Lobby: e.password.lobby, Friend: e.password.friend}
	}
	if e.lastJoin != nil {
		join := *e.lastJoin
		s.LastJoin = &join
	}
	for _, m := range e.team {
		status := "You"
		if !m.isSelf {
			status = e.presence(m).label
		}
		s.Team = append(s.Team, dto.TeamMember{BattleTag: m.battleTag, IsSelf: m.isSelf, IsFriend: m.isFriend, IsOwner: m.isOwner, Status: status})
	}
	for _, m := range e.watched {
		p := e.presence(m)
		s.Watched = append(s.Watched, dto.WatchedMember{BattleTag: m.battleTag, Label: p.label, Open: p.open, Lobby: p.lobby})
	}
	for _, tag := range e.manualTags {
		p := e.presence(member{battleTag: tag, isFriend: true})
		s.Friends = append(s.Friends, dto.WatchedMember{BattleTag: tag, Label: p.label, Open: p.open, Lobby: p.lobby})
	}
	return s
}

// Diagnostics returns the shareable report: counts and states only, never
// names, identifiers or lobby names.
func (e *Engine) Diagnostics() dto.Diagnostics {
	counts := make(map[string]int, len(e.counts))
	maps.Copy(counts, e.counts)
	return dto.Diagnostics{
		Version:      e.version,
		Connected:    e.connected,
		Screen:       e.screen,
		TeamCount:    len(e.team),
		WatchedCount: len(e.watched),
		FriendCount:  len(e.friends),
		EventCounts:  counts,
		Diagnostic:   e.diagnostic,
		Enabled:      e.enabled,
		Busy:         e.busy,
		Queued:       e.queued,
	}
}
