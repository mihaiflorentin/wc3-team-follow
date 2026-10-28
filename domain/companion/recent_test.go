package companion_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Slot kinds as GameLobbySetup sends them.
const (
	human    = 0
	computer = 1
	open     = 0
	occupied = 2
)

func slot(name string, slotType, status int, self, observer bool) map[string]any {
	return map[string]any{"name": name, "slotType": slotType, "slotStatus": status, "isSelf": self, "isObserver": observer}
}

func lobby(mapData map[string]any, lobbyName string, slots ...map[string]any) dto.GameEvent {
	players := make([]any, len(slots))
	for i, s := range slots {
		players[i] = s
	}
	p := map[string]any{"players": players, "lobbyName": lobbyName}
	if mapData != nil {
		p["mapData"] = mapData
	}
	return dto.GameEvent{MessageType: "GameLobbySetup", Payload: p}
}

func screen(name string) dto.GameEvent {
	return dto.GameEvent{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": name}}
}

func recentTags(s dto.FollowState) []string {
	var out []string
	for _, p := range s.Recent {
		out = append(out, p.Tag)
	}
	return out
}

func (h *harness) connected(t *testing.T) {
	t.Helper()
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
}

func (h *harness) recentFor(t *testing.T, tag string) dto.RecentPlayer {
	t.Helper()
	for _, p := range h.svc.State().Recent {
		if p.Tag == tag {
			return p
		}
	}
	t.Fatalf("%s isn't in recent players: %v", tag, recentTags(h.svc.State()))
	return dto.RecentPlayer{}
}

var castleFight = map[string]any{"mapName": "Castle Fight", "mapPath": "Maps/Download/CastleFight 2.0.w3x"}

func TestRecentPlayersRecordsTheOtherHumansInTheLobby(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	if s := h.svc.State(); s.Recent == nil || s.LobbyTags == nil {
		t.Fatalf("recent = %v, lobbyTags = %v; want empty lists", s.Recent, s.LobbyTags)
	}
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF with friends",
		slot("Player#1234", human, occupied, true, false),
		slot("Arthas#1123", human, occupied, false, false),
		slot("Computer (Normal)", computer, occupied, false, false),
		slot("", human, open, false, false),
		slot("Jaina#2214", human, occupied, false, true),
	))
	waitFor(t, "recent players", func() bool { return len(h.svc.State().Recent) == 2 })
	s := h.svc.State()
	if got := recentTags(s); !slices.Contains(got, "Arthas#1123") || !slices.Contains(got, "Jaina#2214") {
		t.Fatalf("recent = %v", got)
	}
	if got := s.LobbyTags; len(got) != 2 || !slices.Contains(got, "Arthas#1123") || !slices.Contains(got, "Jaina#2214") {
		t.Fatalf("lobbyTags = %v", got)
	}
	if p := h.recentFor(t, "Arthas#1123"); p.Map != "Castle Fight" || time.Since(p.LastSeen) > 5*time.Second {
		t.Fatalf("Arthas = %+v", p)
	}
}

func TestRecentPlayersLastSeenFollowsTheLobbyAndMatchUntilTheScoreScreen(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF",
		slot("Player#1234", human, occupied, true, false),
		slot("Arthas#1123", human, occupied, false, false),
		slot("Thrall#1820", human, occupied, false, false),
	))
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 2 })
	joined := h.recentFor(t, "Thrall#1820").LastSeen

	// Thrall leaves the lobby; Arthas stays into the match.
	h.session.Push(lobby(castleFight, "CF",
		slot("Player#1234", human, occupied, true, false),
		slot("Arthas#1123", human, occupied, false, false),
	))
	h.session.Push(screen("LOADING_SCREEN"))
	h.session.Push(screen("DISABLED_SCREEN"))
	waitFor(t, "Thrall gone from the lobby", func() bool { return len(h.svc.State().LobbyTags) == 1 })
	h.skew.Add(int64(10 * time.Minute))
	waitFor(t, "refreshed in the match", func() bool {
		return h.recentFor(t, "Arthas#1123").LastSeen.Sub(joined) >= 10*time.Minute
	})
	if got := h.recentFor(t, "Thrall#1820").LastSeen; got.Sub(joined) > 5*time.Second {
		t.Fatalf("Thrall was refreshed after leaving the lobby: %v", got.Sub(joined))
	}
	if got := recentTags(h.svc.State()); got[0] != "Arthas#1123" {
		t.Fatalf("order = %v; the most recently seen comes first", got)
	}
	if s := h.svc.State(); !slices.Equal(s.LobbyTags, []string{"Arthas#1123"}) {
		t.Fatalf("lobbyTags in the match = %v", s.LobbyTags)
	}

	h.session.Push(screen("SCORE_SCREEN"))
	waitFor(t, "left", func() bool { return len(h.svc.State().LobbyTags) == 0 })
	ended := h.recentFor(t, "Arthas#1123").LastSeen
	h.skew.Add(int64(10 * time.Minute))
	time.Sleep(1200 * time.Millisecond)
	if got := h.recentFor(t, "Arthas#1123").LastSeen; !got.Equal(ended) {
		t.Fatalf("last seen moved after the match ended: %v", got.Sub(ended))
	}
}

func TestRecentPlayersStopRefreshingAfterLeaving(t *testing.T) {
	for _, leave := range []dto.GameEvent{{MessageType: "MultiplayerGameLeave"}, {MessageType: "ExitedGame"}, screen("MAIN_MENU")} {
		t.Run(leave.MessageType, func(t *testing.T) {
			h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
			h.connected(t)
			h.session.Push(lobby(castleFight, "CF", slot("Arthas#1123", human, occupied, false, false)))
			waitFor(t, "recorded", func() bool { return len(h.svc.State().LobbyTags) == 1 })
			h.session.Push(leave)
			waitFor(t, "left", func() bool { return len(h.svc.State().LobbyTags) == 0 })
			if len(h.svc.State().Recent) != 1 {
				t.Fatal("leaving forgot the player")
			}
		})
	}
}

func TestRecentPlayersKeepTheNewest200(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.connected(t)
	for i := range 205 {
		tag := fmt.Sprintf("Player%d#1%03d", i, i)
		h.skew.Add(int64(time.Second))
		h.session.Push(lobby(castleFight, "CF", slot(tag, human, occupied, false, false)))
		waitFor(t, tag, func() bool {
			r := h.svc.State().Recent
			return len(r) > 0 && r[0].Tag == tag
		})
	}
	r := h.svc.State().Recent
	if len(r) != dto.MaxRecentPlayers || r[len(r)-1].Tag != "Player5#1005" {
		t.Fatalf("kept %d, oldest %s", len(r), r[len(r)-1].Tag)
	}
	for i := 1; i < len(r); i++ {
		if r[i].LastSeen.After(r[i-1].LastSeen) {
			t.Fatalf("not newest first at %d: %v after %v", i, r[i].LastSeen, r[i-1].LastSeen)
		}
	}
}

func TestRecentPlayersSurviveARestart(t *testing.T) {
	store := mockrecent.New()
	h := newHarnessWith(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}), store)
	stop := h.start()
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.session.Push(lobby(castleFight, "CF", slot("Arthas#1123", human, occupied, false, false), slot("Jaina#2214", human, occupied, false, false)))
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 2 })
	<-stop()
	if got := len(store.Saved()); got != 2 {
		t.Fatalf("saved %d players on shutdown", got)
	}

	again := newHarnessWith(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}), store)
	if got := recentTags(again.svc.State()); len(got) != 2 || again.recentFor(t, "Jaina#2214").Map != "Castle Fight" {
		t.Fatalf("after a restart recent = %v", got)
	}
	if len(again.svc.State().LobbyTags) != 0 {
		t.Fatal("a restart starts outside any lobby")
	}
}

func TestRecentPlayersAreSavedOnChangeButNotOnEveryTick(t *testing.T) {
	store := mockrecent.New()
	h := newHarnessWith(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}), store)
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF", slot("Arthas#1123", human, occupied, false, false)))
	waitFor(t, "first save", func() bool { return len(store.Saved()) == 1 })

	// A second player within a few seconds waits for the throttle.
	h.session.Push(lobby(castleFight, "CF", slot("Arthas#1123", human, occupied, false, false), slot("Thrall#1820", human, occupied, false, false)))
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 2 })
	time.Sleep(1200 * time.Millisecond)
	if got := len(store.Saved()); got != 1 {
		t.Fatalf("saved %d players before the throttle allowed it", got)
	}
	h.skew.Add(int64(6 * time.Second))
	waitFor(t, "second save", func() bool { return len(store.Saved()) == 2 })

	// Only last-seen times move each second in a lobby: no save until a
	// minute has passed.
	saves := store.Saves()
	h.skew.Add(int64(10 * time.Second))
	time.Sleep(2200 * time.Millisecond)
	if store.Saves() != saves {
		t.Fatalf("saved %d times for last-seen refreshes alone", store.Saves()-saves)
	}
	h.skew.Add(int64(time.Minute))
	waitFor(t, "refresh saved", func() bool { return store.Saves() > saves })
}

func TestClearRecentPlayersLeavesTheFollowGroup(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{"Arthas#1123"}, IncludeTeam: true})
	store := mockrecent.New(dto.RecentPlayer{Tag: "Arthas#1123", Map: "Castle Fight", LastSeen: time.Now()},
		dto.RecentPlayer{Tag: "Jaina#2214", Map: "Legion TD", LastSeen: time.Now().Add(-time.Hour)})
	h := newHarnessWith(t, prefs, store)
	if len(h.svc.State().Recent) != 2 {
		t.Fatal("the saved list wasn't loaded")
	}
	s, err := h.svc.ClearRecentPlayers()
	if err != nil || len(s.Recent) != 0 || s.Recent == nil {
		t.Fatalf("ClearRecentPlayers = %v, %v", s.Recent, err)
	}
	if store.Clears() != 1 || len(store.Saved()) != 0 {
		t.Fatalf("store cleared %d times, holds %v", store.Clears(), store.Saved())
	}
	if !slices.Equal(s.ManualTags, []string{"Arthas#1123"}) || prefs.Saves != 0 {
		t.Fatalf("follow group = %v, %d preference saves", s.ManualTags, prefs.Saves)
	}
}

func TestClearRecentPlayersFailureIsASentence(t *testing.T) {
	store := mockrecent.New(dto.RecentPlayer{Tag: "Arthas#1123", Map: "Castle Fight", LastSeen: time.Now()})
	store.ClearErr = errors.New("remove recent-players.json: permission denied")
	h := newHarnessWith(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}), store)
	s, err := h.svc.ClearRecentPlayers()
	if err == nil || err.Error() != "Recent players couldn't be cleared on this computer. Try again." || len(s.Recent) != 1 {
		t.Fatalf("ClearRecentPlayers = %v, %v", s.Recent, err)
	}
}

func TestRecentPlayersNameTheMap(t *testing.T) {
	cases := []struct {
		name      string
		mapData   map[string]any
		lobbyName string
		want      string
	}{
		{"map name", map[string]any{"mapName": "|cffffcc00Legion TD|r", "mapPath": "Maps/ltd.w3x"}, "LTD", "Legion TD"},
		{"file name", map[string]any{"mapName": "", "mapPath": `Maps\Download\Castle Fight v2.1.w3x`}, "CF", "Castle Fight v2.1"},
		{"lobby name", map[string]any{"mapName": " ", "mapPath": ""}, "Footmen fun", "Footmen fun"},
		{"no map data", nil, "Footmen fun", "Footmen fun"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
			h.connected(t)
			h.session.Push(lobby(c.mapData, c.lobbyName, slot("Arthas#1123", human, occupied, false, false)))
			waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 1 })
			if got := h.recentFor(t, "Arthas#1123").Map; got != c.want {
				t.Fatalf("map = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRecentPlayersFirstSeenStaysAndLobbiesCountOncePerLobby(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.connected(t)
	cf := lobby(castleFight, "CF", slot("Player#1234", human, occupied, true, false), slot("Arthas#1123", human, occupied, false, false))
	h.session.Push(cf)
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 1 })
	first := h.recentFor(t, "Arthas#1123")
	if first.Lobbies != 1 || first.FirstSeen.IsZero() || !first.FirstSeen.Equal(first.LastSeen) {
		t.Fatalf("after the first lobby update Arthas = %+v; want 1 lobby, first seen = last seen", first)
	}

	// Many updates of the same lobby, and time passing, count no new lobby.
	for range 5 {
		h.skew.Add(int64(time.Minute))
		h.session.Push(cf)
	}
	waitFor(t, "refreshed", func() bool {
		return h.recentFor(t, "Arthas#1123").LastSeen.Sub(first.LastSeen) >= 5*time.Minute
	})
	if p := h.recentFor(t, "Arthas#1123"); p.Lobbies != 1 || !p.FirstSeen.Equal(first.FirstSeen) {
		t.Fatalf("after updates of the same lobby Arthas = %+v; want 1 lobby, first seen kept", p)
	}

	// The lobby ends and a new one starts with him in it.
	h.session.Push(dto.GameEvent{MessageType: "MultiplayerGameLeave"})
	waitFor(t, "left", func() bool { return len(h.svc.State().LobbyTags) == 0 })
	h.skew.Add(int64(time.Hour))
	h.session.Push(lobby(castleFight, "CF again", slot("Player#1234", human, occupied, true, false), slot("Arthas#1123", human, occupied, false, false)))
	waitFor(t, "second lobby", func() bool { return h.recentFor(t, "Arthas#1123").Lobbies == 2 })
	if p := h.recentFor(t, "Arthas#1123"); !p.FirstSeen.Equal(first.FirstSeen) || p.LastSeen.Sub(first.LastSeen) < time.Hour {
		t.Fatalf("in the second lobby Arthas = %+v", p)
	}
}

func TestRecentPlayersLobbiesCountAJoinOnceAndADifferentLobby(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.connected(t)
	me := slot("Player#1234", human, occupied, true, false)
	arthas := slot("Arthas#1123", human, occupied, false, false)
	jaina := slot("Jaina#2214", human, occupied, false, false)
	h.session.Push(lobby(castleFight, "CF", me, arthas))
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Recent) == 1 })

	// Jaina joins, leaves and joins the same lobby again: one lobby.
	h.session.Push(lobby(castleFight, "CF", me, arthas, jaina))
	h.session.Push(lobby(castleFight, "CF", me, arthas))
	h.session.Push(lobby(castleFight, "CF", me, arthas, jaina))
	waitFor(t, "Jaina back", func() bool { return len(h.svc.State().LobbyTags) == 2 })
	if p := h.recentFor(t, "Jaina#2214"); p.Lobbies != 1 {
		t.Fatalf("Jaina = %+v; rejoining the same lobby counts once", p)
	}

	// A different lobby without leaving first counts again.
	h.session.Push(lobby(castleFight, "Legion TD night", me, arthas))
	waitFor(t, "new lobby", func() bool { return h.recentFor(t, "Arthas#1123").Lobbies == 2 })
	if p := h.recentFor(t, "Jaina#2214"); p.Lobbies != 1 {
		t.Fatalf("Jaina = %+v; she isn't in the new lobby", p)
	}
}

func TestRecentPlayersSavedBeforeFirstSeenLoadWithOneLobby(t *testing.T) {
	seen := time.Now().Add(-48 * time.Hour).Round(0)
	store := mockrecent.New(dto.RecentPlayer{Tag: "Arthas#1123", Map: "Castle Fight", LastSeen: seen})
	h := newHarnessWith(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}), store)
	if p := h.recentFor(t, "Arthas#1123"); p.Lobbies != 1 || !p.FirstSeen.Equal(seen) || !p.LastSeen.Equal(seen) {
		t.Fatalf("old entry loaded as %+v; want first seen = last seen and 1 lobby", p)
	}
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF", slot("Arthas#1123", human, occupied, false, false)))
	waitFor(t, "second lobby", func() bool { return h.recentFor(t, "Arthas#1123").Lobbies == 2 })
	if p := h.recentFor(t, "Arthas#1123"); !p.FirstSeen.Equal(seen) {
		t.Fatalf("first seen moved to %v", p.FirstSeen)
	}
}
