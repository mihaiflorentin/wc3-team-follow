package follow

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Commands Auto-host sends. They are the ones the game's own Create Game
// screen and lobby send (GlueManager.js on 3.0.1.24371).
const (
	CmdCreateLobby = "CreateLobby"
	CmdLobbyStart  = "LobbyStart"
	CmdLobbyCancel = "LobbyCancel"
	CmdSetTeam     = "SetTeam"
	CmdInvite      = "SendInviteToGame"
	// CmdSendGameChatMessage is the lobby chat box's send, payload
	// {content}. Only sendChat uses it (docs/adr/0006).
	CmdSendGameChatMessage = "SendGameChatMessage"
)

// MaxChatLine is the game chat's own limit for one line, in characters.
const MaxChatLine = 100

// ErrChatLine reports text sendChat refuses to type.
var ErrChatLine = errors.New("chat line must be one non-empty line of at most 100 characters")

// ChatLine trims text and checks it is one chat line Team Follow may type:
// non-empty, at most MaxChatLine characters, without control characters.
// It returns ErrChatLine otherwise. sendChat and the say command share it.
func ChatLine(text string) (string, error) {
	line := strings.TrimSpace(text)
	if line == "" || utf8.RuneCountInString(line) > MaxChatLine || strings.ContainsFunc(line, unicode.IsControl) {
		return "", ErrChatLine
	}
	return line, nil
}

// sendChat types one line in the game's lobby or match chat.
// It trims the text and rejects it, sending nothing, unless it is a single
// non-empty line of at most MaxChatLine characters without control
// characters. Only Auto-host's own commands (docs/adr/0006) and the greeting
// after joining a lobby as a guest (docs/adr/0010) may be typed: callers
// must first check which case applies. The error is ErrChatLine for bad text, or says the
// game connection is closed.
func (e *Engine) sendChat(text string) error {
	line, err := ChatLine(text)
	if err != nil {
		return err
	}
	if !e.send(CmdSendGameChatMessage, map[string]any{"content": line}) {
		return errors.New("send chat line: the game connection is not open")
	}
	return nil
}

const (
	// MaxGameName is the game's own limit for lobby names.
	MaxGameName = 31
	// GameNumber is the placeholder in a lobby name for the game number.
	GameNumber   = "%d"
	maxHostDelay = 10 * time.Minute
	// createTimeout bounds how long the game may take to open the lobby.
	createTimeout = 20 * time.Second
)

// Host phases.
const (
	HostOff      = "off"
	HostWaiting  = "waiting"
	HostCreating = "creating"
	HostLobby    = "lobby"
	HostMatch    = "match"
)

// Lobby slot states, the game's SlotStatusEnum.
const (
	slotOpen     = 0
	slotOccupied = 2
)

// slotComputer is the slotType of a computer player (players are 0),
// checked live on 3.0.1.24371.
const slotComputer = 1

// lobbyScreens are the screens of a lobby the player is in.
var lobbyScreens = []string{"GAME_LOBBY", "CUSTOM_GAME_LOBBY"}

type hostState struct {
	enabled  bool
	settings dto.HostSettings
	gamePath string
	phase    string
	createAt time.Time
	since    time.Time
	// startSent is set once LobbyStart was sent for the current fill, at
	// startSentAt.
	startSent   bool
	startSentAt time.Time
	// startRetried is set once the player was told a start went unanswered;
	// it resets when the lobby isn't full.
	startRetried bool
	// startDue is set when Start should be pressed again; only
	// SendPendingStart presses it, after the companion handled every event
	// already waiting.
	startDue bool
	// countdown and locked mirror the game's LobbyCountdown details.
	countdown bool
	locked    bool
	players   int
	slots     int
	full      bool
	missing   []string
	offTeam   []string
	// moves remembers SetTeam requests already sent this lobby, keyed by
	// player and target team, so a refused move isn't repeated forever.
	moves map[string]int
	// invited remembers who was invited to this lobby.
	invited map[string]bool
	// heldFull is set once the player was told a full lobby is waiting
	// for the group; it resets when the lobby isn't full.
	heldFull bool
	// swap is Same team's !swap state for this lobby.
	swap swapState
	// lobby is the latest GameLobbySetup, read again when a timer ends.
	lobby map[string]any
	// gameMode is Game mode's progress in the current match.
	gameMode gameModeState
	// number is the game number that %d in the lobby name stands for: 1
	// when Auto-host is turned on, one more each time a hosted game starts.
	number int
}

// swapState throttles !swap: one at a time, each waiting for the lobby to
// change or for swapTimeout, and none after maxSwaps without a change.
type swapState struct {
	// sentAt is when the pending swap was typed; zero when none is pending.
	sentAt time.Time
	// seats is the lobby's seating when it was typed.
	seats string
	// member is the group member the pending swap is for.
	member string
	// unanswered counts swaps after which the member still wasn't on the
	// host's team.
	unanswered int
	// waiting holds when each member was first seen able to move into an
	// open slot on the host's team, which only their own Team Follow can do.
	waiting map[string]time.Time
	// recheckAt is when a member's wait ends and the lobby is read again.
	recheckAt time.Time
}

const (
	// swapTimeout is how long a swap waits for the lobby to change.
	swapTimeout = 5 * time.Second
	// maxSwaps is how many unanswered swaps Same team types per lobby.
	maxSwaps = 3
	// selfMoveWait is how long the host waits for a member to move itself
	// into an open slot on the host's team before swapping instead.
	selfMoveWait = 10 * time.Second
	// startRetry is how long Start when full waits for the countdown after
	// LobbyStart before pressing Start again. The game ignores LobbyStart
	// while a player who just joined isn't ready, such as while they still
	// download the map, and sends no LobbyCountdown then.
	startRetry = 5 * time.Second
)

// NormalizeHostSettings trims and checks the settings Auto-host needs.
func NormalizeHostSettings(s dto.HostSettings) (dto.HostSettings, error) {
	s.GameName = strings.TrimSpace(s.GameName)
	s.MapName = strings.TrimSpace(s.MapName)
	switch {
	case utf8.RuneCountInString(strings.ReplaceAll(s.GameName, GameNumber, "999")) > MaxGameName:
		return s, fmt.Errorf("Lobby names can be up to %d characters long, with room for 3 digits for each %%d.", MaxGameName)
	case !slices.Contains([]int{0, 1, 2, 4}, s.Visibility):
		return s, errors.New("Choose a map visibility.")
	case !slices.Contains([]int{0, 16, 32, 4194304}, s.Observers):
		return s, errors.New("Choose who may observe.")
	case s.DelaySeconds < 0 || time.Duration(s.DelaySeconds)*time.Second > maxHostDelay:
		return s, errors.New("Choose a delay between 0 seconds and 10 minutes.")
	}
	var err error
	s.GameMode, s.GameModeDelaySeconds, err = normalizeGameMode(s.GameMode, s.GameModeDelaySeconds)
	return s, err
}

// HostSettings returns the saved Auto-host settings.
func (e *Engine) HostSettings() dto.HostSettings { return e.host.settings }

// ConfigureHost changes the Auto-host settings, or turns Auto-host on or
// off. Turning it on turns Auto-join off: the host waits for friends in its
// own lobby.
func (e *Engine) ConfigureHost(cfg dto.HostConfig) error {
	if cfg.Settings != nil {
		settings, err := NormalizeHostSettings(*cfg.Settings)
		if err != nil {
			return err
		}
		e.host.settings = settings
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		if e.host.enabled {
			e.host = hostState{settings: e.host.settings, phase: HostOff}
			e.Note("Auto-host is off.")
		}
		return nil
	}
	if cfg.Enabled != nil && *cfg.Enabled {
		return e.enableHost(cfg)
	}
	return nil
}

func (e *Engine) enableHost(cfg dto.HostConfig) error {
	s := e.host.settings
	switch {
	case !e.connected:
		return errors.New("Start Warcraft III and wait for it to connect first.")
	case !e.VerifiedBuild():
		return errors.New("This game build has not been verified. Auto-host is disabled.")
	case s.MapPath == "" || cfg.GameMapPath == "":
		return errors.New("Choose a map to host first.")
	case s.GameName == "":
		return errors.New("Give your lobby a name first.")
	case e.busy || e.queued || e.inMatch || e.awaitingMenu:
		return errors.New("Go back to the game's menus before turning on Auto-host.")
	case !cfg.FromMenus && !e.menusKnown:
		return errors.New("Turn on Auto-host while you are in the game's menus.")
	}
	if e.enabled {
		e.disarm()
		e.Note("Auto-join is off while you host.")
	}
	e.host.enabled, e.host.gamePath, e.host.number = true, cfg.GameMapPath, 1
	e.waitToHost()
	if s.DelaySeconds == 0 {
		e.Note("Auto-host is on. Creating " + quote(e.lobbyName()) + " now.")
	} else {
		e.Note(fmt.Sprintf("Auto-host is on. Creating %s in %d seconds.", quote(e.lobbyName()), s.DelaySeconds))
	}
	e.hostStep()
	return nil
}

func (e *Engine) waitToHost() {
	e.host.phase = HostWaiting
	e.host.createAt = e.now().Add(time.Duration(e.host.settings.DelaySeconds) * time.Second)
	e.host.startSent, e.host.startRetried, e.host.startDue, e.host.countdown, e.host.locked = false, false, false, false, false
	e.host.players, e.host.slots, e.host.full, e.host.missing, e.host.offTeam = 0, 0, false, nil, nil
	e.host.moves, e.host.invited, e.host.swap, e.host.lobby = nil, nil, swapState{}, nil
	e.host.gameMode = gameModeState{}
}

func quote(s string) string { return "“" + s + "”" }

// lobbyName is the name of the lobby Auto-host creates next, or has open:
// the settings' name with each %d replaced by the game number.
func (e *Engine) lobbyName() string {
	return LobbyName(e.host.settings.GameName, e.host.number)
}

// LobbyName replaces each %d in name with number, keeping the result within
// the game's MaxGameName characters.
func LobbyName(name string, number int) string {
	out := strings.ReplaceAll(name, GameNumber, strconv.Itoa(max(number, 1)))
	if r := []rune(out); len(r) > MaxGameName {
		out = strings.TrimSpace(string(r[:MaxGameName]))
	}
	return out
}

// hostedGameStarted moves Auto-host to its match and counts the game, so
// the next lobby's %d is one higher.
func (e *Engine) hostedGameStarted() {
	e.host.phase = HostMatch
	e.host.number++
}

// inMenusNow reports that nothing the game has said puts the player in a
// lobby, match or queue.
func (e *Engine) inMenusNow() bool {
	return e.connected && !e.busy && !e.queued && !e.inMatch && !e.awaitingMenu
}

// hostStep runs Auto-host's timers: creating the lobby once the delay is
// over, and giving up when the game doesn't open it.
func (e *Engine) hostStep() {
	h := &e.host
	if !h.enabled {
		return
	}
	switch h.phase {
	case HostWaiting:
		if !e.inMenusNow() || e.now().Before(h.createAt) {
			return
		}
		payload := map[string]any{
			"filename":    h.gamePath,
			"gameName":    e.lobbyName(),
			"gameSpeed":   2, // fast, the game's default
			"privateGame": h.settings.Private,
			// The game's own Create Game defaults.
			"mapSettings": map[string]any{
				"flagLockTeams":             true,
				"flagPlaceTeamsTogether":    true,
				"flagFullSharedUnitControl": false,
				"flagRandomRaces":           false,
				"flagRandomHero":            false,
				"settingObservers":          h.settings.Observers,
				"settingVisibility":         h.settings.Visibility,
			},
		}
		if !e.send(CmdCreateLobby, payload) {
			e.stopHost("The lobby couldn't be created: the game connection failed. Auto-host is off.")
			return
		}
		h.phase, h.since = HostCreating, e.now()
		e.Note("Creating your lobby " + quote(e.lobbyName()) + " on " + h.settings.MapName + ".")
	case HostCreating:
		if e.now().Sub(h.since) > createTimeout {
			e.stopHost("The game didn't open the lobby. Check the map in Warcraft III; Auto-host is off.")
		}
	case HostLobby:
		if !h.swap.sentAt.IsZero() && e.now().Sub(h.swap.sentAt) >= swapTimeout {
			// The lobby didn't change: the swap went unanswered.
			h.swap.sentAt = time.Time{}
			if e.swapUnanswered() && h.lobby != nil {
				e.lobbyUpdate(h.lobby)
			}
		}
		if !h.swap.recheckAt.IsZero() && !e.now().Before(h.swap.recheckAt) {
			h.swap.recheckAt = time.Time{}
			if h.lobby != nil {
				e.lobbyUpdate(h.lobby)
			}
		}
		if h.startSent && !h.countdown && e.now().Sub(h.startSentAt) >= startRetry {
			// The game didn't start the countdown: press Start again, once
			// the companion has handled every event already waiting.
			h.startSent, h.startDue = false, true
			if !h.startRetried {
				h.startRetried = true
				e.Note("Warcraft III didn't start the game yet, maybe someone is still downloading the map. Team Follow presses Start again every few seconds.")
			}
		}
	case HostMatch:
		e.gameModeStep()
	}
}

// HasPendingStart reports whether Start waits for SendPendingStart.
func (e *Engine) HasPendingStart() bool { return e.host.startDue }

// SendPendingStart presses Start again after the game ignored it, once the
// companion has fed the engine every game event already waiting. It reads
// the newest lobby update again and presses Start only when the lobby is
// still full, the whole group is in and, with Same team, on the host's
// team: nobody left meanwhile. It is never called from a timer.
func (e *Engine) SendPendingStart() {
	h := &e.host
	if !h.startDue {
		return
	}
	h.startDue = false
	if h.enabled && h.phase == HostLobby && h.lobby != nil {
		e.lobbyUpdate(h.lobby)
	}
}

func (e *Engine) stopHost(message string) {
	e.host = hostState{settings: e.host.settings, phase: HostOff}
	e.Note(message)
}

// hostReceive follows the host's own lobby through the game's events.
func (e *Engine) hostReceive(kind string, p map[string]any) {
	h := &e.host
	if !h.enabled {
		return
	}
	switch kind {
	case "SetGlueScreen":
		screen, _ := p["screen"].(string)
		switch {
		case slices.Contains(lobbyScreens, screen) && h.phase == HostCreating:
			e.lobbyOpened()
		case slices.Contains(matchScreens, screen) && h.phase == HostLobby:
			e.hostedGameStarted()
		case slices.Contains(scoreScreens, screen) && h.phase == HostMatch:
			e.gameModeCancel()
			e.waitToHost()
			e.Note(fmt.Sprintf("Next lobby in %d seconds.", h.settings.DelaySeconds))
		}
	case "GameLobbySetup":
		if h.phase == HostCreating {
			e.lobbyOpened()
		}
		if h.phase == HostLobby {
			e.lobbyUpdate(p)
		}
	case "FriendsFriendData", "FriendsFriendUpdated":
		// Someone may have come online: invite them now.
		if h.phase == HostLobby {
			e.inviteMissing()
		}
	case "LobbyCountdown":
		if h.phase == HostLobby {
			details := object(p["details"])
			h.countdown, h.locked = isTrue(details["countdownActive"]), isTrue(details["countdownLocked"])
			if !h.countdown {
				h.startSent = false
			}
		}
	case "PauseUI", "GameFound", "LoadingScreenUpdate":
		if h.phase == HostLobby {
			e.hostedGameStarted()
		}
	case "MultiplayerGameLeave":
		if h.phase == HostLobby {
			e.stopHost("You left your lobby, so Auto-host is off.")
		}
	case "ExitedGame":
		if h.phase == HostMatch {
			e.gameModeCancel()
			e.waitToHost()
		}
	}
	e.gameModeReceive(kind, p)
}

func (e *Engine) lobbyOpened() {
	h := &e.host
	h.phase, h.startSent, h.startRetried, h.startDue, h.countdown, h.locked = HostLobby, false, false, false, false, false
	h.moves, h.invited, h.swap = map[string]int{}, map[string]bool{}, swapState{}
	h.gameMode = gameModeState{}
	if h.settings.StartWhenFull {
		e.Note("Your lobby " + quote(e.lobbyName()) + " is open. It starts once it's full and everyone you follow is in.")
	} else {
		e.Note("Your lobby " + quote(e.lobbyName()) + " is open. Press Start in Warcraft III when everyone's in.")
	}
}

// maxMoves is how often Same team asks the game to move one player to one
// team before giving up on it.
const maxMoves = 2

type lobbyPlayer struct {
	slot     any
	name     string
	team     any
	canMove  bool
	self     bool
	occupied bool
	computer bool
}

// lobbyUpdate reads the lobby: slots, who of the group is in, and with
// SameTeam their teams. With StartWhenFull it starts the game once the
// lobby is full, the whole group is in and, with SameTeam, on the host's
// team; it cancels the start if that stops being true before the
// countdown locks.
func (e *Engine) lobbyUpdate(p map[string]any) {
	h := &e.host
	if _, ok := list(p["players"]); !ok {
		return
	}
	h.lobby = p
	players, openTeams, self := readLobby(p)
	open := len(openTeams)
	h.players, h.slots = 0, open
	for _, pl := range players {
		if pl.occupied {
			h.players++
			h.slots++
		}
	}
	h.full = open == 0 && h.players > 0

	h.missing, h.offTeam = nil, nil
	// A pending swap waits for the lobby to change before anything else
	// moves; once it changes, the swap counts as answered.
	if !h.swap.sentAt.IsZero() && seating(players) != h.swap.seats {
		h.swap.sentAt = time.Time{}
		if pl := findPlayer(players, h.swap.member); pl == nil || self == nil || !sameNumber(pl.team, self.team) {
			e.swapUnanswered()
		}
	}
	acted := !h.swap.sentAt.IsZero()
	for _, tag := range e.group() {
		pl := findPlayer(players, tag)
		switch {
		case pl == nil:
			h.missing = append(h.missing, tag)
		case h.settings.SameTeam && self != nil && !sameNumber(pl.team, self.team):
			h.offTeam = append(h.offTeam, tag)
			// One move per lobby update: the next update shows its result.
			if !acted {
				acted = true
				e.bringTogether(players, openTeams, *self, *pl, tag)
			}
		}
	}

	e.inviteMissing()

	if !h.settings.StartWhenFull {
		return
	}
	ready := h.full && len(h.missing) == 0 && len(h.offTeam) == 0
	switch {
	case !h.full:
		h.heldFull, h.startRetried, h.startDue = false, false, false
	case !ready && !h.heldFull:
		h.heldFull = true
		if len(h.missing) > 0 {
			e.Note("The lobby is full, but it won't start until " + joinTags(h.missing) + " joined. You can still press Start yourself.")
		} else {
			verb := " are"
			if len(h.offTeam) == 1 {
				verb = " is"
			}
			e.Note("The lobby is full, but it won't start until " + joinTags(h.offTeam) + verb + " on your team. You can still press Start yourself.")
		}
	}
	starting := h.countdown || h.startSent
	switch {
	case ready && !starting && !h.startDue:
		if e.send(CmdLobbyStart, map[string]any{}) {
			h.startSent, h.startSentAt = true, e.now()
			if !h.startRetried {
				e.Note("The lobby is full and everyone's in. Starting the game.")
			}
		}
	case !ready && starting:
		if h.locked {
			e.Note("Someone left in the last seconds of the countdown; the game can't stop it now.")
			return
		}
		if e.send(CmdLobbyCancel, map[string]any{}) {
			h.startSent, h.countdown = false, false
			e.Note("The lobby changed, so the start was cancelled. It starts again once it's full and everyone's in.")
		}
	}
}

// readLobby reads a GameLobbySetup's playable slots (observers are left
// out), the team of each open slot, and the player's own slot (nil when
// the player has none).
func readLobby(p map[string]any) (players []lobbyPlayer, openTeams []any, self *lobbyPlayer) {
	raw, _ := list(p["players"])
	for _, r := range raw {
		slot := object(r)
		if isTrue(slot["isObserver"]) {
			continue
		}
		name, _ := slot["name"].(string)
		players = append(players, lobbyPlayer{slot: slot["slot"], name: name, team: slot["team"], canMove: isTrue(slot["teamChangeEnabled"]),
			self: isTrue(slot["isSelf"]), occupied: numberIs(slot["slotStatus"], slotOccupied),
			computer: numberIs(slot["slotType"], slotComputer)})
		if numberIs(slot["slotStatus"], slotOpen) {
			openTeams = append(openTeams, slot["team"])
		}
	}
	for i := range players {
		if players[i].self {
			self = &players[i]
			break
		}
	}
	return players, openTeams, self
}

// inviteMissing invites, once per lobby, each group member who isn't in the
// lobby and is in Warcraft III, as the friends list's Invite to game does.
func (e *Engine) inviteMissing() {
	h := &e.host
	if !h.settings.InviteGroup || h.phase != HostLobby {
		return
	}
	for _, tag := range h.missing {
		key := tagKey(tag)
		record, ok := e.roster[key]
		if !ok || h.invited[key] {
			continue
		}
		f := record.data
		if !isTrue(f["isFriend"]) || !isTrue(f["isOnline"]) || f["currentProgram"] != "W3" || e.now().Sub(record.seen) > freshFor {
			continue
		}
		name, _ := f["battleTag"].(string)
		h.invited[key] = true
		if e.send(CmdInvite, map[string]any{"target": name}) {
			e.Note("Invited " + name + " to your lobby.")
		}
	}
}

// findPlayer matches a BattleTag to a lobby player, by full tag or by name.
func findPlayer(players []lobbyPlayer, tag string) *lobbyPlayer {
	for i, pl := range players {
		if pl.occupied && sameTag(pl.name, tag) {
			return &players[i]
		}
	}
	return nil
}

// bringTogether brings a group member onto the host's team. The host never
// moves, and no computer is moved for it: moving the host would split it
// from the rest of the group. In order: move the member when the game
// allows it; wait for the member's own Team Follow to take an open slot on
// the host's team; trade the member with a player on the host's team who
// isn't in the group, using the game's !swap (which only trades players).
func (e *Engine) bringTogether(players []lobbyPlayer, openTeams []any, self, pl lobbyPlayer, tag string) {
	switch {
	case pl.canMove && hasTeam(openTeams, self.team):
		e.moveSlot(pl.slot, self.team, tagKey(tag), "Moving "+tag+" to your team.")
	case hasTeam(openTeams, self.team) && e.waitForSelfMove(tag):
	case e.swapWithStranger(players, self, pl, tag):
	default:
		key := tagKey(tag) + ":stuck"
		if e.host.moves[key] == 0 {
			e.host.moves[key] = 1
			e.Note(tag + " can't be moved to your team: there's no open slot on it and no other player on it to swap with.")
		}
	}
}

// waitForSelfMove gives a member selfMoveWait to move into the open slot on
// the host's team, which the game lets only their own Team Follow do, and
// says so once. It reports false once the wait is over.
func (e *Engine) waitForSelfMove(tag string) bool {
	h := &e.host
	if h.swap.waiting == nil {
		h.swap.waiting = map[string]time.Time{}
	}
	key := tagKey(tag)
	since, ok := h.swap.waiting[key]
	if !ok {
		since = e.now()
		h.swap.waiting[key] = since
		h.swap.recheckAt = since.Add(selfMoveWait)
		e.Note("Waiting for " + tag + " to move to your team. Their Team Follow does that; the game's swap command can't use an open slot.")
	}
	return e.now().Sub(since) < selfMoveWait
}

// swapWithStranger types the game's own lobby command !swap to trade a
// group member with a human on the host's team who isn't in the group.
// !swap numbers the playable positions slot + 1 and only trades two
// players: open slots and the map's computers are never targets (checked
// live on 3.0.1.24371, docs/follow-behaviour.md).
//
// It reports handled: true when swapping took care of the member, because
// a swap was asked for, or swaps are held while the countdown runs, or
// they were given up in this lobby (already said once). It reports false
// when there is no stranger to swap with, or the swap couldn't be typed,
// so the caller says the two can't be put on one team.
func (e *Engine) swapWithStranger(players []lobbyPlayer, self, pl lobbyPlayer, tag string) (handled bool) {
	h := &e.host
	if h.swap.unanswered >= maxSwaps {
		// Already said once that the game didn't answer.
		return true
	}
	if h.countdown || h.locked {
		// Nothing is typed into a starting game.
		return true
	}
	from, ok := position(pl.slot)
	if !ok {
		return false
	}
	group := e.group()
	for _, other := range players {
		to, ok := position(other.slot)
		if !ok || !other.occupied || other.computer || other.self || !sameNumber(other.team, self.team) ||
			slices.ContainsFunc(group, func(t string) bool { return sameTag(other.name, t) }) {
			continue
		}
		if e.sendChat(fmt.Sprintf("!swap %d %d", from, to)) != nil {
			return false
		}
		h.swap.sentAt, h.swap.seats, h.swap.member = e.now(), seating(players), tag
		e.Note("Asked the game to swap " + tag + " with " + other.name + ".")
		return true
	}
	return false
}

// swapUnanswered counts a swap that didn't bring its member to the host's
// team, and says once when Same team stops typing it in this lobby. It
// reports whether more swaps may be typed.
func (e *Engine) swapUnanswered() bool {
	h := &e.host
	h.swap.unanswered++
	if h.swap.unanswered < maxSwaps {
		return true
	}
	if h.swap.unanswered == maxSwaps {
		e.Note(fmt.Sprintf("The game didn't answer the swap command %d times, so Team Follow stopped asking in this lobby. Move players yourself.", maxSwaps))
	}
	return false
}

// seating describes who sits where on which team, to tell whether the
// lobby changed after a swap.
func seating(players []lobbyPlayer) string {
	var b strings.Builder
	for _, pl := range players {
		if pl.occupied {
			fmt.Fprintf(&b, "%v=%s:%v;", pl.slot, tagKey(pl.name), pl.team)
		}
	}
	return b.String()
}

// position is a slot's number in the game's !swap command: slot + 1.
func position(slot any) (int, bool) {
	n, ok := intValue(slot)
	if !ok || n < 0 {
		return 0, false
	}
	return int(n) + 1, true
}

// moveSlot asks the game to move a slot to a team, the way the lobby's team
// menu does, at most maxMoves times per mover and team.
func (e *Engine) moveSlot(slot, team any, who, note string) {
	h := &e.host
	key := who + ":" + fmt.Sprint(team)
	if h.moves[key] >= maxMoves {
		return
	}
	h.moves[key]++
	if e.send(CmdSetTeam, map[string]any{"slot": slot, "team": team}) {
		e.Note(note)
	}
}

// hasTeam reports whether teams holds team.
func hasTeam(teams []any, team any) bool {
	return slices.ContainsFunc(teams, func(t any) bool { return sameNumber(t, team) })
}

// sameNumber reports whether a and b are the same number, however the
// game encoded it, or print the same otherwise.
func sameNumber(a, b any) bool {
	if x, ok := intValue(a); ok {
		if y, ok := intValue(b); ok {
			return x == y
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func (e *Engine) hostSnapshot() dto.HostState {
	h := e.host
	phase := h.phase
	if !h.enabled || phase == "" {
		phase = HostOff
	}
	// Lists are never nil: the window reads them as arrays.
	s := dto.HostState{Enabled: h.enabled, Settings: h.settings, Phase: phase, LobbyName: e.lobbyName(), Players: h.players, Slots: h.slots, Full: h.full,
		Missing: append([]string{}, h.missing...), OffTeam: append([]string{}, h.offTeam...)}
	if h.enabled && phase == HostWaiting {
		at := h.createAt.UTC()
		s.CreateAt = &at
	}
	return s
}

// joinTags lists BattleTags as "A", "A and B" or "A, B and C".
func joinTags(tags []string) string {
	switch len(tags) {
	case 0:
		return ""
	case 1:
		return tags[0]
	}
	return strings.Join(tags[:len(tags)-1], ", ") + " and " + tags[len(tags)-1]
}
