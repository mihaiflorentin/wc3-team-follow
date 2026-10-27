package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	mockgame "github.com/mihaiflorentin/wc3-team-follow/mock/game"
	mockmaps "github.com/mihaiflorentin/wc3-team-follow/mock/maps"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func noMaps() (contract.MapsLocator, error) { return &mockmaps.Locator{}, nil }

func TestDiscoverListsMapsFolders(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 1, Port: 50000, Session: "12345678901"}}
	loc := &mockmaps.Locator{Folders: []dto.MapsFolder{{Path: "/pfx/drive_c/Maps", Source: "Steam Proton"}}, GamePrefix: "C:"}
	cmd := discoverCmd(func() (contract.GameDiscovery, error) { return d, nil }, func() (contract.MapsLocator, error) { return loc, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/pfx/drive_c/Maps  (Steam Proton)") || !strings.Contains(out.String(), "C:/pfx/drive_c/Maps") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestDiscoverPrintsTheConnectionButNotTheSession(t *testing.T) {
	d := &mockgame.Discovery{Conn: dto.GameConnection{PID: 42, Port: 50000, Session: "12345678901234", Version: "3.0.1.24371"}}
	cmd := discoverCmd(func() (contract.GameDiscovery, error) { return d, nil }, noMaps)
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
	cmd := discoverCmd(func() (contract.GameDiscovery, error) { return d, nil }, noMaps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "isn't running") {
		t.Fatalf("err = %v", err)
	}
}
