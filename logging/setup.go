package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/mihaiflorentin/wc3-team-follow/config"
)

const (
	// LogFileName is the main log in the log folder.
	LogFileName = "team-follow.log"
	// CrashFileName receives Go runtime crash reports with every stack.
	CrashFileName = "crash.log"
)

// Dir returns the log folder inside the app's data folder.
func Dir(dataDir string) string { return filepath.Join(dataDir, "logs") }

// Files are the open log files. Close them when the process ends.
type Files struct {
	// Dir is the log folder; empty when file logging is off.
	Dir   string
	log   *RotatingFile
	crash *os.File
}

// Close flushes and closes the files.
func (f *Files) Close() error {
	if f == nil {
		return nil
	}
	var errs []error
	if f.log != nil {
		errs = append(errs, f.log.Close())
	}
	if f.crash != nil {
		errs = append(errs, f.crash.Close())
	}
	return errors.Join(errs...)
}

// Setup builds the process logger: the console logger from New, plus, when
// [logging] file is on, a JSON-lines file in Dir(dataDir) with source
// locations at file_level. It also routes Go runtime crash reports (fatal
// errors and unrecovered panics, with every goroutine's stack) to
// crash.log. When the file cannot be opened, the console logger is
// returned with the error, so the app still runs.
func Setup(cfg *config.LoggingConfig, console io.Writer, dataDir string) (*slog.Logger, *Files, error) {
	logger := New(cfg, console)
	if cfg == nil || !cfg.File || dataDir == "" {
		return logger, &Files{}, nil
	}
	files := &Files{Dir: Dir(dataDir)}
	maxBytes := int64(max(cfg.FileMaxMB, 1)) << 20
	logFile, err := OpenRotatingFile(filepath.Join(files.Dir, LogFileName), maxBytes, cfg.FileKeep)
	if err != nil {
		return logger, &Files{}, fmt.Errorf("open log file: %w", err)
	}
	files.log = logFile

	if crash, err := os.OpenFile(filepath.Join(files.Dir, CrashFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		if err := debug.SetCrashOutput(crash, debug.CrashOptions{}); err == nil {
			files.crash = crash
		} else {
			_ = crash.Close()
		}
	}

	fileHandler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{AddSource: true, Level: parseLevel(cfg.FileLevel)})
	return slog.New(slog.NewMultiHandler(logger.Handler(), fileHandler)), files, nil
}
