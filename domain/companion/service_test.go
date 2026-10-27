package companion_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const build = "3.0.1.24371"

type harness struct {
	svc       *companion.Service
	discovery *mockgame.Discovery
	session   *mockgame.Session
	prefs     *mockprefs.Store
}

func newHarness(t *testing.T, prefs *mockprefs.Store) *harness {
	t.Helper()
	h := &harness{
		discovery: &mockgame.Discovery{Conn: dto.GameConnection{Port: 5000, Session: "12345678901", Version: build}},
		session:   mockgame.NewSession(),
		prefs:     prefs,
	}
	h.svc = companion.New(h.discovery, &mockgame.Dialer{Session: h.session}, prefs, slog.New(slog.DiscardHandler), companion.Options{
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
	h.svc = companion.New(h.discovery, &mockgame.Dialer{Session: h.session}, h.prefs,
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
