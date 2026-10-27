package cli

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	mockexample "github.com/mihaiflorentin/wc3-team-follow/mock/example"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func runExampleJob(svc *mockexample.Service, args ...string) error {
	cmd := exampleJobCmd(func() (jobDeps, error) {
		return jobDeps{Logger: discardLogger(), Example: svc}, nil
	})
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestExampleJob_SucceedsAndRunsTheJobOnce(t *testing.T) {
	svc := &mockexample.Service{}

	if err := runExampleJob(svc, "--name", "nightly"); err != nil {
		t.Fatalf("example-job returned %v, want nil (exit 0)", err)
	}
	want := dto.ExampleJob{Name: "nightly"}
	if got := svc.Jobs(); len(got) != 1 || got[0] != want {
		t.Errorf("jobs run = %+v, want exactly [%+v]", got, want)
	}
}

func TestExampleJob_FailsWhenTheDomainOperationFails(t *testing.T) {
	svc := &mockexample.Service{RunJobErr: errors.New("boom")}

	err := runExampleJob(svc)
	if err == nil || !errors.Is(err, svc.RunJobErr) {
		t.Fatalf("example-job returned %v, want an error wrapping %v (non-zero exit)", err, svc.RunJobErr)
	}
}

func TestExampleJob_FailsWhenDependenciesCannotBeResolved(t *testing.T) {
	resolveErr := errors.New("database down")
	cmd := exampleJobCmd(func() (jobDeps, error) { return jobDeps{}, resolveErr })
	cmd.SetArgs(nil)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); !errors.Is(err, resolveErr) {
		t.Fatalf("example-job returned %v, want an error wrapping %v", err, resolveErr)
	}
}
