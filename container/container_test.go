package container

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/config"
)

func TestLazy_ConcurrentFirstUseBuildsOnce(t *testing.T) {
	var l lazy[*int]
	var builds atomic.Int32
	build := func() (*int, error) {
		builds.Add(1)
		v := 42
		return &v, nil
	}

	var wg sync.WaitGroup
	results := make([]*int, 32)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := l.get(build)
			if err != nil {
				t.Errorf("get: %v", err)
			}
			results[i] = v
		}()
	}
	wg.Wait()

	if n := builds.Load(); n != 1 {
		t.Fatalf("built %d times, want 1", n)
	}
	for _, v := range results {
		if v != results[0] {
			t.Fatal("concurrent callers received different instances")
		}
	}
}

func TestLazy_FailedBuildIsRetried(t *testing.T) {
	var l lazy[string]
	unreachable := errors.New("backend unreachable")

	if _, err := l.get(func() (string, error) { return "", unreachable }); !errors.Is(err, unreachable) {
		t.Fatalf("first get error = %v, want %v", err, unreachable)
	}
	got, err := l.get(func() (string, error) { return "connected", nil })
	if err != nil || got != "connected" {
		t.Fatalf("second get = %q, %v; want the failed build to be retried", got, err)
	}
}

func TestClose_RunsHooksInReverseOrderOnce(t *testing.T) {
	c := New(nil, slog.New(slog.DiscardHandler))
	var order []string
	for _, name := range []string{"first", "second", "third"} {
		c.onClose(name, func(context.Context) error {
			order = append(order, name)
			return nil
		})
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	want := []string{"third", "second", "first"}
	if len(order) != len(want) {
		t.Fatalf("close order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("close order = %v, want %v", order, want)
		}
	}
}

func TestCompanion_ReportsAMissingFollowSection(t *testing.T) {
	c := New(&config.Config{}, slog.New(slog.DiscardHandler))
	if _, err := c.Companion(); err == nil {
		t.Fatal("built the companion without [follow]")
	}
}

func TestCompanion_BuildsFromTheEmbeddedConfig(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, slog.New(slog.DiscardHandler))
	first, err := c.Companion()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := c.Companion()
	if first != second {
		t.Fatal("Companion built twice")
	}
	if state := first.State(); state.Enabled || !state.Diagnostic {
		t.Fatalf("companion did not start off in preview: %+v", state)
	}
}

func TestUpdater_ReportsAMissingUpdatesSection(t *testing.T) {
	c := New(&config.Config{}, slog.New(slog.DiscardHandler))
	if _, err := c.Updater(); err == nil {
		t.Fatal("built the updater without [updates]")
	}
}

func TestUpdater_RefusesAnAPIWithoutHTTPS(t *testing.T) {
	c := New(&config.Config{Updates: &config.UpdatesConfig{APIURL: "http://api.github.com", Repository: "o/r", Timeout: "20s"}},
		slog.New(slog.DiscardHandler))
	if _, err := c.Updater(); err == nil {
		t.Fatal("built an updater that talks plain HTTP")
	}
}

func TestUpdater_BuildsFromTheEmbeddedConfig(t *testing.T) {
	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Updates == nil || cfg.Updates.APIURL != "https://api.github.com" || cfg.Updates.Repository != "mihaiflorentin/wc3-team-follow" {
		t.Fatalf("[updates] = %+v", cfg.Updates)
	}
	c := New(cfg, slog.New(slog.DiscardHandler))
	first, err := c.Updater()
	if err != nil || first == nil {
		t.Fatalf("Updater = %v, %v", first, err)
	}
	if second, _ := c.Updater(); second != first {
		t.Fatal("Updater built twice")
	}
}

func TestCompanion_GetsTheBuildVersionForUpdates(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.App.Version = "v0.7.0"
	c := New(cfg, slog.New(slog.DiscardHandler))
	companion, err := c.Companion()
	if err != nil {
		t.Fatal(err)
	}
	if u := companion.State().Update; !u.Supported || u.CurrentVersion != "0.7.0" || !u.CheckAutomatically ||
		u.ReleasesURL != "https://github.com/mihaiflorentin/wc3-team-follow/releases" {
		t.Fatalf("update = %+v", u)
	}
}

func TestGameProcesses_IsBuiltOnce(t *testing.T) {
	c := New(&config.Config{}, slog.New(slog.DiscardHandler))
	first, err := c.GameProcesses()
	if err != nil || first == nil {
		t.Fatalf("GameProcesses = %v, %v", first, err)
	}
	if second, _ := c.GameProcesses(); second != first {
		t.Fatal("GameProcesses built twice")
	}
}

func TestGameKeyboard_IsBuiltOnceWithoutConfig(t *testing.T) {
	c := New(&config.Config{}, slog.New(slog.DiscardHandler))
	first, err := c.GameKeyboard()
	if err != nil || first == nil {
		t.Fatalf("GameKeyboard = %v, %v", first, err)
	}
	if second, _ := c.GameKeyboard(); second != first {
		t.Fatal("GameKeyboard built twice")
	}
}
