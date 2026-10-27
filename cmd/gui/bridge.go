package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

type dtoState = dto.FollowState

// Bridge is the API the page calls as window.go.gui.Bridge. Errors reach
// the page as rejected promises carrying the message, which is written for
// players.
type Bridge struct {
	ctx        context.Context
	companion  contract.Companion
	appVersion string
	executable string
	logDir     string
	// reveal opens a folder in the system file manager.
	reveal func(dir string) error
}

// NewBridge returns the bridge for companion. logDir is the log folder, or
// "" when file logging is off.
func NewBridge(companion contract.Companion, appVersion, executable, logDir string) *Bridge {
	return &Bridge{companion: companion, appVersion: appVersion, executable: executable, logDir: logDir, reveal: revealFolder}
}

// State returns the current state.
func (b *Bridge) State() dto.FollowState { return b.companion.State() }

// Configure changes the selection or mode, or starts or stops Follow.
func (b *Bridge) Configure(cfg dto.FollowConfig) (dto.FollowState, error) {
	return b.companion.Configure(cfg)
}

// SubmitPassword answers the lobby password request once.
func (b *Bridge) SubmitPassword(password string) (dto.FollowState, error) {
	return b.companion.SubmitPassword(password)
}

// Diagnostics returns the shareable report as indented JSON.
func (b *Bridge) Diagnostics() (string, error) {
	report := b.companion.Diagnostics()
	report.AppVersion = b.appVersion
	report.LogFolder = b.logDir
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode report: %w", err)
	}
	return string(out), nil
}

// ExecutablePath returns this program's path for setup instructions.
func (b *Bridge) ExecutablePath() string { return b.executable }

// LogFolder returns the log folder, or "" when file logging is off.
func (b *Bridge) LogFolder() string { return b.logDir }

// OpenLogFolder shows the log folder in the system file manager.
func (b *Bridge) OpenLogFolder() error {
	if b.logDir == "" {
		return errors.New("Logging to a file is turned off.")
	}
	if err := os.MkdirAll(b.logDir, 0o700); err != nil {
		return fmt.Errorf("The log folder couldn't be created: %w", err)
	}
	if err := b.reveal(b.logDir); err != nil {
		return fmt.Errorf("The log folder couldn't be opened. It is %s", b.logDir)
	}
	return nil
}

func revealFolder(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
