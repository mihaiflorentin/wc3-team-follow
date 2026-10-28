package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/cli"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/crashreport"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

// main is the composition root: it builds config, logger and container once
// and hands the container to the CLI, the only layer that resolves from it.
//
// Opening the window (no arguments) on Windows and Linux first starts a
// supervisor that runs the window as a child and shows an error dialog if
// it fails; elsewhere the window process shows the dialog itself.
func main() {
	attachConsole()
	window := len(os.Args) < 2
	if window && crashreport.Supervises && !crashreport.Supervised() {
		exe, err := os.Executable()
		if err == nil {
			dir := logDir()
			os.Exit(crashreport.Supervise(crashreport.Options{
				Exe: exe, Console: os.Stdout, LogDir: dir, Show: crashreport.Show,
				CrashFile: crashFile(dir), StartupTimeout: time.Minute,
			}))
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if window && !crashreport.Supervised() {
			crashreport.Show(crashreport.Title, crashreport.Message(err.Error(), logDir()))
		}
		os.Exit(1)
	}
}

// crashFile is the crash report file in dir, or "" without a log folder.
func crashFile(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, logging.CrashFileName)
}

// logDir is the log folder named in error dialogs, or "" when file logging
// is off or the data folder is unknown.
func logDir() string {
	cfg, err := config.NewConfig()
	if err != nil || cfg.Logging == nil || !cfg.Logging.File {
		return ""
	}
	dataDir, err := config.DataDir(cfg.Storage)
	if err != nil {
		return ""
	}
	return logging.Dir(dataDir)
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
	// The update check compares releases with the running build.
	cfg.App.Version = version
	// What's new reads the changelog built into the app.
	cfg.App.Changelog = changelog
	logger.Info("app.start",
		slog.String("version", version), slog.String("commit", commit), slog.String("built", built),
		slog.String("os", runtime.GOOS), slog.String("arch", runtime.GOARCH), slog.String("go", runtime.Version()),
		slog.String("data_dir", dataDir), slog.String("log_dir", files.Dir), slog.Any("args", os.Args[1:]))

	// A panic on the main goroutine is logged with its stack before the
	// process crashes, so the log file explains the crash. Without a
	// supervisor to report it, the window process shows the dialog.
	defer func() {
		if r := recover(); r != nil {
			logger.Error("app.panic", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			_ = files.Close()
			if len(os.Args) < 2 && !crashreport.Supervised() {
				crashreport.Show(crashreport.Title, crashreport.Message(fmt.Sprintf("panic: %v", r), files.Dir))
			}
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
