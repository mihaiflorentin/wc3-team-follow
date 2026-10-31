package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestReadFileParsesRealReplays runs only when WF_REPLAY_FILES lists real
// replays (separated by the OS path list separator). Replays hold BattleTags
// and chat, so they are never committed; the test logs only counts.
func TestReadFileParsesRealReplays(t *testing.T) {
	list := os.Getenv("WF_REPLAY_FILES")
	if list == "" {
		t.Skip("set WF_REPLAY_FILES to real .w3g files to run")
	}
	for _, path := range filepath.SplitList(list) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r, err := New().ReadFile(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			humans, tags, actions := 0, 0, 0
			for _, p := range r.Players {
				if !p.Computer {
					humans++
					actions += p.Actions
				}
				if p.BattleTag != "" {
					tags++
				}
			}
			if humans == 0 || r.GameTime == 0 || actions == 0 {
				t.Errorf("humans %d game time %v actions %d", humans, r.GameTime, actions)
			}
			if r.VersionNumber >= 10100 && tags == 0 {
				t.Errorf("no BattleTags in a Reforged replay")
			}
			t.Logf("version %s build %d, %v, %d slots, %d humans, %d BattleTags, %d leaves, %d chat lines, %d actions, winner %+v, warnings %v",
				r.Version, r.Build, r.GameTime, len(r.Players), humans, tags, len(r.Leaves), len(r.Chat), actions, r.Winner, r.Warnings)
		})
	}
}
