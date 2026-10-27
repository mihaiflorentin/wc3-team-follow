package companion_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	mockmaps "github.com/mihaiflorentin/wc3-team-follow/mock/maps"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const build = "3.0.1.24371"

type harness struct {
	svc       *companion.Service
	discovery *mockgame.Discovery
	session   *mockgame.Session
	prefs     *mockprefs.Store
	locator   *mockmaps.Locator
	library   *mockmaps.Library
	processes *mockgame.Processes
}

func newHarness(t *testing.T, prefs *mockprefs.Store) *harness {
	t.Helper()
	h := &harness{
		discovery: &mockgame.Discovery{Conn: dto.GameConnection{Port: 5000, Session: "12345678901", Version: build}},
		session:   mockgame.NewSession(),
		prefs:     prefs,
		processes: &mockgame.Processes{},
	}
	h.locator = &mockmaps.Locator{Folders: []dto.MapsFolder{{Path: "/detected/Maps", Source: "test"}}, GamePrefix: "GAME:"}
	h.library = &mockmaps.Library{Entries: []dto.MapEntry{{Name: "Legion TD", Path: "/detected/Maps/Legion TD.w3x"}, {Name: "Footmen Frenzy", Path: "/detected/Maps/ff.w3x"}}}
	ports := companion.Ports{Discovery: h.discovery, Dialer: &mockgame.Dialer{Session: h.session}, Prefs: prefs, Maps: h.locator, Library: h.library, Processes: h.processes}
	h.svc = companion.New(ports, slog.New(slog.DiscardHandler), companion.Options{
		VerifiedBuilds:    []string{build},
		RefreshInterval:   5 * time.Second,
		DiscoveryInterval: 10 * time.Second,
	})
	return h
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = h.svc.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func sentMessages(s *mockgame.Session) []string {
	var out []string
	for _, c := range s.Sent() {
		out = append(out, c.Message)
	}
	return out
}

func TestLoadsSavedFriendsAndStartsOff(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{"One#1234", "Two#2345"}, IncludeTeam: false}))
	s := h.svc.State()
	if !slices.Equal(s.ManualTags, []string{"One#1234", "Two#2345"}) || s.IncludeTeam || s.Enabled || !s.Diagnostic {
		t.Fatalf("state = %+v", s)
	}
}

func TestUnreadableSavedFriendsAreReported(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{})
	prefs.LoadErr = errors.New("broken")
	h := newHarness(t, prefs)
	if h.svc.State().PreferencesError == "" {
		t.Fatal("no preferences error")
	}
}

func TestConnectsAndAsksForTheTeamAndFriends(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	waitFor(t, "refresh", func() bool {
		sent := sentMessages(h.session)
		return slices.Contains(sent, follow.CmdTeamGetInfo) && slices.Contains(sent, follow.CmdFriendsGetFriends)
	})
	if v := h.svc.State().Version; v != build {
		t.Fatalf("version = %q", v)
	}
}

func TestGameEventsReachTheEngine(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	updates := make(chan dto.FollowState, 64)
	unsubscribe := h.svc.Subscribe(func(s dto.FollowState) {
		select {
		case updates <- s:
		default:
		}
	})
	defer unsubscribe()
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.session.Push(dto.GameEvent{MessageType: "TeamsInformation", Payload: map[string]any{"message": map[string]any{
		"partyMembers": []any{map[string]any{"battleTag": "Self#1111", "isSelf": true}, map[string]any{"battleTag": "Friend#1234", "isFriend": true}},
	}}})
	waitFor(t, "team", func() bool { return len(h.svc.State().Team) == 2 })
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("no state update was published")
	}
}

func TestMalformedEventsSwitchFollowOff(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.session.Push(dto.GameEvent{Malformed: true})
	waitFor(t, "note", func() bool { return strings.Contains(h.svc.State().Status, "unrecognized") })
}

func TestASessionErrorDisconnects(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	_ = h.session.Close()
	waitFor(t, "disconnect", func() bool { return !h.svc.State().Connected })
}

func TestLoggingOutFindsTheGameAgain(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.discovery.Set(dto.GameConnection{}, errors.New("More than one Warcraft III is running. Close the extra ones."))
	h.session.Push(dto.GameEvent{MessageType: "LoggedOut"})
	waitFor(t, "hint", func() bool {
		return h.svc.State().ConnectionHint == "More than one Warcraft III is running. Close the extra ones."
	})
	if h.svc.State().Connected {
		t.Fatal("still connected to the signed-out game")
	}
}

func TestDiscoveryErrorsBecomeAHint(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.discovery.Set(dto.GameConnection{}, fmt.Errorf("list game listeners: %w", errors.New("Warcraft III isn't running.")))
	h.run(t)
	waitFor(t, "hint", func() bool { return h.svc.State().ConnectionHint == "Warcraft III isn't running." })

	h2 := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h2.discovery.Set(dto.GameConnection{}, errors.New("internal failure"))
	h2.run(t)
	waitFor(t, "generic hint", func() bool { return h2.svc.State().ConnectionHint == "Waiting for Warcraft III." })
}

func TestSelectionChangesAreSaved(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true})
	h := newHarness(t, prefs)
	include := false
	if _, err := h.svc.Configure(dto.FollowConfig{Tags: &[]string{" One#1234 ", "one#1234"}, IncludeTeam: &include}); err != nil {
		t.Fatal(err)
	}
	saved, _ := prefs.Load()
	if !slices.Equal(saved.Tags, []string{"One#1234"}) || saved.IncludeTeam {
		t.Fatalf("saved = %+v", saved)
	}
	diagnostic := false
	_, _ = h.svc.Configure(dto.FollowConfig{Diagnostic: &diagnostic})
	if prefs.Saves != 1 {
		t.Fatalf("mode change was saved: %d saves", prefs.Saves)
	}
}

func TestInvalidTagsAreNotSaved(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{"One#1234"}, IncludeTeam: true})
	h := newHarness(t, prefs)
	if _, err := h.svc.Configure(dto.FollowConfig{Tags: &[]string{"bad"}}); err == nil {
		t.Fatal("accepted an invalid tag")
	}
	if prefs.Saves != 0 {
		t.Fatal("saved invalid tags")
	}
}

func TestSaveFailureIsReported(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true})
	prefs.SaveErr = errors.New("disk full")
	h := newHarness(t, prefs)
	state, err := h.svc.Configure(dto.FollowConfig{Tags: &[]string{"One#1234"}})
	if err == nil || state.PreferencesError == "" {
		t.Fatalf("err = %v, state error = %q", err, state.PreferencesError)
	}
}

func TestDiagnosticsNameThePlatform(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{"One#1234"}, IncludeTeam: true}))
	d := h.svc.Diagnostics()
	if d.Platform == "" {
		t.Fatalf("diagnostics = %+v", d)
	}
}

func TestLogsNeverContainTheSessionOrPasswords(t *testing.T) {
	var buf safeBuffer
	h := &harness{
		discovery: &mockgame.Discovery{Conn: dto.GameConnection{Port: 5000, Session: "98765432109876", Version: build}},
		session:   mockgame.NewSession(),
		prefs:     mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}),
	}
	h.svc = companion.New(companion.Ports{Discovery: h.discovery, Dialer: &mockgame.Dialer{Session: h.session}, Prefs: h.prefs, Maps: &mockmaps.Locator{}, Library: &mockmaps.Library{}},
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		companion.Options{VerifiedBuilds: []string{build}, RefreshInterval: 5 * time.Second, DiscoveryInterval: 10 * time.Second})
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	_, _ = h.svc.SubmitPassword("hunter2-lobby-secret")
	_, _ = h.svc.Configure(dto.FollowConfig{Tags: &[]string{"One#1234"}})

	out := buf.String()
	for _, want := range []string{"companion.connected", "follow.configure", "follow.password", "companion.send"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s", want)
		}
	}
	for _, secret := range []string{"98765432109876", "hunter2-lobby-secret"} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q", secret)
		}
	}
}

// safeBuffer is a bytes.Buffer safe for the companion's goroutines.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestAutoHostTranslatesTheMapPathAndSavesTheSetup(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true})
	h := newHarness(t, prefs)
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	settings := dto.HostSettings{MapPath: "/detected/Maps/Legion TD.w3x", MapName: "Legion TD", GameName: "LTD", DelaySeconds: 0}
	on := true
	state, err := h.svc.ConfigureHost(context.Background(), dto.HostConfig{Settings: &settings, Enabled: &on, FromMenus: true})
	if err != nil || !state.Host.Enabled {
		t.Fatalf("err=%v host=%+v", err, state.Host)
	}
	waitFor(t, "CreateLobby", func() bool {
		for _, c := range h.session.Sent() {
			if c.Message == "CreateLobby" && c.Payload["filename"] == "GAME:/detected/Maps/Legion TD.w3x" {
				return true
			}
		}
		return false
	})
	saved, _ := prefs.Load()
	if saved.Host == nil || saved.Host.GameName != "LTD" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestMapsFolderChoiceComesFirstAndIsSaved(t *testing.T) {
	prefs := mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true})
	h := newHarness(t, prefs)
	dir := t.TempDir()
	if _, err := h.svc.SetMapsFolder(dir); err != nil {
		t.Fatal(err)
	}
	folders := h.svc.MapsFolders(context.Background())
	if len(folders) != 2 || folders[0].Path != dir || folders[0].Source != "Chosen by you" {
		t.Fatalf("folders = %+v", folders)
	}
	if saved, _ := prefs.Load(); saved.MapsFolder != dir {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := h.svc.BrowseMaps(context.Background(), ""); err != nil || h.library.LastRoot != dir {
		t.Fatalf("browsed %q, %v", h.library.LastRoot, err)
	}
	if _, err := h.svc.SetMapsFolder(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("accepted a folder that doesn't exist")
	}
}

func TestSearchUsesTheDetectedFolder(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	got, err := h.svc.SearchMaps(context.Background(), "legion")
	if err != nil || len(got.Entries) != 1 || h.library.LastRoot != "/detected/Maps" {
		t.Fatalf("got %+v, %v (root %q)", got, err, h.library.LastRoot)
	}
}

func TestSeveralGamesAreCountedInTheState(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.discovery.Set(dto.GameConnection{}, fmt.Errorf("find game: %w", mockgame.CountedError{Message: "Warcraft III is running 3 times.", Count: 3}))
	h.run(t)
	waitFor(t, "count", func() bool { return h.svc.State().GameCount == 3 })
	if hint := h.svc.State().ConnectionHint; hint != "Warcraft III is running 3 times." {
		t.Fatalf("hint = %q", hint)
	}
	h.discovery.Set(dto.GameConnection{Port: 5000, Session: "12345678901", Version: build}, nil)
	h.processes.Count = 3
	s, err := h.svc.CloseGames(context.Background())
	if err != nil || s.GameCount != 0 || s.ConnectionHint != "" || s.Status != "Closed 3 copies of Warcraft III." {
		t.Fatalf("CloseGames = %+v, %v", s, err)
	}
	// Discovery runs again at once, without waiting for discovery_interval.
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	if h.svc.State().GameCount != 0 {
		t.Fatal("count kept while connected")
	}
}

func TestClosingGamesDropsTheSession(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.run(t)
	waitFor(t, "connection", func() bool { return h.svc.State().Connected })
	h.discovery.Set(dto.GameConnection{}, errors.New("Warcraft III isn't running. Start it through Battle.net."))
	h.processes.Count = 1
	s, err := h.svc.CloseGames(context.Background())
	if err != nil || s.Connected || s.Status != "Closed 1 copy of Warcraft III." {
		t.Fatalf("CloseGames = %+v, %v", s, err)
	}
	if err := h.session.Send(context.Background(), "TeamGetInfo", nil); err == nil {
		t.Fatal("the game session is still open")
	}
	if h.processes.CallCount() != 1 {
		t.Fatalf("CloseAll ran %d times", h.processes.CallCount())
	}
	waitFor(t, "new hint", func() bool { return strings.HasPrefix(h.svc.State().ConnectionHint, "Warcraft III isn't running.") })
}

func TestClosingWhenNoGameRunsSaysSo(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	s, err := h.svc.CloseGames(context.Background())
	if err != nil || s.Status != "No copy of Warcraft III was running." {
		t.Fatalf("CloseGames = %+v, %v", s, err)
	}
}

func TestClosingErrorsAreSentencesForPlayers(t *testing.T) {
	h := newHarness(t, mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true}))
	h.processes.Err = errors.New("close game process 42: operation not permitted")
	if _, err := h.svc.CloseGames(context.Background()); err == nil || err.Error() != "Warcraft III couldn't be closed. Close it in your task manager." {
		t.Fatalf("err = %v", err)
	}
	h.processes.Count = 1
	_, err := h.svc.CloseGames(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "Not every copy") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(h.svc.State().Status, "Closed 1 copy") {
		t.Fatalf("status = %q", h.svc.State().Status)
	}
}
