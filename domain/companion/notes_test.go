package companion_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	mockkeyboard "github.com/mihaiflorentin/wc3-team-follow/mock/keyboard"
	mockmaps "github.com/mihaiflorentin/wc3-team-follow/mock/maps"
	mocknotes "github.com/mihaiflorentin/wc3-team-follow/mock/notes"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockranking "github.com/mihaiflorentin/wc3-team-follow/mock/ranking"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func noPrefs() *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true})
}

func TestNotesStartEmptyAndNeverNil(t *testing.T) {
	h := newHarness(t, noPrefs())
	if s := h.svc.State(); s.Notes == nil || len(s.Notes) != 0 {
		t.Fatalf("notes = %v", s.Notes)
	}
}

func TestSetNoteAddsEditsAndDeletesAndSurvivesARestart(t *testing.T) {
	store := mocknotes.New(nil)
	h := newHarnessWithNotes(t, noPrefs(), mockrecent.New(), store)

	s, err := h.svc.SetNote("Arthas#1123", "  Carries in Legion TD.\n  ")
	if err != nil || s.Notes["arthas#1123"].Text != "Carries in Legion TD." || s.Notes["arthas#1123"].Updated.IsZero() {
		t.Fatalf("add = %+v, %v", s.Notes, err)
	}
	if s, err = h.svc.SetNote("ARTHAS#1123", "Leaves early when losing."); err != nil || len(s.Notes) != 1 ||
		s.Notes["arthas#1123"].Text != "Leaves early when losing." {
		t.Fatalf("edit = %+v, %v", s.Notes, err)
	}
	if _, err = h.svc.SetNote("Jaina#2214", "Friend of Thrall."); err != nil {
		t.Fatal(err)
	}
	if store.Saves() != 3 || store.Saved()["jaina#2214"].Text != "Friend of Thrall." {
		t.Fatalf("saved %d times: %+v", store.Saves(), store.Saved())
	}

	restarted := newHarnessWithNotes(t, noPrefs(), mockrecent.New(), store)
	if got := restarted.svc.State().Notes; len(got) != 2 || got["arthas#1123"].Text != "Leaves early when losing." {
		t.Fatalf("after restart: %+v", got)
	}
	if s, err = restarted.svc.SetNote("Arthas#1123", "   "); err != nil || len(s.Notes) != 1 {
		t.Fatalf("clearing the text = %+v, %v", s.Notes, err)
	}
	if _, ok := store.Saved()["arthas#1123"]; ok {
		t.Fatal("the cleared note is still saved")
	}
}

func TestSetNoteKeepsNotesTo500Characters(t *testing.T) {
	store := mocknotes.New(nil)
	h := newHarnessWithNotes(t, noPrefs(), mockrecent.New(), store)
	// Characters, not bytes: 500 accented letters are 1000 bytes.
	if s, err := h.svc.SetNote("Arthas#1123", strings.Repeat("é", 500)); err != nil || len([]rune(s.Notes["arthas#1123"].Text)) != 500 {
		t.Fatalf("500 characters: %v", err)
	}
	s, err := h.svc.SetNote("Arthas#1123", strings.Repeat("a", 501))
	if err == nil || err.Error() != "A note can be at most 500 characters." {
		t.Fatalf("501 characters: err = %v", err)
	}
	if len([]rune(s.Notes["arthas#1123"].Text)) != 500 || store.Saves() != 1 {
		t.Fatalf("the long note replaced the saved one (%d saves)", store.Saves())
	}
}

func TestSetNoteNeedsABattleTag(t *testing.T) {
	h := newHarness(t, noPrefs())
	if _, err := h.svc.SetNote("  ", "Some text."); err == nil {
		t.Fatal("a note without a BattleTag was accepted")
	}
}

func TestSetNoteFailureIsASentenceAndKeepsTheOldNote(t *testing.T) {
	store := mocknotes.New(map[string]dto.PlayerNote{"arthas#1123": {Text: "Good healer.", Updated: time.Now()}})
	h := newHarnessWithNotes(t, noPrefs(), mockrecent.New(), store)
	store.SaveErr = errors.New("write notes.json: disk full")
	s, err := h.svc.SetNote("Arthas#1123", "Changed.")
	if err == nil || err.Error() != "Your note couldn't be saved on this computer. Try again." {
		t.Fatalf("err = %v", err)
	}
	if s.Notes["arthas#1123"].Text != "Good healer." || h.svc.State().Notes["arthas#1123"].Text != "Good healer." {
		t.Fatalf("notes = %+v", s.Notes)
	}
}

func TestNotesAreNeverLogged(t *testing.T) {
	var buf safeBuffer
	svc := companion.New(companion.Ports{Discovery: &mockgame.Discovery{}, Dialer: &mockgame.Dialer{}, Prefs: noPrefs(),
		Maps: &mockmaps.Locator{}, Library: &mockmaps.Library{}, Keyboard: &mockkeyboard.Keyboard{}, Recent: mockrecent.New(),
		Notes: mocknotes.New(nil), Ranking: mockranking.New()},
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), companion.Options{})
	_, _ = svc.SetNote("Arthas#1123", "secret-note-text")
	_, _ = svc.SetNote("Arthas#1123", "")
	if strings.Contains(buf.String(), "secret-note-text") {
		t.Fatalf("a note reached the log: %s", buf.String())
	}
}
