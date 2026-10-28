package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	mockkeyboard "github.com/mihaiflorentin/wc3-team-follow/mock/keyboard"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func readyKeyboard() *mockkeyboard.Keyboard {
	return &mockkeyboard.Keyboard{Current: dto.KeyboardStatus{Supported: true, Permitted: true}}
}

func runType(t *testing.T, d *mockgame.Discovery, k *mockkeyboard.Keyboard, args ...string) (string, error) {
	t.Helper()
	cmd := typeCmd(func() (contract.GameDiscovery, error) { return d, nil },
		func() (contract.GameKeyboard, error) { return k, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"--delay", "0s"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestTypeTypesOneTrimmedLineIntoTheDiscoveredGame(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 7, Session: "12345678901234", Version: "3.0.1.24371"}}
	k := readyKeyboard()
	out, err := runType(t, d, k, "--", "  -ar  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Lines) != 1 || k.Lines[0] != "-ar" || k.PIDs[0] != 7 {
		t.Fatalf("typed %q into %v", k.Lines, k.PIDs)
	}
	if !strings.Contains(out, "-ar") || strings.Contains(out, "12345678901234") {
		t.Fatalf("output:\n%s", out)
	}
	if k.Requests != 0 {
		t.Fatalf("asked for permission %d times", k.Requests)
	}
}

func TestTypeRejectsBadTextBeforeDiscovering(t *testing.T) {
	for _, text := range []string{"   ", "two\nlines", strings.Repeat("a", 101)} {
		d := &mockgame.Discovery{}
		k := readyKeyboard()
		if _, err := runType(t, d, k, text); err == nil {
			t.Errorf("%q: no error", text)
		}
		if d.Calls != 0 || len(k.Lines) != 0 {
			t.Fatalf("%q: discovered %d times, typed %q", text, d.Calls, k.Lines)
		}
	}
}

func TestTypeStopsWhenTypingIsNotPossible(t *testing.T) {
	for _, st := range []dto.KeyboardStatus{
		{Reason: "Typing in the match chat isn't supported on Linux yet."},
		{Supported: true, Reason: "Team Follow needs permission to send keystrokes."},
	} {
		d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 7}}
		k := &mockkeyboard.Keyboard{Current: st}
		_, err := runType(t, d, k, "--", "-ar")
		if err == nil || !strings.Contains(err.Error(), st.Reason) {
			t.Fatalf("err = %v", err)
		}
		if d.Calls != 0 || len(k.Lines) != 0 || k.Requests != 0 {
			t.Fatalf("discovered %d, typed %q, requested %d", d.Calls, k.Lines, k.Requests)
		}
	}
}

func TestTypeAsksForPermissionOnlyWithTheFlag(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 7}}
	k := &mockkeyboard.Keyboard{Current: dto.KeyboardStatus{Supported: true, Reason: "Allow it."}, PermErr: errors.New("Still not allowed.")}
	_, err := runType(t, d, k, "--ask-permission", "--", "-ar")
	if k.Requests != 1 || err == nil || !strings.Contains(err.Error(), "Still not allowed.") {
		t.Fatalf("requests = %d, err = %v", k.Requests, err)
	}
	if len(k.Lines) != 0 {
		t.Fatalf("typed %q", k.Lines)
	}
}

func TestTypeResolvesEverythingBeforeAskingForPermission(t *testing.T) {
	k := &mockkeyboard.Keyboard{Current: dto.KeyboardStatus{Supported: true, Reason: "Allow it."}}
	cmd := typeCmd(func() (contract.GameDiscovery, error) { return nil, errors.New("resolve game discovery: broken") },
		func() (contract.GameKeyboard, error) { return k, nil })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--delay", "0s", "--ask-permission", "--", "-ar"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err = %v", err)
	}
	if k.Requests != 0 {
		t.Fatalf("asked for permission %d times before every dependency resolved", k.Requests)
	}
}

func TestTypeReportsGameAndKeyboardErrors(t *testing.T) {
	d := &mockgame.Discovery{Err: errors.New("Warcraft III isn't running.")}
	if _, err := runType(t, d, readyKeyboard(), "--", "-ar"); err == nil || !strings.Contains(err.Error(), "isn't running") {
		t.Fatalf("err = %v", err)
	}
	d = &mockgame.Discovery{Conn: dto.GameConnection{PID: 7}}
	k := readyKeyboard()
	k.Err = errors.New("Warcraft III isn't the window in front.")
	if _, err := runType(t, d, k, "--", "-ar"); err == nil || !strings.Contains(err.Error(), "in front") {
		t.Fatalf("err = %v", err)
	}
}
