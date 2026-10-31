package mapdata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNamesReadsRealMaps runs only when WF_MAP_FILES lists real map files
// (separated by the OS path list separator). Maps are never committed: they
// belong to their authors. The test logs only counts.
func TestNamesReadsRealMaps(t *testing.T) {
	list := os.Getenv("WF_MAP_FILES")
	if list == "" {
		t.Skip("set WF_MAP_FILES to real .w3x files to run")
	}
	for _, path := range filepath.SplitList(list) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			names, err := New().Names(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if len(names) == 0 {
				t.Error("no names")
			}
			t.Logf("%d names", len(names))
		})
	}
}
