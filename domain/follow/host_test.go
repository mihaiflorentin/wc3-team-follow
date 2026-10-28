package follow_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func hostSettings() dto.HostSettings {
	return dto.HostSettings{MapPath: "/maps/Legion TD.w3x", MapName: "Legion TD", GameName: "LTD friends", DelaySeconds: 10, Visibility: 2, Observers: 32}
}

func (f *fixture) host(settings dto.HostSettings) error {
	return f.engine.ConfigureHost(dto.HostConfig{Settings: &settings, Enabled: ptr(true), FromMenus: true, GameMapPath: `C:\maps\Legion TD.w3x`})
}

func (f *fixture) mustHost(settings dto.HostSettings) {
	f.t.Helper()
	if err := f.host(settings); err != nil {
		f.t.Fatalf("host: %v", err)
	}
}

func (f *fixture) lastCommand(message string) (command, bool) {
	for i := len(f.commands) - 1; i >= 0; i-- {
		if f.commands[i].message == message {
			return f.commands[i], true
		}
	}
	return command{}, false
}

// seat is one playable lobby slot: an empty name is an open slot.
type seat struct {
	name     string
	team     int
	canMove  bool
	computer bool
}

// lobbyWith builds a GameLobbySetup; the first seat is the host. An open
// observer slot is added, which never counts.
func lobbyWith(seats ...seat) map[string]any {
	var players []any
	for i, s := range seats {
		status := 2
		if s.name == "" {
			status = 0
		}
		slotType := 0
		if s.computer {
			slotType = 1
		}
		players = append(players, map[string]any{"slot": i, "slotStatus": status, "slotType": slotType, "name": s.name, "team": s.team,
			"teamChangeEnabled": s.canMove, "isSelf": i == 0})
	}
	players = append(players, map[string]any{"slot": 99, "slotStatus": 0, "isObserver": true})
	return map[string]any{"players": players}
}

// lobbySetup is a two-slot lobby: the host, plus the teammate Friend#1234
// when the second state is occupied.
func lobbySetup(states ...int) map[string]any {
	seats := []seat{{name: "Self#1111"}}
	for _, st := range states[1:] {
		if st == 2 {
			seats = append(seats, seat{name: "Friend#1234"})
		} else {
			seats = append(seats, seat{})
		}
	}
	return lobbyWith(seats...)
}

func countdown(active, locked bool) map[string]any {
	return map[string]any{"details": map[string]any{"countdownActive": active, "countdownLocked": locked}}
}

func (f *fixture) openOwnLobby() {
	f.receive("MultiplayerGameCreateStarted", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	f.receive("GameLobbySetup", lobbySetup(2, 0))
}

func TestAutoHostCreatesTheLobbyAfterTheDelay(t *testing.T) {
	f := newFixture(t)
	f.mustHost(hostSettings())
	if f.count(follow.CmdCreateLobby) != 0 || f.state().Host.Phase != follow.HostWaiting || f.state().Host.CreateAt == nil {
		t.Fatalf("created before the delay: %+v", f.state().Host)
	}
	f.advance(10 * time.Second)
	f.engine.Tick()
	c, ok := f.lastCommand(follow.CmdCreateLobby)
	if !ok {
		t.Fatal("no CreateLobby after the delay")
	}
	settings := c.payload["mapSettings"].(map[string]any)
	if c.payload["filename"] != `C:\maps\Legion TD.w3x` || c.payload["gameName"] != "LTD friends" || c.payload["privateGame"] != false ||
		c.payload["gameSpeed"] != 2 || settings["settingVisibility"] != 2 || settings["flagLockTeams"] != true || settings["settingObservers"] != 32 {
		t.Fatalf("payload = %+v", c.payload)
	}
	f.openOwnLobby()
	if s := f.state().Host; s.Phase != follow.HostLobby || s.Players != 1 || s.Slots != 2 || s.Full {
		t.Fatalf("host = %+v", s)
	}
	if f.count(follow.CmdLobbyStart) != 0 {
		t.Fatal("started without StartWhenFull")
	}
}

func TestAutoHostTypesNothingWhenTheGamesControlsSuffice(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.StartWhenFull, s.SameTeam = true, true
	f.mustHost(s)
	f.advance(10 * time.Second)
	f.engine.Tick()
	f.openOwnLobby()
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Friend#1234", team: 1}, seat{team: 1}))
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"})
	f.advance(time.Minute)
	f.engine.Tick()
	if f.count(follow.CmdCreateLobby) < 2 {
		t.Fatal("the flow did not run through a full cycle")
	}
	if n := f.count(follow.CmdSendGameChatMessage); n != 0 {
		t.Fatalf("%d chat messages sent", n)
	}
}

func TestAutoHostNeedsMapNameConnectionAndMenus(t *testing.T) {
	f := newFixture(t)
	noMap := hostSettings()
	noMap.MapPath = ""
	wantErr(t, f.host(noMap), "map")
	noName := hostSettings()
	noName.GameName = "  "
	wantErr(t, f.host(noName), "name")
	long := hostSettings()
	long.GameName = strings.Repeat("x", 32)
	wantErr(t, f.host(long), "31")
	badVis := hostSettings()
	badVis.Visibility = 3
	wantErr(t, f.host(badVis), "visibility")
	badObs := hostSettings()
	badObs.Observers = 7
	wantErr(t, f.host(badObs), "observe")
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	wantErr(t, f.host(hostSettings()), "menus")
	f.engine.Connection(false, "")
	wantErr(t, f.host(hostSettings()), "Start Warcraft")
}

func TestStartWhenFullStartsCancelsAndRestarts(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds, s.StartWhenFull = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	f.receive("GameLobbySetup", lobbySetup(2, 2))
	if f.count(follow.CmdLobbyStart) != 1 || !f.state().Host.Full {
		t.Fatalf("did not start a full lobby: starts=%d", f.count(follow.CmdLobbyStart))
	}
	f.receive("GameLobbySetup", lobbySetup(2, 2))
	if f.count(follow.CmdLobbyStart) != 1 {
		t.Fatal("started twice")
	}
	// The countdown runs; then someone leaves.
	f.receive("LobbyCountdown", countdown(true, false))
	f.receive("GameLobbySetup", lobbySetup(2, 0))
	if f.count(follow.CmdLobbyCancel) != 1 || f.count(follow.CmdLobbyStart) != 1 {
		t.Fatalf("cancel=%d start=%d", f.count(follow.CmdLobbyCancel), f.count(follow.CmdLobbyStart))
	}
	// Full again: start again.
	f.receive("GameLobbySetup", lobbySetup(2, 2))
	if f.count(follow.CmdLobbyStart) != 2 {
		t.Fatal("did not restart once full again")
	}
	if notes(f, "start was cancelled") != 1 {
		t.Error("no note about the cancelled start")
	}
}

func TestALockedCountdownIsNotCancelled(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds, s.StartWhenFull = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	f.receive("GameLobbySetup", lobbySetup(2, 2))
	f.receive("LobbyCountdown", countdown(true, true))
	f.receive("GameLobbySetup", lobbySetup(2, 0))
	if f.count(follow.CmdLobbyCancel) != 0 {
		t.Fatal("cancelled a locked countdown")
	}
}

func TestAutoHostRehostsAfterEachMatch(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 5
	f.mustHost(s)
	f.advance(5 * time.Second)
	f.engine.Tick()
	f.openOwnLobby()
	f.receive("GameLobbyGracefulExit", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "DISABLED_SCREEN"})
	if f.state().Host.Phase != follow.HostMatch {
		t.Fatalf("phase = %s", f.state().Host.Phase)
	}
	f.receive("ExitedGame", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"})
	f.receive("MultiplayerGameLeave", nil)
	if f.state().Host.Phase != follow.HostWaiting || !f.state().Host.Enabled {
		t.Fatalf("host = %+v", f.state().Host)
	}
	f.advance(3 * time.Second)
	f.engine.Tick()
	if f.count(follow.CmdCreateLobby) != 1 {
		t.Fatal("re-hosted before the delay")
	}
	f.advance(3 * time.Second)
	f.engine.Tick()
	if f.count(follow.CmdCreateLobby) != 2 {
		t.Fatalf("did not re-host: creates=%d", f.count(follow.CmdCreateLobby))
	}
}

func TestLeavingYourOwnLobbyTurnsAutoHostOff(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.mustHost(s)
	f.openOwnLobby()
	f.receive("MultiplayerGameLeave", nil)
	if f.state().Host.Enabled {
		t.Fatal("still hosting after leaving the lobby")
	}
}

func TestAutoHostGivesUpWhenTheLobbyNeverOpens(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.mustHost(s)
	f.advance(21 * time.Second)
	f.engine.Tick()
	if f.state().Host.Enabled || notes(f, "didn't open the lobby") != 1 || f.count(follow.CmdCreateLobby) != 1 {
		t.Fatalf("host = %+v", f.state().Host)
	}
}

func TestAutoHostAndAutoJoinAreExclusive(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.mustHost(hostSettings())
	if f.state().Enabled {
		t.Fatal("Auto-join stayed on after Auto-host was turned on")
	}
	f.mustArm(false)
	if f.state().Host.Enabled {
		t.Fatal("Auto-host stayed on after Auto-join was turned on")
	}
}

func TestStartWaitsForTheWholeGroup(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214"}})
	s := hostSettings()
	s.DelaySeconds, s.StartWhenFull = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	// Full, but with a stranger instead of Jaina.
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111"}, seat{name: "Friend#1234"}, seat{name: "Stranger#5555"}))
	if f.count(follow.CmdLobbyStart) != 0 {
		t.Fatal("started without the whole group")
	}
	if m := f.state().Host.Missing; len(m) != 1 || m[0] != "Jaina#2214" {
		t.Fatalf("missing = %v", m)
	}
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111"}, seat{name: "Friend#1234"}, seat{name: "jaina"}))
	if f.count(follow.CmdLobbyStart) != 1 {
		t.Fatal("did not start once everyone was in (matched by name)")
	}
}

func sameTeamHost(t *testing.T, startWhenFull bool) *fixture {
	t.Helper()
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds, s.StartWhenFull, s.SameTeam = 0, startWhenFull, true
	f.mustHost(s)
	f.openOwnLobby()
	return f
}

// The live case on 3.0.1.24371 (Castle Fight): the host's force holds the
// host and two locked computers, the friend's force has open slots, and
// the game lets the host move only its own slot.
func TestSameTeamMovesTheHostToTheMembersTeam(t *testing.T) {
	f := sameTeamHost(t, true)
	lobby := lobbyWith(seat{name: "Self#1111", team: 0, canMove: true},
		seat{name: "Computer (Normal)", team: 0, computer: true}, seat{name: "Computer (Normal)", team: 0, computer: true},
		seat{name: "Friend#1234", team: 1}, seat{team: 1}, seat{team: 1})
	f.receive("GameLobbySetup", lobby)
	c, ok := f.lastCommand(follow.CmdSetTeam)
	if !ok || c.payload["slot"] != 0 || c.payload["team"] != 1 || notes(f, "Moving you to Friend#1234's team.") != 1 {
		t.Fatalf("SetTeam = %+v, %v, log %+v", c, ok, f.state().Log)
	}
	if len(f.state().Host.OffTeam) != 1 {
		t.Fatal("the friend wasn't reported off the team")
	}
	// A refused move is retried a limited number of times.
	for range 3 {
		f.receive("GameLobbySetup", lobby)
	}
	if f.count(follow.CmdSetTeam) != 2 {
		t.Fatalf("SetTeam sent %d times", f.count(follow.CmdSetTeam))
	}
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 1, canMove: true},
		seat{name: "Computer (Normal)", team: 0, computer: true}, seat{name: "Computer (Normal)", team: 0, computer: true},
		seat{name: "Friend#1234", team: 1}, seat{team: 0}, seat{team: 1}))
	if len(f.state().Host.OffTeam) != 0 {
		t.Fatalf("still off team: %v", f.state().Host.OffTeam)
	}
}

func TestSameTeamMovesAMovableMemberToTheHostsTeam(t *testing.T) {
	f := sameTeamHost(t, true)
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{team: 0},
		seat{name: "Friend#1234", team: 1, canMove: true}))
	c, ok := f.lastCommand(follow.CmdSetTeam)
	if !ok || c.payload["slot"] != 2 || c.payload["team"] != 0 {
		t.Fatalf("SetTeam = %+v, %v", c, ok)
	}
	if f.count(follow.CmdLobbyStart) != 0 {
		t.Fatal("started before the group was on one team")
	}
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Friend#1234", team: 0, canMove: true},
		seat{name: "Stranger#5555", team: 1}))
	if f.count(follow.CmdLobbyStart) != 1 {
		t.Fatal("did not start once everyone was on the host's team")
	}
}

func TestSameTeamMovesAComputerToMakeRoomForTheHost(t *testing.T) {
	f := sameTeamHost(t, false)
	// The friend's team is full with a computer that can move; the host's
	// team has room for it.
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Computer (Normal)", team: 1, canMove: true, computer: true}))
	c, ok := f.lastCommand(follow.CmdSetTeam)
	if !ok || c.payload["slot"] != 3 || c.payload["team"] != 0 || notes(f, "Moving Computer (Normal) to make room for Friend#1234.") != 1 {
		t.Fatalf("SetTeam = %+v, %v, log %+v", c, ok, f.state().Log)
	}
	// The computer moved, so the friend's team has room: now the host moves.
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Computer (Normal)", team: 0, canMove: true, computer: true},
		seat{name: "Friend#1234", team: 1}, seat{team: 1}))
	c, ok = f.lastCommand(follow.CmdSetTeam)
	if !ok || c.payload["slot"] != 0 || c.payload["team"] != 1 {
		t.Fatalf("SetTeam = %+v, %v", c, ok)
	}
}

// The host's team is full with a stranger, so the game's own controls can't
// help: the host types the game's !swap with lobby positions (slot + 1).
func TestSameTeamSwapsAMemberWithAStrangerOnTheHostsTeam(t *testing.T) {
	f := sameTeamHost(t, true)
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Player#5678", team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Computer (Normal)", team: 1, computer: true}))
	c, ok := f.lastCommand(follow.CmdSendGameChatMessage)
	if !ok || c.payload["content"] != "!swap 3 2" || f.count(follow.CmdSetTeam) != 0 {
		t.Fatalf("chat = %+v, %v, setTeam=%d", c, ok, f.count(follow.CmdSetTeam))
	}
	if notes(f, "Asked the game to swap Friend#1234 with Player#5678.") != 1 {
		t.Fatalf("log = %+v", f.state().Log)
	}
	if f.count(follow.CmdLobbyStart) != 0 {
		t.Fatal("started before the swap")
	}
	// The swap worked: the group is together and the full lobby starts.
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Friend#1234", team: 0},
		seat{name: "Player#5678", team: 1}, seat{name: "Computer (Normal)", team: 1, computer: true}))
	if f.count(follow.CmdSendGameChatMessage) != 1 || f.count(follow.CmdLobbyStart) != 1 {
		t.Fatalf("chat=%d start=%d", f.count(follow.CmdSendGameChatMessage), f.count(follow.CmdLobbyStart))
	}
}

func strangerOnHostsTeam() map[string]any {
	return lobbyWith(seat{name: "Self#1111", team: 0, canMove: true}, seat{name: "Player#5678", team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Computer (Normal)", team: 1, computer: true})
}

func TestSameTeamSwapsOneAtATimeAndGivesUpAfterThreeUnansweredSwaps(t *testing.T) {
	f := sameTeamHost(t, false)
	f.receive("GameLobbySetup", strangerOnHostsTeam())
	// Updates that change nothing wait for the swap's answer.
	f.receive("GameLobbySetup", strangerOnHostsTeam())
	f.advance(2 * time.Second)
	f.engine.Tick()
	if n := f.count(follow.CmdSendGameChatMessage); n != 1 {
		t.Fatalf("%d swaps before the first was answered", n)
	}
	// No change within the timeout: ask again, up to three times.
	for range 5 {
		f.advance(10 * time.Second)
		f.engine.Tick()
		f.receive("GameLobbySetup", strangerOnHostsTeam())
	}
	if n := f.count(follow.CmdSendGameChatMessage); n != 3 {
		t.Fatalf("%d swaps, want 3", n)
	}
	if notes(f, "didn't answer the swap") != 1 {
		t.Fatalf("log = %+v", f.state().Log)
	}
	// The next lobby starts counting again.
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"})
	f.advance(3 * time.Second)
	f.engine.Tick()
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	f.receive("GameLobbySetup", strangerOnHostsTeam())
	if n := f.count(follow.CmdSendGameChatMessage); n != 4 {
		t.Fatalf("%d swaps after a new lobby, want 4", n)
	}
}

func TestSameTeamStopsSwappingOnceTheCountdownRuns(t *testing.T) {
	f := sameTeamHost(t, false)
	f.receive("LobbyCountdown", countdown(true, true))
	f.receive("GameLobbySetup", strangerOnHostsTeam())
	if n := f.count(follow.CmdSendGameChatMessage); n != 0 {
		t.Fatalf("%d swaps typed into a starting game", n)
	}
}

func TestSameTeamNeverSwapsGroupMembersOffTheHostsTeam(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Arthas#1123"}})
	s := hostSettings()
	s.DelaySeconds, s.SameTeam = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0}, seat{name: "Arthas#1123", team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Player#5678", team: 1}))
	if n := f.count(follow.CmdSendGameChatMessage); n != 0 {
		t.Fatalf("%d swaps sent a group member away", n)
	}
}

// An open slot on the host's team can't be used by !swap: the member's own
// Team Follow moves there. The host waits for that, then swaps with a
// stranger in case the member doesn't run Team Follow.
func TestSameTeamWaitsForTheMemberToMoveIntoAnOpenSlot(t *testing.T) {
	f := sameTeamHost(t, false)
	waiting := lobbyWith(seat{name: "Self#1111", team: 0}, seat{name: "Player#5678", team: 0}, seat{team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Arthas#1123", team: 1})
	f.receive("GameLobbySetup", waiting)
	f.advance(5 * time.Second)
	f.engine.Tick()
	if n := f.count(follow.CmdSendGameChatMessage); n != 0 || notes(f, "Waiting for Friend#1234 to move to your team") != 1 {
		t.Fatalf("chat=%d log=%+v", n, f.state().Log)
	}
	f.advance(6 * time.Second)
	f.engine.Tick()
	if c, ok := f.lastCommand(follow.CmdSendGameChatMessage); !ok || c.payload["content"] != "!swap 4 2" {
		t.Fatalf("chat = %+v, %v", c, ok)
	}
}

func TestSameTeamSaysOnceWhenNothingCanMove(t *testing.T) {
	f := sameTeamHost(t, false)
	// Fixed forces: nobody can move, the host's team holds only a computer
	// and there's no open slot.
	for range 2 {
		f.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0}, seat{name: "Computer (Normal)", team: 0, computer: true},
			seat{name: "Friend#1234", team: 1}, seat{name: "Computer (Normal)", team: 1, computer: true}))
	}
	if f.count(follow.CmdSetTeam) != 0 || f.count(follow.CmdSendGameChatMessage) != 0 || notes(f, "You and Friend#1234 can't be put on one team") != 1 {
		t.Fatalf("setTeam=%d chat=%d log=%+v", f.count(follow.CmdSetTeam), f.count(follow.CmdSendGameChatMessage), f.state().Log)
	}
}

func TestAutoInviteInvitesTheGroupOncePerLobby(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214", "Thrall#1820"}})
	f.friendsSnapshot(
		closed("Jaina#2214", 2, map[string]any{"currentProgram": "W3"}),
		closed("Thrall#1820", 3, map[string]any{"isOnline": false}),
	)
	s := hostSettings()
	s.DelaySeconds, s.InviteGroup = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	c, ok := f.lastCommand(follow.CmdInvite)
	if !ok || c.payload["target"] != "Jaina#2214" || f.count(follow.CmdInvite) != 1 {
		t.Fatalf("invites = %d, last = %+v", f.count(follow.CmdInvite), c)
	}
	f.receive("GameLobbySetup", lobbySetup(2, 0))
	if f.count(follow.CmdInvite) != 1 {
		t.Fatal("invited Jaina twice")
	}
	// Thrall comes online in Warcraft III.
	f.receive("FriendsFriendUpdated", map[string]any{"data": map[string]any{"friend": closed("Thrall#1820", 3, map[string]any{"currentProgram": "W3"})}})
	if c, _ := f.lastCommand(follow.CmdInvite); f.count(follow.CmdInvite) != 2 || c.payload["target"] != "Thrall#1820" {
		t.Fatalf("Thrall not invited: %d", f.count(follow.CmdInvite))
	}
}

func TestNoInvitesWithoutTheSwitch(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214"}})
	f.friendsSnapshot(closed("Jaina#2214", 2, map[string]any{"currentProgram": "W3"}))
	s := hostSettings()
	s.DelaySeconds = 0
	f.mustHost(s)
	f.openOwnLobby()
	if f.count(follow.CmdInvite) != 0 {
		t.Fatal("invited without Auto-invite")
	}
}

func TestHostListsAreNeverNil(t *testing.T) {
	f := newFixture(t)
	h := f.state().Host
	if h.Missing == nil || h.OffTeam == nil {
		t.Fatalf("host = %+v", h)
	}
}

func TestAFullLobbyWaitingForTeamsNamesWhoIsOrAreOff(t *testing.T) {
	one := sameTeamHost(t, true)
	one.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0}, seat{name: "Stranger#5555", team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Computer (Normal)", team: 1, computer: true}))
	if n := notes(one, "won't start until Friend#1234 is on your team."); n != 1 {
		t.Fatalf("notes = %d, log = %+v", n, one.state().Log)
	}

	two := sameTeamHost(t, true)
	_ = two.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214"}})
	two.receive("GameLobbySetup", lobbyWith(seat{name: "Self#1111", team: 0}, seat{name: "Stranger#5555", team: 0},
		seat{name: "Friend#1234", team: 1}, seat{name: "Jaina#2214", team: 1}))
	if n := notes(two, "won't start until Friend#1234 and Jaina#2214 are on your team."); n != 1 {
		t.Fatalf("notes = %d, log = %+v", n, two.state().Log)
	}
}

func TestAFullLobbyWaitingForTheGroupSaysSoOnce(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214", "Thrall#1820"}})
	s := hostSettings()
	s.DelaySeconds, s.StartWhenFull = 0, true
	f.mustHost(s)
	f.openOwnLobby()
	full := lobbyWith(seat{name: "Self#1111"}, seat{name: "Friend#1234"}, seat{name: "Computer (Normal)"})
	f.receive("GameLobbySetup", full)
	f.receive("GameLobbySetup", full)
	if n := notes(f, "won't start until Jaina#2214 and Thrall#1820 joined"); n != 1 {
		t.Fatalf("notes = %d, log = %+v", n, f.state().Log)
	}
	if f.count(follow.CmdLobbyStart) != 0 {
		t.Fatal("started without the group")
	}
}
