package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mockmapobjects "github.com/mihaiflorentin/wc3-team-follow/mock/mapobjects"
	mockmaps "github.com/mihaiflorentin/wc3-team-follow/mock/maps"
	mockreplay "github.com/mihaiflorentin/wc3-team-follow/mock/replay"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func sampleReplay() dto.Replay {
	return dto.Replay{
		VersionNumber: 10200, Version: "3.00", Build: 7003, Expansion: true, Multiplayer: true,
		Duration: 482 * time.Second, GameTime: 481 * time.Second, Post202: true,
		GameName: "Sample game", HostID: 1, SaverID: 2,
		Map: dto.ReplayMap{Path: "Maps/Sample.w3x", File: "Sample.w3x", Creator: "Map Maker", Checksum: "efbeadde"},
		Players: []dto.ReplayPlayer{
			{ID: 1, Slot: 0, Name: "Player1", BattleTag: "Player1#1234", Clan: "CLAN", Team: 0, Race: "human", Handicap: 100, Actions: 900, APMActions: 800, APM: 100, Left: 300 * time.Second, Result: "lost"},
			{ID: 2, Slot: 1, Name: "Player2", BattleTag: "Player2#5678", Team: 1, Race: "orc", Handicap: 100, Actions: 500, APMActions: 480, APM: 60},
			{ID: 0, Slot: 2, Name: "Computer", Computer: true, Team: 1, Race: "undead", AIStrength: 2},
		},
		Leaves: []dto.ReplayLeave{{Time: 300 * time.Second, PlayerID: 1, Reason: "remote", Result: "lost"}},
		Chat:   []dto.ReplayChat{{PlayerID: 1, Mode: "all", Message: "secret words"}, {PlayerID: 2, Mode: "allies", Message: "more"}},
		Winner: dto.ReplayWinner{Team: 1, Confidence: "certain", Basis: "every other team was erased"},
	}
}

func runReplay(t *testing.T, r *mockreplay.Reader, args ...string) (string, error) {
	t.Helper()
	return runReplayWith(t, replayDeps{Reader: r, Objects: &mockmapobjects.Reader{}, Locator: &mockmaps.Locator{}}, args...)
}

func runReplayWith(t *testing.T, deps replayDeps, args ...string) (string, error) {
	t.Helper()
	cmd := replayCmd(func() (replayDeps, error) { return deps, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestReplayPrintsWhatWasParsed(t *testing.T) {
	r := &mockreplay.Reader{Replay: sampleReplay()}
	text, err := runReplay(t, r, "game.w3g")
	if err != nil {
		t.Fatal(err)
	}
	if r.LastPath != "game.w3g" {
		t.Errorf("read %q", r.LastPath)
	}
	for _, want := range []string{"3.00 (10200), build 7003", "8m2s", "Sample game", "Sample.w3x", "efbeadde",
		"Player1#1234", "[CLAN]", "human", "100 APM", "lost at 5m0s", "Computer", "2 lines", "team 1 (certain"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "secret words") {
		t.Error("printed a chat line")
	}
}

func TestReplayPrintsJSON(t *testing.T) {
	text, err := runReplay(t, &mockreplay.Reader{Replay: sampleReplay()}, "--json", "game.w3g")
	if err != nil {
		t.Fatal(err)
	}
	var got dto.Replay
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	if got.Players[0].BattleTag != "Player1#1234" || got.Build != 7003 {
		t.Errorf("got %+v", got)
	}
}

func TestReplayReportsFailure(t *testing.T) {
	_, err := runReplay(t, &mockreplay.Reader{Err: errors.New("not a Warcraft III replay")}, "x.w3g")
	if err == nil || !strings.Contains(err.Error(), "not a Warcraft III replay") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplayNamesOrdersFromTheMapInAMapsFolder(t *testing.T) {
	maps := t.TempDir()
	mapFile := filepath.Join(maps, "Download", "Sample.w3x")
	if err := os.MkdirAll(filepath.Dir(mapFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mapFile, []byte("map"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := sampleReplay()
	r.Map.Path = "Maps/Download/Sample.w3x"
	r.Orders = []dto.ReplayOrder{
		{Time: time.Second, PlayerID: 1, Code: "h04P"}, {Time: 2 * time.Second, PlayerID: 1, Code: "h04P"},
		{Time: 3 * time.Second, PlayerID: 1, Code: "u008"}, {Time: 4 * time.Second, PlayerID: 2, Code: "zzzz"},
	}
	objects := &mockmapobjects.Reader{Result: map[string]string{"h04P": "Bowman", "u008": "Upgrade King Hit Points"}}
	deps := replayDeps{
		Reader:  &mockreplay.Reader{Replay: r},
		Objects: objects,
		Locator: &mockmaps.Locator{Folders: []dto.MapsFolder{{Path: filepath.Join(t.TempDir(), "elsewhere")}, {Path: maps}}},
	}
	text, err := runReplayWith(t, deps, "game.w3g")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.Paths) != 1 || objects.Paths[0] != mapFile {
		t.Errorf("read maps %v", objects.Paths)
	}
	for _, want := range []string{"Orders:    4 (named from " + mapFile + ")",
		"Player1#1234 [CLAN]: Bowman ×2, Upgrade King Hit Points ×1", "Player2#5678: zzzz ×1"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}

	text, err = runReplayWith(t, deps, "--json", "game.w3g")
	if err != nil {
		t.Fatal(err)
	}
	var got dto.Replay
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if got.Orders[0].Name != "Bowman" || got.Orders[3].Name != "" {
		t.Errorf("orders = %+v", got.Orders)
	}
}

func TestReplaySaysWhenTheMapIsMissing(t *testing.T) {
	r := sampleReplay()
	r.Orders = []dto.ReplayOrder{{PlayerID: 1, Code: "h04P"}}
	objects := &mockmapobjects.Reader{}
	text, err := runReplayWith(t, replayDeps{Reader: &mockreplay.Reader{Replay: r}, Objects: objects, Locator: &mockmaps.Locator{}}, "game.w3g")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Sample.w3x isn't in a Maps folder; give it with --map") || !strings.Contains(text, "h04P ×1") || len(objects.Paths) != 0 {
		t.Errorf("read %v, output:\n%s", objects.Paths, text)
	}
	if _, err := runReplayWith(t, replayDeps{Reader: &mockreplay.Reader{Replay: r}, Objects: objects, Locator: &mockmaps.Locator{}}, "--map", "/maps/Sample.w3x", "game.w3g"); err != nil || len(objects.Paths) != 1 || objects.Paths[0] != "/maps/Sample.w3x" {
		t.Errorf("--map read %v, err %v", objects.Paths, err)
	}
}
