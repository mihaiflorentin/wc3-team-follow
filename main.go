package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/cli"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

// main is the composition root: it builds config, logger and container once
// and hands the container to the CLI, the only layer that resolves from it.
func main() {
	attachConsole()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dataDir, dirErr := config.DataDir(cfg.Storage)
	logger, files, logErr := logging.Setup(cfg.Logging, os.Stdout, dataDir)
	slog.SetDefault(logger)
	if dirErr != nil {
		logger.Warn("app.data_dir", slog.Any("error", dirErr))
	}
	if logErr != nil {
		logger.Warn("app.log_file", slog.Any("error", logErr))
	}
	version, commit, built := cli.BuildInfo()
	logger.Info("app.start",
		slog.String("version", version), slog.String("commit", commit), slog.String("built", built),
		slog.String("os", runtime.GOOS), slog.String("arch", runtime.GOARCH), slog.String("go", runtime.Version()),
		slog.String("data_dir", dataDir), slog.String("log_dir", files.Dir), slog.Any("args", os.Args[1:]))

	// A panic on the main goroutine is logged with its stack before the
	// process crashes, so the log file explains the crash.
	defer func() {
		if r := recover(); r != nil {
			logger.Error("app.panic", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			_ = files.Close()
			panic(r)
		}
	}()

	c := container.New(cfg, logger)
	err = errors.Join(cli.Execute(c), c.Close())
	if err != nil {
		logger.Error("app.exit", slog.Any("error", err))
	} else {
		logger.Info("app.exit")
	}
	return errors.Join(err, files.Close())
}
