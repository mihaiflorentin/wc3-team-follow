package companion_test

import (
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// hostedLobby is a GameLobbySetup of a lobby the player hosts, with the
// player in slot 0 and names in the next slots ("" is an open slot).
func hostedLobby(names ...string) dto.GameEvent {
	players := []any{map[string]any{"slot": 0, "slotStatus": 2, "slotType": 0, "name": "Self#1111", "team": 0, "isSelf": true}}
	for i, name := range names {
		status := 2
		if name == "" {
			status = 0
		}
		players = append(players, map[string]any{"slot": i + 1, "slotStatus": status, "slotType": 0, "name": name, "team": 0})
	}
	return dto.GameEvent{MessageType: "GameLobbySetup", Payload: map[string]any{"players": players, "isHost": true, "playerHost": "Self#1111"}}
}

func blacklistPrefs() *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, AutoBan: true, BlacklistManualBans: true,
		Blacklist: []dto.BlacklistEntry{{Tag: "Griefer#4242", Added: time.Unix(100, 0).UTC()}}})
}

func bans(h *harness) []any {
	var slots []any
	for _, c := range h.session.Sent() {
		if c.Message == follow.CmdBanPlayer {
			slots = append(slots, c.Payload["slot"])
		}
	}
	return slots
}

// waitForLobby waits until the companion has read the marker update, so
// every event pushed before it has been handled.
func waitForLobby(t *testing.T, h *harness, marker string) {
	t.Helper()
	waitFor(t, "the last lobby update", func() bool {
		for _, tag := range h.svc.State().LobbyTags {
			if tag == marker {
				return true
			}
		}
		return false
	})
}

func TestBansTheBlacklistedPlayerWhoJoinsMyLobby(t *testing.T) {
	h := newHarness(t, blacklistPrefs())
	h.session.Push(hostedLobby("Player#1234", "Griefer#4242"))
	h.run(t)
	waitFor(t, "the ban", func() bool { return len(bans(h)) == 1 })
	if got := bans(h); got[0] != 2 {
		t.Fatalf("bans = %v", got)
	}
}

func TestAPlayerBannedByHandIsSavedToTheBlacklist(t *testing.T) {
	prefs := blacklistPrefs()
	h := newHarness(t, prefs)
	h.session.Push(hostedLobby("Player#1234"))
	h.session.Push(dto.GameEvent{MessageType: "ChatMessage", Payload: map[string]any{"message": map[string]any{
		"source": "gameChat", "content": "Host banned player [Player#1234]"}}})
	h.run(t)
	waitFor(t, "the saved blacklist", func() bool {
		got, _ := prefs.Load()
		return len(got.Blacklist) == 2 && got.Blacklist[0].Tag == "Player#1234"
	})
	if bans(h) != nil {
		t.Fatalf("Team Follow banned someone: %v", bans(h))
	}
}

func TestAQueuedUpdateThatReplacesThePlayerPreventsTheBan(t *testing.T) {
	h := newHarness(t, blacklistPrefs())
	h.session.Push(hostedLobby("Griefer#4242"))
	h.session.Push(hostedLobby("Player#5678"))
	h.session.Push(hostedLobby("Player#5678", "Marker#9999"))
	h.run(t)
	waitForLobby(t, h, "Marker#9999")
	if got := bans(h); len(got) != 0 {
		t.Fatalf("banned from a stale update: %v", got)
	}
}

func TestAQueuedUpdateThatMovesThePlayerBansTheNewSlot(t *testing.T) {
	h := newHarness(t, blacklistPrefs())
	h.session.Push(hostedLobby("Griefer#4242", ""))
	h.session.Push(hostedLobby("Player#5678", "Griefer#4242"))
	h.run(t)
	waitFor(t, "the ban", func() bool { return len(bans(h)) == 1 })
	if got := bans(h); got[0] != 2 {
		t.Fatalf("bans = %v", got)
	}
}
