package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func TestDiscoverPrintsTheConnectionButNotTheSession(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 42, Port: 50000, Session: "12345678901234", Version: "3.0.1.24371"}}
	cmd := discoverCmd(func() (contract.GameDiscovery, error) { return d, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"process 42", "3.0.1.24371", "50000", "14 digits"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "12345678901234") {
		t.Fatal("printed the session")
	}
}

func TestDiscoverReportsFailure(t *testing.T) {
	d := &mockgame.Discovery{Err: errors.New("Warcraft III isn't running.")}
	cmd := discoverCmd(func() (contract.GameDiscovery, error) { return d, nil })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "isn't running") {
		t.Fatalf("err = %v", err)
	}
}
