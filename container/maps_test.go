package container

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/config"
)

func TestMapReleases_ReportsAMissingMapsSection(t *testing.T) {
	c := New(&config.Config{}, slog.New(slog.DiscardHandler))
	if _, err := c.MapReleases(); err == nil || !strings.Contains(err.Error(), "[maps]") {
		t.Fatalf("built map releases without [maps]: %v", err)
	}
}

func TestMapReleases_RefusesAnAPIWithoutHTTPS(t *testing.T) {
	c := New(&config.Config{Maps: &config.MapsConfig{APIURL: "http://api.github.com", Timeout: "20s"}}, slog.New(slog.DiscardHandler))
	if _, err := c.MapReleases(); err == nil {
		t.Fatal("built map releases that talk plain HTTP")
	}
}

func TestCompanion_TracksLegionTDFromTheEmbeddedConfig(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Maps == nil || len(cfg.Maps.Tracked) != 1 || cfg.Maps.Tracked[0].Repository != "team-oze/Legion-TD" || cfg.Maps.MaxSizeMB != 256 {
		t.Fatalf("[maps] = %+v", cfg.Maps)
	}
	c := New(cfg, slog.New(slog.DiscardHandler))
	companion, err := c.Companion()
	if err != nil {
		t.Fatal(err)
	}
	if m := companion.State().MapUpdates; !m.Enabled || len(m.Maps) != 1 || m.Maps[0].Name != "Legion TD" {
		t.Fatalf("map updates = %+v", m)
	}
}

func TestCompanion_RefusesABadTrackedMap(t *testing.T) {
	for name, bad := range map[string]config.TrackedMapConfig{
		"no version group": {Name: "X", Repository: "o/r", Pattern: `^X_.*\.w3x$`, Folder: "Download"},
		"folder outside":   {Name: "X", Repository: "o/r", Pattern: `^X_(?P<version>.+)\.w3x$`, Folder: "../elsewhere"},
		"absolute folder":  {Name: "X", Repository: "o/r", Pattern: `^X_(?P<version>.+)\.w3x$`, Folder: "/tmp"},
		"bad repository":   {Name: "X", Repository: "o/r/../x", Pattern: `^X_(?P<version>.+)\.w3x$`, Folder: "Download"},
		"no name":          {Repository: "o/r", Pattern: `^X_(?P<version>.+)\.w3x$`, Folder: "Download"},
	} {
		t.Setenv(config.DataDirEnv, t.TempDir())
		cfg, err := config.NewConfig()
		if err != nil {
			t.Fatal(err)
		}
		cfg.Maps.Tracked = []config.TrackedMapConfig{bad}
		if _, err := New(cfg, slog.New(slog.DiscardHandler)).Companion(); err == nil {
			t.Fatalf("%s: built the companion", name)
		}
	}
}
