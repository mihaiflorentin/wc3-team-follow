package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DataDirEnv overrides the data folder, e.g. for isolated tests.
const DataDirEnv = "WF_DATA_DIR"

// DataDir returns the per-user folder the app keeps its files in:
// %LOCALAPPDATA%\<dir_name> on Windows, ~/Library/Application
// Support/<dir_name> on macOS and $XDG_CONFIG_HOME/<dir_name> on Linux.
func DataDir(cfg *StorageConfig) (string, error) {
	if dir := os.Getenv(DataDirEnv); dir != "" {
		return filepath.Abs(dir)
	}
	if cfg == nil || cfg.DirName == "" {
		return "", errors.New("missing [storage] dir_name")
	}
	base, err := os.UserConfigDir()
	if runtime.GOOS == "windows" {
		// UserCacheDir is %LOCALAPPDATA%: files stay on this PC instead of
		// roaming, as in earlier releases.
		base, err = os.UserCacheDir()
	}
	if err != nil {
		return "", fmt.Errorf("locate user data folder: %w", err)
	}
	return filepath.Join(base, cfg.DirName), nil
}
