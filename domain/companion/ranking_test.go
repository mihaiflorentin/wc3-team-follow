package companion_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"

	mocknotes "github.com/mihaiflorentin/wc3-team-follow/mock/notes"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockranking "github.com/mihaiflorentin/wc3-team-follow/mock/ranking"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// teamSlot is a human slot with a team.
func teamSlot(name string, team int, self bool) map[string]any {
	s := slot(name, human, occupied, self, false)
	s["team"] = float64(team)
	return s
}

func scoreInfo(won bool) dto.GameEvent {
	return dto.GameEvent{MessageType: "UpdateScoreInfo", Payload: map[string]any{
		"scoreInfo": map[string]any{"localPlayerWon": won, "localPlayerRace": "human", "players": []any{}}}}
}

func wins(s dto.FollowState, tag string) (w, l int) {
	for _, e := range s.Ranking {
		if e.Tag == tag {
			return e.Wins, e.Losses
		}
	}
	return -1, -1
}

func rankingHarness(t *testing.T, store *mockranking.Store) *harness {
	t.Helper()
	prefs := mockprefs.New(dto.Preferences{Tags: []string{"Arthas#1123"}, IncludeTeam: false})
	return newHarnessFull(t, prefs, mockrecent.New(), mocknotes.New(nil), store, func(o *companion.Options) { o.RecordRanking = true })
}

func TestRankingIsNotRecordedByDefault(t *testing.T) {
	store := mockranking.New()
	prefs := mockprefs.New(dto.Preferences{Tags: []string{"Arthas#1123"}})
	h := newHarnessFull(t, prefs, mockrecent.New(), mocknotes.New(nil), store)
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF", teamSlot("Me#1111", 0, true), teamSlot("Arthas#1123", 1, false)))
	h.session.Push(screen("LOADING_SCREEN"))
	h.session.Push(scoreInfo(true))
	h.session.Push(screen("MAIN_MENU"))
	waitFor(t, "menu", func() bool { return h.svc.State().Connected })
	time.Sleep(200 * time.Millisecond)
	if n := len(h.svc.State().Ranking); n != 0 || store.Saves() != 0 {
		t.Fatalf("recorded %d players in %d saves; recording is off", n, store.Saves())
	}
}

func TestRankingRecordsAMatchSavesAndPublishesIt(t *testing.T) {
	store := mockranking.New()
	h := rankingHarness(t, store)
	if s := h.svc.State(); s.Ranking == nil || len(s.Ranking) != 0 {
		t.Fatalf("ranking = %v; want an empty list, not nil", s.Ranking)
	}
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF",
		teamSlot("Me#1111", 0, true), teamSlot("Arthas#1123", 0, false), teamSlot("Jaina#2214", 1, false)))
	h.session.Push(screen("LOADING_SCREEN"))
	h.session.Push(screen("SCORE_SCREEN"))
	h.session.Push(scoreInfo(true))
	waitFor(t, "ranking", func() bool { return len(h.svc.State().Ranking) == 3 })
	s := h.svc.State()
	if w, l := wins(s, "Me#1111"); w != 1 || l != 0 {
		t.Fatalf("Me = %d-%d", w, l)
	}
	if w, l := wins(s, "Arthas#1123"); w != 1 || l != 0 {
		t.Fatalf("Arthas = %d-%d", w, l)
	}
	if w, l := wins(s, "Jaina#2214"); w != 0 || l != 1 {
		t.Fatalf("Jaina, on the other team and not in the group, = %d-%d", w, l)
	}
	if got := store.Saved(); len(got) != 3 || store.Saves() != 1 {
		t.Fatalf("saved %+v in %d saves", got, store.Saves())
	}

	// The same score again counts nothing.
	h.session.Push(scoreInfo(true))
	h.session.Push(screen("MAIN_MENU"))
	waitFor(t, "menu", func() bool { return h.svc.State().Connected })
	if w, _ := wins(h.svc.State(), "Me#1111"); w != 1 || store.Saves() != 1 {
		t.Fatalf("repeat counted: wins %d, saves %d", w, store.Saves())
	}
}

func TestRankingSurvivesARestartAndClearEmptiesIt(t *testing.T) {
	store := mockranking.New(dto.RankingEntry{Tag: "Arthas#1123", Wins: 3, Losses: 1})
	h := rankingHarness(t, store)
	if w, l := wins(h.svc.State(), "Arthas#1123"); w != 3 || l != 1 {
		t.Fatalf("after a restart = %d-%d", w, l)
	}
	s, err := h.svc.ClearRanking()
	if err != nil || s.Ranking == nil || len(s.Ranking) != 0 || store.Clears() != 1 || len(store.Saved()) != 0 {
		t.Fatalf("ClearRanking = %v, %v (clears %d)", s.Ranking, err, store.Clears())
	}
	if len(h.svc.State().Ranking) != 0 {
		t.Fatal("the ranking came back")
	}
}

func TestRankingClearFailureKeepsTheEntries(t *testing.T) {
	store := mockranking.New(dto.RankingEntry{Tag: "Arthas#1123", Wins: 1})
	store.ClearErr = errors.New("disk full")
	h := rankingHarness(t, store)
	s, err := h.svc.ClearRanking()
	if err == nil || err.Error() != "The scoreboard couldn't be cleared on this computer. Try again." || len(s.Ranking) != 1 {
		t.Fatalf("ClearRanking = %v, %v", s.Ranking, err)
	}
}

func TestRankingFailedSaveIsRetried(t *testing.T) {
	store := mockranking.New()
	store.FailSaves(errors.New("disk full"))
	h := rankingHarness(t, store)
	h.connected(t)
	h.session.Push(lobby(castleFight, "CF", teamSlot("Me#1111", 0, true), teamSlot("Arthas#1123", 1, false)))
	h.session.Push(screen("LOADING_SCREEN"))
	h.session.Push(scoreInfo(true))
	waitFor(t, "recorded", func() bool { return len(h.svc.State().Ranking) == 2 })
	store.FailSaves(nil)
	waitFor(t, "retried save", func() bool { return len(store.Saved()) == 2 })
}
