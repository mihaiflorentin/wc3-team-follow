package container

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
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

func TestReadinessChecks_ListsRegisteredChecksInOrder(t *testing.T) {
	c := New(nil, slog.New(slog.DiscardHandler))
	if got := c.ReadinessChecks(); len(got) != 0 {
		t.Fatalf("new container has %d readiness checks, want 0", len(got))
	}

	down := errors.New("connection refused")
	c.onReady("mysql", func(context.Context) error { return nil })
	c.onReady("redis", func(context.Context) error { return down })

	checks := c.ReadinessChecks()
	if len(checks) != 2 {
		t.Fatalf("got %d readiness checks, want 2", len(checks))
	}
	if checks[0].Name() != "mysql" || checks[0].Check(context.Background()) != nil {
		t.Errorf("first check = %s, want a passing mysql check", checks[0].Name())
	}
	if checks[1].Name() != "redis" || !errors.Is(checks[1].Check(context.Background()), down) {
		t.Errorf("second check = %s, want a failing redis check", checks[1].Name())
	}
}
