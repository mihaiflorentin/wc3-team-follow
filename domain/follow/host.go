package follow

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
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
)

const (
	// MaxGameName is the game's own limit for lobby names.
	MaxGameName  = 31
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
	// startSent is set once LobbyStart was sent for the current fill.
	startSent bool
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
}

// NormalizeHostSettings trims and checks the settings Auto-host needs.
func NormalizeHostSettings(s dto.HostSettings) (dto.HostSettings, error) {
	s.GameName = strings.TrimSpace(s.GameName)
	s.MapName = strings.TrimSpace(s.MapName)
	switch {
	case utf8.RuneCountInString(s.GameName) > MaxGameName:
		return s, fmt.Errorf("Lobby names can be up to %d characters long.", MaxGameName)
	case !slices.Contains([]int{0, 1, 2, 4}, s.Visibility):
		return s, errors.New("Choose a map visibility.")
	case !slices.Contains([]int{0, 16, 32, 4194304}, s.Observers):
		return s, errors.New("Choose who may observe.")
	case s.DelaySeconds < 0 || time.Duration(s.DelaySeconds)*time.Second > maxHostDelay:
		return s, errors.New("Choose a delay between 0 seconds and 10 minutes.")
	}
	return s, nil
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
	e.host.enabled, e.host.gamePath = true, cfg.GameMapPath
	e.waitToHost()
	if s.DelaySeconds == 0 {
		e.Note("Auto-host is on. Creating " + quote(s.GameName) + " now.")
	} else {
		e.Note(fmt.Sprintf("Auto-host is on. Creating %s in %d seconds.", quote(s.GameName), s.DelaySeconds))
	}
	e.hostStep()
	return nil
}

func (e *Engine) waitToHost() {
	e.host.phase = HostWaiting
	e.host.createAt = e.now().Add(time.Duration(e.host.settings.DelaySeconds) * time.Second)
	e.host.startSent, e.host.countdown, e.host.locked = false, false, false
	e.host.players, e.host.slots, e.host.full, e.host.missing, e.host.offTeam = 0, 0, false, nil, nil
	e.host.moves, e.host.invited = nil, nil
}

func quote(s string) string { return "“" + s + "”" }

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
			"gameName":    h.settings.GameName,
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
		e.Note("Creating your lobby " + quote(h.settings.GameName) + " on " + h.settings.MapName + ".")
	case HostCreating:
		if e.now().Sub(h.since) > createTimeout {
			e.stopHost("The game didn't open the lobby. Check the map in Warcraft III; Auto-host is off.")
		}
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
			h.phase = HostMatch
		case slices.Contains(scoreScreens, screen) && h.phase == HostMatch:
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
			h.phase = HostMatch
		}
	case "MultiplayerGameLeave":
		if h.phase == HostLobby {
			e.stopHost("You left your lobby, so Auto-host is off.")
		}
	case "ExitedGame":
		if h.phase == HostMatch {
			e.waitToHost()
		}
	}
}

func (e *Engine) lobbyOpened() {
	h := &e.host
	h.phase, h.startSent, h.countdown, h.locked = HostLobby, false, false, false
	h.moves, h.invited = map[string]int{}, map[string]bool{}
	if h.settings.StartWhenFull {
		e.Note("Your lobby " + quote(h.settings.GameName) + " is open. It starts once it's full and everyone you follow is in.")
	} else {
		e.Note("Your lobby " + quote(h.settings.GameName) + " is open. Press Start in Warcraft III when everyone's in.")
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
	raw, ok := list(p["players"])
	if !ok {
		return
	}
	var players []lobbyPlayer
	var openTeams []any
	open := 0
	var self *lobbyPlayer
	for _, r := range raw {
		slot := object(r)
		if isTrue(slot["isObserver"]) {
			continue
		}
		name, _ := slot["name"].(string)
		pl := lobbyPlayer{slot: slot["slot"], name: name, team: slot["team"], canMove: isTrue(slot["teamChangeEnabled"]),
			self: isTrue(slot["isSelf"]), occupied: numberIs(slot["slotStatus"], slotOccupied),
			computer: numberIs(slot["slotType"], slotComputer)}
		if numberIs(slot["slotStatus"], slotOpen) {
			open++
			openTeams = append(openTeams, slot["team"])
		}
		players = append(players, pl)
		if pl.self {
			self = &players[len(players)-1]
		}
	}
	h.players, h.slots = 0, open
	for _, pl := range players {
		if pl.occupied {
			h.players++
			h.slots++
		}
	}
	h.full = open == 0 && h.players > 0

	h.missing, h.offTeam = nil, nil
	acted := false
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
		h.heldFull = false
	case !ready && !h.heldFull:
		h.heldFull = true
		if len(h.missing) > 0 {
			e.Note("The lobby is full, but it won't start until " + joinTags(h.missing) + " joined. You can still press Start yourself.")
		} else {
			e.Note("The lobby is full, but it won't start until " + joinTags(h.offTeam) + " are on your team. You can still press Start yourself.")
		}
	}
	starting := h.countdown || h.startSent
	switch {
	case ready && !starting:
		if e.send(CmdLobbyStart, map[string]any{}) {
			h.startSent = true
			e.Note("The lobby is full and everyone's in. Starting the game.")
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
	key := tagKey(tag)
	name, _, _ := strings.Cut(key, "#")
	for i, pl := range players {
		n := tagKey(pl.name)
		if pl.occupied && (n == key || n == name) {
			return &players[i]
		}
	}
	return nil
}

// bringTogether puts the host and a group member on one team with what the
// game lets the host do (checked live on 3.0.1.24371): move its own slot
// and, on maps without fixed forces, computers, each into an open slot.
// Other players' slots can't be moved by the host (teamChangeEnabled is
// false for them).
func (e *Engine) bringTogether(players []lobbyPlayer, openTeams []any, self, pl lobbyPlayer, tag string) {
	switch {
	case pl.canMove && hasTeam(openTeams, self.team):
		e.moveSlot(pl.slot, self.team, tagKey(tag), "Moving "+tag+" to your team.")
	case self.canMove && hasTeam(openTeams, pl.team):
		e.moveSlot(self.slot, pl.team, "self", "Moving you to "+tag+"'s team.")
	case pl.canMove && e.makeRoom(players, openTeams, self.team, pl.team, tag):
	case self.canMove && e.makeRoom(players, openTeams, pl.team, self.team, tag):
	default:
		key := tagKey(tag) + ":stuck"
		if e.host.moves[key] == 0 {
			e.host.moves[key] = 1
			e.Note("You and " + tag + " can't be put on one team: the game only lets the host move itself and computers, and there's no open slot for that.")
		}
	}
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

// makeRoom moves a computer off a full team to a team with an open slot,
// preferring prefer, so a player can take its place on the next lobby
// update. It reports false when no computer can move, there's no open slot
// for it, or the move was already tried maxMoves times.
func (e *Engine) makeRoom(players []lobbyPlayer, openTeams []any, full, prefer any, tag string) bool {
	var comp *lobbyPlayer
	for i, pl := range players {
		if pl.occupied && pl.computer && pl.canMove && sameNumber(pl.team, full) {
			comp = &players[i]
			break
		}
	}
	var dest any
	if hasTeam(openTeams, prefer) {
		dest = prefer
	} else {
		for _, t := range openTeams {
			if !sameNumber(t, full) {
				dest = t
				break
			}
		}
	}
	if comp == nil || dest == nil || e.host.moves["computer:"+fmt.Sprint(comp.slot)+":"+fmt.Sprint(dest)] >= maxMoves {
		return false
	}
	e.moveSlot(comp.slot, dest, "computer:"+fmt.Sprint(comp.slot), "Moving "+comp.name+" to make room for "+tag+".")
	return true
}

// hasTeam reports whether teams holds team.
func hasTeam(teams []any, team any) bool {
	return slices.ContainsFunc(teams, func(t any) bool { return sameNumber(t, team) })
}

func sameNumber(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func (e *Engine) hostSnapshot() dto.HostState {
	h := e.host
	phase := h.phase
	if !h.enabled || phase == "" {
		phase = HostOff
	}
	// Lists are never nil: the window reads them as arrays.
	s := dto.HostState{Enabled: h.enabled, Settings: h.settings, Phase: phase, Players: h.players, Slots: h.slots, Full: h.full,
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
