package follow_test

import (
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// hostEachLobby turns Auto-host on with the map resolved again before
// each lobby, as the companion does for the newest installed version of a
// tracked map.
func (f *fixture) hostEachLobby(settings dto.HostSettings) {
	f.t.Helper()
	err := f.engine.ConfigureHost(dto.HostConfig{Settings: &settings, Enabled: ptr(true), FromMenus: true,
		GameMapPath: "C:/Maps/Legion_TD_11.4c-hf5_Team_OZE.w3x", MapLabel: "Legion TD 11.4c-hf5", MapEachLobby: true})
	if err != nil {
		f.t.Fatalf("host: %v", err)
	}
}

func (f *fixture) createdFiles() []string {
	var files []string
	for _, c := range f.commands {
		if c.message == follow.CmdCreateLobby {
			files = append(files, c.payload["filename"].(string))
		}
	}
	return files
}

func TestTheFirstLobbyUsesTheMapResolvedWhenAutoHostWasTurnedOn(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.hostEachLobby(s)
	if f.engine.HostMapDue() {
		t.Fatal("asked for the map again right after it was resolved")
	}
	if got := f.createdFiles(); len(got) != 1 || got[0] != "C:/Maps/Legion_TD_11.4c-hf5_Team_OZE.w3x" {
		t.Fatalf("created on %q", got)
	}
	if f.state().Status != "Creating your lobby “LTD friends” on Legion TD 11.4c-hf5." {
		t.Fatalf("status = %q", f.state().Status)
	}
}

func TestEachLaterLobbyWaitsForTheNewestMap(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.hostEachLobby(s)
	f.openOwnLobby()
	f.playHostedGame()
	if !f.engine.HostMapDue() {
		t.Fatal("the next lobby doesn't ask for the map")
	}
	f.engine.Tick()
	if n := len(f.createdFiles()); n != 1 {
		t.Fatalf("created %d lobbies before the map was resolved", n)
	}
	f.engine.SetHostMap("C:/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x", "Legion TD 11.4c-hf9")
	if f.engine.HostMapDue() {
		t.Fatal("still waiting for the map")
	}
	got := f.createdFiles()
	if len(got) != 2 || got[1] != "C:/Maps/Download/Legion_TD_11.4c-hf9_Team_OZE.w3x" {
		t.Fatalf("created on %q", got)
	}
	if f.state().Status != "Creating your lobby “LTD friends” on Legion TD 11.4c-hf9." {
		t.Fatalf("status = %q", f.state().Status)
	}
}

func TestAnUnresolvedMapKeepsTheLastFile(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.hostEachLobby(s)
	f.openOwnLobby()
	f.playHostedGame()
	f.engine.SetHostMap("", "")
	f.engine.Tick()
	if got := f.createdFiles(); len(got) != 2 || got[1] != "C:/Maps/Legion_TD_11.4c-hf5_Team_OZE.w3x" {
		t.Fatalf("created on %q", got)
	}
}

func TestWithoutMapEachLobbyEveryLobbyUsesTheChosenFile(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 0
	f.mustHost(s)
	f.openOwnLobby()
	f.playHostedGame()
	if f.engine.HostMapDue() {
		t.Fatal("asked for the map without MapEachLobby")
	}
	f.engine.Tick()
	got := f.createdFiles()
	if len(got) != 2 || got[0] != `C:\maps\Legion TD.w3x` || got[1] != `C:\maps\Legion TD.w3x` {
		t.Fatalf("created on %q", got)
	}
}

func TestSetHostMapIsIgnoredWhenNothingWaitsForIt(t *testing.T) {
	f := newFixture(t)
	s := hostSettings()
	s.DelaySeconds = 5
	f.hostEachLobby(s)
	f.engine.SetHostMap("C:/Maps/other.w3x", "Other")
	f.advance(5 * time.Second)
	f.engine.Tick()
	if got := f.createdFiles(); len(got) != 1 || got[0] != "C:/Maps/Legion_TD_11.4c-hf5_Team_OZE.w3x" {
		t.Fatalf("created on %q", got)
	}
	off := false
	_ = f.engine.ConfigureHost(dto.HostConfig{Enabled: &off})
	f.engine.SetHostMap("C:/Maps/other.w3x", "Other")
	if f.engine.HostMapDue() || f.state().Host.Enabled {
		t.Fatal("SetHostMap changed Auto-host while it was off")
	}
}
