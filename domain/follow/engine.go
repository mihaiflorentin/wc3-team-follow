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
	freshFor    = 20 * time.Second
	joinTimeout = 15 * time.Second
	// listWait is how long a join waits for the game list it asked for
	// before confirming anyway.
	listWait        = 4 * time.Second
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
	// CmdSendGameListing switches the custom game list on and CmdGetGameList
	// asks for it, as the Custom Games screen does when it opens (checked
	// live: without SendGameListing the list stays empty). Joining by name
	// only finds lobbies in that list.
	CmdSendGameListing = "SendGameListing"
	CmdGetGameList     = "GetGameList"
)

// Sender delivers one command to the game. It reports false when the game
// connection is not open.
type Sender func(message string, payload map[string]any) bool

var (
	// Screen names are the game menu's ScreenEnum keys (its GlueManager.js),
	// as SetGlueScreen sends them. The game's own Join Friend has no screen
	// condition, so joining is allowed from every menu screen; it pauses in
	// lobbies (including the one a host is creating), matches, score
	// screens, and before login.
	busyScreens = []string{
		"GAME_LOBBY", "CUSTOM_GAME_LOBBY", "CREATE_GAME_LOBBY", "SINGLE_PLAYER_CUSTOM_LOBBY", "JOIN_LOBBY",
		"LOADING_SCREEN", "DISABLED_SCREEN", "OUT_OF_MENUS", "BLACK_SCREEN",
		"SCORE_SCREEN", "RANKED_SCORE_SCREEN", "CAMPAIGN_SCORE", "RANK_UP_ANIMATION", "RATING_SPLASH_SCREEN",
		"LOGIN_DOORS", "LOGIN_OPTIONS", "DEMO_SPLASH_SCREEN",
	}
	matchScreens = []string{"LOADING_SCREEN", "DISABLED_SCREEN", "OUT_OF_MENUS", "BLACK_SCREEN"}
	// scoreScreens follow a match; leaving them is a return to the menus.
	scoreScreens = []string{"SCORE_SCREEN", "RANKED_SCORE_SCREEN", "CAMPAIGN_SCORE", "RANK_UP_ANIMATION", "RATING_SPLASH_SCREEN"}
	menuScreens  = []string{
		"MAIN_MENU", "MULTIPLAYER", "CHAT", "SYSTEM_MENU",
		"VERSUS", "VERSUS_SETUP", "CUSTOM_LOBBIES", "MULTIPLAYER_CUSTOM_GAMES_SETUP", "LEADERBOARD", "TOURNAMENTS", "TOURNAMENTS_RESULTS_SCREEN",
		"CAMPAIGNS", "CAMPAIGN_SELECTION", "CAMPAIGN_EXPANSIONS", "CAMPAIGN_MISSIONS", "MISSION_SELECTION", "CUSTOM_CAMPAIGNS", "LOAD_SAVED_CAMPAIGN",
		"SINGLE_PLAYER_MENU", "LOAD_GAME", "REPLAYS_SCREEN", "PROFILE", "ARMORY", "COLLECTION", "CREDITS_SCREEN",
	}
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
	// listAsked is set while the join waits for the game list it asked
	// for, before confirming the lobby.
	listAsked time.Time
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
	// friends is the presence the engine may act on; it is emptied whenever
	// a fresh snapshot is required.
	friends map[string]friendRecord
	// roster is the whole Battle.net friends list for display. It is
	// replaced by each full snapshot and kept across arming.
	roster map[string]friendRecord

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
	// askPassword lets a lobby that wants a password ask the player; off
	// (the default), such a lobby is skipped.
	askPassword bool
	password    *passwordRequest
	// joinHostTeam moves the player to the host's team in a lobby Auto-join
	// joined; on by default.
	joinHostTeam bool
	// joined is the lobby Auto-join joined, while the player is in it.
	joined *joinedLobby
	// blacklist is who auto-ban keeps out of lobbies the player hosts,
	// newest first; autoBan switches it on (the default).
	blacklist []dto.BlacklistEntry
	autoBan   bool
	ban       banState
	// manualBans adds players the player bans by hand to the blacklist (on
	// by default); blacklistChanged reports such an add for saving.
	manualBans       bool
	blacklistChanged bool
	// banMistakes are warnings about wrong bans, for TakeBanMistakes.
	banMistakes []string
	// ignoredBanLines are the senders of ban lines not sent as the player,
	// for TakeIgnoredBanLines.
	ignoredBanLines []string
	// selfTag is the player's own BattleTag, once the team or a lobby
	// showed it.
	selfTag string

	inMatch      bool
	awaitingMenu bool
	menuReady    bool
	// menusKnown is set once the game itself has shown the player is in its
	// menus: a menu screen, the custom game list, or a resume after a match.
	// Menu clicks are not reported, so this can't track every screen.
	menusKnown bool
	resumeAt   time.Time

	// host is Auto-host.
	host hostState
	// greet is the greeting typed after joining a lobby as a guest.
	greet  greeting
	random Random
	// chatLines are lines Game mode wants typed, for TakeChatLine.
	chatLines []string

	attempted      []string
	presenceEpochs map[string]presenceEpoch
	// seenOpen remembers which group lobbies were already announced while
	// watching, keyed like attempts.
	seenOpen      map[string]bool
	nextAttemptAt time.Time
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
		joinHostTeam:   true,
		autoBan:        true,
		manualBans:     true,
		status:         "Waiting for Warcraft III.",
		friends:        map[string]friendRecord{},
		roster:         map[string]friendRecord{},
		announced:      map[string]bool{},
		counts:         map[string]int{},
		presenceEpochs: map[string]presenceEpoch{},
		seenOpen:       map[string]bool{},
		random:         newRandom(),
		host:           hostState{settings: dto.HostSettings{GameModeDelaySeconds: DefaultGameModeDelay}},
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

// Status returns the latest status line.
func (e *Engine) Status() string { return e.status }

// Connected reports whether the game connection is open.
func (e *Engine) Connected() bool { return e.connected }

// Connection resets everything learned from the game. Follow always ends up
// off; the friend selection and mode are kept.
func (e *Engine) Connection(connected bool, version string) {
	e.connected, e.version = connected, version
	e.enabled, e.pending, e.password, e.joined = false, nil, nil, nil
	e.ban = banState{}
	e.endGreeting()
	e.team, e.teamAt = nil, time.Time{}
	clear(e.friends)
	clear(e.roster)
	e.watched = nil
	e.busy, e.queued, e.screen = false, false, ""
	e.inMatch, e.awaitingMenu, e.menuReady, e.resumeAt = false, false, false, time.Time{}
	e.menusKnown = false
	e.host = hostState{settings: e.host.settings, phase: HostOff}
	e.attempted = nil
	clear(e.presenceEpochs)
	clear(e.seenOpen)
	e.nextAttemptAt = time.Time{}
	if connected {
		e.Note("Connected to Warcraft III. Form a team in the game, or add friends here.")
	} else {
		e.Note("Waiting for Warcraft III. Follow is off.")
	}
}

func (e *Engine) disarm() { e.enabled, e.pending, e.password = false, nil, nil }

// selectionChanged stops Follow, which only ever follows the group chosen
// when it was started.
func (e *Engine) selectionChanged() {
	if e.enabled {
		e.Note("Who you follow changed, so Follow stopped. Start again to follow the new group.")
	}
	e.disarm()
}

// Configure applies a selection or mode change, or switches Follow on or
// off. Any selection or mode change switches Follow off.
func (e *Engine) Configure(cfg dto.FollowConfig) error {
	if err := e.configureBlacklist(cfg); err != nil {
		return err
	}
	if cfg.Tags != nil {
		tags, err := NormalizeTags(*cfg.Tags)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if e.blacklisted(tag) {
				return errors.New(tag + " is on your blacklist. Remove them from it before following them.")
			}
		}
		e.manualTags = tags
		e.selectionChanged()
	}
	if cfg.IncludeTeam != nil {
		e.includeTeam = *cfg.IncludeTeam
		e.selectionChanged()
	}
	if cfg.AskPassword != nil {
		e.askPassword = *cfg.AskPassword
		if !e.askPassword && e.password != nil {
			e.password = nil
			e.Note("Lobby passwords are off. Waiting for a new lobby.")
		}
	}
	if cfg.JoinHostTeam != nil {
		e.joinHostTeam = *cfg.JoinHostTeam
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
		return errors.New("Go back to the game's menus before turning on Auto-join.")
	case !fromMenus:
		return errors.New("Turn on Auto-join while you are in the game's menus.")
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
		// The roster, not the presence the engine acts on, says whether the
		// friend exists: arming empties that, so re-arming moments after a
		// group change must still find them. Joining waits for fresh data.
		record, ok := e.roster[tagKey(tag)]
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

	if e.host.enabled {
		e.stopHost("Auto-host is off while Auto-join is on.")
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
				if isTrue(x["isSelf"]) && tagPattern.MatchString(tag) {
					e.selfTag = tag
				}
			}
			e.teamAt = e.now()
		}
	case "TeamsLeft", "TeamsDisbanded":
		e.team, e.teamAt = nil, time.Time{}
	case "FriendsFriendData":
		if friends, ok := list(field(p, "data", "friends")); ok {
			clear(e.friends)
			clear(e.roster)
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
				e.joined = &joinedLobby{friend: e.lastJoin.Friend, moves: map[string]int{}}
				e.Note("Joined " + e.lastJoin.Friend + "'s lobby. Follow stays on and is paused here.")
			}
			e.moveToHostTeam(p)
		} else if e.pending != nil {
			e.Note("A lobby update arrived after the join request. Check the lobby in Warcraft III.")
		}
		e.pending, e.password = nil, nil
		e.busy, e.awaitingMenu, e.menuReady = true, false, false
		for _, raw := range players {
			x := object(raw)
			if name, _ := x["name"].(string); isTrue(x["isSelf"]) && tagPattern.MatchString(strings.TrimSpace(name)) {
				e.selfTag = strings.TrimSpace(name)
			}
		}
		e.banBlacklisted(p)
		e.greetLobby(p, inSlot, name)
	case "LobbyCountdown":
		details := object(p["details"])
		if e.joined != nil {
			e.joined.countdown = isTrue(details["countdownActive"])
		}
		e.ban.locked = isTrue(details["countdownActive"]) && isTrue(details["countdownLocked"])
		if isTrue(details["countdownActive"]) {
			e.cancelGreeting()
		}
	case "ChatMessage":
		e.confirmBan(p)
		e.greetReply(p)
	case "IsGameUIActive":
		switch p["isActive"] {
		case "true", true:
			e.inMatch, e.busy, e.menuReady = true, true, false
			e.endGreeting()
			e.pause("Follow is on but paused during the match.")
		case "false", false:
			e.inMatch = false
		}
	case "MultiplayerGameJoinStarted":
		e.busy, e.menuReady = true, false
		e.pause("Follow is on but paused while entering a lobby.")
	case "PauseUI", "GameFound", "LoadingScreenUpdate":
		e.joined, e.ban = nil, banState{}
		e.endGreeting()
		e.inMatch, e.busy, e.menuReady = true, true, false
		e.pause("Follow is on but paused while the game enters a match.")
	case "MultiplayerGameLeave", "ExitedGame":
		e.joined, e.ban = nil, banState{}
		e.endGreeting()
		if kind == "ExitedGame" {
			e.inMatch = false
		}
		// Leaving keeps the evidence of a score screen already shown: the
		// game sends MultiplayerGameLeave after it.
		e.busy, e.awaitingMenu = true, true
		e.menuReady = e.menuReady && slices.Contains(scoreScreens, e.screen)
		if !e.menuReady {
			e.resumeAt = e.now().Add(resumeDelay)
		}
		e.pending, e.password = nil, nil
		clear(e.friends)
		switch {
		case e.enabled && e.menuReady:
			e.Note("The match is over. Follow resumes in a moment.")
		case e.enabled:
			e.Note("Follow is still on. It resumes when you're back in the game's menus.")
		default:
			e.Note("Returned from a game or lobby. Follow is off.")
		}
	case "GameList":
		if games, _ := list(p["games"]); e.pending != nil && !e.pending.listAsked.IsZero() && len(games) > 0 {
			e.confirmLobby()
		}
		// The game's Custom Games screen requests listings; the companion
		// does only while joining, from the menus. A listing after leaving
		// shows Custom Games is open, which is one of the menu screens.
		if _, ok := list(p["games"]); ok && !e.inMatch && !e.queued {
			if e.awaitingMenu {
				e.menuReady = true
			} else if !e.busy {
				e.menusKnown = true
			}
		}
	case "SetGlueScreen":
		e.screen, _ = p["screen"].(string)
		switch {
		case slices.Contains(scoreScreens, e.screen):
			// The game shows a score screen only once the match is over, and
			// leaving it for the menus is a click the game doesn't report. So
			// the score screen itself is the return to the menus; the game's
			// own Join Friend works from it.
			e.busy, e.inMatch = true, false
			e.endGreeting()
			e.awaitingMenu, e.menuReady = true, true
			e.resumeAt = e.now().Add(resumeDelay)
			e.pending, e.password = nil, nil
			if e.enabled {
				e.Note("The match is over. Follow resumes in a moment.")
			}
		case slices.Contains(busyScreens, e.screen):
			e.busy, e.menuReady = true, false
			if slices.Contains(matchScreens, e.screen) {
				e.inMatch = true
				e.endGreeting()
			}
			e.pause("Follow is on but paused in this lobby or game screen.")
		case slices.Contains(menuScreens, e.screen):
			e.inMatch = false
			e.menusKnown = true
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
	e.hostReceive(kind, p)
	e.resumeIfReady()
	e.hostStep()
	e.greetStep()
	e.announceOpenLobbies()
	e.evaluate()
}

// Group returns the BattleTags the player follows now. The local player is
// not in it.
func (e *Engine) Group() []string { return e.group() }

// group returns the BattleTags the player has chosen to follow: the team's
// other members when the team is included, then the chosen friends.
func (e *Engine) group() []string {
	var tags []string
	seen := map[string]bool{}
	add := func(tag string) {
		if tag != "" && !seen[tagKey(tag)] {
			seen[tagKey(tag)] = true
			tags = append(tags, tag)
		}
	}
	if e.includeTeam {
		for _, m := range e.team {
			if !m.isSelf {
				add(m.battleTag)
			}
		}
	}
	for _, tag := range e.manualTags {
		add(tag)
	}
	return tags
}

// announceOpenLobbies notes, once per lobby, when someone in the chosen
// group opens a joinable lobby. Watching needs no start: it runs whenever
// the game is connected, whether or not auto-join is on.
func (e *Engine) announceOpenLobbies() {
	if !e.connected {
		return
	}
	for _, tag := range e.group() {
		record, ok := e.roster[tagKey(tag)]
		if !ok {
			continue
		}
		p := e.presenceOf(record)
		marker := attemptKey(tag, p)
		if !p.open || e.seenOpen[marker] {
			continue
		}
		e.seenOpen[marker] = true
		if len(e.seenOpen) > maxAttempts {
			clear(e.seenOpen)
			e.seenOpen[marker] = true
		}
		name, _ := record.data["battleTag"].(string)
		if p.lobby != "" {
			e.Note(name + " opened a lobby: " + p.lobby + ".")
		} else {
			e.Note(name + " opened a lobby.")
		}
	}
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
	record := friendRecord{data: f, seen: e.now(), epoch: epoch}
	e.friends[key] = record
	e.roster[key] = record
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
	e.menusKnown = true
	clear(e.friends)
	e.send(CmdFriendsGetFriends, map[string]any{})
	if e.enabled {
		e.Note("Back in the game's menus. Follow has resumed.")
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
	if !e.pending.listAsked.IsZero() {
		// The same request repeated while the game list loads.
		return
	}
	if e.pending.emptyConfirmationSent {
		friend := e.pending.friend
		e.pending = nil
		if !e.askPassword {
			// Each lobby is tried once, so this one isn't tried again while
			// it stays open.
			e.Note(friend + "'s lobby " + quote(name) + " needs a password, so it was skipped. Switch on Ask for lobby passwords to type one.")
			return
		}
		e.password = &passwordRequest{lobby: name, friend: friend, started: e.now()}
		e.Note("The game is asking for a lobby password.")
		return
	}
	e.pending.started = e.now()
	e.pending.lobby = name
	if e.lastJoin != nil {
		e.lastJoin.Lobby = name
	}
	// The game finds the lobby by name only in the custom game list, which
	// exists once Custom Games has loaded it: switch it on, ask for it each
	// second, and confirm once it has lobbies (or after listWait).
	if !e.send(CmdSendGameListing, map[string]any{}) {
		e.pending = nil
		e.Note("The lobby confirmation could not be sent. Waiting for a new lobby.")
		return
	}
	e.pending.listAsked = e.now()
	e.Note("Finding the lobby " + quote(name) + ".")
}

// confirmLobby answers the game's confirmation with an empty password once
// the game list has loaded.
func (e *Engine) confirmLobby() {
	name := e.pending.lobby
	e.pending.listAsked = time.Time{}
	e.pending.emptyConfirmationSent = true
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
	return e.presenceOf(record)
}

func (e *Engine) presenceOf(record friendRecord) presence {
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
	case isTrue(f["isAway"]):
		label = "Away"
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
	if e.pending != nil && !e.pending.listAsked.IsZero() {
		if e.now().Sub(e.pending.listAsked) >= listWait {
			e.confirmLobby()
		} else {
			// The list fills in the background: ask until it has lobbies.
			e.send(CmdGetGameList, map[string]any{})
		}
	}
	if e.pending != nil && e.now().Sub(e.pending.started) > joinTimeout {
		e.pending = nil
		e.Note("The game didn't confirm the join. Check Warcraft III before trying again.")
	}
	e.hostStep()
	e.greetStep()
	e.evaluate()
}

// ManualTags returns the manually added friend BattleTags.
func (e *Engine) ManualTags() []string { return slices.Clone(e.manualTags) }

// IncludeTeam reports whether the current team is part of the selection.
func (e *Engine) IncludeTeam() bool { return e.includeTeam }

// AskPassword reports whether a lobby that wants a password may ask for it.
func (e *Engine) AskPassword() bool { return e.askPassword }

// JoinHostTeam reports whether Auto-join moves the player to the host's
// team.
func (e *Engine) JoinHostTeam() bool { return e.joinHostTeam }

// Snapshot returns the state for the interface. It never includes account
// identifiers or passwords.
func (e *Engine) Snapshot() dto.FollowState {
	s := dto.FollowState{
		Connected:           e.connected,
		Version:             e.version,
		VerifiedBuild:       e.VerifiedBuild(),
		Enabled:             e.enabled,
		Diagnostic:          e.diagnostic,
		ManualTags:          e.ManualTags(),
		IncludeTeam:         e.includeTeam,
		AskPassword:         e.askPassword,
		JoinHostTeam:        e.joinHostTeam,
		Blacklist:           e.Blacklist(),
		AutoBan:             e.autoBan,
		BlacklistManualBans: e.manualBans,
		Status:              e.status,
		Screen:              e.screen,
		Busy:                e.busy,
		Queued:              e.queued,
		InMatch:             e.inMatch,
		Paused:              e.enabled && (e.busy || e.queued || e.inMatch || e.awaitingMenu),
		InMenus:             e.connected && e.menusKnown && !e.busy && !e.queued && !e.inMatch && !e.awaitingMenu,
		JoinPending:         e.pending != nil,
		SelfTag:             e.selfTag,
		Team:                []dto.TeamMember{},
		Watched:             []dto.WatchedMember{},
		Friends:             []dto.WatchedMember{},
		Roster:              []dto.Friend{},
		Log:                 slices.Clone(e.log),
	}
	if s.ManualTags == nil {
		s.ManualTags = []string{}
	}
	if s.Blacklist == nil {
		s.Blacklist = []dto.BlacklistEntry{}
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
		if !hasRecord(e.friends, tag) {
			// Between snapshots, show what the roster last said.
			if record, found := e.roster[tagKey(tag)]; found {
				p = e.presenceOf(record)
			}
		}
		s.Friends = append(s.Friends, dto.WatchedMember{BattleTag: tag, Label: p.label, Open: p.open, Lobby: p.lobby})
	}
	s.Roster = e.rosterSnapshot()
	s.Host = e.hostSnapshot()
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

func hasRecord(m map[string]friendRecord, tag string) bool {
	_, found := m[tagKey(tag)]
	return found
}

// rosterSnapshot lists every Battle.net friend from the latest snapshot,
// sorted by BattleTag, marking the ones in the follow group.
func (e *Engine) rosterSnapshot() []dto.Friend {
	selected := map[string]bool{}
	for _, tag := range e.manualTags {
		selected[tagKey(tag)] = true
	}
	out := make([]dto.Friend, 0, len(e.roster))
	for key, record := range e.roster {
		f := record.data
		if !isTrue(f["isFriend"]) {
			continue
		}
		tag, _ := f["battleTag"].(string)
		p := e.presenceOf(record)
		out = append(out, dto.Friend{
			BattleTag: tag,
			Label:     p.label,
			Lobby:     p.lobby,
			Online:    isTrue(f["isOnline"]),
			Away:      isTrue(f["isAway"]),
			InGame:    f["currentProgram"] == "W3",
			Open:      p.open,
			Selected:  selected[key],
		})
	}
	slices.SortFunc(out, func(a, b dto.Friend) int { return strings.Compare(tagKey(a.BattleTag), tagKey(b.BattleTag)) })
	return out
}
