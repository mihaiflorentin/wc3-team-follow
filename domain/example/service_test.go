package example_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/domain/example"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestService_PingReportsNoStoredExamples(t *testing.T) {
	got, err := example.NewService(discardLogger()).Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got.Examples != 0 {
		t.Errorf("Ping().Examples = %d, want 0", got.Examples)
	}
}

func TestService_RunJobRejectsAJobWithoutAName(t *testing.T) {
	svc := example.NewService(discardLogger())

	err := svc.RunJob(context.Background(), dto.ExampleJob{Name: "  "})
	if !errors.Is(err, example.ErrInvalidJob) {
		t.Fatalf("RunJob() error = %v, want ErrInvalidJob", err)
	}
}
