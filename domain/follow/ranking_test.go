package follow_test

import (
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

func record(r *follow.Ranking, group []string, events ...dto.GameEvent) {
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
	record(r, []string{"Arthas#1123", "Jaina#2214"}, rlobby(twoTeams...), score(false))
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
	record(r, []string{"Absent#4444"}, rlobby(twoTeams...), score(true))
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

func TestRankingNeedsAScoreEvent(t *testing.T) {
	endings := []dto.GameEvent{
		{MessageType: "MultiplayerGameLeave"}, {MessageType: "ExitedGame"}, {MessageType: "LoggedOut"},
		glue("MAIN_MENU"), glue("CUSTOM_LOBBIES"),
	}
	for _, end := range endings {
		r := newRanking()
		record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), end, score(true))
		expectRanking(t, r, map[string][2]int{})
	}
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("LOADING_SCREEN"), glue("SCORE_SCREEN"))
	expectRanking(t, r, map[string][2]int{})
	r.End()
	record(r, nil, score(true))
	expectRanking(t, r, map[string][2]int{})
}

func TestRankingScoreScreenMayComeBeforeTheScoreEvent(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), glue("SCORE_SCREEN"), score(true))
	expectRanking(t, r, map[string][2]int{"Me#1111": {1, 0}, "Arthas#1123": {1, 0}})
}

func TestRankingCountsAMatchOnce(t *testing.T) {
	r := newRanking()
	record(r, []string{"Arthas#1123"}, rlobby(twoTeams...), score(true), score(true), glue("SCORE_SCREEN"), score(true))
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
	record(r, []string{"arthas#1123"}, rlobby(twoTeams...), score(true))
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
	record(r, []string{"arthas#1123"}, rlobby(twoTeams...), score(true))
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
	record(r, nil, rlobby(twoTeams...), score(true))
	r.Clear()
	if got := r.Entries(); got == nil || len(got) != 0 || r.Unsaved() {
		t.Fatalf("after Clear = %v, unsaved %v", got, r.Unsaved())
	}
}

func TestRankingPerTeamResultTakesPrecedence(t *testing.T) {
	// Three teams, so the inference alone gives Jaina nothing; the score
	// says team 1 won although the local team lost.
	seats := []rseat{
		{name: "Me#1111", team: 0, self: true}, {name: "Jaina#2214", team: 1}, {name: "Arthas#1123", team: 2},
	}
	r := newRanking()
	record(r, []string{"Jaina#2214", "Arthas#1123"}, rlobby(seats...), score(false,
		map[string]any{"teamIndex": float64(0), "won": false},
		map[string]any{"teamIndex": float64(1), "won": true},
		map[string]any{"teamIndex": float64(2), "won": false}))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}, "Jaina#2214": {1, 0}, "Arthas#1123": {0, 1}})
}

func TestRankingPerTeamResultIsIgnoredWhenItContradictsTheLocalResult(t *testing.T) {
	seats := []rseat{{name: "Me#1111", team: 0, self: true}, {name: "Jaina#2214", team: 1}, {name: "Arthas#1123", team: 2}}
	r := newRanking()
	// Team indexes that don't line up with the lobby's teams.
	record(r, []string{"Jaina#2214", "Arthas#1123"}, rlobby(seats...), score(false,
		map[string]any{"teamIndex": float64(0), "won": true},
		map[string]any{"teamIndex": float64(1), "won": false}))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}})
}

func TestRankingPerTeamResultWithMixedPlayersOfATeamIsIgnored(t *testing.T) {
	seats := []rseat{{name: "Me#1111", team: 0, self: true}, {name: "Jaina#2214", team: 1}, {name: "Arthas#1123", team: 2}}
	r := newRanking()
	record(r, []string{"Jaina#2214", "Arthas#1123"}, rlobby(seats...), score(false,
		map[string]any{"teamIndex": float64(0), "won": false},
		map[string]any{"teamIndex": float64(1), "won": true},
		map[string]any{"teamIndex": float64(1), "won": false}))
	expectRanking(t, r, map[string][2]int{"Me#1111": {0, 1}})
}
