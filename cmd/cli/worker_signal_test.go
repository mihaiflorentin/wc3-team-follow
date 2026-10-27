//go:build unix

package cli

import (
	"io"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	mockexample "github.com/mihaiflorentin/wc3-team-follow/mock/example"
)

func TestWorker_StopsOnSIGTERM(t *testing.T) {
	// Keep SIGTERM from killing the test binary if it arrives before the
	// worker installs its own handler.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	cmd := workerCmd(func() (workerDeps, error) {
		return workerDeps{
			Logger:  discardLogger(),
			Example: &mockexample.Service{},
		}, nil
	})
	cmd.SetArgs(nil)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()

	select {
	case err := <-done:
		t.Fatalf("worker exited before SIGTERM: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	const timeout = 5 * time.Second
	deadline := time.After(timeout)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("worker returned %v after SIGTERM, want nil (clean stop)", err)
			}
			return
		case <-tick.C:
			if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
				t.Fatalf("send SIGTERM: %v", err)
			}
		case <-deadline:
			t.Fatalf("worker did not stop within %s of SIGTERM", timeout)
		}
	}
}
