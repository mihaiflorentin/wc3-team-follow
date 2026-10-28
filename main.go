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
	// The path is read before an update can rename the running program.
	exe, exeErr := os.Executable()
	// The config is built once; the supervisor and error dialogs read their
	// folders from it too.
	cfg, cfgErr := config.NewConfig()
	if window {
		// After an update, the new version waits for the old one to exit,
		// so it doesn't meet the old window's single-instance lock.
		crashreport.WaitForPrevious(restartWait)
	}
	if window && crashreport.Supervises && !crashreport.Supervised() {
		if exeErr == nil {
			dir := logDir(cfg)
			opts := crashreport.Options{
				Exe: exe, Console: os.Stdout, LogDir: dir, Show: crashreport.Show,
				CrashFile: crashFile(dir), StartupTimeout: time.Minute,
			}
			if crashreport.Updated() {
				// Right after an update the old window's web view may still
				// hold its data folder: a window that fails at once gets one
				// more try, after waiting for the folder (ADR 0009).
				opts.RetryWithin = retryWithin
				opts.BeforeRetry = func() { waitForWebview(webviewDir(cfg)) }
			}
			os.Exit(crashreport.Supervise(opts))
		}
	}
	if err := run(cfg, cfgErr, exe); err != nil {
		if errors.Is(err, cli.ErrRestart) {
			os.Exit(restart(exe, exeErr))
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		if window && !crashreport.Supervised() {
			crashreport.Show(crashreport.Title, crashreport.Message(err.Error(), logDir(cfg)))
		}
		os.Exit(1)
	}
}

// restartWait is how long a new version waits for the old one to exit;
// retryWithin is how soon a window that fails after an update is retried.
const (
	restartWait = 30 * time.Second
	retryWithin = 10 * time.Second
)

// dataFolder is the per-user data folder, or "" when the config failed to
// load or the folder is unknown.
func dataFolder(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	dir, err := config.DataDir(cfg.Storage)
	if err != nil {
		return ""
	}
	return dir
}

// webviewDir is the WebView2 data folder (Windows), or "" when the data
// folder is unknown. cmd/cli hands the same folder to the window.
func webviewDir(cfg *config.Config) string {
	dir := dataFolder(cfg)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "webview")
}

// waitForWebview waits, on Windows, until no WebView2 browser holds dir:
// the old window's browser processes can outlive it, and a second browser
// on the same data folder fails to start. It returns how long it waited
// and whether the folder got free.
func waitForWebview(dir string) (time.Duration, bool) {
	if dir == "" || runtime.GOOS != "windows" {
		return 0, true
	}
	return crashreport.WaitUntilFree(func() bool { return crashreport.WebviewFree(dir) }, crashreport.WebviewWait, 200*time.Millisecond)
}

// restart starts the updated version after the window closed for an update
// and returns the exit code. A supervised window exits with RestartCode and
// its supervisor starts the new version; otherwise (macOS) this process
// starts it, and it waits for this one to exit.
func restart(exe string, exeErr error) int {
	if crashreport.Supervised() {
		return crashreport.RestartCode
	}
	err := exeErr
	if err == nil {
		err = crashreport.Relaunch(exe)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: start the updated version:", err)
		crashreport.Show(crashreport.Title, crashreport.UpdatedNotStartedMessage(err))
		return 1
	}
	return 0
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
func logDir(cfg *config.Config) string {
	dir := dataFolder(cfg)
	if dir == "" || cfg.Logging == nil || !cfg.Logging.File {
		return ""
	}
	return logging.Dir(dir)
}

func run(cfg *config.Config, cfgErr error, exe string) (err error) {
	if cfgErr != nil {
		return fmt.Errorf("load config: %w", cfgErr)
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
	// An update replaces this program; the first start after one says what
	// to redo.
	cfg.App.Executable, cfg.App.Updated = exe, crashreport.Updated()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		cfg.App.Executable = real
	}
	if len(os.Args) < 2 && cfg.App.Updated && runtime.GOOS == "windows" {
		waited, free := waitForWebview(webviewDir(cfg))
		logger.Info("app.webview_wait", slog.Duration("waited", waited), slog.Bool("free", free))
	}
	// What's new reads the changelog built into the app.
	cfg.App.Changelog = changelog
	logger.Info("app.start",
		slog.String("version", version), slog.String("commit", commit), slog.String("built", built),
		slog.String("os", runtime.GOOS), slog.String("arch", runtime.GOARCH), slog.String("go", runtime.Version()),
		slog.String("data_dir", dataDir), slog.String("log_dir", files.Dir), slog.Any("args", os.Args[1:]), slog.Bool("updated", cfg.App.Updated))

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
	if errors.Is(err, cli.ErrRestart) {
		logger.Info("app.restart", slog.Bool("supervised", crashreport.Supervised()))
	} else if err != nil {
		logger.Error("app.exit", slog.Any("error", err))
	} else {
		logger.Info("app.exit")
	}
	return errors.Join(err, files.Close())
}
