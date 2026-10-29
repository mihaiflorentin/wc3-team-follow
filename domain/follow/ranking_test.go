package follow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// rseat is one lobby slot for the ranking tests.
type rseat struct {
	name     string
	team     int
	self     bool
	observer bool
	computer bool
}

func rlobby(seats ...rseat) dto.GameEvent {
	players := make([]any, len(seats))
	for i, s := range seats {
		kind := 0
		if s.computer {
			kind = 1
		}
		players[i] = map[string]any{"name": s.name, "team": float64(s.team), "isSelf": s.self, "isObserver": s.observer,
			"slotType": float64(kind), "slotStatus": float64(2), "slot": float64(i)}
	}
	return dto.GameEvent{MessageType: "GameLobbySetup", Payload: map[string]any{"players": players}}
}

func score(won bool, teams ...map[string]any) dto.GameEvent {
	players := make([]any, len(teams))
	for i, t := range teams {
		players[i] = t
	}
	return dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: map[string]any{
		"scoreInfo": map[string]any{"localPlayerWon": won, "players": players}}}
}

func glue(screen string) dto.GameEvent {
	return dto.GameEvent{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": screen}}
}

// record feeds events, arming the match after each lobby the way the game's
// loading screen does.
func record(r *follow.Ranking, group []string, events ...dto.GameEvent) {
	for _, ev := range events {
		r.Receive(ev, group)
		if ev.MessageType == "GameLobbySetup" {
			r.Receive(glue("LOADING_SCREEN"), group)
		}
	}
}

// feed feeds events exactly as given.
func feed(r *follow.Ranking, group []string, events ...dto.GameEvent) {
	for _, ev := range events {
		r.Receive(ev, group)
	}
}

func expectRanking(t *testing.T, r *follow.Ranking, want map[string][2]int) {
	t.Helper()
	got := map[string][2]int{}
	for _, e := range r.Entries() {
		got[e.Tag] = [2]int{e.Wins, e.Losses}
	}
	if len(got) != len(want) {
		t.Fatalf("ranking = %v, want %v", got, want)
	}
	for tag, v := range want {
		if got[tag] != v {
			t.Fatalf("ranking = %v, want %v", got, want)
		}
	}
}

var rankClock = time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)

func newRanking() *follow.Ranking {
	return follow.NewRanking(func() time.Time { return rankClock }, nil)
}

var twoTeams = []rseat{
	{name: "Me#1111", team: 0, self: true}, {name: "Arthas#1123", team: 0}, {name: "Jaina#2214", team: 1}, {name: "Stranger#9999", team: 1},
}

func TestRankingWinCountsMyTeamAndLossTheOther(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}, "Jaina#2214": {0, 1}})
	if e := r.Entries()[0]; !e.LastPlayed.Equal(rankClock) {
		t.Fatalf("lastPlayed = %v", e.LastPlayed)
	}
	if !r.Unsaved() {
		t.Fatal("a recorded match is unsaved")
	}
	r.MarkSaved()
	if r.Unsaved() {
		t.Fatal("MarkSaved")
	}
}

func TestRankingLossInTwoTeamsGivesTheOtherTeamTheWin(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(false))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}, "Arthas#1123": {0, 1}, "Jaina#2214": {1, 0}})
}

func TestRankingComputersCountAsTeams(t *testing.T) {
	r := newRanking()
	// Three teams (a computer is the third): losing says nothing about Jaina.
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(
		rseat{name: "Me#1111", team: 0, self: true}, rseat{name: "Arthas#1123", team: 0},
		rseat{name: "Jaina#2214", team: 1}, rseat{name: "Computer (Normal)", team: 2, computer: true}), score(false))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}, "Arthas#1123": {0, 1}})
}

func TestRankingFreeForAllLossRecordsOnlyMe(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(
		rseat{name: "Me#1111", team: 0, self: true}, rseat{name: "Arthas#1123", team: 1}, rseat{name: "Jaina#2214", team: 2}), score(false))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}})
}

func TestRankingFreeForAllWinGivesEveryoneInTheGroupALoss(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(
		rseat{name: "Me#1111", team: 0, self: true}, rseat{name: "Arthas#1123", team: 1}, rseat{name: "Jaina#2214", team: 2}), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {0, 1}})
}

func TestRankingIgnoresStrangersObserversAndComputers(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(
		rseat{name: "Me#1111", team: 0, self: true}, rseat{name: "Arthas#1123", team: 1, observer: true},
		rseat{name: "Computer (Easy)", team: 1, computer: true}, rseat{name: "Stranger#9999", team: 1}), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}})
}

func TestRankingGroupMemberNotInTheLobbyGetsNothing(t *testing.T) {
	r := newRanking()
	record(r, []string{"Absent#4444"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}})
}

func TestRankingObserverSelfRecordsNothing(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(
		rseat{name: "Me#1111", team: 0, self: true, observer: true}, rseat{name: "Arthas#1123", team: 1}), score(true))
	expectRanking(t, r, map[string][2]int{})
	if r.Unsaved() {
		t.Fatal("nothing changed")
	}
}

func TestRankingDisarmsWithoutAResult(t *testing.T) {
	disarming := []dto.GameEvent{
		{MessageType: "LoggedOut"}, glue("MAIN_MENU"), glue("CUSTOM_LOBBIES"),
		rlobby(rseat{name: "Me#1111", team: 0, self: true}), // the next lobby
	}
	for _, end := range disarming {
		r := newRanking()
		feed(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), end, score(true))
		expectRanking(t, r, map[string][2]int{})
	}
	r := newRanking()
	feed(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"))
	r.End()
	feed(r, nil, score(true))
	expectRanking(t, r, map[string][2]int{})
	feed(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"))
	r.Clear()
	feed(r, nil, score(true))
	expectRanking(t, r, map[string][2]int{})
}

func TestRankingNeedsTheLoadingScreen(t *testing.T) {
	r := newRanking()
	feed(r, []string{"Arthas#1123"}, rlobby(twoTeams...), score(true))
	expectRanking(t, r, map[string][2]int{})
}

func TestRankingLeavingTheGameAndTheScoreScreenDontDisarm(t *testing.T) {
	r := newRanking()
	feed(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"),
		dto.GameEvent{MessageType: "ExitedGame"}, glue("SCORE_SCREEN"), dto.GameEvent{MessageType: "MultiplayerGameLeave"}, score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingScoreScreenMayComeBeforeTheScoreEvent(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), glue("SCORE_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingCountsAMatchOnce(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true), score(true), glue("SCORE_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingIgnoresCampaignAndMalformedScores(t *testing.T) {
	campaign := dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: map[string]any{"data": map[string]any{"localPlayerWon": true}}}
	noFlag := dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: map[string]any{"scoreInfo": map[string]any{"localPlayerWon": "yes"}}}
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), campaign, noFlag)
	expectRanking(t, r, map[string][2]int{})
	// The lobby is still there for the real score.
	record(r, []string{"Arthas#1123"}, score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingNeedsALobbyFirst(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, score(true))
	expectRanking(t, r, map[string][2]int{})
}

func TestRankingTheNewestLobbyIsTheMatch(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...),
		rlobby(rseat{name: "Me#1111", team: 0, self: true}, rseat{name: "Jaina#2214", team: 1}), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Jaina#2214": {0, 1}})
}

func TestRankingMatchesTagsWithoutRegardToCase(t *testing.T) {
	r := newRanking()
	record(r, []string{"arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true))
	record(r, []string{"ARTHAS#1123"}, rlobby(rseat{name: "me#1111", team: 0, self: true}, rseat{name: "ARTHAS#1123", team: 0}), score(false))
	got := r.Entries()
	if len(got) != 2 {
		t.Fatalf("entries = %+v", got)
	}
	for _, e := range got {
		if e.Wins != 1 || e.Losses != 1 {
			t.Fatalf("entry = %+v, want 1 win and 1 loss", e)
		}
	}
}

func TestRankingSavedEntriesCarryOn(t *testing.T) {
	saved := []dto.RankingEntry{{Tag: "Arthas#1123", Wins: 4, Losses: 2, LastPlayed: rankClock.Add(-time.Hour)}}
	r := follow.NewRanking(func() time.Time { return rankClock }, saved)
	record(r, []string{"arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Arthas#1123": {5, 2}, "Me#1111": {1, 0}})
	if r.Entries()[0].Tag == "" || len(r.Entries()) != 2 {
		t.Fatal("entries")
	}
}

func TestRankingEntriesAreNeverNilAndClearEmptiesThem(t *testing.T) {
	r := newRanking()
	if r.Entries() == nil {
		t.Fatal("entries are nil")
	}
	record(r, nil, rlobby(twoTeams...), glue("LOADING_SCREEN"), score(true))
	r.Clear()
	if got := r.Entries(); got == nil || len(got) != 0 || r.Unsaved() {
		t.Fatalf("after Clear = %v, unsaved %v", got, r.Unsaved())
	}
}

// pl is one entry of scoreInfo.players.
func pl(tag string, team int, won bool) map[string]any {
	return map[string]any{"battleTag": tag, "toonName": tag, "teamIndex": float64(team), "won": won, "isAi": false}
}

func cpu(team int, won bool) map[string]any {
	return map[string]any{"battleTag": "", "toonName": "Player 5", "teamIndex": float64(team), "won": won, "isAi": true}
}

func scoreWith(localWon bool, info map[string]any, players ...map[string]any) dto.GameEvent {
	list := make([]any, len(players))
	for i, p := range players {
		list[i] = p
	}
	si := map[string]any{"localPlayerWon": localWon, "players": list}
	for k, v := range info {
		si[k] = v
	}
	return dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: map[string]any{"scoreInfo": si}}
}

func loadFixture(t *testing.T, name string) dto.GameEvent {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: payload}
}

func TestRankingLiveCaptureHostLeftEarlyRecordsOneLoss(t *testing.T) {
	// Event order captured from game 3.0.1.24371 (Legion TD, host left early).
	lobby := rlobby(rseat{name: "Mihai#22569", team: 0, self: true}, rseat{name: "Player 5", team: 1, computer: true}, rseat{name: "Mihai", team: 0, computer: true})
	fixture := loadFixture(t, "score-left-early.json")
	r := newRanking()
	feed(r, []string{"Friend#1234"}, lobby,
		dto.GameEvent{MessageType: "GameLobbyGracefulExit"}, glue("LOADING_SCREEN"),
		dto.GameEvent{MessageType: "ExitedGame"}, fixture, glue("SCORE_SCREEN"),
		dto.GameEvent{MessageType: "MultiplayerGameLeave"}, fixture)
	expectRanking(t, r, map[string][2]int{"Mihai#22569": {0, 1}})
}

func TestRankingFinishedMatchUsesEachPlayersWonFlag(t *testing.T) {
	seats := []rseat{{name: "Me#1111", team: 0, self: true}, {name: "Arthas#1123", team: 0}, {name: "Jaina#2214", team: 1}, {name: "Stranger#9999", team: 1}, {name: "CPU", team: 1, computer: true}}
	r := newRanking()
	record(r, []string{"arthas#1123", "Jaina#2214"}, rlobby(seats...), scoreWith(true, nil,
		pl("Me#1111", 0, true), pl("Arthas#1123", 0, true), pl("Jaina#2214", 1, false), pl("Stranger#9999", 1, false), cpu(1, false)))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}, "Jaina#2214": {0, 1}})
}

func TestRankingFinishedMatchTheLocalPlayerLost(t *testing.T) {
	r := newRanking()
	record(r, []string{"Jaina#2214"}, rlobby(twoTeams...), scoreWith(false, nil,
		pl("Me#1111", 0, false), pl("Arthas#1123", 0, false), pl("Jaina#2214", 1, true), pl("Stranger#9999", 1, true)))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}, "Jaina#2214": {1, 0}})
}

func TestRankingFinishedMatchIgnoresComputersEvenWithATag(t *testing.T) {
	r := newRanking()
	ai := pl("Bot#0001", 1, true)
	ai["isAi"] = true
	record(r, []string{"Bot#0001"}, rlobby(twoTeams...), scoreWith(false, nil, pl("Me#1111", 0, false), ai))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}})
}

func TestRankingUnfinishedMatchRecordsOnlyTheLocalLoss(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), scoreWith(false, nil,
		pl("Me#1111", 0, false), pl("Arthas#1123", 0, false), pl("Jaina#2214", 1, false)))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}})
}

func TestRankingUnfinishedWinGivesTeammatesTheWin(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), scoreWith(true, nil,
		pl("Me#1111", 0, false), pl("Arthas#1123", 0, false), pl("Jaina#2214", 1, false)))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingIgnoresReplaysAndOtherGameTypes(t *testing.T) {
	for _, info := range []map[string]any{
		{"mapInfo": map[string]any{"isReplay": true, "gameType": "custom"}},
		{"mapInfo": map[string]any{"isReplay": false, "gameType": "ladder"}},
	} {
		r := newRanking()
		record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), scoreWith(true, info, pl("Me#1111", 0, true), pl("Arthas#1123", 0, true)))
		expectRanking(t, r, map[string][2]int{})
		// The match is over; a following real score has nothing to count.
		record(r, []string{"Arthas#1123"}, score(true))
		expectRanking(t, r, map[string][2]int{})
	}
}

func TestRankingScoreWithoutWonFlagsFallsBackToTheLobby(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), score(false,
		map[string]any{"teamIndex": float64(0)}, map[string]any{"teamIndex": float64(1)}))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}, "Arthas#1123": {0, 1}, "Jaina#2214": {1, 0}})
}

func TestRankingNeedsAKnownLocalPlayerToRecordTheirResult(t *testing.T) {
	r := newRanking()
	feed(r, []string{"Arthas#1123"}, glue("LOADING_SCREEN"), scoreWith(false, nil, pl("Me#1111", 0, false), pl("Arthas#1123", 0, false)))
	expectRanking(t, r, map[string][2]int{})
}
